package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type ownerContextKey struct{}

func withOwnerID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ownerContextKey{}, userID)
}

func OwnerIDFromContext(ctx context.Context) string {
	ownerID, _ := ctx.Value(ownerContextKey{}).(string)
	return ownerID
}

var ErrInvalidOwner = errors.New("invalid current user")

func ownerUUID(userID string) (pgtype.UUID, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return pgtype.UUID{}, ErrInvalidOwner
	}
	return pgtype.UUID{Bytes: id, Valid: true}, nil
}

func requiredOwnerUUID(userID string) (uuid.UUID, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return uuid.Nil, ErrInvalidOwner
	}
	return id, nil
}
