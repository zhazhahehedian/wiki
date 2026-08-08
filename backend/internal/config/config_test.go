package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadRAGDefaults(t *testing.T) {
	setRequiredEnv(t)
	clearRAGEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.RAGTopK != 8 {
		t.Fatalf("RAGTopK = %d, want 8", cfg.RAGTopK)
	}
	if cfg.RAGMinScore != 0 {
		t.Fatalf("RAGMinScore = %f, want 0", cfg.RAGMinScore)
	}
	if cfg.RAGHistoryMessages != 10 {
		t.Fatalf("RAGHistoryMessages = %d, want 10", cfg.RAGHistoryMessages)
	}
}

func TestLoadRAGBounds(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("RAG_TOP_K", "0")
	t.Setenv("RAG_MIN_SCORE", "0.42")
	t.Setenv("RAG_HISTORY_MESSAGES", "-1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.RAGTopK != 8 {
		t.Fatalf("RAGTopK = %d, want 8", cfg.RAGTopK)
	}
	if cfg.RAGMinScore != 0.42 {
		t.Fatalf("RAGMinScore = %f, want 0.42", cfg.RAGMinScore)
	}
	if cfg.RAGHistoryMessages != 10 {
		t.Fatalf("RAGHistoryMessages = %d, want 10", cfg.RAGHistoryMessages)
	}
}

func TestLoadFeishuAndSessionConfig(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("FEISHU_APP_ID", "cli_app")
	t.Setenv("FEISHU_APP_SECRET", "app-secret")
	t.Setenv("FEISHU_REDIRECT_URL", "https://wiki.example.com/api/v1/auth/feishu/callback")
	t.Setenv("FEISHU_TENANT_KEY", "tenant-key")
	t.Setenv("OAUTH_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("SESSION_COOKIE_SECURE", "true")
	t.Setenv("SESSION_TTL", "12h")
	t.Setenv("FRONTEND_ORIGIN", "https://wiki.example.com")
	t.Setenv("BOOTSTRAP_OWNER_FEISHU_OPEN_ID", "ou_bootstrap")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.FeishuEnabled {
		t.Fatal("FeishuEnabled = false, want true")
	}
	if cfg.FeishuAppID != "cli_app" || cfg.FeishuAppSecret != "app-secret" {
		t.Fatalf("Feishu credentials = %q/%q", cfg.FeishuAppID, cfg.FeishuAppSecret)
	}
	if cfg.FeishuRedirectURL != "https://wiki.example.com/api/v1/auth/feishu/callback" {
		t.Fatalf("FeishuRedirectURL = %q", cfg.FeishuRedirectURL)
	}
	if cfg.FeishuTenantKey != "tenant-key" {
		t.Fatalf("FeishuTenantKey = %q", cfg.FeishuTenantKey)
	}
	if cfg.OAuthEncryptionKey != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("OAuthEncryptionKey = %q", cfg.OAuthEncryptionKey)
	}
	if !cfg.SessionCookieSecure {
		t.Fatal("SessionCookieSecure = false, want true")
	}
	if cfg.SessionTTL != 12*time.Hour {
		t.Fatalf("SessionTTL = %s, want 12h", cfg.SessionTTL)
	}
	if cfg.FrontendOrigin != "https://wiki.example.com" {
		t.Fatalf("FrontendOrigin = %q", cfg.FrontendOrigin)
	}
	if cfg.BootstrapOwnerFeishuOpenID != "ou_bootstrap" {
		t.Fatalf("BootstrapOwnerFeishuOpenID = %q", cfg.BootstrapOwnerFeishuOpenID)
	}
}

func TestLoadSessionDefaultsWhenFeishuDisabled(t *testing.T) {
	setRequiredEnv(t)
	clearFeishuEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.FeishuEnabled {
		t.Fatal("FeishuEnabled = true, want false")
	}
	if cfg.SessionCookieSecure {
		t.Fatal("SessionCookieSecure = true, want false")
	}
	if cfg.SessionTTL != 24*time.Hour {
		t.Fatalf("SessionTTL = %s, want 24h", cfg.SessionTTL)
	}
}

func TestLoadRejectsIncompleteFeishuConfig(t *testing.T) {
	required := []string{
		"FEISHU_APP_ID",
		"FEISHU_APP_SECRET",
		"FEISHU_REDIRECT_URL",
		"FEISHU_TENANT_KEY",
		"OAUTH_ENCRYPTION_KEY",
		"FRONTEND_ORIGIN",
	}

	for _, missing := range required {
		t.Run(missing, func(t *testing.T) {
			setRequiredEnv(t)
			setCompleteFeishuEnv(t)
			t.Setenv(missing, "")

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() error = nil, want missing %s", missing)
			}
			if !strings.Contains(err.Error(), missing) {
				t.Fatalf("Load() error = %q, want it to mention %s", err, missing)
			}
		})
	}
}

func TestLoadRejectsInvalidSessionConfig(t *testing.T) {
	setRequiredEnv(t)
	clearFeishuEnv(t)
	t.Setenv("SESSION_COOKIE_SECURE", "sometimes")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SESSION_COOKIE_SECURE") {
		t.Fatalf("Load() error = %v, want invalid SESSION_COOKIE_SECURE", err)
	}

	t.Setenv("SESSION_COOKIE_SECURE", "false")
	t.Setenv("SESSION_TTL", "tomorrow")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SESSION_TTL") {
		t.Fatalf("Load() error = %v, want invalid SESSION_TTL", err)
	}
}

func TestLoadRejectsNonPositiveSessionTTL(t *testing.T) {
	for _, ttl := range []string{"0s", "-1m"} {
		t.Run(ttl, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv("SESSION_TTL", ttl)

			if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SESSION_TTL") {
				t.Fatalf("Load() error = %v, want non-positive SESSION_TTL rejection", err)
			}
		})
	}
}

func setRequiredEnv(t *testing.T) {
	t.Helper()
	clearFeishuEnv(t)
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("S3_ENDPOINT", "http://localhost:9000")
	t.Setenv("S3_ACCESS_KEY", "access")
	t.Setenv("S3_SECRET_KEY", "secret")
	t.Setenv("S3_BUCKET", "bucket")
}

func clearRAGEnv(t *testing.T) {
	t.Helper()
	t.Setenv("RAG_TOP_K", "")
	t.Setenv("RAG_MIN_SCORE", "")
	t.Setenv("RAG_HISTORY_MESSAGES", "")
}

func setCompleteFeishuEnv(t *testing.T) {
	t.Helper()
	t.Setenv("FEISHU_APP_ID", "cli_app")
	t.Setenv("FEISHU_APP_SECRET", "app-secret")
	t.Setenv("FEISHU_REDIRECT_URL", "https://wiki.example.com/api/v1/auth/feishu/callback")
	t.Setenv("FEISHU_TENANT_KEY", "tenant-key")
	t.Setenv("OAUTH_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("FRONTEND_ORIGIN", "https://wiki.example.com")
}

func clearFeishuEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"FEISHU_APP_ID",
		"FEISHU_APP_SECRET",
		"FEISHU_REDIRECT_URL",
		"FEISHU_TENANT_KEY",
		"OAUTH_ENCRYPTION_KEY",
		"FRONTEND_ORIGIN",
		"BOOTSTRAP_OWNER_FEISHU_OPEN_ID",
		"SESSION_COOKIE_SECURE",
		"SESSION_TTL",
	} {
		t.Setenv(key, "")
	}
}
