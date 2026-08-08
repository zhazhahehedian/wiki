package main

import (
	"context"
	"net/http"
	"net/http/httptest"
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
