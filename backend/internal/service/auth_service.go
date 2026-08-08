package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type AuthErrorCode string

const (
	AuthErrorInvalidState      AuthErrorCode = "invalid_oauth_state"
	AuthErrorTenantNotAllowed  AuthErrorCode = "tenant_not_allowed"
	AuthErrorInsufficientScope AuthErrorCode = "insufficient_scope"
	AuthErrorReauthRequired    AuthErrorCode = "reauth_required"
)

type AuthError struct {
	Code  AuthErrorCode
	Cause error
}

func (e *AuthError) Error() string { return string(e.Code) }
func (e *AuthError) Unwrap() error { return e.Cause }

func IsAuthError(err error, code AuthErrorCode) bool {
	var authErr *AuthError
	return errors.As(err, &authErr) && authErr.Code == code
}

type AuthConfig struct {
	TenantKey      string
	RequiredScopes []string
	SessionTTL     time.Duration
	Now            func() time.Time
}

type AuthResult struct {
	User         domain.User
	Account      domain.OAuthAccount
	Session      domain.Session
	SessionToken string
	CSRFToken    string
}

type Auth struct {
	config       AuthConfig
	oauth        ports.OAuthClient
	protector    ports.TokenProtector
	sessions     ports.SessionStore
	states       ports.OAuthStateStore
	repository   ports.AuthRepository
	refreshLocks sync.Map
}

func NewAuth(config AuthConfig, oauth ports.OAuthClient, protector ports.TokenProtector, sessions ports.SessionStore, states ports.OAuthStateStore, repository ports.AuthRepository) *Auth {
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Auth{config: config, oauth: oauth, protector: protector, sessions: sessions, states: states, repository: repository}
}

func (s *Auth) BeginOAuth(ctx context.Context) (string, error) {
	_, rawState, err := s.states.Create(ctx, s.config.TenantKey, s.config.Now().Add(5*time.Minute))
	if err != nil {
		return "", fmt.Errorf("create oauth state: %w", err)
	}
	return rawState, nil
}

func (s *Auth) CompleteOAuth(ctx context.Context, rawState, code string) (AuthResult, error) {
	state, err := s.states.Consume(ctx, rawState, s.config.TenantKey)
	if err != nil {
		return AuthResult{}, &AuthError{Code: AuthErrorInvalidState, Cause: err}
	}
	token, err := s.oauth.ExchangeCode(ctx, code)
	if err != nil {
		return AuthResult{}, fmt.Errorf("exchange oauth authorization code: %w", err)
	}
	if !containsAllScopes(token.Scopes, s.config.RequiredScopes) {
		return AuthResult{}, &AuthError{Code: AuthErrorInsufficientScope}
	}
	identity, err := s.oauth.UserInfo(ctx, token.AccessToken)
	if err != nil {
		return AuthResult{}, fmt.Errorf("get oauth user info: %w", err)
	}
	if identity.TenantKey != state.TenantKey {
		return AuthResult{}, &AuthError{Code: AuthErrorTenantNotAllowed}
	}
	accessTokenEncrypted, err := s.protector.Encrypt(token.AccessToken)
	if err != nil {
		return AuthResult{}, fmt.Errorf("protect oauth access token: %w", err)
	}
	refreshTokenEncrypted, err := s.protector.Encrypt(token.RefreshToken)
	if err != nil {
		return AuthResult{}, fmt.Errorf("protect oauth refresh token: %w", err)
	}
	account := domain.OAuthAccount{
		Provider:              "feishu",
		TenantKey:             identity.TenantKey,
		AccessTokenEncrypted:  accessTokenEncrypted,
		RefreshTokenEncrypted: refreshTokenEncrypted,
		AccessTokenExpiresAt:  token.AccessTokenExpiresAt,
		RefreshTokenExpiresAt: token.RefreshTokenExpiresAt,
		Scopes:                append([]string(nil), token.Scopes...),
	}
	user, account, err := s.repository.UpsertOAuthIdentity(ctx, identity, account)
	if err != nil {
		return AuthResult{}, fmt.Errorf("persist oauth identity: %w", err)
	}
	session, sessionToken, csrfToken, err := s.sessions.Create(ctx, user.ID, s.config.Now().Add(s.config.SessionTTL))
	if err != nil {
		return AuthResult{}, fmt.Errorf("create local session: %w", err)
	}
	return AuthResult{User: user, Account: account, Session: session, SessionToken: sessionToken, CSRFToken: csrfToken}, nil
}

func (s *Auth) AccessToken(ctx context.Context, accountID string) (string, error) {
	lockValue, _ := s.refreshLocks.LoadOrStore(accountID, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	account, err := s.repository.OAuthAccount(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("load oauth account: %w", err)
	}
	if account.ReauthRequired {
		return "", &AuthError{Code: AuthErrorReauthRequired}
	}
	if s.config.Now().Before(account.AccessTokenExpiresAt) {
		accessToken, err := s.protector.Decrypt(account.AccessTokenEncrypted)
		if err != nil {
			return "", fmt.Errorf("decrypt oauth access token: %w", err)
		}
		return accessToken, nil
	}
	refreshToken, err := s.protector.Decrypt(account.RefreshTokenEncrypted)
	if err != nil {
		return "", fmt.Errorf("decrypt oauth refresh token: %w", err)
	}
	refreshed, err := s.oauth.RefreshToken(ctx, refreshToken)
	if err != nil {
		var oauthErr *ports.OAuthError
		if errors.As(err, &oauthErr) && oauthErr.Code == "invalid_grant" {
			if markErr := s.repository.MarkOAuthAccountReauthRequired(ctx, account.ID); markErr != nil {
				return "", fmt.Errorf("mark oauth account for reauthentication: %w", markErr)
			}
			return "", &AuthError{Code: AuthErrorReauthRequired, Cause: err}
		}
		return "", fmt.Errorf("refresh oauth token: %w", err)
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = refreshToken
	}
	accessEncrypted, err := s.protector.Encrypt(refreshed.AccessToken)
	if err != nil {
		return "", fmt.Errorf("protect refreshed access token: %w", err)
	}
	refreshEncrypted, err := s.protector.Encrypt(refreshed.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("protect refreshed refresh token: %w", err)
	}
	account.AccessTokenEncrypted = accessEncrypted
	account.RefreshTokenEncrypted = refreshEncrypted
	account.AccessTokenExpiresAt = refreshed.AccessTokenExpiresAt
	account.RefreshTokenExpiresAt = refreshed.RefreshTokenExpiresAt
	if len(refreshed.Scopes) > 0 {
		account.Scopes = append([]string(nil), refreshed.Scopes...)
	}
	account.ReauthRequired = false
	if err := s.repository.UpdateOAuthTokens(ctx, account); err != nil {
		return "", fmt.Errorf("persist refreshed oauth token: %w", err)
	}
	return refreshed.AccessToken, nil
}

func containsAllScopes(granted, required []string) bool {
	set := make(map[string]struct{}, len(granted))
	for _, scope := range granted {
		set[scope] = struct{}{}
	}
	for _, scope := range required {
		if _, ok := set[scope]; !ok {
			return false
		}
	}
	return true
}
