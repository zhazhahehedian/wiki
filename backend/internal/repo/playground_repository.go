package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zenith-wang/it-wiki/backend/internal/playground"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type PlaygroundRepository struct{ queries *generated.Queries }

func NewPlaygroundRepository(db generated.DBTX) *PlaygroundRepository {
	return &PlaygroundRepository{generated.New(db)}
}
func modelConnection(row generated.UserLlmKey) playground.Connection {
	return playground.Connection{Protocol: row.Protocol, UserID: row.UserID.String(), BaseURL: row.BaseUrl, Ciphertext: row.KeyCiphertext, Models: row.Models, DefaultModel: row.DefaultModel, Version: row.Version}
}
func (r *PlaygroundRepository) Get(ctx context.Context, user string) (playground.Connection, error) {
	id, e := uuid.Parse(user)
	if e != nil {
		return playground.Connection{}, playground.ErrInvalid
	}
	row, e := r.queries.GetModelConnection(ctx, id)
	if errors.Is(e, pgx.ErrNoRows) {
		e = playground.ErrNotConfigured
	}
	return modelConnection(row), e
}
func (r *PlaygroundRepository) Save(ctx context.Context, c playground.Connection, expected int64) (playground.Connection, error) {
	id, e := uuid.Parse(c.UserID)
	if e != nil {
		return playground.Connection{}, playground.ErrInvalid
	}
	row, e := r.queries.SaveModelConnection(ctx, generated.SaveModelConnectionParams{Protocol: c.Protocol, UserID: id, BaseUrl: c.BaseURL, KeyCiphertext: c.Ciphertext, Models: c.Models, DefaultModel: c.DefaultModel, ExpectedVersion: expected})
	if errors.Is(e, pgx.ErrNoRows) {
		e = playground.ErrConflict
	}
	return modelConnection(row), e
}
func (r *PlaygroundRepository) Delete(ctx context.Context, user string) error {
	id, e := uuid.Parse(user)
	if e != nil {
		return playground.ErrInvalid
	}
	return r.queries.DeleteModelConnection(ctx, id)
}
