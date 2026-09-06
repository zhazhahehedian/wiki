package playground

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestAnthropicModelsAndStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "claude-test-key" || r.Header.Get("anthropic-version") != "2023-06-01" || r.Header.Get("Authorization") != "" {
			t.Error("incorrect Anthropic authentication")
		}
		switch r.URL.Path {
		case "/proxy/v1/models":
			if r.URL.Query().Get("after_id") == "" {
				fmt.Fprint(w, `{"data":[{"id":"claude-a"}],"has_more":true,"last_id":"claude-a"}`)
			} else {
				fmt.Fprint(w, `{"data":[{"id":"claude-b"}],"has_more":false}`)
			}
		case "/proxy/v1/messages":
			var body struct {
				System    string    `json:"system"`
				MaxTokens int       `json:"max_tokens"`
				Messages  []Message `json:"messages"`
				Stream    bool      `json:"stream"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.System != "system instructions" || body.MaxTokens != 4096 || len(body.Messages) != 1 || body.Messages[0].Role != "user" || !body.Stream {
				t.Error("incorrect Anthropic payload")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Claude reply\"}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		default:
			t.Error("incorrect Anthropic endpoint", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client, _ := NewHTTPClient(server.URL)
	s, _ := newTestService(t, client)
	base := server.URL + "/proxy/v1/"
	c, err := s.Save(context.Background(), "alice", SaveRequest{Protocol: ProtocolAnthropic, BaseURL: base, APIKey: "claude-test-key", Models: []string{"claude-a"}, DefaultModel: "claude-a"})
	if err != nil || c.BaseURL != server.URL+"/proxy" {
		t.Fatal("base normalization", err)
	}
	models, err := s.DiscoverModels(context.Background(), "alice", ModelsRequest{Protocol: ProtocolAnthropic, BaseURL: base})
	if err != nil || !reflect.DeepEqual(models, []string{"claude-a", "claude-b"}) {
		t.Fatal("models pagination", models, err)
	}
	text := ""
	err = s.Stream(context.Background(), "alice", ChatRequest{Model: "claude-a", Messages: []Message{{"system", "system instructions"}, {"user", "hello"}}}, func(delta string) error { text += delta; return nil })
	if err != nil || text != "Claude reply" {
		t.Fatal("Anthropic stream", err, text)
	}
	c.Protocol = ProtocolOpenAI
	if _, err = s.key(c); !errors.Is(err, ErrUpstream) {
		t.Fatal("ciphertext accepted under different protocol")
	}
	if _, err = s.Save(context.Background(), "alice", SaveRequest{Protocol: ProtocolOpenAI, BaseURL: c.BaseURL, Models: c.Models, DefaultModel: c.DefaultModel, Version: c.Version}); !errors.Is(err, ErrInvalid) {
		t.Fatal("key reused across protocols")
	}
}

func TestLegacyEncryptedOpenAIKeyRemainsReadable(t *testing.T) {
	s, _ := newTestService(t, http.DefaultClient)
	// Existing envelopes predate the protocol column.
	cipher, err := s.protector.Encrypt(`{"User":"alice","BaseURL":"https://models.example/v1","Key":"old-key"}`)
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.key(Connection{Protocol: ProtocolOpenAI, UserID: "alice", BaseURL: "https://models.example/v1", Ciphertext: cipher})
	if err != nil || key != "old-key" {
		t.Fatal("legacy key migration failed", err)
	}
}

func TestAnthropicStreamRequiresMessageStopAndSanitizesError(t *testing.T) {
	for _, body := range []string{
		"data: {\"type\":\"error\",\"error\":{\"message\":\"private-key\"}}\n\n",
		"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"partial\"}}\n\n",
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, body)
		}))
		client, _ := NewHTTPClient(server.URL)
		s, _ := newTestService(t, client)
		_, err := s.Save(context.Background(), "alice", SaveRequest{Protocol: ProtocolAnthropic, BaseURL: server.URL, APIKey: "key", Models: []string{"model"}, DefaultModel: "model"})
		if err != nil {
			t.Fatal(err)
		}
		err = s.Stream(context.Background(), "alice", ChatRequest{Model: "model", Messages: []Message{{"user", "hello"}}}, func(string) error { return nil })
		server.Close()
		if !errors.Is(err, ErrUpstream) {
			t.Fatal("invalid stream reported success", err)
		}
	}
}
