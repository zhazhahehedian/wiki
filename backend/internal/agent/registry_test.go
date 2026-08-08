package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type stubRunner struct{ id string }

func (r *stubRunner) Run(context.Context, []ports.Message, []ports.Tool, ports.AgentEventSink) (*ports.AgentResult, error) {
	return &ports.AgentResult{Content: r.id}, nil
}

func TestRegistryResolvesDefaultAgent(t *testing.T) {
	registry := NewRegistry()
	want := &stubRunner{id: ports.DefaultAgentID}
	if err := registry.Register(ports.DefaultAgentID, want); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	for _, id := range []string{"", ports.DefaultAgentID} {
		got, err := registry.Resolve(id)
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v", id, err)
		}
		if got != want {
			t.Fatalf("Resolve(%q) = %p, want %p", id, got, want)
		}
	}
}

func TestRegistryRejectsUnknownAndDuplicateAgents(t *testing.T) {
	registry := NewRegistry()
	runner := &stubRunner{id: ports.DefaultAgentID}
	if err := registry.Register(ports.DefaultAgentID, runner); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.Register(ports.DefaultAgentID, runner); err == nil {
		t.Fatal("duplicate Register() error = nil")
	}

	_, err := registry.Resolve("missing")
	var unknown *ErrUnknownAgent
	if !errors.As(err, &unknown) || unknown.AgentID != "missing" {
		t.Fatalf("Resolve(missing) error = %#v, want ErrUnknownAgent", err)
	}
}

func TestFactoryCreatesRegisteredDefaultAgent(t *testing.T) {
	factory := NewFactory()
	want := &stubRunner{id: ports.DefaultAgentID}
	if err := factory.Register(ports.DefaultAgentID, func() (ports.AgentRunner, error) {
		return want, nil
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, err := factory.Create("")
	if err != nil {
		t.Fatalf("Create(default) error = %v", err)
	}
	if got != want {
		t.Fatalf("Create(default) = %p, want %p", got, want)
	}
	if err := factory.Register(ports.DefaultAgentID, func() (ports.AgentRunner, error) { return want, nil }); err == nil {
		t.Fatal("duplicate factory Register() error = nil")
	}
	if _, err := factory.Create("missing"); err == nil {
		t.Fatal("Create(missing) error = nil")
	}
}
func TestRegistrySupportsConcurrentRegistrationAndResolution(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(ports.DefaultAgentID, &stubRunner{id: ports.DefaultAgentID}); err != nil {
		t.Fatalf("Register(default) error = %v", err)
	}

	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers*2)
	for i := 0; i < workers; i++ {
		i := i
		wg.Add(2)
		go func() {
			defer wg.Done()
			id := fmt.Sprintf("agent-%02d", i)
			if err := registry.Register(id, &stubRunner{id: id}); err != nil {
				errs <- err
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := registry.Resolve(""); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent operation error = %v", err)
	}

	for i := 0; i < workers; i++ {
		id := fmt.Sprintf("agent-%02d", i)
		if _, err := registry.Resolve(id); err != nil {
			t.Errorf("Resolve(%q) error = %v", id, err)
		}
	}
}
