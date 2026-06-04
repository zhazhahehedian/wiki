package llm

import (
	"context"
	"encoding/json"
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

func TestParseStreamPayloadReadsUsage(t *testing.T) {
	finishReason := "stop"
	payload := `{"choices":[{"delta":{"content":""},"finish_reason":"` + finishReason + `"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`

	chunk, err := parseStreamPayload(payload)
	if err != nil {
		t.Fatalf("parseStreamPayload() error = %v", err)
	}
	if !chunk.Done {
		t.Fatalf("Done = false, want true")
	}
	if chunk.Usage == nil || chunk.Usage.TotalTokens != 8 {
		t.Fatalf("Usage = %#v, want total tokens", chunk.Usage)
	}
}

func TestParseStreamPayloadReturnsAPIError(t *testing.T) {
	_, err := parseStreamPayload(`{"error":{"message":"bad key"}}`)
	if err == nil {
		t.Fatal("parseStreamPayload() error = nil, want API error")
	}
	if !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("parseStreamPayload() error = %v, want API message", err)
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
