package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type ErrDocNotFound struct{ ID string }

func (e *ErrDocNotFound) Error() string { return "document not found: " + e.ID }

type Document struct {
	queries DocumentQueries
}

type DocumentQueries interface {
	GetKnowledgeBaseForOwner(context.Context, generated.GetKnowledgeBaseForOwnerParams) (generated.KnowledgeBase, error)
	GetDocumentForOwner(context.Context, generated.GetDocumentForOwnerParams) (generated.Document, error)
	ListDocumentsByKBForOwner(context.Context, generated.ListDocumentsByKBForOwnerParams) ([]generated.Document, error)
	CountDocumentsByKBForOwner(context.Context, generated.CountDocumentsByKBForOwnerParams) (int64, error)
	DeleteDocumentForOwner(context.Context, generated.DeleteDocumentForOwnerParams) error
}

func NewDocument(q DocumentQueries) *Document {
	return &Document{queries: q}
}

func (s *Document) Get(ctx context.Context, userID, id string) (*domain.Document, error) {
	ownerID, err := ownerUUID(userID)
	if err != nil {
		return nil, err
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return nil, &ErrDocNotFound{ID: id}
	}
	row, err := s.queries.GetDocumentForOwner(ctx, generated.GetDocumentForOwnerParams{ID: u, OwnerUserID: ownerID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ErrDocNotFound{ID: id}
		}
		return nil, fmt.Errorf("get document: %w", err)
	}
	return rowToDocFull(row), nil
}

func (s *Document) ListByKB(ctx context.Context, userID, kbID string, statusFilter *string, limit, offset int) ([]*domain.Document, int, error) {
	ownerID, err := ownerUUID(userID)
	if err != nil {
		return nil, 0, err
	}
	u, err := uuid.Parse(kbID)
	if err != nil {
		return nil, 0, &ErrKBNotFound{ID: kbID}
	}
	if _, err := s.queries.GetKnowledgeBaseForOwner(ctx, generated.GetKnowledgeBaseForOwnerParams{ID: u, OwnerUserID: ownerID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, &ErrKBNotFound{ID: kbID}
		}
		return nil, 0, fmt.Errorf("get kb: %w", err)
	}

	var statusParam *string
	if statusFilter != nil {
		s := *statusFilter
		statusParam = &s
	}

	rows, err := s.queries.ListDocumentsByKBForOwner(ctx, generated.ListDocumentsByKBForOwnerParams{
		KbID: u, OwnerUserID: ownerID, Status: statusParam,
		Limit: int32(limit), Offset: int32(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list docs: %w", err)
	}
	total, err := s.queries.CountDocumentsByKBForOwner(ctx, generated.CountDocumentsByKBForOwnerParams{
		KbID: u, OwnerUserID: ownerID, Status: statusParam,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count docs: %w", err)
	}

	out := make([]*domain.Document, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToDocFull(r))
	}
	return out, int(total), nil
}

func (s *Document) Delete(ctx context.Context, userID, id string) error {
	ownerID, err := ownerUUID(userID)
	if err != nil {
		return err
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return &ErrDocNotFound{ID: id}
	}
	if _, err := s.queries.GetDocumentForOwner(ctx, generated.GetDocumentForOwnerParams{ID: u, OwnerUserID: ownerID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &ErrDocNotFound{ID: id}
		}
		return err
	}
	return s.queries.DeleteDocumentForOwner(ctx, generated.DeleteDocumentForOwnerParams{ID: u, OwnerUserID: ownerID})
}
