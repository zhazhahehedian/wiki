package ports

import (
	"context"
	"fmt"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

type OAuthClient interface {
	ExchangeCode(ctx context.Context, code string) (domain.OAuthToken, error)
	UserInfo(ctx context.Context, accessToken string) (domain.FeishuIdentity, error)
	RefreshToken(ctx context.Context, refreshToken string) (domain.OAuthToken, error)
}

type OAuthError struct {
	Code    string
	Message string
	Cause   error
}

func (e *OAuthError) Error() string {
	if e.Message == "" {
		return "oauth provider error: " + e.Code
	}
	return fmt.Sprintf("oauth provider error (%s): %s", e.Code, e.Message)
}

func (e *OAuthError) Unwrap() error { return e.Cause }

type TokenProtector interface {
	Encrypt(plaintext string) ([]byte, error)
	Decrypt(ciphertext []byte) (string, error)
}

type SessionStore interface {
	Create(ctx context.Context, userID string, expiresAt time.Time) (domain.Session, string, string, error)
	Get(ctx context.Context, rawToken string) (domain.Session, error)
	Revoke(ctx context.Context, rawToken string) error
	CleanupExpired(ctx context.Context) (int, error)
}

type OAuthStateStore interface {
	Create(ctx context.Context, tenantKey string, expiresAt time.Time) (domain.OAuthState, string, error)
	Consume(ctx context.Context, rawState, tenantKey string) (domain.OAuthState, error)
}

type AuthRepository interface {
	UpsertOAuthIdentity(ctx context.Context, identity domain.FeishuIdentity, account domain.OAuthAccount) (domain.User, domain.OAuthAccount, error)
	OAuthAccount(ctx context.Context, id string) (domain.OAuthAccount, error)
	UpdateOAuthTokens(ctx context.Context, account domain.OAuthAccount) error
	MarkOAuthAccountReauthRequired(ctx context.Context, id string) error
}
