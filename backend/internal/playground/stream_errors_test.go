package playground

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamDistinguishesOutputLimitFromNormalCompletion(t *testing.T) {
	for _, tc := range []struct {
		name, protocol, body, text string
		limited                    bool
	}{
		{"openai length with final text", ProtocolOpenAI, `data: {"choices":[{"delta":{"content":"partial"},"finish_reason":"length"}]}` + "\n\ndata: [DONE]\n\n", "partial", true},
		{"openai length after text", ProtocolOpenAI, `data: {"choices":[{"delta":{"content":"partial"},"finish_reason":null}]}` + "\n\n" + `data: {"choices":[{"delta":{},"finish_reason":"length"}]}` + "\n\ndata: [DONE]\n\n", "partial", true},
		{"openai length without text", ProtocolOpenAI, `data: {"choices":[{"delta":{},"finish_reason":"length"}]}` + "\n\ndata: [DONE]\n\n", "", true},
		{"openai normal stop", ProtocolOpenAI, `data: {"choices":[{"delta":{"content":"complete"},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n", "complete", false},
		{"anthropic max tokens", ProtocolAnthropic, `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"partial"}}` + "\n\n" + `data: {"type":"message_delta","delta":{"stop_reason":"max_tokens"}}` + "\n\n" + `data: {"type":"message_stop"}` + "\n\n", "partial", true},
		{"anthropic end turn", ProtocolAnthropic, `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"complete"}}` + "\n\n" + `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}` + "\n\n" + `data: {"type":"message_stop"}` + "\n\n", "complete", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, tc.body)
			}))
			defer upstream.Close()
			s, _ := newTestService(t, upstream.Client())
			if _, err := s.Save(context.Background(), "alice", SaveRequest{Protocol: tc.protocol, BaseURL: upstream.URL, APIKey: "test-key", Models: []string{"model"}, DefaultModel: "model"}); err != nil {
				t.Fatal(err)
			}
			var text string
			err := s.Stream(context.Background(), "alice", ChatRequest{Model: "model", Messages: []Message{{"user", "generate"}}}, func(delta string) error { text += delta; return nil })
			if text != tc.text || (tc.limited && !errors.Is(err, ErrOutputLimit)) || (!tc.limited && err != nil) {
				t.Fatalf("wrong completion classification: text=%q err=%v", text, err)
			}
		})
	}
}

func TestStreamTimeoutAndCancellationRemainDistinct(t *testing.T) {
	for _, phase := range []string{"headers", "body", "cancel"} {
		t.Run(phase, func(t *testing.T) {
			stopped := make(chan struct{})
			release := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if phase != "headers" {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
					w.(http.Flusher).Flush()
				}
				select {
				case <-r.Context().Done():
				case <-release:
				}
				close(stopped)
			}))
			defer func() { close(release); upstream.Close() }()
			client, _ := NewHTTPClient(upstream.URL)
			s, _ := newTestService(t, client)
			s.streamTimeout = 100 * time.Millisecond
			saveTestConnection(t, s, upstream.URL)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var output string
			err := s.Stream(ctx, "alice", ChatRequest{Model: "model", Messages: []Message{{"user", "generate"}}}, func(delta string) error {
				output += delta
				if phase == "cancel" {
					cancel()
				}
				return nil
			})
			if phase == "cancel" {
				if !errors.Is(err, context.Canceled) || errors.Is(err, ErrTimeout) {
					t.Fatalf("cancellation classified as provider failure: %v", err)
				}
			} else if !errors.Is(err, ErrTimeout) || !errors.Is(err, ErrUpstream) {
				t.Fatalf("timeout is not distinguishable: %v", err)
			}
			if phase != "headers" && output != "partial" {
				t.Fatalf("lost partial text: %q", output)
			}
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("upstream request was not released")
			}
		})
	}
}

func TestStreamIncompleteAndRejectedResponses(t *testing.T) {
	for _, status := range []int{200, 401, 429, 502} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(status)
				if status == 200 {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
				} else {
					fmt.Fprint(w, "secret-test-key private upstream diagnostic")
				}
			}))
			defer upstream.Close()
			client, _ := NewHTTPClient(upstream.URL)
			s, _ := newTestService(t, client)
			saveTestConnection(t, s, upstream.URL)
			err := s.Stream(context.Background(), "alice", ChatRequest{Model: "model", Messages: []Message{{"user", "generate"}}}, func(string) error { return nil })
			if status == 200 {
				if errors.Is(err, ErrTimeout) || errors.Is(err, ErrOutputLimit) {
					t.Fatalf("unfinished stream misclassified: %v", err)
				}
			} else {
				var rejected *UpstreamHTTPError
				if !errors.As(err, &rejected) || rejected.StatusCode != status {
					t.Fatalf("missing safe upstream status: %v", err)
				}
			}
			if !errors.Is(err, ErrUpstream) || strings.Contains(err.Error(), "secret-test-key") {
				t.Fatalf("unsafe provider error: %v", err)
			}
		})
	}
}
