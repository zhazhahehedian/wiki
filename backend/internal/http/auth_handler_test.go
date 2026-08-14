package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

type fakeAuthFlow struct {
	state         string
	result        service.AuthResult
	err           error
	completeCalls int
}

func (f *fakeAuthFlow) BeginOAuth(context.Context) (string, error) { return f.state, f.err }
func (f *fakeAuthFlow) CompleteOAuth(context.Context, string, string) (service.AuthResult, error) {
	f.completeCalls++
	return f.result, f.err
}

type fakeSessionStore struct {
	session domain.Session
	getErr  error
	revoked string
}

func (f *fakeSessionStore) Create(context.Context, string, time.Time) (domain.Session, string, string, error) {
	return domain.Session{}, "", "", errors.New("not implemented")
}
func (f *fakeSessionStore) Get(context.Context, string) (domain.Session, error) {
	return f.session, f.getErr
}
func (f *fakeSessionStore) Revoke(_ context.Context, token string) error {
	f.revoked = token
	return f.getErr
}
func (f *fakeSessionStore) CleanupExpired(context.Context) (int, error) { return 0, nil }

type fakeUserResolver struct {
	user domain.User
	err  error
}

func (f fakeUserResolver) User(context.Context, string) (domain.User, error) { return f.user, f.err }

func newTestAuthHandler(t *testing.T, flow *fakeAuthFlow, sessions *fakeSessionStore, users fakeUserResolver) *AuthHandler {
	t.Helper()
	h, err := NewAuthHandler(AuthHandlerConfig{
		AuthorizeURL:   "https://accounts.feishu.cn/open-apis/authen/v1/authorize",
		AppID:          "app-id",
		RedirectURL:    "https://api.example.test/api/v1/auth/feishu/callback",
		Scopes:         []string{"offline_access", "contact:user.base:readonly"},
		FrontendOrigin: "https://app.example.test",
		FrontendPath:   "/auth/callback",
		CookieSecure:   true,
		SessionTTL:     time.Hour,
	}, flow, sessions, users)
	if err != nil {
		t.Fatalf("NewAuthHandler() error = %v", err)
	}
	return h
}

