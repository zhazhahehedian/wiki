package registry

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
)

const (
	// MaxMCPBundleBytes bounds a stored MCP package. Browsers upload packages
	// of this size in parts; see upload.go.
	MaxMCPBundleBytes = 500 << 20
	// MaxMCPRequestBytes bounds a package sent in one multipart request (the
	// publishing API), which is buffered in memory.
	MaxMCPRequestBytes  = 100 << 20
	MaxMCPExpandedBytes = 2 << 30
	MaxMCPBundleEntries = 20000
)

func validPackageName(name string) bool {
	return validBundlePath(name) && !strings.Contains(name, "/") && strings.HasSuffix(strings.ToLower(name), ".zip")
}

// validateMCPBundle checks a package received in a single request.
func validateMCPBundle(ctx context.Context, files []File) ([]byte, []BundleFinding, error) {
	if len(files) != 1 || !validPackageName(files[0].Name) || len(files[0].Data) > MaxMCPRequestBytes {
		return nil, nil, ErrMCPPackage
	}
	data := files[0].Data
	findings, err := inspectMCPArchive(ctx, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, err
	}
	return data, findings, nil
}

// MCP ZIPs are stored byte-for-byte. Validate without extracting or executing
// content, with bounded decompression and cancellation even for large
// archives. Reads are sequential so r may be backed by ranged object reads.
func inspectMCPArchive(ctx context.Context, r io.ReaderAt, size int64) ([]BundleFinding, error) {
	if size > MaxMCPBundleBytes {
		return nil, ErrMCPPackage
	}
	z, err := zip.NewReader(r, size)
	if err != nil || len(z.File) == 0 || len(z.File) > MaxMCPBundleEntries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil && !isArchiveError(err) {
			return nil, err
		}
		return nil, ErrMCPPackage
	}
	tree := bundleTree{}
	scan := &bundleScanner{}
	var total uint64
	regular := 0
	for _, f := range z.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		directory := f.FileInfo().IsDir()
		scan.add(strings.TrimSuffix(f.Name, "/"), scanBundleName(strings.TrimSuffix(f.Name, "/"), directory))
		if (!directory && !f.Mode().IsRegular()) || f.Flags&1 != 0 || (f.Method != zip.Store && f.Method != zip.Deflate) || !tree.add(strings.TrimSuffix(f.Name, "/"), directory) {
			return nil, ErrMCPPackage
		}
		if f.UncompressedSize64 > MaxMCPExpandedBytes-total || (directory && f.UncompressedSize64 != 0) {
			return nil, ErrMCPPackage
		}
		total += f.UncompressedSize64
		if !directory {
			regular++
		}
	}
	if regular == 0 || !tree.consistent() {
		return nil, ErrMCPPackage
	}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if !isArchiveError(err) {
				return nil, err
			}
			return nil, ErrMCPPackage
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
			return nil, ctx.Err()
		}
		if err != nil && !isArchiveError(err) {
			return nil, err
		}
		if err != nil || uint64(n) != f.UncompressedSize64 {
			return nil, ErrMCPPackage
		}
	}
	return scan.findings, nil
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
