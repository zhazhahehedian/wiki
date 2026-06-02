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
	queries *generated.Queries
}

func NewDocument(q *generated.Queries) *Document {
	return &Document{queries: q}
}

func (s *Document) Get(ctx context.Context, id string) (*domain.Document, error) {
	u, err := uuid.Parse(id)
	if err != nil {
		return nil, &ErrDocNotFound{ID: id}
	}
	row, err := s.queries.GetDocument(ctx, u)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ErrDocNotFound{ID: id}
		}
		return nil, fmt.Errorf("get document: %w", err)
	}
	return rowToDocFull(row), nil
}

func (s *Document) ListByKB(ctx context.Context, kbID string, statusFilter *string, limit, offset int) ([]*domain.Document, int, error) {
	u, err := uuid.Parse(kbID)
	if err != nil {
		return nil, 0, &ErrKBNotFound{ID: kbID}
	}

	var statusParam *string
	if statusFilter != nil {
		s := *statusFilter
		statusParam = &s
	}

	rows, err := s.queries.ListDocumentsByKB(ctx, generated.ListDocumentsByKBParams{
		KbID:   u,
		Status: statusParam,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list docs: %w", err)
	}
	total, err := s.queries.CountDocumentsByKB(ctx, generated.CountDocumentsByKBParams{
		KbID:   u,
		Status: statusParam,
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

func (s *Document) Delete(ctx context.Context, id string) error {
	u, err := uuid.Parse(id)
	if err != nil {
		return &ErrDocNotFound{ID: id}
	}
	if _, err := s.queries.GetDocument(ctx, u); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &ErrDocNotFound{ID: id}
		}
		return err
	}
	return s.queries.DeleteDocument(ctx, u)
}
