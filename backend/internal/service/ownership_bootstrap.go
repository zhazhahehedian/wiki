package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

var ErrBootstrapOwnerRequired = errors.New("BOOTSTRAP_OWNER_FEISHU_OPEN_ID is required while legacy rows lack ownership")

type OwnershipBootstrapRepository interface {
	CountOrphanOwnership(context.Context) (int64, error)
	BootstrapOwner(context.Context, uuid.UUID, string) error
}

type OwnershipBootstrap struct{ repo OwnershipBootstrapRepository }

func NewOwnershipBootstrap(repo OwnershipBootstrapRepository) *OwnershipBootstrap {
	return &OwnershipBootstrap{repo: repo}
}

func (s *OwnershipBootstrap) Run(ctx context.Context, configuredOpenID string) error {
	orphans, err := s.repo.CountOrphanOwnership(ctx)
	if err != nil {
		return fmt.Errorf("count orphan ownership: %w", err)
	}
	if orphans == 0 {
		return nil
	}
	openID := strings.TrimSpace(configuredOpenID)
	if openID == "" {
		return ErrBootstrapOwnerRequired
	}
	candidateID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("feishu:"+openID))
	if err := s.repo.BootstrapOwner(ctx, candidateID, openID); err != nil {
		return fmt.Errorf("bootstrap owner: %w", err)
	}
	return nil
}
