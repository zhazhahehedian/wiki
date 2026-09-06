package playground

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authstore "github.com/zenith-wang/it-wiki/backend/internal/auth"
)

type memoryStore struct{ rows map[string]Connection }

func (m *memoryStore) Get(_ context.Context, u string) (Connection, error) {
	c, ok := m.rows[u]
	if !ok {
		return c, ErrNotConfigured
	}
	return c, nil
}
func (m *memoryStore) Save(_ context.Context, c Connection, v int64) (Connection, error) {
	if m.rows[c.UserID].Version != v {
		return Connection{}, ErrConflict
	}
	c.Version = v + 1
	m.rows[c.UserID] = c
	return c, nil
}
func (m *memoryStore) Delete(_ context.Context, u string) error { delete(m.rows, u); return nil }
func newTestService(t *testing.T, client *http.Client) (*Service, *memoryStore) {
	t.Helper()
	p, e := authstore.NewAESGCMProtector([]byte("12345678901234567890123456789012"))
	if e != nil {
		t.Fatal(e)
	}
	m := &memoryStore{map[string]Connection{}}
	return New(m, p, client), m
}
func saveTestConnection(t *testing.T, s *Service, base string) {
	t.Helper()
	_, e := s.Save(context.Background(), "alice", SaveRequest{BaseURL: base, APIKey: "secret-test-key", Models: []string{"model"}, DefaultModel: "model"})
	if e != nil {
		t.Fatal(e)
	}
}
func TestEncryptedConfigurationOwnershipAndConflict(t *testing.T) {
	s, m := newTestService(t, http.DefaultClient)
	saveTestConnection(t, s, "https://model.example/v1")
	c, _ := s.Get(context.Background(), "alice")
	data, _ := json.Marshal(c)
	if strings.Contains(string(data), "secret-test-key") || strings.Contains(string(c.Ciphertext), "secret-test-key") {
		t.Fatal("key disclosed")
	}
	if _, e := s.Get(context.Background(), "bob"); !errors.Is(e, ErrNotConfigured) {
		t.Fatal(e)
	}
	req := SaveRequest{BaseURL: c.BaseURL, Models: c.Models, DefaultModel: c.DefaultModel, Version: c.Version}
	if _, e := s.Save(context.Background(), "alice", req); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Save(context.Background(), "alice", req); !errors.Is(e, ErrConflict) {
		t.Fatal("stale update accepted")
	}
	req.Version = 2
	req.BaseURL = "https://other.example/v1"
	if _, e := s.Save(context.Background(), "alice", req); !errors.Is(e, ErrInvalid) {
		t.Fatal("reused key for changed destination")
	}
	swapped := m.rows["alice"]
	swapped.UserID = "bob"
	if _, e := s.key(swapped); e == nil {
		t.Fatal("cross-user ciphertext accepted")
	}
	swapped = m.rows["alice"]
	swapped.BaseURL = "https://evil.example"
	if _, e := s.key(swapped); e == nil {
		t.Fatal("cross-endpoint ciphertext accepted")
	}
}
func TestStreamValidatesAndSanitizesUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-test-key" {
			t.Error("missing user key")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "model" || body["stream"] != true {
			t.Error("bad request")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()
	client, _ := NewHTTPClient(upstream.URL)
	s, _ := newTestService(t, client)
	saveTestConnection(t, s, upstream.URL+"/v1")
	text := ""
	req := ChatRequest{Model: "model", Messages: []Message{{"user", "hello"}}}
	e := s.Stream(context.Background(), "alice", req, func(delta string) error { text += delta; return nil })
	if e != nil || text != "你好" {
		t.Fatalf("stream=%q err=%v", text, e)
	}
	req.Model = "unconfigured"
	if e := s.Stream(context.Background(), "alice", req, func(string) error { return nil }); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
}
func TestStreamCancellation(t *testing.T) {
	cancelled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(cancelled)
	}))
	defer upstream.Close()
	client, _ := NewHTTPClient(upstream.URL)
	s, _ := newTestService(t, client)
	saveTestConnection(t, s, upstream.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = s.Stream(ctx, "alice", ChatRequest{Model: "model", Messages: []Message{{"user", "hello"}}}, func(string) error { cancel(); return nil })
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("upstream was not cancelled")
	}
}
func TestStreamRejectsProviderFailures(t *testing.T) {
	for _, tc := range []struct {
		name              string
		status            int
		contentType, body string
	}{
		{"http error", 502, "text/plain", "secret-test-key provider diagnostic"},
		{"wrong content type", 200, "application/json", `{"key":"secret-test-key"}`},
		{"stream error", 200, "text/event-stream", "data: {\"error\":{\"message\":\"secret-test-key\"}}\n\n"},
		{"truncated", 200, "text/event-stream", "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"},
		{"malformed", 200, "text/event-stream", "data: secret-test-key\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer upstream.Close()
			client, _ := NewHTTPClient(upstream.URL)
			s, _ := newTestService(t, client)
			saveTestConnection(t, s, upstream.URL)
			var output string
			err := s.Stream(context.Background(), "alice", ChatRequest{Model: "model", Messages: []Message{{"user", "hello"}}}, func(delta string) error { output += delta; return nil })
			if !errors.Is(err, ErrUpstream) || strings.Contains(output, "secret-test-key") {
				t.Fatalf("provider error was not sanitized: %v", err)
			}
		})
	}
}
func TestDestinationPolicyAndRedirects(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "::1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "::ffff:192.168.1.1", "198.18.0.1"} {
		if publicIP(net.ParseIP(ip)) {
			t.Errorf("allowed %s", ip)
		}
	}
	if !publicIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("public ip denied")
	}
	for _, raw := range []string{"file:///tmp/key", "https://user:secret@example.com/v1", "https://example.com/v1?key=x", "https://example.com/v1/chat/completions"} {
		if _, e := NormalizeURL(raw); e == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	called := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer target.Close()
	client, _ := NewHTTPClient("")
	if _, e := client.Get(target.URL); e == nil || called {
		t.Fatal("untrusted private origin accessed")
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer redirect.Close()
	client, _ = NewHTTPClient(redirect.URL)
	response, e := client.Get(redirect.URL)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 302 || called {
		t.Fatal("redirect followed")
	}
}
