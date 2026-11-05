package registry

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
)

const MaxMCPBundleBytes = 100 << 20
const MaxMCPExpandedBytes = 500 << 20
const MaxMCPBundleEntries = 2000

// MCP ZIPs are stored byte-for-byte. Validate without extracting or executing
// content, with bounded decompression and cancellation even for large archives.
func validateMCPBundle(ctx context.Context, files []File) ([]byte, error) {
	if len(files) != 1 || !validBundlePath(files[0].Name) || strings.Contains(files[0].Name, "/") || !strings.HasSuffix(strings.ToLower(files[0].Name), ".zip") || len(files[0].Data) > MaxMCPBundleBytes {
		return nil, ErrMCPPackage
	}
	data := files[0].Data
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(z.File) == 0 || len(z.File) > MaxMCPBundleEntries {
		return nil, ErrMCPPackage
	}
	entries := map[string]bool{} // true for directories
	var total uint64
	regular := 0
	for _, f := range z.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(f.Name, "/")
		folded := strings.ToLower(name)
		_, duplicate := entries[folded]
		directory := f.FileInfo().IsDir()
		if !validBundlePath(name) || duplicate || (!directory && !f.Mode().IsRegular()) || f.Flags&1 != 0 || (f.Method != zip.Store && f.Method != zip.Deflate) {
			return nil, ErrMCPPackage
		}
		entries[folded] = directory
		if f.UncompressedSize64 > MaxMCPExpandedBytes-total || (directory && f.UncompressedSize64 != 0) {
			return nil, ErrMCPPackage
		}
		total += f.UncompressedSize64
		if !directory {
			regular++
		}
	}
	if regular == 0 {
		return nil, ErrMCPPackage
	}
	for name := range entries {
		for i := strings.LastIndex(name, "/"); i >= 0; i = strings.LastIndex(name[:i], "/") {
			if dir, exists := entries[name[:i]]; exists && !dir {
				return nil, ErrMCPPackage
			}
		}
	}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			return nil, ErrMCPPackage
		}
		n, err := io.Copy(io.Discard, io.LimitReader(contextReader{ctx, r}, int64(f.UncompressedSize64)+1))
		_ = r.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil || uint64(n) != f.UncompressedSize64 {
			return nil, ErrMCPPackage
		}
	}
	return data, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
