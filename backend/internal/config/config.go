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

	DatabaseURL      string
	DatabaseProxyURL string

	S3Endpoint     string
	S3AccessKey    string
	S3SecretKey    string
	S3Bucket       string
	S3Region       string
	S3UsePathStyle bool

	FeishuEnabled      bool
	FeishuAppID        string
	FeishuAppSecret    string
	FeishuRedirectURL  string
	FeishuTenantKey    string
	OAuthEncryptionKey string

	SessionCookieSecure bool
	SessionTTL          time.Duration
	FrontendOrigin      string

	BootstrapAdminFeishuOpenID string
}

// LoadPlatform loads the Capability Hub server configuration. Knowledge
// embedding settings are read by the knowledge wiring itself.
func LoadPlatform() (*Config, error) {
	usePathStyle, _ := strconv.ParseBool(getEnv("S3_USE_PATH_STYLE", "true"))

	sessionCookieSecure, err := strconv.ParseBool(getEnv("SESSION_COOKIE_SECURE", "false"))
	if err != nil {
		return nil, fmt.Errorf("invalid SESSION_COOKIE_SECURE: %w", err)
	}
	sessionTTL, err := time.ParseDuration(getEnv("SESSION_TTL", "24h"))
	if err != nil {
		return nil, fmt.Errorf("invalid SESSION_TTL: %w", err)
	}
	if sessionTTL <= 0 {
		return nil, fmt.Errorf("invalid SESSION_TTL: must be positive")
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
		"OAUTH_ENCRYPTION_KEY",
	} {
		feishuEnabled = feishuEnabled || feishuValues[key] != ""
	}
	if feishuEnabled {
		for _, key := range []string{
			"FEISHU_APP_ID",
			"FEISHU_APP_SECRET",
			"FEISHU_REDIRECT_URL",
			"OAUTH_ENCRYPTION_KEY",
			"FRONTEND_ORIGIN",
		} {
			if feishuValues[key] == "" {
				return nil, fmt.Errorf("missing required env var: %s", key)
			}
		}
	}

	for _, key := range []string{"DATABASE_URL", "S3_ENDPOINT", "S3_ACCESS_KEY", "S3_SECRET_KEY", "S3_BUCKET"} {
		if os.Getenv(key) == "" {
			return nil, fmt.Errorf("missing required env var: %s", key)
		}
	}
	cfg := &Config{
		Port:             getEnv("PORT", "8080"),
		LogLevel:         getEnv("LOG_LEVEL", "info"),
		DatabaseURL:      mustEnv("DATABASE_URL"),
		DatabaseProxyURL: os.Getenv("DATABASE_PROXY_URL"),
		S3Endpoint:       mustEnv("S3_ENDPOINT"),
		S3AccessKey:      mustEnv("S3_ACCESS_KEY"),
		S3SecretKey:      mustEnv("S3_SECRET_KEY"),
		S3Bucket:         mustEnv("S3_BUCKET"),
		S3Region:         getEnv("S3_REGION", "us-east-1"),
		S3UsePathStyle:   usePathStyle,

		FeishuEnabled:      feishuEnabled,
		FeishuAppID:        feishuValues["FEISHU_APP_ID"],
		FeishuAppSecret:    feishuValues["FEISHU_APP_SECRET"],
		FeishuRedirectURL:  feishuValues["FEISHU_REDIRECT_URL"],
		FeishuTenantKey:    feishuValues["FEISHU_TENANT_KEY"],
		OAuthEncryptionKey: feishuValues["OAUTH_ENCRYPTION_KEY"],

		SessionCookieSecure: sessionCookieSecure,
		SessionTTL:          sessionTTL,
		FrontendOrigin:      feishuValues["FRONTEND_ORIGIN"],

		BootstrapAdminFeishuOpenID: getEnv("BOOTSTRAP_ADMIN_FEISHU_OPEN_ID", ""),
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
