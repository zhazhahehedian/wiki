package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port     string
	LogLevel string

	DatabaseURL string

	S3Endpoint     string
	S3AccessKey    string
	S3SecretKey    string
	S3Bucket       string
	S3Region       string
	S3UsePathStyle bool

	LLMBaseURL string
	LLMAPIKey  string
	LLMModel   string

	EmbeddingBaseURL string
	EmbeddingAPIKey  string
	EmbeddingModel   string
	EmbeddingDim     int

	TokenizerEncoding string
	ChunkSize         int
	ChunkOverlap      int
	EmbedBatchSize    int
	UploadMaxBytes    int64
	RiverMaxWorkers   int

	RAGTopK            int
	RAGMinScore        float32
	RAGHistoryMessages int

	FeishuEnabled      bool
	FeishuAppID        string
	FeishuAppSecret    string
	FeishuRedirectURL  string
	FeishuTenantKey    string
	OAuthEncryptionKey string

	SessionCookieSecure bool
	SessionTTL          time.Duration
	FrontendOrigin      string

	BootstrapOwnerFeishuOpenID string
}

func Load() (*Config, error) {
	dim, err := strconv.Atoi(getEnv("EMBEDDING_DIM", "1024"))
	if err != nil {
		return nil, fmt.Errorf("invalid EMBEDDING_DIM: %w", err)
	}
	usePathStyle, _ := strconv.ParseBool(getEnv("S3_USE_PATH_STYLE", "true"))

	sessionCookieSecure, err := strconv.ParseBool(getEnv("SESSION_COOKIE_SECURE", "false"))
	if err != nil {
		return nil, fmt.Errorf("invalid SESSION_COOKIE_SECURE: %w", err)
	}
	sessionTTL, err := time.ParseDuration(getEnv("SESSION_TTL", "24h"))
	if err != nil {
		return nil, fmt.Errorf("invalid SESSION_TTL: %w", err)
	}

	feishuValues := map[string]string{
		"FEISHU_APP_ID":        getEnv("FEISHU_APP_ID", ""),
		"FEISHU_APP_SECRET":    getEnv("FEISHU_APP_SECRET", ""),
		"FEISHU_REDIRECT_URL":  getEnv("FEISHU_REDIRECT_URL", ""),
		"FEISHU_TENANT_KEY":    getEnv("FEISHU_TENANT_KEY", ""),
		"OAUTH_ENCRYPTION_KEY": getEnv("OAUTH_ENCRYPTION_KEY", ""),
		"FRONTEND_ORIGIN":      getEnv("FRONTEND_ORIGIN", ""),
	}
	feishuEnabled := false
	for _, key := range []string{
		"FEISHU_APP_ID",
		"FEISHU_APP_SECRET",
		"FEISHU_REDIRECT_URL",
		"FEISHU_TENANT_KEY",
		"OAUTH_ENCRYPTION_KEY",
	} {
		feishuEnabled = feishuEnabled || feishuValues[key] != ""
	}
	if feishuEnabled {
		for _, key := range []string{
			"FEISHU_APP_ID",
			"FEISHU_APP_SECRET",
			"FEISHU_REDIRECT_URL",
			"FEISHU_TENANT_KEY",
			"OAUTH_ENCRYPTION_KEY",
			"FRONTEND_ORIGIN",
		} {
			if feishuValues[key] == "" {
				return nil, fmt.Errorf("missing required env var: %s", key)
			}
		}
	}

	cfg := &Config{
		Port:             getEnv("PORT", "8080"),
		LogLevel:         getEnv("LOG_LEVEL", "info"),
		DatabaseURL:      mustEnv("DATABASE_URL"),
		S3Endpoint:       mustEnv("S3_ENDPOINT"),
		S3AccessKey:      mustEnv("S3_ACCESS_KEY"),
		S3SecretKey:      mustEnv("S3_SECRET_KEY"),
		S3Bucket:         mustEnv("S3_BUCKET"),
		S3Region:         getEnv("S3_REGION", "us-east-1"),
		S3UsePathStyle:   usePathStyle,
		LLMBaseURL:       getEnv("LLM_BASE_URL", ""),
		LLMAPIKey:        getEnv("LLM_API_KEY", ""),
		LLMModel:         getEnv("LLM_MODEL", ""),
		EmbeddingBaseURL: getEnv("EMBEDDING_BASE_URL", ""),
		EmbeddingAPIKey:  getEnv("EMBEDDING_API_KEY", ""),
		EmbeddingModel:   getEnv("EMBEDDING_MODEL", ""),
		EmbeddingDim:     dim,

		FeishuEnabled:      feishuEnabled,
		FeishuAppID:        feishuValues["FEISHU_APP_ID"],
		FeishuAppSecret:    feishuValues["FEISHU_APP_SECRET"],
		FeishuRedirectURL:  feishuValues["FEISHU_REDIRECT_URL"],
		FeishuTenantKey:    feishuValues["FEISHU_TENANT_KEY"],
		OAuthEncryptionKey: feishuValues["OAUTH_ENCRYPTION_KEY"],

		SessionCookieSecure: sessionCookieSecure,
		SessionTTL:          sessionTTL,
		FrontendOrigin:      feishuValues["FRONTEND_ORIGIN"],

		BootstrapOwnerFeishuOpenID: getEnv("BOOTSTRAP_OWNER_FEISHU_OPEN_ID", ""),
	}

	cfg.TokenizerEncoding = getEnv("TOKENIZER_ENCODING", "cl100k_base")
	cfg.ChunkSize, _ = strconv.Atoi(getEnv("CHUNK_SIZE", "400"))
	cfg.ChunkOverlap, _ = strconv.Atoi(getEnv("CHUNK_OVERLAP", "60"))
	cfg.EmbedBatchSize, _ = strconv.Atoi(getEnv("EMBED_BATCH_SIZE", "64"))
	maxMB, _ := strconv.ParseInt(getEnv("UPLOAD_MAX_MB", "50"), 10, 64)
	cfg.UploadMaxBytes = maxMB * 1024 * 1024
	cfg.RiverMaxWorkers, _ = strconv.Atoi(getEnv("RIVER_MAX_WORKERS", "4"))
	cfg.RAGTopK, _ = strconv.Atoi(getEnv("RAG_TOP_K", "8"))
	minScore, _ := strconv.ParseFloat(getEnv("RAG_MIN_SCORE", "0.0"), 32)
	cfg.RAGMinScore = float32(minScore)
	cfg.RAGHistoryMessages, _ = strconv.Atoi(getEnv("RAG_HISTORY_MESSAGES", "10"))
	if cfg.RAGTopK < 1 {
		cfg.RAGTopK = 8
	}
	if cfg.RAGHistoryMessages < 0 {
		cfg.RAGHistoryMessages = 10
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("missing required env var: %s", key))
	}
	return v
}
