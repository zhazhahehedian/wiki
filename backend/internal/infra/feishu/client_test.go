package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

func TestClientRetriesRateLimitUsingRetryAfterWithoutLeakingToken(t *testing.T) {
	const token = "secret-user-token"
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Fatalf("Authorization = %q", got)
		}
		if attempts == 1 {
			w.Header().Set("Retry-After", "2")
			http.Error(w, `{"token":"secret-user-token"}`, http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"value":"ok"}}`))
	}))
	defer server.Close()

	var waits []time.Duration
	client := NewClient(ClientConfig{
		BaseURL:    server.URL,
		Timeout:    time.Second,
		MaxRetries: 2,
		Wait: func(_ context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			return nil
		},
	}, server.Client())
	var data struct {
		Value string `json:"value"`
	}
	err := client.Get(context.Background(), token, "/resource", nil, &data)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if attempts != 2 || len(waits) != 1 || waits[0] != 2*time.Second || data.Value != "ok" {
		t.Fatalf("attempts/waits/data = %d/%v/%+v", attempts, waits, data)
	}
}

func TestClientReturnsStableRedactedErrorAfterBoundedRetries(t *testing.T) {
	const token = "secret-user-token"
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		http.Error(w, token, http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{BaseURL: server.URL, MaxRetries: 1, Wait: noWait}, server.Client())
	err := client.Get(context.Background(), token, "/resource", nil, &struct{}{})
	var loadErr *ports.SourceLoadError
	if !errors.As(err, &loadErr) || loadErr.Code != ports.SourceLoadAPIError {
		t.Fatalf("Get() error = %#v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("error leaked token: %q", err)
	}
}

func TestClientRejectsMalformedEnvelopeAndPageTokenLoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{}} trailing`))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{BaseURL: server.URL}, server.Client())
	err := client.Get(context.Background(), "token", "/resource", nil, &struct{}{})
	var loadErr *ports.SourceLoadError
	if !errors.As(err, &loadErr) || loadErr.Code != ports.SourceLoadMalformed {
		t.Fatalf("Get() error = %#v", err)
	}

	tracker := newPageTokenTracker()
	if err := tracker.Advance("next"); err != nil {
		t.Fatalf("first Advance() error = %v", err)
	}
	if err := tracker.Advance("next"); !errors.As(err, &loadErr) || loadErr.Code != ports.SourceLoadMalformed {
		t.Fatalf("repeated Advance() error = %#v", err)
	}
}

func TestSourceLoadErrorRedactsUntrustedCauseFromJSON(t *testing.T) {
	const secret = "provider-copied-secret-token"
	err := ports.NewSourceLoadError(ports.SourceLoadAPIError, leakingError{Secret: secret})
	encoded, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(string(encoded), secret) {
		t.Fatalf("typed error leaked cause: error=%q json=%s", err, encoded)
	}
}

func TestClientRetriesServerErrorEvenWhenErrorBodyExceedsSuccessLimit(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(strings.Repeat("x", maxAPIResponseBytes+1)))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"value":"ok"}}`))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{BaseURL: server.URL, MaxRetries: 1, Wait: noWait}, server.Client())
	var data struct {
		Value string `json:"value"`
	}
	if err := client.Get(context.Background(), "token", "/resource", nil, &data); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if attempts != 2 || data.Value != "ok" {
		t.Fatalf("attempts/data = %d/%+v", attempts, data)
	}
}

func TestClientHonorsHTTPDateRetryAfterAndPerRequestTimeout(t *testing.T) {
	now := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	client := NewClient(ClientConfig{Now: func() time.Time { return now }}, nil)
	if got := client.retryDelay(0, now.Add(3*time.Second).Format(http.TimeFormat)); got != 3*time.Second {
		t.Fatalf("retryDelay() = %v", got)
	}
	if got := client.retryDelay(0, "999999999999999999"); got != defaultMaxRetryDelay {
		t.Fatalf("capped retryDelay() = %v", got)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	client = NewClient(ClientConfig{BaseURL: server.URL, Timeout: 10 * time.Millisecond, MaxRetries: -1}, server.Client())
	err := client.Get(context.Background(), "token", "/slow", nil, &struct{}{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Get() error = %v, want deadline exceeded", err)
	}
}

type leakingError struct{ Secret string }

func (e leakingError) Error() string { return e.Secret }

func noWait(context.Context, time.Duration) error { return nil }
