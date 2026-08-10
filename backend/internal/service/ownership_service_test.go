package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type recordingKBQueries struct {
	create generated.CreateKnowledgeBaseForOwnerParams
	get    generated.GetKnowledgeBaseForOwnerParams
	list   generated.ListKnowledgeBasesForOwnerParams
	count  pgtype.UUID
	delete generated.DeleteKnowledgeBaseForOwnerParams
	row    generated.KnowledgeBase
}

func (q *recordingKBQueries) CreateKnowledgeBaseForOwner(_ context.Context, arg generated.CreateKnowledgeBaseForOwnerParams) (generated.KnowledgeBase, error) {
	q.create = arg
	return q.row, nil
}
func (q *recordingKBQueries) GetKnowledgeBaseForOwner(_ context.Context, arg generated.GetKnowledgeBaseForOwnerParams) (generated.KnowledgeBase, error) {
	q.get = arg
	return q.row, nil
}
func (q *recordingKBQueries) ListKnowledgeBasesForOwner(_ context.Context, arg generated.ListKnowledgeBasesForOwnerParams) ([]generated.KnowledgeBase, error) {
	q.list = arg
	return []generated.KnowledgeBase{q.row}, nil
}
func (q *recordingKBQueries) CountKnowledgeBasesForOwner(_ context.Context, owner pgtype.UUID) (int64, error) {
	q.count = owner
	return 1, nil
}
func (q *recordingKBQueries) DeleteKnowledgeBaseForOwner(_ context.Context, arg generated.DeleteKnowledgeBaseForOwnerParams) error {
	q.delete = arg
	return nil
}

func TestKBOwnerIDReachesEveryQuery(t *testing.T) {
	ownerID, kbID := uuid.New(), uuid.New()
	q := &recordingKBQueries{row: generated.KnowledgeBase{ID: kbID, Name: "owned", Settings: []byte("{}")}}
	svc := NewKB(q, "embed", 1536)

	if _, err := svc.Create(context.Background(), ownerID.String(), CreateKBInput{Name: "owned"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(context.Background(), ownerID.String(), kbID.String()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.List(context.Background(), ownerID.String(), 20, 0); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(context.Background(), ownerID.String(), kbID.String()); err != nil {
		t.Fatal(err)
	}

	want := pgtype.UUID{Bytes: ownerID, Valid: true}
	if q.create.OwnerUserID != want || q.get.OwnerUserID != want || q.list.OwnerUserID != want || q.count != want || q.delete.OwnerUserID != want {
		t.Fatalf("owner propagation mismatch: create=%v get=%v list=%v count=%v delete=%v", q.create.OwnerUserID, q.get.OwnerUserID, q.list.OwnerUserID, q.count, q.delete.OwnerUserID)
	}
}

func TestKBRejectsInvalidOwnerBeforeQuery(t *testing.T) {
	q := &recordingKBQueries{}
	if _, err := NewKB(q, "embed", 1536).Get(context.Background(), "not-a-uuid", uuid.NewString()); err == nil {
		t.Fatal("Get() error = nil, want invalid owner rejection")
	}
	if q.get.OwnerUserID.Valid {
		t.Fatal("owner-scoped query called with invalid owner")
	}
}
