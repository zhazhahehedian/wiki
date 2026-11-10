package ports

import (
	"context"
	"io"
)

type ObjectStorage interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// RangeReader reads part of an object so large archives can be inspected
// without holding them in memory.
type RangeReader interface {
	GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error)
	// ObjectSize returns the size of a stored object; a missing object is an error.
	ObjectSize(ctx context.Context, key string) (int64, error)
}

// StoragePart is one received part of a multipart upload.
type StoragePart struct {
	Number int32
	Size   int64
	ETag   string
}

// MultipartStorage assembles a large object from independently uploaded parts.
type MultipartStorage interface {
	CreateMultipart(ctx context.Context, key, contentType string) (string, error)
	UploadPart(ctx context.Context, key, uploadID string, number int32, body io.ReadSeeker, size int64) error
	ListParts(ctx context.Context, key, uploadID string) ([]StoragePart, error)
	CompleteMultipart(ctx context.Context, key, uploadID string, parts []StoragePart) error
	AbortMultipart(ctx context.Context, key, uploadID string) error
}
