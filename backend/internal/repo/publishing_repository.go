package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zenith-wang/it-wiki/backend/internal/publishing"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type PublishingRepository struct {
	pool *pgxpool.Pool
	q    *generated.Queries
}

func NewPublishingRepository(pool *pgxpool.Pool) *PublishingRepository {
	return &PublishingRepository{pool, generated.New(pool)}
}
func publishingCredential(c generated.PublishingCredential) publishing.Credential {
	return publishing.Credential{ID: c.ID.String(), Name: c.Name, Prefix: c.TokenPrefix, Scopes: c.Scopes, ExpiresAt: c.ExpiresAt, RevokedAt: timePointer(c.RevokedAt), LastUsedAt: timePointer(c.LastUsedAt), CreatedAt: c.CreatedAt}
}
func (r *PublishingRepository) Create(ctx context.Context, user string, input publishing.CreateRequest, hash []byte, prefix string) (publishing.Credential, error) {
	id, e := uuid.Parse(user)
	if e != nil {
		return publishing.Credential{}, publishing.ErrUnauthenticated
	}
	tx, e := r.pool.Begin(ctx)
	if e != nil {
		return publishing.Credential{}, e
	}
	defer tx.Rollback(context.Background())
	q := r.q.WithTx(tx)
	who, e := identity(ctx, q, user)
	if e != nil {
		return publishing.Credential{}, e
	}
	c, e := q.CreatePublishingCredential(ctx, generated.CreatePublishingCredentialParams{UserID: id, Name: input.Name, TokenHash: hash, TokenPrefix: prefix, Scopes: input.Scopes, ExpiresAt: input.ExpiresAt})
	if e != nil {
		return publishing.Credential{}, e
	}
	e = writeAudit(ctx, q, who.OpenID, "create_api_credential", "api_credential", c.ID.String(), map[string]any{"name": c.Name, "scopes": c.Scopes, "expires_at": c.ExpiresAt})
	if e != nil {
		return publishing.Credential{}, e
	}
	return publishingCredential(c), tx.Commit(ctx)
}
func (r *PublishingRepository) List(ctx context.Context, user string) ([]publishing.Credential, error) {
	id, e := uuid.Parse(user)
	if e != nil {
		return nil, publishing.ErrUnauthenticated
	}
	rows, e := r.q.ListPublishingCredentials(ctx, id)
	if e != nil {
		return nil, e
	}
	out := []publishing.Credential{}
	for _, c := range rows {
		out = append(out, publishingCredential(c))
	}
	return out, nil
}
func (r *PublishingRepository) Revoke(ctx context.Context, user, credential string) error {
	id, e := uuid.Parse(user)
	if e != nil {
		return publishing.ErrUnauthenticated
	}
	cid, e := uuid.Parse(credential)
	if e != nil {
		return publishing.ErrNotFound
	}
	tx, e := r.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	q := r.q.WithTx(tx)
	who, e := identity(ctx, q, user)
	if e != nil {
		return e
	}
	_, e = q.RevokePublishingCredential(ctx, generated.RevokePublishingCredentialParams{UserID: id, ID: cid})
	if errors.Is(e, pgx.ErrNoRows) {
		return publishing.ErrNotFound
	}
	if e != nil {
		return e
	}
	if e = writeAudit(ctx, q, who.OpenID, "revoke_api_credential", "api_credential", credential, map[string]any{}); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (r *PublishingRepository) Authenticate(ctx context.Context, hash []byte) (publishing.Principal, error) {
	p, e := r.q.AuthenticatePublishingCredential(ctx, hash)
	if errors.Is(e, pgx.ErrNoRows) {
		return publishing.Principal{}, publishing.ErrUnauthenticated
	}
	if e != nil {
		return publishing.Principal{}, e
	}
	return publishing.Principal{UserID: p.UserID.String(), CredentialID: p.ID.String(), Scopes: p.Scopes}, nil
}
