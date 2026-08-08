package ports

import (
	"context"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

const DefaultAgentID = "knowledge-rag"

// AgentEventSink is the request-scoped event surface used by an agent runner.
type AgentEventSink interface {
	SendToken(ctx context.Context, text string) error
	SendToolCall(ctx context.Context, ev domain.ToolCallEvent) error
	SendToolResult(ctx context.Context, ev domain.ToolResultEvent) error
}

type AgentResult struct {
	Content string
	Steps   []domain.ToolCallStep
	Usage   *TokenUsage
}

// AgentRunner matches the existing ReAct execution boundary. Persistence and
// deterministic RAG orchestration remain owned by the chat service.
type AgentRunner interface {
	Run(ctx context.Context, messages []Message, tools []Tool, sink AgentEventSink) (*AgentResult, error)
}

type AgentResolver interface {
	Resolve(agentID string) (AgentRunner, error)
}

type RetrievalResult struct {
	Question      string
	EvidenceLevel string
	Hits          []VectorSearchHit
	Citations     []domain.Citation
}

type RetrievalCallback func(ctx context.Context, result *RetrievalResult) error

type ToolRegistry interface {
	ToolsFor(ctx context.Context, agentID, kbID string, callback RetrievalCallback) ([]Tool, error)
}
