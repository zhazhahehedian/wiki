package domain

import "time"

type Chunk struct {
	ID         string         `json:"id"`
	KBID       string         `json:"kb_id"`
	DocumentID string         `json:"document_id"`
	Seq        int            `json:"seq"`
	Content    string         `json:"content"`
	TokenCount int            `json:"token_count"`
	Metadata   map[string]any `json:"metadata"`
	CreatedAt  time.Time      `json:"created_at"`
}

type ChunkWithEmbedding struct {
	Chunk
	Embedding []float32
}
