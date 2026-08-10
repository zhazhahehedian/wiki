package ports

import (
	"context"
	"encoding/json"
)

type ToolDefinition struct {
	Name        string
	Description string
	Parameters  json.RawMessage // JSON Schema
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // raw JSON string
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // role=assistant 发起的调用
	ToolCallID string     `json:"tool_call_id,omitempty"` // role=tool 回填结果
}

type ChatOptions struct {
	Model       string
	Temperature float32
	MaxTokens   int
	Tools       []ToolDefinition // 非空时启用 tool calling
}

type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

type StreamChunk struct {
	Text      string
	ToolCalls []ToolCall // infra 层聚合完成后一次性给出，上层不处理增量
	Done      bool
	Usage     *TokenUsage
	Err       error
}

type LLMClient interface {
	Chat(ctx context.Context, msgs []Message, opts ChatOptions) (*Message, error)
	ChatStream(ctx context.Context, msgs []Message, opts ChatOptions) (<-chan StreamChunk, error)
}
