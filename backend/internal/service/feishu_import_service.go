package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

const InitialFeishuRevision = "initial"

type FeishuImportInput struct {
	UserID         string
	KBID           string
	OAuthAccountID string
	URL            string
}

type CreateFeishuDocumentInput struct {
	OwnerUserID    uuid.UUID
	KBID           uuid.UUID
	OAuthAccountID uuid.UUID
	SourceType     string
	SourceRef      string
	SourceURL      string
	Title          string
}

type FeishuImportRepository interface {
	CreateFeishuDocumentAndEnqueue(ctx context.Context, input CreateFeishuDocumentInput, enqueuer FeishuSyncEnqueuer) (generated.Document, error)
}

type FeishuSyncEnqueuer interface {
	EnqueueFeishuSync(ctx context.Context, documentID, requestedRevision string) error
}

type FeishuSyncTxEnqueuer interface {
	EnqueueFeishuSyncTx(ctx context.Context, tx pgx.Tx, documentID, requestedRevision string) error
}

type FeishuImport struct {
	resolver ports.SourceResolver
	repo     FeishuImportRepository
	queue    FeishuSyncEnqueuer
}

func NewFeishuImport(resolver ports.SourceResolver, repo FeishuImportRepository, queue FeishuSyncEnqueuer) *FeishuImport {
	return &FeishuImport{resolver: resolver, repo: repo, queue: queue}
}

type ErrFeishuAlreadyImported struct{}

func (*ErrFeishuAlreadyImported) Error() string { return "Feishu resource already imported" }

type ErrFeishuImportOwnership struct{}

func (*ErrFeishuImportOwnership) Error() string { return "knowledge base or OAuth account not found" }

func (s *FeishuImport) Import(ctx context.Context, input FeishuImportInput) (*domain.Document, error) {
	ref, err := s.resolver.Resolve(input.URL)
	if err != nil {
		return nil, err
	}
	userID, err := uuid.Parse(input.UserID)
	if err != nil {
		return nil, &ErrFeishuImportOwnership{}
	}
	kbID, err := uuid.Parse(input.KBID)
	if err != nil {
		return nil, &ErrFeishuImportOwnership{}
	}
	accountID, err := uuid.Parse(input.OAuthAccountID)
	if err != nil {
		return nil, &ErrFeishuImportOwnership{}
	}

	row, err := s.repo.CreateFeishuDocumentAndEnqueue(ctx, CreateFeishuDocumentInput{
		OwnerUserID: userID, KBID: kbID, OAuthAccountID: accountID,
		SourceType: "feishu-" + string(ref.Type), SourceRef: ref.Identity,
		SourceURL: ref.OriginalURL.String(), Title: "Feishu " + string(ref.Type),
	}, s.queue)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ErrFeishuImportOwnership{}
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uniq_docs_kb_source" {
			return nil, &ErrFeishuAlreadyImported{}
		}
		if errors.Is(err, errFeishuSyncEnqueue) {
			return nil, errors.New("sync enqueue failed")
		}
		return nil, fmt.Errorf("create Feishu document: %w", err)
	}
	return rowToDocFull(row), nil
}

var errFeishuSyncEnqueue = errors.New("sync enqueue failed")

type SQLFeishuImportRepository struct {
	pool importTxBeginner
}

type importTxBeginner interface {
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}

func NewSQLFeishuImportRepository(pool importTxBeginner) *SQLFeishuImportRepository {
	return &SQLFeishuImportRepository{pool: pool}
}

func (r *SQLFeishuImportRepository) CreateFeishuDocumentAndEnqueue(ctx context.Context, input CreateFeishuDocumentInput, enqueuer FeishuSyncEnqueuer) (generated.Document, error) {
	txEnqueuer, ok := enqueuer.(FeishuSyncTxEnqueuer)
	if !ok {
		return generated.Document{}, errFeishuSyncEnqueue
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return generated.Document{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	sourceURL := input.SourceURL
	row, err := generated.New(tx).CreateFeishuDocumentForOwner(ctx, generated.CreateFeishuDocumentForOwnerParams{
		SourceType: input.SourceType, SourceRef: input.SourceRef, SourceUrl: &sourceURL,
		OauthAccountID: input.OAuthAccountID, OwnerUserID: input.OwnerUserID,
		KbID: input.KBID, Title: input.Title,
	})
	if err != nil {
		return generated.Document{}, err
	}
	if err := txEnqueuer.EnqueueFeishuSyncTx(ctx, tx, row.ID.String(), InitialFeishuRevision); err != nil {
		return generated.Document{}, errFeishuSyncEnqueue
	}
	if err := tx.Commit(ctx); err != nil {
		return generated.Document{}, err
	}
	return row, nil
}
