package config

import (
	"strings"
	"testing"
	"time"
)

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

	cfg, err := LoadPlatform()
	if err != nil {
		t.Fatalf("LoadPlatform() error = %v", err)
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
}

func TestLoadFeishuConfigAllowsOptionalTenantKey(t *testing.T) {
	setRequiredEnv(t)
	setCompleteFeishuEnv(t)
	t.Setenv("FEISHU_TENANT_KEY", "")

	cfg, err := LoadPlatform()
	if err != nil {
		t.Fatalf("LoadPlatform() error = %v", err)
	}

	if !cfg.FeishuEnabled {
		t.Fatal("FeishuEnabled = false, want true")
	}
	if cfg.FeishuTenantKey != "" {
		t.Fatalf("FeishuTenantKey = %q, want empty", cfg.FeishuTenantKey)
	}
}

func TestLoadSessionDefaultsWhenFeishuDisabled(t *testing.T) {
	setRequiredEnv(t)
	clearFeishuEnv(t)

	cfg, err := LoadPlatform()
	if err != nil {
		t.Fatalf("LoadPlatform() error = %v", err)
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
		"OAUTH_ENCRYPTION_KEY",
		"FRONTEND_ORIGIN",
	}

	for _, missing := range required {
		t.Run(missing, func(t *testing.T) {
			setRequiredEnv(t)
			setCompleteFeishuEnv(t)
			t.Setenv(missing, "")

			_, err := LoadPlatform()
			if err == nil {
				t.Fatalf("LoadPlatform() error = nil, want missing %s", missing)
			}
			if !strings.Contains(err.Error(), missing) {
				t.Fatalf("LoadPlatform() error = %q, want it to mention %s", err, missing)
			}
		})
	}
}

func TestLoadRejectsInvalidSessionConfig(t *testing.T) {
	setRequiredEnv(t)
	clearFeishuEnv(t)
	t.Setenv("SESSION_COOKIE_SECURE", "sometimes")

	if _, err := LoadPlatform(); err == nil || !strings.Contains(err.Error(), "SESSION_COOKIE_SECURE") {
		t.Fatalf("LoadPlatform() error = %v, want invalid SESSION_COOKIE_SECURE", err)
	}

	t.Setenv("SESSION_COOKIE_SECURE", "false")
	t.Setenv("SESSION_TTL", "tomorrow")
	if _, err := LoadPlatform(); err == nil || !strings.Contains(err.Error(), "SESSION_TTL") {
		t.Fatalf("LoadPlatform() error = %v, want invalid SESSION_TTL", err)
	}
}

func TestLoadRejectsNonPositiveSessionTTL(t *testing.T) {
	for _, ttl := range []string{"0s", "-1m"} {
		t.Run(ttl, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv("SESSION_TTL", ttl)

			if _, err := LoadPlatform(); err == nil || !strings.Contains(err.Error(), "SESSION_TTL") {
				t.Fatalf("LoadPlatform() error = %v, want non-positive SESSION_TTL rejection", err)
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
		"SESSION_COOKIE_SECURE",
		"SESSION_TTL",
	} {
		t.Setenv(key, "")
	}
}
