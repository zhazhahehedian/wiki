package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type registryTool struct {
	name     string
	kbID     string
	callback ports.RetrievalCallback
}

func (t *registryTool) Name() string        { return t.name }
func (t *registryTool) Description() string { return t.name }
func (t *registryTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (t *registryTool) Invoke(ctx context.Context, _ string) (string, error) {
	if t.callback != nil {
		if err := t.callback(ctx, &ports.RetrievalResult{Question: t.kbID}); err != nil {
			return "", err
		}
	}
	return t.kbID, nil
}

func toolBuilder(name string, useCallback bool) ToolBuilder {
	return func(_ context.Context, kbID string, callback ports.RetrievalCallback) (ports.Tool, error) {
		tool := &registryTool{name: name, kbID: kbID}
		if useCallback {
			tool.callback = callback
		}
		return tool, nil
	}
}

func TestToolRegistryBuildsPerAgentToolsInConfiguredOrder(t *testing.T) {
	registry := NewToolRegistry()
	if err := registry.Register("retrieve", toolBuilder("retrieve", true)); err != nil {
		t.Fatalf("Register(retrieve) error = %v", err)
	}
	if err := registry.Register("list", toolBuilder("list", false)); err != nil {
		t.Fatalf("Register(list) error = %v", err)
	}
	if err := registry.RegisterAgent(ports.DefaultAgentID, "list", "retrieve"); err != nil {
		t.Fatalf("RegisterAgent() error = %v", err)
	}

	callback := func(context.Context, *ports.RetrievalResult) error { return nil }
	tools, err := registry.ToolsFor(context.Background(), "", "kb-42", callback)
	if err != nil {
		t.Fatalf("ToolsFor() error = %v", err)
	}
	if len(tools) != 2 || tools[0].Name() != "list" || tools[1].Name() != "retrieve" {
		t.Fatalf("tool order = [%v, %v], want [list, retrieve]", tools[0].Name(), tools[1].Name())
	}
	for _, tool := range tools {
		got, err := tool.Invoke(context.Background(), `{}`)
		if err != nil || got != "kb-42" {
			t.Errorf("Invoke(%s) = %q, %v", tool.Name(), got, err)
		}
	}
}

func TestToolRegistryRejectsDuplicateAndUnknownRegistrations(t *testing.T) {
	registry := NewToolRegistry()
	if err := registry.Register("retrieve", toolBuilder("retrieve", true)); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.Register("retrieve", toolBuilder("retrieve", true)); err == nil {
		t.Fatal("duplicate tool Register() error = nil")
	}
	if err := registry.RegisterAgent(ports.DefaultAgentID, "missing"); err == nil {
		t.Fatal("RegisterAgent(unknown tool) error = nil")
	}
	if err := registry.RegisterAgent(ports.DefaultAgentID, "retrieve", "retrieve"); err == nil {
		t.Fatal("RegisterAgent(duplicate tool) error = nil")
	}
	if _, err := registry.ToolsFor(context.Background(), "missing-agent", "kb", nil); err == nil {
		t.Fatal("ToolsFor(unknown agent) error = nil")
	}
}

func TestToolRegistryDoesNotRetainRequestScopedInputs(t *testing.T) {
	registry := NewToolRegistry()
	if err := registry.Register("retrieve", toolBuilder("retrieve", true)); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterAgent(ports.DefaultAgentID, "retrieve"); err != nil {
		t.Fatal(err)
	}

	var firstCalls, secondCalls int
	first, err := registry.ToolsFor(context.Background(), "", "kb-first", func(context.Context, *ports.RetrievalResult) error {
		firstCalls++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.ToolsFor(context.Background(), "", "kb-second", func(context.Context, *ports.RetrievalResult) error {
		secondCalls++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if first[0] == second[0] {
		t.Fatal("ToolsFor() reused a request-scoped tool instance")
	}
	if _, err := first[0].Invoke(context.Background(), `{}`); err != nil {
		t.Fatal(err)
	}
	if _, err := second[0].Invoke(context.Background(), `{}`); err != nil {
		t.Fatal(err)
	}
	if firstCalls != 1 || secondCalls != 1 {
		t.Fatalf("callback calls = (%d, %d), want (1, 1)", firstCalls, secondCalls)
	}
}

func TestToolRegistryIsolatesOrderedToolsPerAgent(t *testing.T) {
	registry := NewToolRegistry()
	for _, name := range []string{"retrieve", "list", "summarize"} {
		if err := registry.Register(name, toolBuilder(name, false)); err != nil {
			t.Fatalf("Register(%q) error = %v", name, err)
		}
	}
	if err := registry.RegisterAgent(ports.DefaultAgentID, "list", "retrieve"); err != nil {
		t.Fatalf("RegisterAgent(default) error = %v", err)
	}
	if err := registry.RegisterAgent("summary-agent", "summarize", "list"); err != nil {
		t.Fatalf("RegisterAgent(summary) error = %v", err)
	}

	assertTools := func(agentID, kbID string, wantNames []string) error {
		tools, err := registry.ToolsFor(context.Background(), agentID, kbID, nil)
		if err != nil {
			return err
		}
		if len(tools) != len(wantNames) {
			return fmt.Errorf("ToolsFor(%q) count = %d, want %d", agentID, len(tools), len(wantNames))
		}
		for i, tool := range tools {
			if tool.Name() != wantNames[i] {
				return fmt.Errorf("ToolsFor(%q)[%d] = %q, want %q", agentID, i, tool.Name(), wantNames[i])
			}
			gotKB, err := tool.Invoke(context.Background(), `{}`)
			if err != nil {
				return fmt.Errorf("Invoke(%q): %w", tool.Name(), err)
			}
			if gotKB != kbID {
				return fmt.Errorf("Invoke(%q) kb = %q, want %q", tool.Name(), gotKB, kbID)
			}
		}
		return nil
	}

	for i := 0; i < 5; i++ {
		if err := assertTools("", fmt.Sprintf("default-kb-%d", i), []string{"list", "retrieve"}); err != nil {
			t.Fatal(err)
		}
		if err := assertTools("summary-agent", fmt.Sprintf("summary-kb-%d", i), []string{"summarize", "list"}); err != nil {
			t.Fatal(err)
		}
	}

	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				errs <- assertTools("", fmt.Sprintf("default-concurrent-%d", i), []string{"list", "retrieve"})
				return
			}
			errs <- assertTools("summary-agent", fmt.Sprintf("summary-concurrent-%d", i), []string{"summarize", "list"})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}

func TestToolRegistrySupportsConcurrentReadsAndRegistration(t *testing.T) {
	registry := NewToolRegistry()
	if err := registry.Register("base", toolBuilder("base", false)); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterAgent(ports.DefaultAgentID, "base"); err != nil {
		t.Fatal(err)
	}

	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers*2)
	for i := 0; i < workers; i++ {
		i := i
		wg.Add(2)
		go func() {
			defer wg.Done()
			name := fmt.Sprintf("tool-%02d", i)
			if err := registry.Register(name, toolBuilder(name, false)); err != nil {
				errs <- err
			}
		}()
		go func() {
			defer wg.Done()
			tools, err := registry.ToolsFor(context.Background(), "", fmt.Sprintf("kb-%02d", i), nil)
			if err != nil {
				errs <- err
				return
			}
			if len(tools) != 1 || tools[0].Name() != "base" {
				errs <- fmt.Errorf("unexpected tools: %#v", tools)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent operation error = %v", err)
	}
}
