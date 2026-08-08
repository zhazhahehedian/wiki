package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type ToolBuilder func(ctx context.Context, kbID string, callback ports.RetrievalCallback) (ports.Tool, error)

type ErrUnknownTool struct {
	ToolName string
}

func (e *ErrUnknownTool) Error() string { return fmt.Sprintf("unknown tool: %s", e.ToolName) }

type ErrDuplicateTool struct {
	ToolName string
}

func (e *ErrDuplicateTool) Error() string {
	return fmt.Sprintf("tool already registered: %s", e.ToolName)
}

type ToolRegistry struct {
	mu       sync.RWMutex
	builders map[string]ToolBuilder
	agents   map[string][]string
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		builders: make(map[string]ToolBuilder),
		agents:   make(map[string][]string),
	}
}

func (r *ToolRegistry) Register(name string, builder ToolBuilder) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("tool name is required")
	}
	if isNilInterface(builder) {
		return fmt.Errorf("builder for tool %s is required", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.builders[name]; exists {
		return &ErrDuplicateTool{ToolName: name}
	}
	r.builders[name] = builder
	return nil
}

func (r *ToolRegistry) RegisterAgent(agentID string, toolNames ...string) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return fmt.Errorf("agent id is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.agents[agentID]; exists {
		return &ErrDuplicateAgent{AgentID: agentID}
	}

	names := make([]string, 0, len(toolNames))
	seen := make(map[string]struct{}, len(toolNames))
	for _, name := range toolNames {
		name = strings.TrimSpace(name)
		if _, duplicate := seen[name]; duplicate {
			return &ErrDuplicateTool{ToolName: name}
		}
		if _, exists := r.builders[name]; !exists {
			return &ErrUnknownTool{ToolName: name}
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	r.agents[agentID] = names
	return nil
}

func (r *ToolRegistry) ToolsFor(ctx context.Context, agentID, kbID string, callback ports.RetrievalCallback) ([]ports.Tool, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		agentID = ports.DefaultAgentID
	}

	r.mu.RLock()
	names, ok := r.agents[agentID]
	if !ok {
		r.mu.RUnlock()
		return nil, ports.NewUnknownAgentError(agentID)
	}
	builders := make([]ToolBuilder, len(names))
	for i, name := range names {
		builders[i] = r.builders[name]
	}
	r.mu.RUnlock()

	tools := make([]ports.Tool, 0, len(builders))
	for i, builder := range builders {
		tool, err := builder(ctx, kbID, callback)
		if err != nil {
			return nil, fmt.Errorf("build tool %s: %w", names[i], err)
		}
		if isNilInterface(tool) {
			return nil, fmt.Errorf("build tool %s: builder returned nil", names[i])
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

var _ ports.ToolRegistry = (*ToolRegistry)(nil)
