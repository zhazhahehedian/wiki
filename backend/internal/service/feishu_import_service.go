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
	CreateFeishuDocument(ctx context.Context, input CreateFeishuDocumentInput) (generated.Document, error)
	FailFeishuImportEnqueue(ctx context.Context, documentID uuid.UUID, safeError string) (bool, error)
}

type FeishuSyncEnqueuer interface {
	EnqueueFeishuSync(ctx context.Context, documentID, requestedRevision string) error
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

	row, err := s.repo.CreateFeishuDocument(ctx, CreateFeishuDocumentInput{
		OwnerUserID: userID, KBID: kbID, OAuthAccountID: accountID,
		SourceType: "feishu-" + string(ref.Type), SourceRef: ref.Identity,
		SourceURL: ref.OriginalURL.String(), Title: "Feishu " + string(ref.Type),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ErrFeishuImportOwnership{}
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uniq_docs_kb_source" {
			return nil, &ErrFeishuAlreadyImported{}
		}
		return nil, fmt.Errorf("create Feishu document: %w", err)
	}
	if err := s.queue.EnqueueFeishuSync(ctx, row.ID.String(), InitialFeishuRevision); err != nil {
		_, _ = s.repo.FailFeishuImportEnqueue(ctx, row.ID, "sync enqueue failed")
		return nil, errors.New("sync enqueue failed")
	}
	return rowToDocFull(row), nil
}

type SQLFeishuImportRepository struct {
	queries *generated.Queries
}

func NewSQLFeishuImportRepository(queries *generated.Queries) *SQLFeishuImportRepository {
	return &SQLFeishuImportRepository{queries: queries}
}

func (r *SQLFeishuImportRepository) CreateFeishuDocument(ctx context.Context, input CreateFeishuDocumentInput) (generated.Document, error) {
	sourceURL := input.SourceURL
	return r.queries.CreateFeishuDocumentForOwner(ctx, generated.CreateFeishuDocumentForOwnerParams{
		SourceType: input.SourceType, SourceRef: input.SourceRef, SourceUrl: &sourceURL,
		OauthAccountID: input.OAuthAccountID, OwnerUserID: input.OwnerUserID,
		KbID: input.KBID, Title: input.Title,
	})
}

func (r *SQLFeishuImportRepository) FailFeishuImportEnqueue(ctx context.Context, documentID uuid.UUID, safeError string) (bool, error) {
	rows, err := r.queries.FailFeishuImportEnqueue(ctx, generated.FailFeishuImportEnqueueParams{SafeError: &safeError, ID: documentID})
	return rows == 1, err
}
