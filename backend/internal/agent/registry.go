package agent

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

const DefaultAgentID = ports.DefaultAgentID

type ErrUnknownAgent = ports.UnknownAgentError

type ErrDuplicateAgent struct {
	AgentID string
}

func (e *ErrDuplicateAgent) Error() string {
	return fmt.Sprintf("agent already registered: %s", e.AgentID)
}

type Registry struct {
	mu        sync.RWMutex
	defaultID string
	runners   map[string]ports.AgentRunner
}

func NewRegistry(defaultID ...string) *Registry {
	id := ports.DefaultAgentID
	if len(defaultID) > 0 && strings.TrimSpace(defaultID[0]) != "" {
		id = strings.TrimSpace(defaultID[0])
	}
	return &Registry{defaultID: id, runners: make(map[string]ports.AgentRunner)}
}

func (r *Registry) Register(agentID string, runner ports.AgentRunner) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return fmt.Errorf("agent id is required")
	}
	if isNilInterface(runner) {
		return fmt.Errorf("runner for agent %s is required", agentID)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.runners[agentID]; exists {
		return &ErrDuplicateAgent{AgentID: agentID}
	}
	r.runners[agentID] = runner
	return nil
}

func (r *Registry) Resolve(agentID string) (ports.AgentRunner, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		agentID = r.defaultID
	}

	r.mu.RLock()
	runner, ok := r.runners[agentID]
	r.mu.RUnlock()
	if !ok {
		return nil, ports.NewUnknownAgentError(agentID)
	}
	return runner, nil
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

var _ ports.AgentResolver = (*Registry)(nil)
