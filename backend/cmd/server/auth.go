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

const (
	feishuScopeOfflineAccess = "offline_access"
	feishuScopeUserBaseRead  = "contact:user.base:readonly"
	feishuScopeUserEmailRead = "contact:user.email:readonly"
	feishuScopeDocxRead      = "docx:document:readonly"
	feishuScopeSheetsRead    = "sheets:spreadsheet:readonly"
	feishuScopeBitableRead   = "bitable:app:readonly"
	feishuScopeWikiRead      = "wiki:wiki:readonly"
)

func requiredFeishuScopes() []string {
	return []string{
		feishuScopeOfflineAccess,
		feishuScopeUserBaseRead,
		feishuScopeUserEmailRead,
		feishuScopeDocxRead,
		feishuScopeSheetsRead,
		feishuScopeBitableRead,
		feishuScopeWikiRead,
	}
}

func buildAuthHandler(cfg *config.Config, db generated.DBTX) (*httpx.AuthHandler, error) {
	return buildAuthHandlerWithClient(cfg, db, stdhttp.DefaultClient)
}

func buildAuthHandlerWithClient(cfg *config.Config, db generated.DBTX, httpClient *stdhttp.Client) (*httpx.AuthHandler, error) {
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
	}, httpClient)
	authService := service.NewAuth(service.AuthConfig{
		TenantKey: cfg.FeishuTenantKey, RequiredScopes: requiredFeishuScopes(), SessionTTL: cfg.SessionTTL,
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
