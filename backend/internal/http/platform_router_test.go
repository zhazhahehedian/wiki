package http

import (
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"testing"

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
