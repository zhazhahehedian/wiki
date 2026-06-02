package ports

import "context"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatOptions struct {
	Model       string
	Temperature float32
	MaxTokens   int
}

type LLMClient interface {
	Chat(ctx context.Context, msgs []Message, opts ChatOptions) (*Message, error)
}
