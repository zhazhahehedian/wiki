package registry

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"unicode/utf8"
)

const (
	maxConcurrentPreviews  = 2
	maxPreviewFileBytes    = 256 << 10
	maxPreviewContentBytes = 4 << 20
)

// mcpFiles lists a stored MCP package and inlines small UTF-8 files so
// reviewers can read README and source without extracting anything to disk.
func (s *Service) mcpFiles(ctx context.Context, v Version) ([]ReviewFile, error) {
	select {
	case s.previews <- struct{}{}:
		defer func() { <-s.previews }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	reader, err := s.storage.Get(ctx, v.BundleKey)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, MaxMCPBundleBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxMCPBundleBytes {
		return nil, ErrInvalid
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(z.File) > MaxMCPBundleEntries {
		return nil, ErrInvalid
	}
	result := []ReviewFile{}
	inlined := 0
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		file := ReviewFile{Name: f.Name, Size: int(f.UncompressedSize64)}
		if f.UncompressedSize64 <= maxPreviewFileBytes && inlined+int(f.UncompressedSize64) <= maxPreviewContentBytes {
			if content, ok := readPreview(ctx, f); ok {
				inlined += len(content)
				text := string(content)
				file.Content = &text
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result = append(result, file)
	}
	return result, nil
}

func readPreview(ctx context.Context, f *zip.File) ([]byte, bool) {
	r, err := f.Open()
	if err != nil {
		return nil, false
	}
	defer r.Close()
	content, err := io.ReadAll(io.LimitReader(contextReader{ctx, r}, maxPreviewFileBytes+1))
	if err != nil || len(content) > maxPreviewFileBytes || !utf8.Valid(content) || bytes.ContainsRune(content, 0) {
		return nil, false
	}
	return content, true
}
