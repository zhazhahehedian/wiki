package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	authstore "github.com/zenith-wang/it-wiki/backend/internal/auth"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type AuthRepository struct {
	queries *generated.Queries
}

func NewAuthRepository(db generated.DBTX) *AuthRepository {
	return &AuthRepository{queries: generated.New(db)}
}

func OAuthUserID(provider, providerUserID string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(provider+":"+providerUserID)).String()
}

func (r *AuthRepository) UpsertOAuthIdentity(ctx context.Context, identity domain.FeishuIdentity, account domain.OAuthAccount) (domain.User, domain.OAuthAccount, error) {
	candidateID, err := uuid.Parse(OAuthUserID(account.Provider, identity.OpenID))
	if err != nil {
		return domain.User{}, domain.OAuthAccount{}, fmt.Errorf("parse candidate user id: %w", err)
	}
	row, err := r.queries.UpsertOAuthIdentity(ctx, generated.UpsertOAuthIdentityParams{
		PProvider:              account.Provider,
		PProviderUserID:        identity.OpenID,
		PCandidateUserID:       candidateID,
		PDisplayName:           identity.DisplayName,
		PAvatarUrl:             identity.AvatarURL,
		PEmail:                 identity.Email,
		PTenantKey:             account.TenantKey,
		PAccessTokenEncrypted:  append([]byte(nil), account.AccessTokenEncrypted...),
		PRefreshTokenEncrypted: append([]byte(nil), account.RefreshTokenEncrypted...),
		PAccessTokenExpiresAt:  account.AccessTokenExpiresAt,
		PRefreshTokenExpiresAt: nullableTime(account.RefreshTokenExpiresAt),
		PScopes:                append([]string(nil), account.Scopes...),
	})
	if err != nil {
		return domain.User{}, domain.OAuthAccount{}, err
	}
	email := ""
	if row.Email != nil {
		email = *row.Email
	}
	user := domain.User{
		ID: row.UserID.String(), DisplayName: row.DisplayName, AvatarURL: row.AvatarUrl,
		Email: email, CreatedAt: row.UserCreatedAt, UpdatedAt: row.UserUpdatedAt,
	}
	return user, domain.OAuthAccount{
		ID: row.AccountID.String(), UserID: row.UserID.String(),
		Provider: row.Provider, ProviderUserID: row.ProviderUserID, TenantKey: row.TenantKey,
		AccessTokenEncrypted:  append([]byte(nil), row.AccessTokenEncrypted...),
		RefreshTokenEncrypted: append([]byte(nil), row.RefreshTokenEncrypted...),
		AccessTokenExpiresAt:  row.AccessTokenExpiresAt, RefreshTokenExpiresAt: timePointer(row.RefreshTokenExpiresAt),
		Scopes: append([]string(nil), row.Scopes...), ReauthRequired: row.ReauthRequired,
		CreatedAt: row.AccountCreatedAt, UpdatedAt: row.AccountUpdatedAt,
	}, nil
}

func (r *AuthRepository) OAuthAccount(ctx context.Context, id string) (domain.OAuthAccount, error) {
	accountID, err := uuid.Parse(id)
	if err != nil {
		return domain.OAuthAccount{}, err
	}
	row, err := r.queries.GetOAuthAccount(ctx, accountID)
	if err != nil {
		return domain.OAuthAccount{}, err
	}
	return oauthAccount(row), nil
}

func (r *AuthRepository) UpdateOAuthTokens(ctx context.Context, account domain.OAuthAccount) error {
	accountID, err := uuid.Parse(account.ID)
	if err != nil {
		return err
	}
	return r.queries.UpdateOAuthAccountTokens(ctx, generated.UpdateOAuthAccountTokensParams{
		PID: accountID, PAccessTokenEncrypted: append([]byte(nil), account.AccessTokenEncrypted...),
		PRefreshTokenEncrypted: append([]byte(nil), account.RefreshTokenEncrypted...),
		PAccessTokenExpiresAt:  account.AccessTokenExpiresAt,
		PRefreshTokenExpiresAt: nullableTime(account.RefreshTokenExpiresAt),
		PScopes:                append([]string(nil), account.Scopes...), PReauthRequired: account.ReauthRequired,
	})
}

func (r *AuthRepository) MarkOAuthAccountReauthRequired(ctx context.Context, id string) error {
	accountID, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	return r.queries.MarkOAuthAccountReauthRequired(ctx, accountID)
}

func (r *AuthRepository) User(ctx context.Context, userID string) (domain.User, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return domain.User{}, err
	}
	row, err := r.queries.GetAuthUser(ctx, id)
	if err != nil {
		return domain.User{}, err
	}
	email := ""
	if row.Email != nil {
		email = *row.Email
	}
	return domain.User{
		ID: row.ID.String(), DisplayName: row.DisplayName, AvatarURL: row.AvatarUrl,
		Email: email, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

func (r *AuthRepository) CreateSession(ctx context.Context, session domain.Session) (domain.Session, error) {
	id, err := uuid.Parse(session.ID)
	if err != nil {
		return domain.Session{}, err
	}
	userID, err := uuid.Parse(session.UserID)
	if err != nil {
		return domain.Session{}, err
	}
	row, err := r.queries.CreateUserSession(ctx, generated.CreateUserSessionParams{
		PID: id, PUserID: userID, PTokenHash: append([]byte(nil), session.TokenHash...),
		PCsrfTokenHash: append([]byte(nil), session.CSRFTokenHash...),
		PExpiresAt:     session.ExpiresAt, PCreatedAt: session.CreatedAt,
	})
	if err != nil {
		return domain.Session{}, err
	}
	return userSession(row), nil
}

func (r *AuthRepository) SessionByTokenHash(ctx context.Context, tokenHash []byte) (domain.Session, error) {
	row, err := r.queries.GetUserSessionByTokenHash(ctx, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, authstore.ErrSessionNotFound
	}
	if err != nil {
		return domain.Session{}, err
	}
	return userSession(row), nil
}

func (r *AuthRepository) DeleteSessionByTokenHash(ctx context.Context, tokenHash []byte) error {
	count, err := r.queries.DeleteUserSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return err
	}
	if count == 0 {
		return authstore.ErrSessionNotFound
	}
	return nil
}

func (r *AuthRepository) DeleteExpiredSessions(ctx context.Context, now time.Time) (int, error) {
	count, err := r.queries.DeleteExpiredUserSessions(ctx, now)
	return int(count), err
}

func userSession(row generated.UserSession) domain.Session {
	return domain.Session{
		ID: row.ID.String(), UserID: row.UserID.String(),
		TokenHash:     append([]byte(nil), row.TokenHash...),
		CSRFTokenHash: append([]byte(nil), row.CsrfTokenHash...),
		ExpiresAt:     row.ExpiresAt, CreatedAt: row.CreatedAt,
	}
}

func oauthAccount(row generated.OauthAccount) domain.OAuthAccount {
	return domain.OAuthAccount{
		ID: row.ID.String(), UserID: row.UserID.String(),
		Provider: row.Provider, ProviderUserID: row.ProviderUserID, TenantKey: row.TenantKey,
		AccessTokenEncrypted:  append([]byte(nil), row.AccessTokenEncrypted...),
		RefreshTokenEncrypted: append([]byte(nil), row.RefreshTokenEncrypted...),
		AccessTokenExpiresAt:  row.AccessTokenExpiresAt, RefreshTokenExpiresAt: timePointer(row.RefreshTokenExpiresAt),
		Scopes: append([]string(nil), row.Scopes...), ReauthRequired: row.ReauthRequired,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func nullableTime(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

func timePointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}
