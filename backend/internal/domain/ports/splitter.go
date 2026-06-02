package ports

import "context"

type SplitOptions struct {
	ChunkSize int
	Overlap   int
}

type SplitChunk struct {
	Seq        int
	Content    string
	TokenCount int
}

type Splitter interface {
	Split(ctx context.Context, text string, opts SplitOptions) ([]SplitChunk, error)
}
