package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type ErrKBNotFound struct{ ID string }

func (e *ErrKBNotFound) Error() string { return "knowledge base not found: " + e.ID }

type KB struct {
	queries    KBQueries
	embedModel string
	embedDim   int
}

type KBQueries interface {
	CreateKnowledgeBaseForOwner(context.Context, generated.CreateKnowledgeBaseForOwnerParams) (generated.KnowledgeBase, error)
	GetKnowledgeBaseForOwner(context.Context, generated.GetKnowledgeBaseForOwnerParams) (generated.KnowledgeBase, error)
	ListKnowledgeBasesForOwner(context.Context, generated.ListKnowledgeBasesForOwnerParams) ([]generated.KnowledgeBase, error)
	CountKnowledgeBasesForOwner(context.Context, pgtype.UUID) (int64, error)
	DeleteKnowledgeBaseForOwner(context.Context, generated.DeleteKnowledgeBaseForOwnerParams) error
}

func NewKB(q KBQueries, embedModel string, embedDim int) *KB {
	return &KB{queries: q, embedModel: embedModel, embedDim: embedDim}
}

type CreateKBInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *KB) Create(ctx context.Context, userID string, in CreateKBInput) (*domain.KnowledgeBase, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	ownerID, err := ownerUUID(userID)
	if err != nil {
		return nil, err
	}
	row, err := s.queries.CreateKnowledgeBaseForOwner(ctx, generated.CreateKnowledgeBaseForOwnerParams{
		Name:        in.Name,
		Description: in.Description,
		EmbedModel:  s.embedModel,
		EmbedDim:    int32(s.embedDim),
		Settings:    []byte("{}"),
		OwnerUserID: ownerID,
	})
	if err != nil {
		return nil, fmt.Errorf("create kb: %w", err)
	}
	return rowToKB(row), nil
}

func (s *KB) Get(ctx context.Context, userID, id string) (*domain.KnowledgeBase, error) {
	ownerID, err := ownerUUID(userID)
	if err != nil {
		return nil, err
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return nil, &ErrKBNotFound{ID: id}
	}
	row, err := s.queries.GetKnowledgeBaseForOwner(ctx, generated.GetKnowledgeBaseForOwnerParams{ID: u, OwnerUserID: ownerID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ErrKBNotFound{ID: id}
		}
		return nil, fmt.Errorf("get kb: %w", err)
	}
	return rowToKB(row), nil
}

func (s *KB) List(ctx context.Context, userID string, limit, offset int) ([]*domain.KnowledgeBase, int, error) {
	ownerID, err := ownerUUID(userID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.queries.ListKnowledgeBasesForOwner(ctx, generated.ListKnowledgeBasesForOwnerParams{
		OwnerUserID: ownerID,
		Limit:       int32(limit),
		Offset:      int32(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list kbs: %w", err)
	}
	total, err := s.queries.CountKnowledgeBasesForOwner(ctx, ownerID)
	if err != nil {
		return nil, 0, fmt.Errorf("count kbs: %w", err)
	}
	out := make([]*domain.KnowledgeBase, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToKB(r))
	}
	return out, int(total), nil
}

func (s *KB) Delete(ctx context.Context, userID, id string) error {
	ownerID, err := ownerUUID(userID)
	if err != nil {
		return err
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return &ErrKBNotFound{ID: id}
	}
	if _, err := s.queries.GetKnowledgeBaseForOwner(ctx, generated.GetKnowledgeBaseForOwnerParams{ID: u, OwnerUserID: ownerID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &ErrKBNotFound{ID: id}
		}
		return err
	}
	return s.queries.DeleteKnowledgeBaseForOwner(ctx, generated.DeleteKnowledgeBaseForOwnerParams{ID: u, OwnerUserID: ownerID})
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
