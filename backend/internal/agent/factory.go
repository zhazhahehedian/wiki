package agent

import (
	"fmt"
	"strings"
	"sync"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type RunnerBuilder func() (ports.AgentRunner, error)

// Factory keeps construction separate from resolution. Builders are static in
// this phase; no configuration watching or hot reload is performed.
type Factory struct {
	mu       sync.RWMutex
	builders map[string]RunnerBuilder
}

func NewFactory() *Factory {
	return &Factory{builders: make(map[string]RunnerBuilder)}
}

func (f *Factory) Register(agentID string, builder RunnerBuilder) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return fmt.Errorf("agent id is required")
	}
	if builder == nil {
		return fmt.Errorf("builder for agent %s is required", agentID)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.builders[agentID]; exists {
		return &ErrDuplicateAgent{AgentID: agentID}
	}
	f.builders[agentID] = builder
	return nil
}

func (f *Factory) Create(agentID string) (ports.AgentRunner, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		agentID = ports.DefaultAgentID
	}

	f.mu.RLock()
	builder, ok := f.builders[agentID]
	f.mu.RUnlock()
	if !ok {
		return nil, &ErrUnknownAgent{AgentID: agentID}
	}
	runner, err := builder()
	if err != nil {
		return nil, fmt.Errorf("create agent %s: %w", agentID, err)
	}
	if runner == nil {
		return nil, fmt.Errorf("create agent %s: builder returned nil", agentID)
	}
	return runner, nil
}
