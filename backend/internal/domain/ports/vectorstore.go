package ports

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

var ErrStaleDocumentPromotion = errors.New("stale document promotion")

type PendingDocumentPromotion struct {
	DocumentID     uuid.UUID
	ContentRef     string
	Checksum       string
	RemoteRevision string
	Title          string
	Bytes          int64
	Metadata       json.RawMessage
}

type StagedVectorStore interface {
	ReplaceChunksAndPromote(ctx context.Context, promotion PendingDocumentPromotion, items []domain.ChunkWithEmbedding) error
}

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
	// ReplaceChunks 原子替换文档的全部 chunks(同一事务内先删后插)。
	ReplaceChunks(ctx context.Context, documentID string, items []domain.ChunkWithEmbedding) error
	DeleteByDocument(ctx context.Context, documentID string) error
	Search(ctx context.Context, kbID string, query []float32, opts VectorSearchOptions) ([]VectorSearchHit, error)
}
