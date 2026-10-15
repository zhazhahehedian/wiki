package http

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

func TestPlatformRoutesRetireKnowledgeBaseProducts(t *testing.T) {
	user := domain.User{ID: "11111111-1111-4111-8111-111111111111", DisplayName: "Ada"}
	sessions := &fakeSessionStore{session: domain.Session{UserID: user.ID}}
	auth := newTestAuthHandler(t, &fakeAuthFlow{}, sessions, fakeUserResolver{user: user})
	router := NewPlatformRouter(auth)
	for _, path := range []string{"/api/v1/kbs", "/api/v1/kbs/kb/docs", "/api/v1/kbs/kb/feishu-imports", "/api/v1/docs/doc", "/api/v1/docs/doc/sync", "/api/v1/conversations/chat/messages/stream"} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
			req := httptest.NewRequest(method, path, nil)
			req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "session"})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s %s = %d, want 404", method, path, rec.Code)
			}
		}
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "session"})
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated me = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPlatformLogoutPreservesCSRFAndRevocation(t *testing.T) {
	user := domain.User{ID: "11111111-1111-4111-8111-111111111111"}
	hash := sha256.Sum256([]byte("csrf"))
	sessions := &fakeSessionStore{session: domain.Session{UserID: user.ID, CSRFTokenHash: hash[:]}}
	router := NewPlatformRouter(newTestAuthHandler(t, &fakeAuthFlow{}, sessions, fakeUserResolver{user: user}))
	for _, tc := range []struct {
		origin, csrf string
		want         int
	}{
		{"", "", http.StatusForbidden},
		{"https://evil.test", "csrf", http.StatusForbidden},
		{"https://app.example.test", "bad", http.StatusForbidden},
		{"https://app.example.test", "csrf", http.StatusNoContent},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "session"})
		req.Header.Set("Origin", tc.origin)
		req.Header.Set(CSRFHeaderName, tc.csrf)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("logout = %d, want %d", rec.Code, tc.want)
		}
		if tc.want == http.StatusForbidden && sessions.revoked != "" {
			t.Fatal("invalid CSRF revoked session")
		}
	}
	if sessions.revoked != "session" {
		t.Fatal("valid logout did not revoke session")
	}
}

func TestRequestTimeoutIsScopedAndPreservesParentCancellation(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		model        bool
	}{
		{"POST", "/api/v1/skill-builder/turn", true},
		{"POST", "/api/v1/playground/chat/stream", true},
		{"POST", "/api/v1/playground/models", false},
		{"POST", "/api/v1/skill-builder/drafts", false},
		{"GET", "/api/v1/skill-builder/turn", false},
		{"POST", "/api/v1/skill-builder/turn/unknown", false},
		{"GET", "/healthz", false},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			handler := requestTimeout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				deadline, ok := r.Context().Deadline()
				remaining := time.Until(deadline)
				minimum, maximum := 50*time.Second, time.Minute
				if tc.model {
					minimum, maximum = 5*time.Minute, 6*time.Minute
				}
				if !ok || remaining < minimum || remaining > maximum {
					t.Errorf("unexpected route deadline: %v", remaining)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != http.StatusNoContent {
				t.Fatal(w.Code)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest("POST", "/api/v1/skill-builder/turn", nil).WithContext(ctx)
	handler := requestTimeout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		if !errors.Is(r.Context().Err(), context.Canceled) {
			t.Fatal("caller cancellation detached")
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), req)
}
