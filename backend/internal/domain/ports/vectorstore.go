package ports

import (
	"context"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

type VectorStore interface {
	InsertChunks(ctx context.Context, items []domain.ChunkWithEmbedding) error
}
