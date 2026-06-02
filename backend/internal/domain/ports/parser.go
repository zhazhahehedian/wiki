package ports

import (
	"context"
	"io"
)

type ParseResult struct {
	Text     string
	Metadata map[string]any
}

type Parser interface {
	Supports(mime string) bool
	Parse(ctx context.Context, r io.Reader, mime string) (*ParseResult, error)
}
