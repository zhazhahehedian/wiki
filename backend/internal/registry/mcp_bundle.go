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
func validateMCPBundle(ctx context.Context, files []File) ([]byte, []BundleFinding, error) {
	if len(files) != 1 || !validBundlePath(files[0].Name) || strings.Contains(files[0].Name, "/") || !strings.HasSuffix(strings.ToLower(files[0].Name), ".zip") || len(files[0].Data) > MaxMCPBundleBytes {
		return nil, nil, ErrMCPPackage
	}
	data := files[0].Data
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(z.File) == 0 || len(z.File) > MaxMCPBundleEntries {
		return nil, nil, ErrMCPPackage
	}
	tree := bundleTree{}
	scan := &bundleScanner{}
	var total uint64
	regular := 0
	for _, f := range z.File {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		directory := f.FileInfo().IsDir()
		scan.add(strings.TrimSuffix(f.Name, "/"), scanBundleName(strings.TrimSuffix(f.Name, "/"), directory))
		if (!directory && !f.Mode().IsRegular()) || f.Flags&1 != 0 || (f.Method != zip.Store && f.Method != zip.Deflate) || !tree.add(strings.TrimSuffix(f.Name, "/"), directory) {
			return nil, nil, ErrMCPPackage
		}
		if f.UncompressedSize64 > MaxMCPExpandedBytes-total || (directory && f.UncompressedSize64 != 0) {
			return nil, nil, ErrMCPPackage
		}
		total += f.UncompressedSize64
		if !directory {
			regular++
		}
	}
	if regular == 0 || !tree.consistent() {
		return nil, nil, ErrMCPPackage
	}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			return nil, nil, ErrMCPPackage
		}
		limited := io.LimitReader(contextReader{ctx, r}, int64(f.UncompressedSize64)+1)
		var n int64
		if !f.FileInfo().IsDir() && scan.wantsContent(f.UncompressedSize64) {
			var content []byte
			content, err = io.ReadAll(limited)
			n = int64(len(content))
			scan.scanned += len(content)
			scan.add(f.Name, scanBundleContent(content))
		} else {
			n, err = io.Copy(io.Discard, limited)
		}
		_ = r.Close()
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		if err != nil || uint64(n) != f.UncompressedSize64 {
			return nil, nil, ErrMCPPackage
		}
	}
	return data, scan.findings, nil
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
