package ports

import "context"

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dim() int
	Model() string
}

// EmbeddingInputLimiter optionally supplies a conservative UTF-8 byte budget
// for each complete input, including headings. Zero means no extra limit.
type EmbeddingInputLimiter interface {
	MaxInputBytes() int
}
