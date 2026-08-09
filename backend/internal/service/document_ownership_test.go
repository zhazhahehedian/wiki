package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type recordingDocumentQueries struct {
	get      generated.GetDocumentForOwnerParams
	list     generated.ListDocumentsByKBForOwnerParams
	count    generated.CountDocumentsByKBForOwnerParams
	delete   generated.DeleteDocumentForOwnerParams
	row      generated.Document
	getKBErr error
}

func (q *recordingDocumentQueries) GetKnowledgeBaseForOwner(context.Context, generated.GetKnowledgeBaseForOwnerParams) (generated.KnowledgeBase, error) {
	return generated.KnowledgeBase{ID: q.row.KbID}, q.getKBErr
}

func (q *recordingDocumentQueries) GetDocumentForOwner(_ context.Context, arg generated.GetDocumentForOwnerParams) (generated.Document, error) {
	q.get = arg
	return q.row, nil
}

func TestDocumentListReturnsNotFoundForForeignKnowledgeBase(t *testing.T) {
	kbID := uuid.New()
	q := &recordingDocumentQueries{row: generated.Document{KbID: kbID}, getKBErr: pgx.ErrNoRows}
	_, _, err := NewDocument(q, nil).ListByKB(context.Background(), uuid.NewString(), kbID.String(), nil, 20, 0)
	var notFound *ErrKBNotFound
	if !errors.As(err, &notFound) || q.list.OwnerUserID.Valid {
		t.Fatalf("ListByKB() error=%v listCalled=%v", err, q.list.OwnerUserID.Valid)
	}
}
func (q *recordingDocumentQueries) ListDocumentsByKBForOwner(_ context.Context, arg generated.ListDocumentsByKBForOwnerParams) ([]generated.Document, error) {
	q.list = arg
	return []generated.Document{q.row}, nil
}
func (q *recordingDocumentQueries) CountDocumentsByKBForOwner(_ context.Context, arg generated.CountDocumentsByKBForOwnerParams) (int64, error) {
	q.count = arg
	return 1, nil
}
func (q *recordingDocumentQueries) DeleteDocumentForOwner(_ context.Context, arg generated.DeleteDocumentForOwnerParams) (generated.DeleteDocumentForOwnerRow, error) {
	q.delete = arg
	return generated.DeleteDocumentForOwnerRow{ContentRef: q.row.ContentRef, PendingContentRef: q.row.PendingContentRef}, nil
}

