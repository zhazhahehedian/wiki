package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type ErrKBNotFound struct{ ID string }

func (e *ErrKBNotFound) Error() string { return "knowledge base not found: " + e.ID }

type KB struct {
	queries    *generated.Queries
	embedModel string
	embedDim   int
}

func NewKB(q *generated.Queries, embedModel string, embedDim int) *KB {
	return &KB{queries: q, embedModel: embedModel, embedDim: embedDim}
}

type CreateKBInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *KB) Create(ctx context.Context, in CreateKBInput) (*domain.KnowledgeBase, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	row, err := s.queries.CreateKnowledgeBase(ctx, generated.CreateKnowledgeBaseParams{
		Name:        in.Name,
		Description: in.Description,
		EmbedModel:  s.embedModel,
		EmbedDim:    int32(s.embedDim),
		Settings:    []byte("{}"),
	})
	if err != nil {
		return nil, fmt.Errorf("create kb: %w", err)
	}
	return rowToKB(row), nil
}

func (s *KB) Get(ctx context.Context, id string) (*domain.KnowledgeBase, error) {
	u, err := uuid.Parse(id)
	if err != nil {
		return nil, &ErrKBNotFound{ID: id}
	}
	row, err := s.queries.GetKnowledgeBase(ctx, u)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ErrKBNotFound{ID: id}
		}
		return nil, fmt.Errorf("get kb: %w", err)
	}
	return rowToKB(row), nil
}

func (s *KB) List(ctx context.Context, limit, offset int) ([]*domain.KnowledgeBase, int, error) {
	rows, err := s.queries.ListKnowledgeBases(ctx, generated.ListKnowledgeBasesParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list kbs: %w", err)
	}
	total, err := s.queries.CountKnowledgeBases(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count kbs: %w", err)
	}
	out := make([]*domain.KnowledgeBase, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToKB(r))
	}
	return out, int(total), nil
}

func (s *KB) Delete(ctx context.Context, id string) error {
	u, err := uuid.Parse(id)
	if err != nil {
		return &ErrKBNotFound{ID: id}
	}
	if _, err := s.queries.GetKnowledgeBase(ctx, u); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &ErrKBNotFound{ID: id}
		}
		return err
	}
	return s.queries.DeleteKnowledgeBase(ctx, u)
}

func rowToKB(r generated.KnowledgeBase) *domain.KnowledgeBase {
	kb := &domain.KnowledgeBase{
		ID:          r.ID.String(),
		Name:        r.Name,
		Description: r.Description,
		OwnerID:     r.OwnerID,
		EmbedModel:  r.EmbedModel,
		EmbedDim:    int(r.EmbedDim),
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
	kb.Settings = map[string]any{}
	if len(r.Settings) > 0 {
		_ = json.Unmarshal(r.Settings, &kb.Settings)
	}
	return kb
}
