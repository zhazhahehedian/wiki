package main

import (
	"fmt"
	stdhttp "net/http"

	authstore "github.com/zenith-wang/it-wiki/backend/internal/auth"
	"github.com/zenith-wang/it-wiki/backend/internal/config"
	httpx "github.com/zenith-wang/it-wiki/backend/internal/http"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/feishu"
	"github.com/zenith-wang/it-wiki/backend/internal/repo"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

func buildAuthHandler(cfg *config.Config, db generated.DBTX) (*httpx.AuthHandler, error) {
	if !cfg.FeishuEnabled {
		return httpx.NewDisabledAuthHandler(), nil
	}

	protector, err := authstore.NewAESGCMProtector([]byte(cfg.OAuthEncryptionKey))
	if err != nil {
		return nil, fmt.Errorf("oauth token protector: %w", err)
	}
	repository := repo.NewAuthRepository(db)
	sessions := authstore.NewPersistentSessionStore(repository, nil)
	states := authstore.NewMemoryOAuthStateStore(nil)
	oauthClient := feishu.NewOAuthClient(feishu.OAuthConfig{
		AppID: cfg.FeishuAppID, AppSecret: cfg.FeishuAppSecret, RedirectURL: cfg.FeishuRedirectURL,
	}, stdhttp.DefaultClient)
	authService := service.NewAuth(service.AuthConfig{
		TenantKey: cfg.FeishuTenantKey, SessionTTL: cfg.SessionTTL,
	}, oauthClient, protector, sessions, states, repository)

	handler, err := httpx.NewAuthHandler(httpx.AuthHandlerConfig{
		AppID: cfg.FeishuAppID, RedirectURL: cfg.FeishuRedirectURL,
		FrontendOrigin: cfg.FrontendOrigin, FrontendPath: "/",
		CookieSecure: cfg.SessionCookieSecure, SessionTTL: cfg.SessionTTL,
	}, authService, sessions, repository)
	if err != nil {
		return nil, fmt.Errorf("auth http handler: %w", err)
	}
	return handler, nil
}
