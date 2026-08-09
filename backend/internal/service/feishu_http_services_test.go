package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type fakeFeishuAccountQueries struct{ row generated.OauthAccount }

func (q fakeFeishuAccountQueries) GetOAuthAccountForUser(context.Context, uuid.UUID) (generated.OauthAccount, error) {
	return q.row, nil
}

func TestFeishuAccountResolverRejectsReauthRequired(t *testing.T) {
	userID := uuid.New()
	_, err := NewFeishuAccounts(fakeFeishuAccountQueries{row: generated.OauthAccount{UserID: userID, ReauthRequired: true}}).Resolve(context.Background(), userID.String())
	var reauth *ErrFeishuReauthRequired
	if !errors.As(err, &reauth) {
		t.Fatalf("Resolve() error=%v, want ErrFeishuReauthRequired", err)
	}
}

type fakeFeishuSyncRequestQueries struct {
	arg generated.GetFeishuDocumentForOwnerAndAccountParams
	row generated.Document
}

func (q *fakeFeishuSyncRequestQueries) GetDocumentForOwner(_ context.Context, arg generated.GetDocumentForOwnerParams) (generated.Document, error) {
	return q.row, nil
}

func (q *fakeFeishuSyncRequestQueries) GetFeishuDocumentForOwnerAndAccount(_ context.Context, arg generated.GetFeishuDocumentForOwnerAndAccountParams) (generated.Document, error) {
	q.arg = arg
	return q.row, nil
}

type fakeFeishuSyncQueue struct{ documentID, revision string }

func (q *fakeFeishuSyncQueue) EnqueueFeishuSync(_ context.Context, documentID, revision string) error {
	q.documentID, q.revision = documentID, revision
	return nil
}

func TestFeishuSyncRequestScopesDocumentToOwnerAndServerResolvedAccount(t *testing.T) {
	ownerID, accountID, docID := uuid.New(), uuid.New(), uuid.New()
	revision := "rev-7"
	repo := &fakeFeishuSyncRequestQueries{row: generated.Document{ID: docID, SourceType: "feishu-docx", RemoteRevision: &revision, SyncStatus: "idle", Metadata: []byte("{}")}}
	queue := &fakeFeishuSyncQueue{}
	doc, err := NewFeishuSync(repo, queue).Sync(context.Background(), ownerID.String(), accountID.String(), docID.String())
	if err != nil {
		t.Fatal(err)
	}
	if doc.ID != docID.String() || repo.arg.OwnerUserID.Bytes != ownerID || repo.arg.OauthAccountID != accountID || queue.documentID != docID.String() || queue.revision != revision {
		t.Fatalf("sync propagation mismatch: doc=%#v arg=%#v queue=%#v", doc, repo.arg, queue)
	}
}

func TestFeishuSyncRequestRejectsInProgressWithoutEnqueue(t *testing.T) {
	docID := uuid.New()
	repo := &fakeFeishuSyncRequestQueries{row: generated.Document{ID: docID, SourceType: "feishu-docx", SyncStatus: "syncing"}}
	queue := &fakeFeishuSyncQueue{}
	_, err := NewFeishuSync(repo, queue).Sync(context.Background(), uuid.NewString(), uuid.NewString(), docID.String())
	var busy *ErrFeishuSyncInProgress
	if !errors.As(err, &busy) || queue.documentID != "" {
		t.Fatalf("Sync() error=%v queue=%#v", err, queue)
	}
}
