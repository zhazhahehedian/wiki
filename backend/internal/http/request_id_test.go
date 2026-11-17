package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

func TestRequestIDRejectsInjectionAndKeepsSafeIDs(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = middleware.GetReqID(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	safe := httptest.NewRequest(http.MethodGet, "/", nil)
	safe.Header.Set("X-Request-Id", "client-123")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, safe)
	if seen != "client-123" || rec.Header().Get("X-Request-Id") != "client-123" {
		t.Fatalf("safe id seen=%q header=%q", seen, rec.Header().Get("X-Request-Id"))
	}

	bad := httptest.NewRequest(http.MethodGet, "/", nil)
	bad.Header.Set("X-Request-Id", "ok\nlevel=error forged")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, bad)
	if seen == "" || strings.Contains(seen, "\n") || strings.Contains(seen, "forged") || rec.Header().Get("X-Request-Id") != seen {
		t.Fatalf("unsafe id kept: seen=%q header=%q", seen, rec.Header().Get("X-Request-Id"))
	}
}
