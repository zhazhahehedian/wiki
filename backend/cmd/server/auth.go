package main

import (
	"fmt"
	stdhttp "net/http"

	authstore "github.com/zenith-wang/it-wiki/backend/internal/auth"
	"github.com/zenith-wang/it-wiki/backend/internal/config"
	"github.com/zenith-wang/it-wiki/backend/internal/directory"
	httpx "github.com/zenith-wang/it-wiki/backend/internal/http"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/feishu"
	"github.com/zenith-wang/it-wiki/backend/internal/repo"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

const (
	feishuScopeOfflineAccess = "offline_access"
	feishuScopeUserBaseRead  = "contact:user.base:readonly"
	feishuScopeUserEmailRead = "contact:user.email:readonly"
)

func requiredFeishuScopes() []string {
	return []string{
		feishuScopeOfflineAccess,
		feishuScopeUserBaseRead,
		feishuScopeUserEmailRead,
	}
}

func buildAuthHandler(cfg *config.Config, db generated.DBTX) (*httpx.AuthHandler, error) {
	return buildAuthHandlerWithClient(cfg, db, stdhttp.DefaultClient)
}

func buildAuthHandlerWithClient(cfg *config.Config, db generated.DBTX, httpClient *stdhttp.Client) (*httpx.AuthHandler, error) {
	handler, _, err := buildAuthRuntime(cfg, db, httpClient)
	return handler, err
}

func buildAuthRuntime(cfg *config.Config, db generated.DBTX, httpClient *stdhttp.Client) (*httpx.AuthHandler, *service.Auth, error) {
	if !cfg.FeishuEnabled {
		return httpx.NewDisabledAuthHandler(), nil, nil
	}

	protector, err := authstore.NewAESGCMProtector([]byte(cfg.OAuthEncryptionKey))
	if err != nil {
		return nil, nil, fmt.Errorf("oauth token protector: %w", err)
	}
	repository := repo.NewAuthRepository(db)
	sessions := authstore.NewPersistentSessionStore(repository, nil)
	states := authstore.NewMemoryOAuthStateStore(nil)
	oauthClient := feishu.NewOAuthClient(feishu.OAuthConfig{
		AppID: cfg.FeishuAppID, AppSecret: cfg.FeishuAppSecret, RedirectURL: cfg.FeishuRedirectURL,
	}, httpClient)
	authService := service.NewAuth(service.AuthConfig{
		TenantKey: cfg.FeishuTenantKey, RequiredScopes: requiredFeishuScopes(), SessionTTL: cfg.SessionTTL,
	}, oauthClient, protector, sessions, states, repository)

	handler, err := httpx.NewAuthHandler(platformAuthHandlerConfig(cfg), authService, sessions, repository)
	if err != nil {
		return nil, nil, fmt.Errorf("auth http handler: %w", err)
	}
	return handler, authService, nil
}

func platformAuthHandlerConfig(cfg *config.Config) httpx.AuthHandlerConfig {
	scopes := requiredFeishuScopes()
	for _, scope := range directory.RequiredScopes {
		found := false
		for _, existing := range scopes {
			if existing == scope {
				found = true
				break
			}
		}
		if !found {
			scopes = append(scopes, scope)
		}
	}
	return httpx.AuthHandlerConfig{
		AppID: cfg.FeishuAppID, RedirectURL: cfg.FeishuRedirectURL,
		Scopes:         scopes,
		FrontendOrigin: cfg.FrontendOrigin, FrontendPath: "/hub",
		CookieSecure: cfg.SessionCookieSecure, SessionTTL: cfg.SessionTTL,
	}
}