func TestAuthStartRedirectsToFeishuWithServerState(t *testing.T) {
	h := newTestAuthHandler(t, &fakeAuthFlow{state: "opaque-state"}, &fakeSessionStore{}, fakeUserResolver{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/feishu/start?redirect=https://evil.test", nil)
	rec := httptest.NewRecorder()

	h.Start(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if location.Host != "accounts.feishu.cn" || location.Query().Get("state") != "opaque-state" {
		t.Fatalf("Location = %q", location.String())
	}
	if location.Query().Get("app_id") != "app-id" || location.Query().Get("client_id") != "app-id" ||
		location.Query().Get("response_type") != "code" || location.Query().Get("redirect_uri") == "" ||
		location.Query().Get("scope") != "offline_access contact:user.base:readonly" {
		t.Fatalf("Location query = %v", location.Query())
	}
	if strings.Contains(location.String(), "evil") {
		t.Fatalf("Location accepted caller redirect: %q", location.String())
	}
}

func TestAuthCallbackSetsBoundedCookiesAndUsesConfiguredRedirect(t *testing.T) {
	expires := time.Now().Add(time.Hour)
	flow := &fakeAuthFlow{result: service.AuthResult{
		Session:      domain.Session{ExpiresAt: expires},
		SessionToken: "opaque-session",
		CSRFToken:    "opaque-csrf",
	}}
	h := newTestAuthHandler(t, flow, &fakeSessionStore{}, fakeUserResolver{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/feishu/callback?state=s&code=c&redirect=https://evil.test", nil)
	req.AddCookie(&http.Cookie{Name: OAuthStateCookieName, Value: "s"})
	rec := httptest.NewRecorder()

	h.Callback(rec, req)

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://app.example.test/auth/callback" {
		t.Fatalf("status/location = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	cookies := rec.Result().Cookies()
	clearedState := findCookie(cookies, OAuthStateCookieName)
	if clearedState == nil || clearedState.MaxAge >= 0 {
		t.Fatalf("state cookie not cleared after success: %#v", clearedState)
	}
	session := findCookie(cookies, SessionCookieName)
	if session == nil {
		t.Fatalf("cookies = %v, missing session", cookies)
	}
	if session.Name != SessionCookieName || session.Value != "opaque-session" || !session.HttpOnly || !session.Secure || session.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie = %#v", session)
	}
	if session.MaxAge <= 0 || session.MaxAge > 3600 {
		t.Fatalf("session MaxAge = %d", session.MaxAge)
	}
	csrf := findCookie(cookies, CSRFCookieName)
	if csrf == nil {
		t.Fatalf("cookies = %v, missing csrf", cookies)
	}
	if csrf.Name != CSRFCookieName || csrf.Value != "opaque-csrf" || csrf.HttpOnly || !csrf.Secure || csrf.SameSite != http.SameSiteLaxMode {
		t.Fatalf("csrf cookie = %#v", csrf)
	}
}

func TestAuthHandlerRejectsUnsafeFrontendPath(t *testing.T) {
	_, err := NewAuthHandler(AuthHandlerConfig{
		AuthorizeURL:   "https://accounts.feishu.cn/open-apis/authen/v1/authorize",
		FrontendOrigin: "https://app.example.test",
		FrontendPath:   "//evil.test",
	}, &fakeAuthFlow{}, &fakeSessionStore{}, fakeUserResolver{})
	if err == nil {
		t.Fatal("NewAuthHandler() error = nil, want unsafe redirect rejection")
	}
}

func TestAuthMeReturnsContextUser(t *testing.T) {
	user := domain.User{ID: "user-1", DisplayName: "Ada", Email: "ada@example.test"}
	h := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{}, fakeUserResolver{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req = req.WithContext(WithCurrentUser(req.Context(), user))
	rec := httptest.NewRecorder()

	h.Me(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got domain.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil || got.ID != user.ID {
		t.Fatalf("response = %#v, err = %v", got, err)
	}
}

func TestAuthLogoutRevokesSessionAndClearsCookies(t *testing.T) {
	sessions := &fakeSessionStore{}
	h := newTestAuthHandler(t, &fakeAuthFlow{}, sessions, fakeUserResolver{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "opaque-session"})
	rec := httptest.NewRecorder()

	h.Logout(rec, req)

	if rec.Code != http.StatusNoContent || sessions.revoked != "opaque-session" {
		t.Fatalf("status/revoked = %d %q", rec.Code, sessions.revoked)
	}
	for _, cookie := range rec.Result().Cookies() {
		if cookie.MaxAge >= 0 {
			t.Fatalf("cookie not cleared: %#v", cookie)
		}
	}
}

func TestAuthStartBindsStateToHttpOnlyCookie(t *testing.T) {
	h := newTestAuthHandler(t, &fakeAuthFlow{state: "opaque-state"}, &fakeSessionStore{}, fakeUserResolver{})
	rec := httptest.NewRecorder()

	h.Start(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/feishu/start", nil))

	cookie := findCookie(rec.Result().Cookies(), OAuthStateCookieName)
	if cookie == nil || cookie.Value != "opaque-state" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("state cookie = %#v", cookie)
	}
	if cookie.MaxAge <= 0 || cookie.MaxAge > 300 {
		t.Fatalf("state cookie MaxAge = %d", cookie.MaxAge)
	}
}

func TestAuthCallbackRejectsMissingOrMismatchedStateCookieBeforeExchange(t *testing.T) {
	for _, tt := range []struct {
		name   string
		cookie string
	}{
		{name: "missing"},
		{name: "mismatch", cookie: "other-state"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			flow := &fakeAuthFlow{}
			h := newTestAuthHandler(t, flow, &fakeSessionStore{}, fakeUserResolver{})
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/feishu/callback?state=query-state&code=code", nil)
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: OAuthStateCookieName, Value: tt.cookie})
			}
			rec := httptest.NewRecorder()

			h.Callback(rec, req)

			if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://app.example.test/login?error=oauth_state_invalid" || flow.completeCalls != 0 {
				t.Fatalf("status/calls = %d/%d", rec.Code, flow.completeCalls)
			}
			if cleared := findCookie(rec.Result().Cookies(), OAuthStateCookieName); cleared != nil {
				t.Fatalf("invalid callback changed active state cookie: %#v", cleared)
			}
		})
	}
}

func TestAuthCallbackClearsStateCookieOnProviderFailureAndReplay(t *testing.T) {
	flow := &fakeAuthFlow{err: errors.New("provider failed")}
	h := newTestAuthHandler(t, flow, &fakeSessionStore{}, fakeUserResolver{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/feishu/callback?state=state&code=code", nil)
	req.AddCookie(&http.Cookie{Name: OAuthStateCookieName, Value: "state"})
	rec := httptest.NewRecorder()

	h.Callback(rec, req)

	if flow.completeCalls != 1 {
		t.Fatalf("CompleteOAuth calls = %d", flow.completeCalls)
	}
	cleared := findCookie(rec.Result().Cookies(), OAuthStateCookieName)
	if cleared == nil || cleared.MaxAge >= 0 {
		t.Fatalf("state cookie not cleared after failure: %#v", cleared)
	}

	replay := httptest.NewRecorder()
	h.Callback(replay, httptest.NewRequest(http.MethodGet, "/api/v1/auth/feishu/callback?state=state&code=code", nil))
	if replay.Code != http.StatusFound || replay.Header().Get("Location") != "https://app.example.test/login?error=oauth_state_invalid" || flow.completeCalls != 1 {
		t.Fatalf("replay status/calls = %d/%d", replay.Code, flow.completeCalls)
	}
}

func TestAuthCallbackRedirectsFailuresToConfiguredLogin(t *testing.T) {
	tests := []struct {
		name         string
		rawQuery     string
		cookie       string
		flowErr      error
		wantCode     string
		wantComplete int
	}{
		{
			name:     "provider cancellation",
			rawQuery: "state=state&error=access_denied&error_description=provider-secret&next=https%3A%2F%2Fevil.test",
			cookie:   "state",
			wantCode: "oauth_cancelled",
		},
		{
			name:     "provider error missing state",
			rawQuery: "error=access_denied&error_description=provider-secret",
			wantCode: "oauth_state_invalid",
		},
		{
			name:     "provider error mismatched state",
			rawQuery: "state=state&error=access_denied&error_description=provider-secret",
			cookie:   "other-state",
			wantCode: "oauth_state_invalid",
		},
		{
			name:     "invalid state",
			rawQuery: "state=state&code=code",
			cookie:   "other-state",
			wantCode: "oauth_state_invalid",
		},
		{
			name:         "consumed or expired state",
			rawQuery:     "state=state&code=code",
			cookie:       "state",
			flowErr:      &service.AuthError{Code: service.AuthErrorInvalidState, Cause: errors.New("state-secret")},
			wantCode:     "oauth_state_invalid",
			wantComplete: 1,
		},
		{
			name:         "tenant rejected",
			rawQuery:     "state=state&code=code",
			cookie:       "state",
			flowErr:      &service.AuthError{Code: service.AuthErrorTenantNotAllowed, Cause: errors.New("tenant-secret")},
			wantCode:     "tenant_not_allowed",
			wantComplete: 1,
		},
		{
			name:         "scope rejected",
			rawQuery:     "state=state&code=code",
			cookie:       "state",
			flowErr:      &service.AuthError{Code: service.AuthErrorInsufficientScope, Cause: errors.New("scope-secret")},
			wantCode:     "feishu_reauth_required",
			wantComplete: 1,
		},
		{
			name:         "reauth required",
			rawQuery:     "state=state&code=code",
			cookie:       "state",
			flowErr:      &service.AuthError{Code: service.AuthErrorReauthRequired, Cause: errors.New("token-secret")},
			wantCode:     "feishu_reauth_required",
			wantComplete: 1,
		},
		{
			name:         "provider or service unavailable",
			rawQuery:     "state=state&code=code",
			cookie:       "state",
			flowErr:      errors.New("provider body access_token=secret-token"),
			wantCode:     "auth_service_unavailable",
			wantComplete: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flow := &fakeAuthFlow{err: tt.flowErr}
			h := newTestAuthHandler(t, flow, &fakeSessionStore{}, fakeUserResolver{})
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/feishu/callback?"+tt.rawQuery, nil)
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: OAuthStateCookieName, Value: tt.cookie})
			}
			rec := httptest.NewRecorder()

			h.Callback(rec, req)

			wantLocation := "https://app.example.test/login?error=" + url.QueryEscape(tt.wantCode)
			if rec.Code != http.StatusFound || rec.Header().Get("Location") != wantLocation {
				t.Fatalf("status/location = %d %q, want %d %q", rec.Code, rec.Header().Get("Location"), http.StatusFound, wantLocation)
			}
			if flow.completeCalls != tt.wantComplete {
				t.Fatalf("CompleteOAuth calls = %d, want %d", flow.completeCalls, tt.wantComplete)
			}
			response := rec.Header().Get("Location") + rec.Body.String()
			for _, forbidden := range []string{"evil.test", "provider-secret", "state-secret", "tenant-secret", "scope-secret", "token-secret", "secret-token", "access_token"} {
				if strings.Contains(response, forbidden) {
					t.Fatalf("callback response exposed %q: %s", forbidden, response)
				}
			}
		})
	}
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}
