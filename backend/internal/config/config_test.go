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

func TestLoadWorkerLivenessDefaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.FeishuSyncJobTimeout != 10*time.Minute || cfg.IngestionJobTimeout != 20*time.Minute || cfg.FeishuReconcileJobTimeout != 2*time.Minute {
		t.Fatalf("worker timeouts = %s/%s/%s", cfg.FeishuSyncJobTimeout, cfg.IngestionJobTimeout, cfg.FeishuReconcileJobTimeout)
	}
	if cfg.FeishuReconcileInterval != 5*time.Minute || cfg.FeishuSyncLease != 45*time.Minute || cfg.RiverRescueStuckJobsAfter != 30*time.Minute {
		t.Fatalf("liveness durations = %s/%s/%s", cfg.FeishuReconcileInterval, cfg.FeishuSyncLease, cfg.RiverRescueStuckJobsAfter)
	}
	if cfg.FeishuReconcileBatchSize != 100 || cfg.FeishuReconcileMaxBatches != 10 {
		t.Fatalf("reconcile bounds = %d/%d", cfg.FeishuReconcileBatchSize, cfg.FeishuReconcileMaxBatches)
	}
}

func TestLoadWorkerLivenessOverrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("FEISHU_SYNC_JOB_TIMEOUT", "4m")
	t.Setenv("INGESTION_JOB_TIMEOUT", "8m")
	t.Setenv("FEISHU_RECONCILE_JOB_TIMEOUT", "90s")
	t.Setenv("FEISHU_RECONCILE_INTERVAL", "2m")
	t.Setenv("FEISHU_SYNC_LEASE", "20m")
	t.Setenv("RIVER_RESCUE_STUCK_JOBS_AFTER", "10m")
	t.Setenv("FEISHU_RECONCILE_BATCH_SIZE", "25")
	t.Setenv("FEISHU_RECONCILE_MAX_BATCHES", "4")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.FeishuSyncJobTimeout != 4*time.Minute || cfg.IngestionJobTimeout != 8*time.Minute || cfg.FeishuReconcileJobTimeout != 90*time.Second {
		t.Fatalf("worker timeouts = %s/%s/%s", cfg.FeishuSyncJobTimeout, cfg.IngestionJobTimeout, cfg.FeishuReconcileJobTimeout)
	}
	if cfg.FeishuReconcileInterval != 2*time.Minute || cfg.FeishuSyncLease != 20*time.Minute || cfg.RiverRescueStuckJobsAfter != 10*time.Minute {
		t.Fatalf("liveness durations = %s/%s/%s", cfg.FeishuReconcileInterval, cfg.FeishuSyncLease, cfg.RiverRescueStuckJobsAfter)
	}
	if cfg.FeishuReconcileBatchSize != 25 || cfg.FeishuReconcileMaxBatches != 4 {
		t.Fatalf("reconcile bounds = %d/%d", cfg.FeishuReconcileBatchSize, cfg.FeishuReconcileMaxBatches)
	}
}

func TestLoadRejectsUnsafeWorkerLivenessConfig(t *testing.T) {
	tests := []struct {
		name, key, value string
	}{
		{name: "invalid duration", key: "FEISHU_SYNC_JOB_TIMEOUT", value: "later"},
		{name: "nonpositive duration", key: "INGESTION_JOB_TIMEOUT", value: "0s"},
		{name: "interval reaches lease", key: "FEISHU_RECONCILE_INTERVAL", value: "45m"},
		{name: "lease cannot cover pipeline", key: "FEISHU_SYNC_LEASE", value: "30m"},
		{name: "rescue reaches worker timeout", key: "RIVER_RESCUE_STUCK_JOBS_AFTER", value: "20m"},
		{name: "batch too small", key: "FEISHU_RECONCILE_BATCH_SIZE", value: "0"},
		{name: "batch too large", key: "FEISHU_RECONCILE_BATCH_SIZE", value: "501"},
		{name: "pages too small", key: "FEISHU_RECONCILE_MAX_BATCHES", value: "0"},
		{name: "pages too large", key: "FEISHU_RECONCILE_MAX_BATCHES", value: "21"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv(tt.key, tt.value)

			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tt.key) {
				t.Fatalf("Load() error = %v, want %s rejection", err, tt.key)
			}
		})
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
	clearWorkerLivenessEnv(t)
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("S3_ENDPOINT", "http://localhost:9000")
	t.Setenv("S3_ACCESS_KEY", "access")
	t.Setenv("S3_SECRET_KEY", "secret")
	t.Setenv("S3_BUCKET", "bucket")
}

func clearWorkerLivenessEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"FEISHU_SYNC_JOB_TIMEOUT",
		"INGESTION_JOB_TIMEOUT",
		"FEISHU_RECONCILE_JOB_TIMEOUT",
		"FEISHU_RECONCILE_INTERVAL",
		"FEISHU_SYNC_LEASE",
		"RIVER_RESCUE_STUCK_JOBS_AFTER",
		"FEISHU_RECONCILE_BATCH_SIZE",
		"FEISHU_RECONCILE_MAX_BATCHES",
	} {
		t.Setenv(key, "")
	}
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
