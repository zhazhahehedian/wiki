package ports

import (
	"context"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

type VectorSearchOptions struct {
	TopK     int
	MinScore float32
}

type VectorSearchHit struct {
	ChunkID       string
	KBID          string
	DocumentID    string
	DocumentTitle string
	Seq           int
	Content       string
	Score         float32
	Metadata      map[string]any
}

type VectorStore interface {
	InsertChunks(ctx context.Context, items []domain.ChunkWithEmbedding) error
	Search(ctx context.Context, kbID string, query []float32, opts VectorSearchOptions) ([]VectorSearchHit, error)
}
