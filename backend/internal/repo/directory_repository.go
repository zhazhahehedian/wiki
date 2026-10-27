package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/zenith-wang/it-wiki/backend/internal/directory"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

func (r *AuthRepository) FeishuAccount(ctx context.Context, user string) (domain.OAuthAccount, error) {
	id, e := uuid.Parse(user)
	if e != nil {
		return domain.OAuthAccount{}, e
	}
	a, e := r.queries.GetOAuthAccountForUser(ctx, id)
	if e != nil {
		return domain.OAuthAccount{}, e
	}
	return oauthAccount(a), nil
}
func (r *AuthRepository) Membership(ctx context.Context, account string) (directory.Membership, error) {
	id, e := uuid.Parse(account)
	if e != nil {
		return directory.Membership{}, e
	}
	m, e := r.queries.GetFeishuMembership(ctx, id)
	return directory.Membership{DepartmentIDs: m.DepartmentIds, ExpiresAt: m.ExpiresAt}, e
}
func (r *AuthRepository) SaveMembership(ctx context.Context, account string, ids []string, expires time.Time) error {
	id, e := uuid.Parse(account)
	if e != nil {
		return e
	}
	return r.queries.SaveFeishuMembership(ctx, generated.SaveFeishuMembershipParams{AccountID: id, DepartmentIds: ids, ExpiresAt: expires})
}
