package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/zenith-wang/it-wiki/backend/internal/config"
	httpx "github.com/zenith-wang/it-wiki/backend/internal/http"
)

type noopAuthDB struct{}

func (noopAuthDB) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	panic("unexpected database call during composition")
}
func (noopAuthDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	panic("unexpected database call during composition")
}
func (noopAuthDB) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	panic("unexpected database call during composition")
}

func TestBuildAuthHandlerConstructsProductionRouter(t *testing.T) {
	cfg := &config.Config{
		FeishuEnabled: true, FeishuAppID: "app-id", FeishuAppSecret: "app-secret",
		FeishuRedirectURL: "https://api.example.test/api/v1/auth/feishu/callback",
		FeishuTenantKey:   "tenant", OAuthEncryptionKey: "0123456789abcdef0123456789abcdef",
		SessionCookieSecure: true, SessionTTL: time.Hour, FrontendOrigin: "https://app.example.test",
	}
	authHandler, err := buildAuthHandler(cfg, noopAuthDB{})
	if err != nil {
		t.Fatalf("buildAuthHandler() error = %v", err)
	}
	router := httpx.NewRouter(httpx.Handlers{Auth: authHandler})

	start := httptest.NewRecorder()
	router.ServeHTTP(start, httptest.NewRequest(http.MethodGet, "/api/v1/auth/feishu/start", nil))
	if start.Code != http.StatusFound {
		t.Fatalf("OAuth start status = %d, want 302", start.Code)
	}
}

func TestBuildAuthHandlerFailsClosedWhenFeishuDisabled(t *testing.T) {
	authHandler, err := buildAuthHandler(&config.Config{}, noopAuthDB{})
	if err != nil {
		t.Fatalf("buildAuthHandler() error = %v", err)
	}
	router := httpx.NewRouter(httpx.Handlers{Auth: authHandler})

	health := httptest.NewRecorder()
	router.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d", health.Code)
	}
	business := httptest.NewRecorder()
	router.ServeHTTP(business, httptest.NewRequest(http.MethodGet, "/api/v1/kbs", nil))
	if business.Code != http.StatusUnauthorized {
		t.Fatalf("business status = %d, want 401", business.Code)
	}
}

type authRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f authRoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRequiredFeishuScopesCoverSupportedImports(t *testing.T) {
	want := []string{
		"offline_access",
		"contact:user.base:readonly",
		"contact:user.email:readonly",
		"docx:document:readonly",
		"sheets:spreadsheet:readonly",
		"bitable:app:readonly",
		"wiki:wiki:readonly",
	}
	if !slices.Equal(requiredFeishuScopes(), want) {
		t.Fatalf("requiredFeishuScopes() = %#v, want %#v", requiredFeishuScopes(), want)
	}
}

func TestProductionAuthRejectsMissingRequiredScopeBeforeSession(t *testing.T) {
	cfg := &config.Config{
		FeishuEnabled: true, FeishuAppID: "app-id", FeishuAppSecret: "app-secret",
		FeishuRedirectURL: "https://api.example.test/api/v1/auth/feishu/callback",
		FeishuTenantKey:   "tenant", OAuthEncryptionKey: "0123456789abcdef0123456789abcdef",
		SessionCookieSecure: true, SessionTTL: time.Hour, FrontendOrigin: "https://app.example.test",
	}
	granted := requiredFeishuScopes()
	granted = granted[:len(granted)-1]
	client := &http.Client{Transport: authRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body := "{\"code\":0,\"access_token\":\"access\",\"refresh_token\":\"refresh\",\"expires_in\":3600,\"scope\":\"" + strings.Join(granted, " ") + "\"}"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}
	authHandler, err := buildAuthHandlerWithClient(cfg, noopAuthDB{}, client)
	if err != nil {
		t.Fatalf("buildAuthHandlerWithClient() error = %v", err)
	}

	start := httptest.NewRecorder()
	authHandler.Start(start, httptest.NewRequest(http.MethodGet, "/api/v1/auth/feishu/start", nil))
	location, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse OAuth location: %v", err)
	}
	state := location.Query().Get("state")
	var stateCookie *http.Cookie
	for _, cookie := range start.Result().Cookies() {
		if cookie.Name == httpx.OAuthStateCookieName {
			stateCookie = cookie
		}
	}
	if state == "" || stateCookie == nil {
		t.Fatalf("state/cookie = %q %#v", state, stateCookie)
	}

	callback := httptest.NewRequest(http.MethodGet, "/api/v1/auth/feishu/callback?state="+url.QueryEscape(state)+"&code=code", nil)
	callback.AddCookie(stateCookie)
	rec := httptest.NewRecorder()
	authHandler.Callback(rec, callback)

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://app.example.test/login?error=feishu_reauth_required" {
		t.Fatalf("callback status/location = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == httpx.SessionCookieName && cookie.Value != "" {
			t.Fatalf("callback issued session cookie after scope failure: %#v", cookie)
		}
	}
}
