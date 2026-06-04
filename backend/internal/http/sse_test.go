package http

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteSSEEventFormatsJSON(t *testing.T) {
	rec := httptest.NewRecorder()

	WriteSSEHeaders(rec)
	err := WriteSSEEvent(rec, "token", map[string]string{"text": "hello"})
	if err != nil {
		t.Fatalf("WriteSSEEvent() error = %v", err)
	}

	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: token\n") {
		t.Fatalf("SSE body missing event line: %q", body)
	}
	if !strings.Contains(body, "data: {\"text\":\"hello\"}\n") {
		t.Fatalf("SSE body missing data line: %q", body)
	}
	if !strings.HasSuffix(body, "\n\n") {
		t.Fatalf("SSE body = %q, want blank line suffix", body)
	}
}