func TestDocumentOwnerIDReachesReadListCountAndDeleteQueries(t *testing.T) {
	ownerID, kbID, docID := uuid.New(), uuid.New(), uuid.New()
	q := &recordingDocumentQueries{row: generated.Document{ID: docID, KbID: kbID, Status: string(domain.StatusReady), Metadata: []byte("{}")}}
	svc := NewDocument(q, nil)
	if _, err := svc.Get(context.Background(), ownerID.String(), docID.String()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ListByKB(context.Background(), ownerID.String(), kbID.String(), nil, 20, 0); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(context.Background(), ownerID.String(), docID.String()); err != nil {
		t.Fatal(err)
	}
	if q.get.OwnerUserID.Bytes != ownerID || q.list.OwnerUserID.Bytes != ownerID || q.count.OwnerUserID.Bytes != ownerID || q.delete.OwnerUserID.Bytes != ownerID {
		t.Fatalf("owner propagation mismatch: get=%v list=%v count=%v delete=%v", q.get, q.list, q.count, q.delete)
	}
}

type recordingIngestionQueries struct {
	find     generated.FindDocumentByChecksumForOwnerParams
	create   generated.CreateDocumentForOwnerParams
	get      generated.GetDocumentForOwnerParams
	update   generated.UpdateDocumentStatusForOwnerParams
	updates  []generated.UpdateDocumentStatusForOwnerParams
	list     generated.ListDocumentsByKBForOwnerParams
	row      generated.Document
	rows     []generated.Document
	getKBErr error
}

func (q *recordingIngestionQueries) FindDocumentByChecksumForOwner(_ context.Context, arg generated.FindDocumentByChecksumForOwnerParams) (generated.Document, error) {
	q.find = arg
	return generated.Document{}, pgx.ErrNoRows
}
func (q *recordingIngestionQueries) CreateDocumentForOwner(_ context.Context, arg generated.CreateDocumentForOwnerParams) (generated.Document, error) {
	q.create = arg
	return q.row, nil
}
func (q *recordingIngestionQueries) GetDocumentForOwner(_ context.Context, arg generated.GetDocumentForOwnerParams) (generated.Document, error) {
	q.get = arg
	for _, row := range q.rows {
		if row.ID == arg.ID {
			return row, nil
		}
	}
	return q.row, nil
}
func (q *recordingIngestionQueries) UpdateDocumentStatusForOwner(_ context.Context, arg generated.UpdateDocumentStatusForOwnerParams) error {
	q.update = arg
	q.updates = append(q.updates, arg)
	return nil
}
func (q *recordingIngestionQueries) ListDocumentsByKBForOwner(_ context.Context, arg generated.ListDocumentsByKBForOwnerParams) ([]generated.Document, error) {
	q.list = arg
	if q.rows != nil {
		return q.rows, nil
	}
	return []generated.Document{q.row}, nil
}
func (q *recordingIngestionQueries) GetKnowledgeBaseForOwner(context.Context, generated.GetKnowledgeBaseForOwnerParams) (generated.KnowledgeBase, error) {
	return generated.KnowledgeBase{ID: q.row.KbID}, q.getKBErr
}

type memoryObjectStorage struct{}

func (memoryObjectStorage) Put(context.Context, string, io.Reader, int64, string) error { return nil }
func (memoryObjectStorage) Get(context.Context, string) (io.ReadCloser, error)          { return nil, nil }
func (memoryObjectStorage) Delete(context.Context, string) error                        { return nil }

type recordingEnqueuer struct {
	documentID  string
	documentIDs []string
}

func (e *recordingEnqueuer) EnqueueIngestion(_ context.Context, documentID string) error {
	e.documentID = documentID
	e.documentIDs = append(e.documentIDs, documentID)
	return nil
}

func TestUploadAndReingestOwnerIDReachQueries(t *testing.T) {
	ownerID, kbID, docID := uuid.New(), uuid.New(), uuid.New()
	q := &recordingIngestionQueries{row: generated.Document{ID: docID, KbID: kbID, SourceType: "local-upload", Status: string(domain.StatusReady), Metadata: []byte("{}")}}
	svc := NewIngestion(q, memoryObjectStorage{}, &recordingEnqueuer{})
	if _, err := svc.Upload(context.Background(), ownerID.String(), UploadInput{KBID: kbID.String(), Title: "a.txt", MimeType: "text/plain", Body: strings.NewReader("owned"), Size: 5}); err != nil {
		t.Fatal(err)
	}
	if q.find.OwnerUserID.Bytes != ownerID || q.create.OwnerUserID.Bytes != ownerID {
		t.Fatalf("upload owner propagation mismatch: find=%v create=%v", q.find, q.create)
	}
	if _, err := svc.Reingest(context.Background(), ownerID.String(), docID.String()); err != nil {
		t.Fatal(err)
	}
	if q.get.OwnerUserID.Bytes != ownerID || q.update.OwnerUserID.Bytes != ownerID {
		t.Fatalf("reingest owner propagation mismatch: get=%v update=%v", q.get, q.update)
	}
}

func TestReingestRejectsRemoteDocumentWithoutMutation(t *testing.T) {
	ownerID, kbID, docID := uuid.New(), uuid.New(), uuid.New()
	q := &recordingIngestionQueries{row: generated.Document{
		ID: docID, KbID: kbID, SourceType: "feishu-docx", Status: string(domain.StatusReady), Metadata: []byte("{}"),
	}}
	queue := &recordingEnqueuer{}

	_, err := NewIngestion(q, memoryObjectStorage{}, queue).Reingest(context.Background(), ownerID.String(), docID.String())

	var unsupported *ErrRemoteReingestUnsupported
	if !errors.As(err, &unsupported) {
		t.Fatalf("Reingest() error=%v, want ErrRemoteReingestUnsupported", err)
	}
	if q.get.OwnerUserID.Bytes != ownerID {
		t.Fatalf("owner lookup=%v, want %s", q.get.OwnerUserID, ownerID)
	}
	if len(q.updates) != 0 || len(queue.documentIDs) != 0 {
		t.Fatalf("remote reingest mutated state: updates=%v enqueued=%v", q.updates, queue.documentIDs)
	}
}

func TestReingestKBOnlyEnqueuesReadyOrFailedLocalUploads(t *testing.T) {
	ownerID, kbID := uuid.New(), uuid.New()
	localReadyID, localFailedID := uuid.New(), uuid.New()
	remoteReadyID, remoteFailedID := uuid.New(), uuid.New()
	q := &recordingIngestionQueries{row: generated.Document{KbID: kbID}, rows: []generated.Document{
		{ID: remoteReadyID, KbID: kbID, SourceType: "feishu-docx", Status: string(domain.StatusReady), Metadata: []byte("{}")},
		{ID: localReadyID, KbID: kbID, SourceType: "local-upload", Status: string(domain.StatusReady), Metadata: []byte("{}")},
		{ID: remoteFailedID, KbID: kbID, SourceType: "feishu-sheet", Status: string(domain.StatusFailed), Metadata: []byte("{}")},
		{ID: localFailedID, KbID: kbID, SourceType: "local-upload", Status: string(domain.StatusFailed), Metadata: []byte("{}")},
	}}
	queue := &recordingEnqueuer{}

	got, err := NewIngestion(q, memoryObjectStorage{}, queue).ReingestKB(context.Background(), ownerID.String(), kbID.String())

	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Fatalf("ReingestKB()=%d, want 2", got)
	}
	wantIDs := map[string]bool{localReadyID.String(): true, localFailedID.String(): true}
	if len(queue.documentIDs) != len(wantIDs) {
		t.Fatalf("enqueued=%v, want only local uploads", queue.documentIDs)
	}
	for _, id := range queue.documentIDs {
		if !wantIDs[id] {
			t.Fatalf("remote document enqueued: %s", id)
		}
	}
	for _, update := range q.updates {
		if update.ID == remoteReadyID || update.ID == remoteFailedID {
			t.Fatalf("remote document status changed: %+v", update)
		}
	}
	if q.list.OwnerUserID.Bytes != ownerID {
		t.Fatalf("bulk owner lookup=%v, want %s", q.list.OwnerUserID, ownerID)
	}
}

func TestUploadRejectsForeignKnowledgeBaseBeforeChecksumOrStorage(t *testing.T) {
	kbID := uuid.New()
	q := &recordingIngestionQueries{row: generated.Document{KbID: kbID}, getKBErr: pgx.ErrNoRows}
	_, err := NewIngestion(q, memoryObjectStorage{}, &recordingEnqueuer{}).Upload(context.Background(), uuid.NewString(), UploadInput{KBID: kbID.String(), Title: "a.txt", Body: strings.NewReader("owned"), Size: 5})
	var notFound *ErrKBNotFound
	if !errors.As(err, &notFound) || q.find.OwnerUserID.Valid {
		t.Fatalf("Upload() error=%v checksumQueryCalled=%v", err, q.find.OwnerUserID.Valid)
	}
}
