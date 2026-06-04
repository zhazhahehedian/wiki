package domain

import "time"

const (
	ConversationModeRAG = "rag"
	RoleUser            = "user"
	RoleAssistant       = "assistant"
	EvidenceNone        = "none"
	EvidenceWeak        = "weak"
	EvidenceSufficient  = "sufficient"
)

type Conversation struct {
	ID        string    `json:"id"`
	KBID      string    `json:"kb_id"`
	Title     string    `json:"title"`
	Mode      string    `json:"mode"`
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

type ChatMessage struct {
	ID             string         `json:"id"`
	ConversationID string         `json:"conversation_id"`
	Role           string         `json:"role"`
	Content        string         `json:"content"`
	Citations      []Citation     `json:"citations"`
	ToolCalls      []any          `json:"tool_calls"`
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
