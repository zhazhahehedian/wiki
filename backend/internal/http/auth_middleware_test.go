package http

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	authstore "github.com/zenith-wang/it-wiki/backend/internal/auth"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

func TestAuthMiddlewareRejectsMissingAndExpiredSessions(t *testing.T) {
	tests := []struct {
		name     string
		cookie   string
		storeErr error
	}{
		{name: "missing cookie"},
		{name: "expired session", cookie: "expired", storeErr: authstore.ErrSessionExpired},
		{name: "revoked session", cookie: "revoked", storeErr: authstore.ErrSessionNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{getErr: tt.storeErr}, fakeUserResolver{})
			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("protected handler called") })
			req := httptest.NewRequest(http.MethodGet, "/api/v1/kbs", nil)
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: tt.cookie})
			}
			rec := httptest.NewRecorder()

			h.Middleware(next).ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			_ = json.NewDecoder(rec.Body).Decode(&body)
			if body.Error.Code != CodeUnauthenticated {
				t.Fatalf("error code = %q", body.Error.Code)
			}
		})
	}
}

func TestAuthMiddlewareStoresCurrentUserAndUserID(t *testing.T) {
	user := domain.User{ID: "user-1", DisplayName: "Ada"}
	h := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{session: domain.Session{UserID: user.ID}}, fakeUserResolver{user: user})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, ok := CurrentUserFromContext(r.Context())
		if !ok || gotUser.ID != user.ID || UserIDFromContext(r.Context()) != user.ID {
			t.Fatalf("context user/id = %#v %q", gotUser, UserIDFromContext(r.Context()))
		}
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kbs", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "valid"})
	rec := httptest.NewRecorder()

	h.Middleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAuthMiddlewareEnforcesOriginAndSessionBoundCSRFForUnsafeMethods(t *testing.T) {
	csrf := "session-csrf"
	hash := sha256.Sum256([]byte(csrf))
	h := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{session: domain.Session{UserID: "user-1", CSRFTokenHash: hash[:]}}, fakeUserResolver{user: domain.User{ID: "user-1"}})
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

	tests := []struct {
		name   string
		method string
		origin string
		csrf   string
		want   int
	}{
		{name: "safe method", method: http.MethodGet, want: http.StatusNoContent},
		{name: "valid mutation", method: http.MethodPost, origin: "https://app.example.test", csrf: csrf, want: http.StatusNoContent},
		{name: "missing origin", method: http.MethodPost, csrf: csrf, want: http.StatusForbidden},
		{name: "foreign origin", method: http.MethodPost, origin: "https://evil.test", csrf: csrf, want: http.StatusForbidden},
		{name: "wrong csrf", method: http.MethodDelete, origin: "https://app.example.test", csrf: "wrong", want: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/v1/kbs/1", nil)
			req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "valid"})
			req.Header.Set("Origin", tt.origin)
			req.Header.Set(CSRFHeaderName, tt.csrf)
			rec := httptest.NewRecorder()

			h.Middleware(next).ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestRouterKeepsHealthAndOAuthPublicButProtectsBusinessRoutes(t *testing.T) {
	h := newTestAuthHandler(t, &fakeAuthFlow{state: "state"}, &fakeSessionStore{}, fakeUserResolver{})
	router := NewRouter(Handlers{Auth: h, FrontendOrigin: "https://app.example.test"})

	for _, path := range []string{"/healthz", "/api/healthz", "/api/v1/auth/feishu/start"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code == http.StatusUnauthorized {
			t.Fatalf("public route %s returned 401", path)
		}
	}

	for _, path := range []string{"/api/v1/auth/me", "/api/v1/kbs", "/api/v1/docs/id", "/api/v1/conversations/id/messages"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("protected route %s status = %d, want 401", path, rec.Code)
		}
	}
}
