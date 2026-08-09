package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type fakeOwnershipBootstrapRepo struct {
	orphans       int64
	assigned      bool
	upsertOwnerID uuid.UUID
	upsertOpenID  string
	err           error
}

func (r *fakeOwnershipBootstrapRepo) CountOrphanOwnership(context.Context) (int64, error) {
	return r.orphans, r.err
}
func (r *fakeOwnershipBootstrapRepo) BootstrapOwner(_ context.Context, ownerID uuid.UUID, openID string) error {
	if r.err != nil {
		return r.err
	}
	r.upsertOwnerID = ownerID
	r.upsertOpenID = openID
	r.assigned = true
	return nil
}

func TestOwnershipBootstrapRequiresConfigForLegacyRows(t *testing.T) {
	repo := &fakeOwnershipBootstrapRepo{orphans: 2}
	err := NewOwnershipBootstrap(repo).Run(context.Background(), "")
	if !errors.Is(err, ErrBootstrapOwnerRequired) || repo.assigned {
		t.Fatalf("Run() error = %v, assigned=%v", err, repo.assigned)
	}
}

func TestOwnershipBootstrapAssignsStableOwnerAndIsIdempotent(t *testing.T) {
	openID := "ou_bootstrap"
	repo := &fakeOwnershipBootstrapRepo{orphans: 2}
	if err := NewOwnershipBootstrap(repo).Run(context.Background(), openID); err != nil {
		t.Fatal(err)
	}
	wantID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("feishu:"+openID))
	if !repo.assigned || repo.upsertOwnerID != wantID || repo.upsertOpenID != openID {
		t.Fatalf("bootstrap assignment = %#v, want owner %s", repo, wantID)
	}

	repo.orphans, repo.assigned = 0, false
	if err := NewOwnershipBootstrap(repo).Run(context.Background(), ""); err != nil || repo.assigned {
		t.Fatalf("idempotent Run() error=%v assigned=%v", err, repo.assigned)
	}
}

func TestOwnershipBootstrapPropagatesRepositoryFailure(t *testing.T) {
	want := errors.New("transaction failed")
	err := NewOwnershipBootstrap(&fakeOwnershipBootstrapRepo{orphans: 1, err: want}).Run(context.Background(), "ou_bootstrap")
	if !errors.Is(err, want) {
		t.Fatalf("Run() error=%v, want %v", err, want)
	}
}
