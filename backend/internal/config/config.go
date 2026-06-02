package config

import (
	"fmt"
	"os"
	"strconv"
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
}

func Load() (*Config, error) {
	dim, err := strconv.Atoi(getEnv("EMBEDDING_DIM", "1024"))
	if err != nil {
		return nil, fmt.Errorf("invalid EMBEDDING_DIM: %w", err)
	}
	usePathStyle, _ := strconv.ParseBool(getEnv("S3_USE_PATH_STYLE", "true"))

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
	}

	cfg.TokenizerEncoding = getEnv("TOKENIZER_ENCODING", "cl100k_base")
	cfg.ChunkSize, _ = strconv.Atoi(getEnv("CHUNK_SIZE", "800"))
	cfg.ChunkOverlap, _ = strconv.Atoi(getEnv("CHUNK_OVERLAP", "120"))
	cfg.EmbedBatchSize, _ = strconv.Atoi(getEnv("EMBED_BATCH_SIZE", "64"))
	maxMB, _ := strconv.ParseInt(getEnv("UPLOAD_MAX_MB", "50"), 10, 64)
	cfg.UploadMaxBytes = maxMB * 1024 * 1024
	cfg.RiverMaxWorkers, _ = strconv.Atoi(getEnv("RIVER_MAX_WORKERS", "4"))

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
