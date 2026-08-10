package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type ownershipBootstrapPool interface {
	generated.DBTX
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

type OwnershipBootstrapRepository struct {
	pool    ownershipBootstrapPool
	queries *generated.Queries
}

func NewOwnershipBootstrapRepository(pool ownershipBootstrapPool) *OwnershipBootstrapRepository {
	return &OwnershipBootstrapRepository{pool: pool, queries: generated.New(pool)}
}

func (r *OwnershipBootstrapRepository) CountOrphanOwnership(ctx context.Context) (int64, error) {
	count, err := r.queries.CountOrphanOwnership(ctx)
	return int64(count), err
}

func (r *OwnershipBootstrapRepository) BootstrapOwner(ctx context.Context, candidateID uuid.UUID, openID string) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := generated.New(tx)
	ownerID, err := q.GetOAuthUserIDByProviderIdentity(ctx, generated.GetOAuthUserIDByProviderIdentityParams{Provider: "feishu", ProviderUserID: openID})
	if errors.Is(err, pgx.ErrNoRows) {
		ownerID = candidateID
		if err := q.UpsertBootstrapUser(ctx, generated.UpsertBootstrapUserParams{ID: ownerID, DisplayName: "Legacy owner"}); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	owner := pgtype.UUID{Bytes: ownerID, Valid: true}
	if err := q.AssignOrphanKnowledgeBases(ctx, owner); err != nil {
		return err
	}
	if err := q.AssignOrphanConversations(ctx, owner); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
