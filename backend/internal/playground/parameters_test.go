package playground

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOptionalChatParametersReachProvider(t *testing.T) {
	zero, half, penalty, tokens := 0.0, 0.5, -1.0, 1024
	for _, tc := range []struct {
		name, protocol string
		input          ChatRequest
		want           map[string]any
		absent         []string
	}{
		{"openai defaults", ProtocolOpenAI, ChatRequest{}, map[string]any{}, []string{"temperature", "top_p", "max_tokens", "frequency_penalty", "presence_penalty"}},
		{"openai explicit zero", ProtocolOpenAI, ChatRequest{Temperature: &zero, MaxTokens: &tokens, FrequencyPenalty: &zero, PresencePenalty: &penalty}, map[string]any{"temperature": 0.0, "max_tokens": 1024.0, "frequency_penalty": 0.0, "presence_penalty": -1.0}, []string{"top_p"}},
		{"openai top p", ProtocolOpenAI, ChatRequest{TopP: &half}, map[string]any{"top_p": 0.5}, []string{"temperature"}},
		{"claude defaults", ProtocolAnthropic, ChatRequest{}, map[string]any{"max_tokens": 4096.0}, []string{"temperature", "top_p", "frequency_penalty", "presence_penalty"}},
		{"claude top p", ProtocolAnthropic, ChatRequest{TopP: &half, MaxTokens: &tokens}, map[string]any{"top_p": 0.5, "max_tokens": 1024.0}, []string{"temperature", "frequency_penalty", "presence_penalty"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var received map[string]any
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
					t.Error(err)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if tc.protocol == ProtocolAnthropic {
					fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
				} else {
					fmt.Fprint(w, "data: [DONE]\n\n")
				}
			}))
			defer upstream.Close()
			s, _ := newTestService(t, upstream.Client())
			_, err := s.Save(context.Background(), "alice", SaveRequest{Protocol: tc.protocol, BaseURL: upstream.URL, APIKey: "test-only-key", Models: []string{"model"}, DefaultModel: "model"})
			if err != nil {
				t.Fatal(err)
			}
			input := tc.input
			input.Model = "model"
			input.Messages = []Message{{"user", "hello"}}
			if err := s.Stream(context.Background(), "alice", input, func(string) error { return nil }); err != nil {
				t.Fatal(err)
			}
			for key, value := range tc.want {
				if received[key] != value {
					t.Errorf("%s = %v; want %v", key, received[key], value)
				}
			}
			for _, key := range tc.absent {
				if _, ok := received[key]; ok {
					t.Errorf("disabled field %s sent", key)
				}
			}
		})
	}
}

func TestInvalidChatParametersNeverReachProvider(t *testing.T) {
	low, high, tooHigh, temp, tokens := -0.1, 1.1, 2.1, 0.7, 32769
	for _, tc := range []struct {
		name, protocol string
		input          ChatRequest
	}{
		{"top p below zero", ProtocolOpenAI, ChatRequest{TopP: &low}},
		{"top p above one", ProtocolOpenAI, ChatRequest{TopP: &high}},
		{"frequency out of range", ProtocolOpenAI, ChatRequest{FrequencyPenalty: &tooHigh}},
		{"presence out of range", ProtocolOpenAI, ChatRequest{PresencePenalty: &tooHigh}},
		{"output limit", ProtocolOpenAI, ChatRequest{MaxTokens: &tokens}},
		{"claude temperature", ProtocolAnthropic, ChatRequest{Temperature: &high}},
		{"claude mixed sampling", ProtocolAnthropic, ChatRequest{Temperature: &temp, TopP: &temp}},
		{"claude frequency", ProtocolAnthropic, ChatRequest{FrequencyPenalty: &temp}},
		{"claude presence", ProtocolAnthropic, ChatRequest{PresencePenalty: &temp}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid parameters reached upstream") }))
			defer upstream.Close()
			s, _ := newTestService(t, upstream.Client())
			_, err := s.Save(context.Background(), "alice", SaveRequest{Protocol: tc.protocol, BaseURL: upstream.URL, APIKey: "test-only-key", Models: []string{"model"}, DefaultModel: "model"})
			if err != nil {
				t.Fatal(err)
			}
			input := tc.input
			input.Model = "model"
			input.Messages = []Message{{"user", "hello"}}
			if err := s.Stream(context.Background(), "alice", input, func(string) error { return nil }); !errors.Is(err, ErrInvalid) {
				t.Fatalf("got %v", err)
			}
		})
	}
}
