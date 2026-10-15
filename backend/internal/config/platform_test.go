package config

import "testing"

func TestLoadPlatformIgnoresRetiredRuntime(t *testing.T) {
	setRequiredEnv(t)
	for _, key := range []string{"EMBEDDING_DIM", "FEISHU_SYNC_JOB_TIMEOUT", "INGESTION_JOB_TIMEOUT", "FEISHU_SYNC_LEASE", "FEISHU_RECONCILE_BATCH_SIZE", "TOKENIZER_ENCODING"} {
		t.Setenv(key, "retired-invalid-value")
	}
	if _, err := LoadPlatform(); err != nil {
		t.Fatalf("platform requires retired settings: %v", err)
	}
}

func TestLoadPlatformRequiresStorageAndDatabase(t *testing.T) {
	for _, key := range []string{"DATABASE_URL", "S3_ENDPOINT", "S3_ACCESS_KEY", "S3_SECRET_KEY", "S3_BUCKET"} {
		t.Run(key, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv(key, "")
			if _, err := LoadPlatform(); err == nil {
				t.Fatalf("missing %s accepted", key)
			}
		})
	}
}

func TestPlatformLoadsAdminBootstrap(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("BOOTSTRAP_ADMIN_FEISHU_OPEN_ID", "ou_platform_admin")
	cfg, err := LoadPlatform()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BootstrapAdminFeishuOpenID != "ou_platform_admin" {
		t.Fatal("platform admin bootstrap not loaded")
	}
}
