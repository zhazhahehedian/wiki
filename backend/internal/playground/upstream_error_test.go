package playground

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestProviderRejectionRetainsOnlyKnownCategories(t *testing.T) {
	for _, tc := range []struct {
		body, want string
	}{
		{`{"error":{"code":"context_length_exceeded","type":"invalid_request_error","message":"private-material secret-key"}}`, "context_limit"},
		{`{"error":{"type":"overloaded_error","message":"private-material"}}`, "overloaded"},
		{`{"error":{"code":"no_available_channel"}}`, "model_unavailable"},
		{`{"error":{"code":"rate_limit_exceeded"}}`, "rate_limited"},
		{`{"error":{"code":"insufficient_quota"}}`, "quota_exhausted"},
		{`{"error":{"code":"unsupported_parameter","type":"invalid_request_error"}}`, "invalid_parameters"},
		{`{"error":{"type":"invalid_request_error"}}`, "invalid_request"},
		{`{"error":{"code":"private-material","type":"secret-key","message":"context_length_exceeded"}}`, ""},
		{`<html>private-material secret-key</html>`, ""},
		{strings.Repeat("x", 16*1024+1), ""},
	} {
		err := readUpstreamError(&http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(tc.body))})
		if err.StatusCode != 503 || err.Reason != tc.want || !errors.Is(err, ErrUpstream) || strings.Contains(err.Error(), "private-material") || strings.Contains(err.Error(), "secret-key") {
			t.Fatalf("unsafe or incorrect rejection category: %#v", err)
		}
	}
}

func TestProviderRejectionCategorySurvivesTheTransport(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(503)
		_, _ = io.WriteString(w, `{"error":{"code":"context_length_exceeded","message":"private-material"}}`)
	}))
	defer upstream.Close()
	s, _ := newTestService(t, upstream.Client())
	saveTestConnection(t, s, upstream.URL)
	err := s.Stream(context.Background(), "alice", ChatRequest{Model: "model", Messages: []Message{{"user", "generate"}}}, func(string) error { t.Error("rejected response emitted text"); return nil })
	var rejected *UpstreamHTTPError
	if !errors.As(err, &rejected) || rejected.StatusCode != 503 || rejected.Reason != "context_limit" {
		t.Fatalf("upstream reason lost: %v", err)
	}
}

type blockedErrorBody struct{ ctx context.Context }

func (b blockedErrorBody) Read([]byte) (int, error) { <-b.ctx.Done(); return 0, b.ctx.Err() }
func (blockedErrorBody) Close() error               { return nil }

func TestRejectionDiagnosticsDoNotWaitForTheGenerationTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, _ := newTestService(t, &http.Client{Transport: streamTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 503, Header: make(http.Header), Body: blockedErrorBody{r.Context()}}, nil
		})})
		saveTestConnection(t, s, "https://model.example/v1")
		started := time.Now()
		err := s.Stream(context.Background(), "alice", ChatRequest{Model: "model", Messages: []Message{{"user", "generate"}}}, func(string) error { return nil })
		var rejected *UpstreamHTTPError
		if !errors.As(err, &rejected) || rejected.StatusCode != 503 || time.Since(started) > 3*time.Second {
			t.Fatalf("error-body wait hid the known rejection: %v", err)
		}
	})
}
