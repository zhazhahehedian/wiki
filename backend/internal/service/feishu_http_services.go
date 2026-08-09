package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type ErrFeishuAccountNotFound struct{}

func (*ErrFeishuAccountNotFound) Error() string { return "Feishu account not found" }

type ErrFeishuReauthRequired struct{}

func (*ErrFeishuReauthRequired) Error() string { return "Feishu account requires reauthentication" }

type FeishuAccountQueries interface {
	GetOAuthAccountForUser(context.Context, uuid.UUID) (generated.OauthAccount, error)
}

type FeishuAccounts struct{ queries FeishuAccountQueries }

func NewFeishuAccounts(queries FeishuAccountQueries) *FeishuAccounts {
	return &FeishuAccounts{queries: queries}
}

func (s *FeishuAccounts) Resolve(ctx context.Context, userID string) (domain.OAuthAccount, error) {
	id, err := requiredOwnerUUID(userID)
	if err != nil {
		return domain.OAuthAccount{}, err
	}
	row, err := s.queries.GetOAuthAccountForUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.OAuthAccount{}, &ErrFeishuAccountNotFound{}
	}
	if err != nil {
		return domain.OAuthAccount{}, fmt.Errorf("resolve Feishu account: %w", err)
	}
	if row.ReauthRequired {
		return domain.OAuthAccount{}, &ErrFeishuReauthRequired{}
	}
	return domain.OAuthAccount{ID: row.ID.String(), UserID: row.UserID.String(), Provider: row.Provider, ProviderUserID: row.ProviderUserID, TenantKey: row.TenantKey}, nil
}

type ErrFeishuSyncInProgress struct{}

func (*ErrFeishuSyncInProgress) Error() string { return "Feishu document sync is already in progress" }

type ErrUnsupportedFeishuDocument struct{}

func (*ErrUnsupportedFeishuDocument) Error() string { return "document is not a Feishu resource" }

type FeishuSyncQueries interface {
	GetDocumentForOwner(context.Context, generated.GetDocumentForOwnerParams) (generated.Document, error)
	GetFeishuDocumentForOwnerAndAccount(context.Context, generated.GetFeishuDocumentForOwnerAndAccountParams) (generated.Document, error)
}

type FeishuSync struct {
	queries FeishuSyncQueries
	queue   FeishuSyncEnqueuer
}

func NewFeishuSync(queries FeishuSyncQueries, queue FeishuSyncEnqueuer) *FeishuSync {
	return &FeishuSync{queries: queries, queue: queue}
}

func (s *FeishuSync) Sync(ctx context.Context, userID, accountID, documentID string) (*domain.Document, error) {
	ownerID, err := ownerUUID(userID)
	if err != nil {
		return nil, err
	}
	docID, err := uuid.Parse(documentID)
	if err != nil {
		return nil, &ErrDocNotFound{ID: documentID}
	}
	accountUUID, err := uuid.Parse(accountID)
	if err != nil {
		return nil, &ErrFeishuAccountNotFound{}
	}
	owned, err := s.queries.GetDocumentForOwner(ctx, generated.GetDocumentForOwnerParams{ID: docID, OwnerUserID: ownerID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &ErrDocNotFound{ID: documentID}
	}
	if err != nil {
		return nil, fmt.Errorf("get document for sync: %w", err)
	}
	if !strings.HasPrefix(owned.SourceType, "feishu-") {
		return nil, &ErrUnsupportedFeishuDocument{}
	}
	if owned.SyncStatus == "syncing" {
		return nil, &ErrFeishuSyncInProgress{}
	}
	row, err := s.queries.GetFeishuDocumentForOwnerAndAccount(ctx, generated.GetFeishuDocumentForOwnerAndAccountParams{ID: docID, OwnerUserID: ownerID, OauthAccountID: accountUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &ErrDocNotFound{ID: documentID}
	}
	if err != nil {
		return nil, fmt.Errorf("authorize document sync: %w", err)
	}
	revision := InitialFeishuRevision
	if row.RemoteRevision != nil && *row.RemoteRevision != "" {
		revision = *row.RemoteRevision
	}
	if err := s.queue.EnqueueFeishuSync(ctx, row.ID.String(), revision); err != nil {
		return nil, fmt.Errorf("enqueue Feishu sync: %w", err)
	}
	return rowToDocFull(row), nil
}
