package config

import "testing"

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

func setRequiredEnv(t *testing.T) {
	t.Helper()
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
