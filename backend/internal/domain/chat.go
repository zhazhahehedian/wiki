package domain

import (
	"encoding/json"
	"time"
)

const (
	ConversationModeRAG   = "rag"
	ConversationModeReAct = "react"
	RoleUser              = "user"
	RoleAssistant         = "assistant"
	RoleTool              = "tool"
	EvidenceNone          = "none"
	EvidenceWeak          = "weak"
	EvidenceSufficient    = "sufficient"
	ToolExecutionFailed   = "tool execution failed"
)

type Conversation struct {
	ID        string    `json:"id"`
	KBID      string    `json:"kb_id"`
	Title     string    `json:"title"`
	Mode      string    `json:"mode"`
	AgentID   string    `json:"agent_id"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Citation struct {
	ID            string  `json:"id"`
	ChunkID       string  `json:"chunk_id"`
	DocumentID    string  `json:"document_id"`
	DocumentTitle string  `json:"document_title"`
	Seq           int     `json:"seq"`
	Score         float32 `json:"score"`
	Snippet       string  `json:"snippet"`
}

// ToolCallStep 是持久化在 messages.tool_calls JSONB 里的单步轨迹（spec §6.2）。
type ToolCallStep struct {
	Step       int             `json:"step"`
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Thought    string          `json:"thought,omitempty"`
	Arguments  json.RawMessage `json:"arguments"`
	Result     string          `json:"result"`
	DurationMs int64           `json:"duration_ms"`
	Error      string          `json:"error,omitempty"`
}

// ToolCallEvent / ToolResultEvent 是 SSE tool_call / tool_result 事件载荷（spec §6.1）。
type ToolCallEvent struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolResultEvent struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Result     string `json:"result"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

type ChatMessage struct {
	ID             string         `json:"id"`
	ConversationID string         `json:"conversation_id"`
	Role           string         `json:"role"`
	Content        string         `json:"content"`
	Citations      []Citation     `json:"citations"`
	ToolCalls      []ToolCallStep `json:"tool_calls"`
	TokenUsage     map[string]any `json:"token_usage"`
	CreatedAt      time.Time      `json:"created_at"`
}

type NeighborChunk struct {
	ID         string `json:"id"`
	DocumentID string `json:"document_id"`
	Seq        int    `json:"seq"`
	Content    string `json:"content"`
	IsPrimary  bool   `json:"is_primary"`
}

type ChunkNeighbors struct {
	PrimaryChunkID string          `json:"primary_chunk_id"`
	Window         int             `json:"window"`
	Chunks         []NeighborChunk `json:"chunks"`
}
