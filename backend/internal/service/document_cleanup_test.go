package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type cleanupDocumentQueries struct {
	doc       generated.Document
	getErr    error
	deleteErr error
	deleteRow generated.DeleteDocumentForOwnerRow
	events    *[]string
	get       generated.GetDocumentForOwnerParams
	delete    generated.DeleteDocumentForOwnerParams
}

func (q *cleanupDocumentQueries) GetKnowledgeBaseForOwner(context.Context, generated.GetKnowledgeBaseForOwnerParams) (generated.KnowledgeBase, error) {
	return generated.KnowledgeBase{}, nil
}

func (q *cleanupDocumentQueries) GetDocumentForOwner(_ context.Context, arg generated.GetDocumentForOwnerParams) (generated.Document, error) {
	q.get = arg
	q.record("get")
	return q.doc, q.getErr
}

func (q *cleanupDocumentQueries) ListDocumentsByKBForOwner(context.Context, generated.ListDocumentsByKBForOwnerParams) ([]generated.Document, error) {
	return nil, nil
}

func (q *cleanupDocumentQueries) CountDocumentsByKBForOwner(context.Context, generated.CountDocumentsByKBForOwnerParams) (int64, error) {
	return 0, nil
}

func (q *cleanupDocumentQueries) DeleteDocumentForOwner(_ context.Context, arg generated.DeleteDocumentForOwnerParams) (generated.DeleteDocumentForOwnerRow, error) {
	q.delete = arg
	q.record("repository-delete")
	return q.deleteRow, q.deleteErr
}

func (q *cleanupDocumentQueries) record(event string) {
	if q.events != nil {
		*q.events = append(*q.events, event)
	}
}

type cleanupDocumentStorage struct {
	events           *[]string
	deleted          []string
	deleteErr        error
	deleteContextErr error
}

func (*cleanupDocumentStorage) Put(context.Context, string, io.Reader, int64, string) error {
	return nil
}

func (*cleanupDocumentStorage) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, nil
}

func (s *cleanupDocumentStorage) Delete(ctx context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	s.deleteContextErr = ctx.Err()
	if s.events != nil {
		*s.events = append(*s.events, "storage-delete:"+key)
	}
	return s.deleteErr
}

func TestDocumentDeleteCleansRefsReturnedByOwnerScopedRepositoryDelete(t *testing.T) {
	ownerID, docID := uuid.New(), uuid.New()
	staleRef, activeRef, pendingRef := "stale.md", "active.md", "pending.md"
	events := []string{}
	repo := &cleanupDocumentQueries{doc: generated.Document{
		ID: docID, ContentRef: &staleRef,
	}, deleteRow: generated.DeleteDocumentForOwnerRow{
		ContentRef: &activeRef, PendingContentRef: &pendingRef,
	}, events: &events}
	storage := &cleanupDocumentStorage{events: &events}

	err := NewDocument(repo, storage).Delete(context.Background(), ownerID.String(), docID.String())

	if err != nil {
		t.Fatal(err)
	}
	wantEvents := []string{"repository-delete", "storage-delete:active.md", "storage-delete:pending.md"}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events=%v, want %v", events, wantEvents)
	}
	if repo.delete.OwnerUserID.Bytes != ownerID {
		t.Fatalf("owner propagation mismatch: delete=%v", repo.delete)
	}
}

func TestDocumentDeleteCleansSharedObjectOnlyOnceAndSkipsEmptyRefs(t *testing.T) {
	ownerID := uuid.NewString()
	sharedRef, emptyRef := "shared.md", ""
	tests := []struct {
		name    string
		doc     generated.Document
		wantRef []string
	}{
		{name: "shared", doc: generated.Document{ID: uuid.New(), ContentRef: &sharedRef, PendingContentRef: &sharedRef}, wantRef: []string{"shared.md"}},
		{name: "empty", doc: generated.Document{ID: uuid.New(), ContentRef: &emptyRef, PendingContentRef: &emptyRef}},
		{name: "nil", doc: generated.Document{ID: uuid.New()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &cleanupDocumentQueries{doc: tt.doc, deleteRow: generated.DeleteDocumentForOwnerRow{
				ContentRef: tt.doc.ContentRef, PendingContentRef: tt.doc.PendingContentRef,
			}}
			storage := &cleanupDocumentStorage{}
			if err := NewDocument(repo, storage).Delete(context.Background(), ownerID, tt.doc.ID.String()); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(storage.deleted, tt.wantRef) {
				t.Fatalf("deleted=%v, want %v", storage.deleted, tt.wantRef)
			}
		})
	}
}

func TestDocumentDeleteDoesNotCleanStorageWhenOwnerScopedRepositoryDeleteFails(t *testing.T) {
	docID := uuid.New()
	activeRef := "active.md"
	tests := []struct {
		name      string
		deleteErr error
		wantNF    bool
	}{
		{name: "owner delete", deleteErr: pgx.ErrNoRows, wantNF: true},
		{name: "repository delete", deleteErr: errors.New("database failed")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &cleanupDocumentQueries{doc: generated.Document{ID: docID, ContentRef: &activeRef}, deleteErr: tt.deleteErr}
			storage := &cleanupDocumentStorage{}
			err := NewDocument(repo, storage).Delete(context.Background(), uuid.NewString(), docID.String())
			var notFound *ErrDocNotFound
			if tt.wantNF != errors.As(err, &notFound) {
				t.Fatalf("Delete() error=%v, wantNotFound=%v", err, tt.wantNF)
			}
			if err == nil {
				t.Fatal("Delete() error=nil, want failure")
			}
			if len(storage.deleted) != 0 {
				t.Fatalf("failed delete cleaned storage: %v", storage.deleted)
			}
		})
	}
}

func TestDocumentDeleteCleanupFailureIsBestEffortDetachedAndRedacted(t *testing.T) {
	docID := uuid.MustParse("d046f3f4-f690-4aa3-86a5-10dfd48c59cd")
	activeRef := "tenant/token/https://secret.example/body.md"
	repo := &cleanupDocumentQueries{
		doc:       generated.Document{ID: docID, ContentRef: &activeRef},
		deleteRow: generated.DeleteDocumentForOwnerRow{ContentRef: &activeRef},
	}
	storage := &cleanupDocumentStorage{deleteErr: errors.New("provider SECRET https://secret.example response body")}
	var output bytes.Buffer
	svc := NewDocument(repo, storage)
	svc.logger = slog.New(slog.NewJSONHandler(&output, nil))
	svc.cleanupTimeout = time.Second
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.Delete(ctx, uuid.NewString(), docID.String())

	if err != nil {
		t.Fatalf("cleanup changed successful delete result: %v", err)
	}
	if len(storage.deleted) != 1 || storage.deleteContextErr != nil {
		t.Fatalf("cleanup=%v contextErr=%v", storage.deleted, storage.deleteContextErr)
	}
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode log=%v output=%q", err, output.String())
	}
	if record["event"] != "document_snapshot_cleanup_failed" || record["error_code"] != "snapshot_delete_failed" ||
		record["document_id"] != docID.String() || record["storage_key_hash"] != "9436fb8335fdc735" {
		t.Fatalf("cleanup warning=%+v", record)
	}
	for _, secret := range []string{activeRef, "SECRET", "secret.example", "provider", "response body"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("cleanup warning leaked %q: %s", secret, output.String())
		}
	}
}
