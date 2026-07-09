package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

func TestChatSendsRequestAndParsesMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s, want /chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("Authorization = %q, want bearer secret", got)
		}

		var req struct {
			Model       string          `json:"model"`
			Messages    []ports.Message `json:"messages"`
			Temperature float32         `json:"temperature"`
			MaxTokens   int             `json:"max_tokens"`
			Stream      bool            `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "override-model" || req.Stream {
			t.Fatalf("request = %+v, want override model and non-stream", req)
		}
		if len(req.Messages) != 1 || req.Messages[0].Content != "hello" {
			t.Fatalf("messages = %#v", req.Messages)
		}
		if req.Temperature != 0.2 || req.MaxTokens != 128 {
			t.Fatalf("options = temp %v max %d", req.Temperature, req.MaxTokens)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hi there"}}]}`))
	}))
	defer server.Close()

	client := New(Config{BaseURL: server.URL + "/", APIKey: "secret", Model: "fallback-model"})
	got, err := client.Chat(context.Background(), []ports.Message{{Role: "user", Content: "hello"}}, ports.ChatOptions{
		Model:       "override-model",
		Temperature: 0.2,
		MaxTokens:   128,
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if got.Role != "assistant" || got.Content != "hi there" {
		t.Fatalf("Chat() = %+v", got)
	}
}

func TestChatStreamParsesTokenAndDone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "fallback-model" || !req.Stream {
			t.Fatalf("stream request = %+v", req)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := New(Config{BaseURL: server.URL, Model: "fallback-model"})
	ch, err := client.ChatStream(context.Background(), []ports.Message{{Role: "user", Content: "hello"}}, ports.ChatOptions{})
	if err != nil {
		t.Fatalf("ChatStream() error = %v", err)
	}

	chunks := readStreamChunks(t, ch)
	if len(chunks) != 3 {
		t.Fatalf("len(chunks) = %d, want 3: %#v", len(chunks), chunks)
	}
	if chunks[0].Text != "hello" || chunks[1].Text != " world" {
		t.Fatalf("token chunks = %#v", chunks)
	}
	if !chunks[2].Done {
		t.Fatalf("done chunk = %+v, want Done", chunks[2])
	}
}

func TestChatStreamReturnsHTTPErrorBeforeChannel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream failed", http.StatusBadGateway)
	}))
	defer server.Close()

	client := New(Config{BaseURL: server.URL, Model: "fallback-model"})
	ch, err := client.ChatStream(context.Background(), nil, ports.ChatOptions{})
	if err == nil {
		t.Fatal("ChatStream() error = nil, want HTTP error")
	}
	if ch != nil {
		t.Fatalf("ChatStream() channel = %#v, want nil", ch)
	}
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("ChatStream() error = %v, want status code", err)
	}
}

func TestChatStreamReadsUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`+"\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := New(Config{BaseURL: server.URL, Model: "fallback-model"})
	ch, err := client.ChatStream(context.Background(), nil, ports.ChatOptions{})
	if err != nil {
		t.Fatalf("ChatStream() error = %v", err)
	}
	chunks := readStreamChunks(t, ch)
	if len(chunks) == 0 || !chunks[0].Done {
		t.Fatalf("chunks = %#v, want first chunk Done", chunks)
	}
	if chunks[0].Usage == nil || chunks[0].Usage.TotalTokens != 8 {
		t.Fatalf("Usage = %#v, want total tokens", chunks[0].Usage)
	}
}

func TestChatStreamReturnsAPIErrorChunk(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"error":{"message":"bad key"}}`+"\n\n")
	}))
	defer server.Close()

	client := New(Config{BaseURL: server.URL, Model: "fallback-model"})
	ch, err := client.ChatStream(context.Background(), nil, ports.ChatOptions{})
	if err != nil {
		t.Fatalf("ChatStream() error = %v", err)
	}
	var streamErr error
	for chunk := range ch {
		if chunk.Err != nil {
			streamErr = chunk.Err
		}
	}
	if streamErr == nil {
		t.Fatal("stream error = nil, want API error")
	}
	if !strings.Contains(streamErr.Error(), "bad key") {
		t.Fatalf("stream error = %v, want API message", streamErr)
	}
}

func TestChatStreamAggregatesToolCallDeltas(t *testing.T) {
	lines := []string{
		`data: {"choices":[{"delta":{"content":"让我查一下。"},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"kb_retrieval","arguments":""}}]},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"query\":"}}]},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"部署\"}"}}]},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, l := range lines {
			_, _ = io.WriteString(w, l+"\n\n")
		}
	}))
	defer srv.Close()

	client := New(Config{BaseURL: srv.URL, Model: "test"})
	stream, err := client.ChatStream(context.Background(), []ports.Message{{Role: "user", Content: "hi"}}, ports.ChatOptions{})
	if err != nil {
		t.Fatalf("ChatStream() error = %v", err)
	}

	var text string
	var calls []ports.ToolCall
	for chunk := range stream {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error: %v", chunk.Err)
		}
		text += chunk.Text
		calls = append(calls, chunk.ToolCalls...)
	}

	if text != "让我查一下。" {
		t.Errorf("text = %q, want 让我查一下。", text)
	}
	if len(calls) != 1 {
		t.Fatalf("aggregated calls = %d, want 1", len(calls))
	}
	want := ports.ToolCall{ID: "call_1", Name: "kb_retrieval", Arguments: `{"query":"部署"}`}
	if calls[0] != want {
		t.Errorf("call = %+v, want %+v", calls[0], want)
	}
}

func TestChatStreamSendsToolsAndToolMessagesOnWire(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	client := New(Config{BaseURL: srv.URL, Model: "test"})
	msgs := []ports.Message{
		{Role: "assistant", Content: "查一下", ToolCalls: []ports.ToolCall{{ID: "call_1", Name: "kb_retrieval", Arguments: `{"query":"x"}`}}},
		{Role: "tool", Content: "result text", ToolCallID: "call_1"},
	}
	opts := ports.ChatOptions{Tools: []ports.ToolDefinition{{
		Name: "kb_retrieval", Description: "search kb",
		Parameters: json.RawMessage(`{"type":"object"}`),
	}}}
	stream, err := client.ChatStream(context.Background(), msgs, opts)
	if err != nil {
		t.Fatalf("ChatStream() error = %v", err)
	}
	for range stream {
	}

	body := string(gotBody)
	for _, want := range []string{
		`"tools":[{"type":"function","function":{"name":"kb_retrieval","description":"search kb","parameters":{"type":"object"}}}]`,
		`"tool_calls":[{"id":"call_1","type":"function","function":{"name":"kb_retrieval","arguments":"{\"query\":\"x\"}"}}]`,
		`"tool_call_id":"call_1"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("request body missing %s\nbody: %s", want, body)
		}
	}
}

func readStreamChunks(t *testing.T, ch <-chan ports.StreamChunk) []ports.StreamChunk {
	t.Helper()

	var chunks []ports.StreamChunk
	timeout := time.After(2 * time.Second)
	for {
		select {
		case chunk, ok := <-ch:
			if !ok {
				return chunks
			}
			if chunk.Err != nil {
				t.Fatalf("stream chunk error = %v", chunk.Err)
			}
			chunks = append(chunks, chunk)
		case <-timeout:
			t.Fatal("timed out waiting for stream chunks")
		}
	}
}
