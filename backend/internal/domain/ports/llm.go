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

type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

type StreamChunk struct {
	Text  string
	Done  bool
	Usage *TokenUsage
	Err   error
}

type LLMClient interface {
	Chat(ctx context.Context, msgs []Message, opts ChatOptions) (*Message, error)
	ChatStream(ctx context.Context, msgs []Message, opts ChatOptions) (<-chan StreamChunk, error)
}
