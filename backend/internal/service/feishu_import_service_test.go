package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/zenith-wang/it-wiki/backend/internal/infra/feishu"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

func TestFeishuImportValidatesURLBeforeCreatingDocument(t *testing.T) {
	repo := &fakeFeishuImportRepository{}
	queue := &fakeFeishuSyncEnqueuer{}
	svc := NewFeishuImport(feishu.NewURLResolver(), repo, queue)

	_, err := svc.Import(context.Background(), FeishuImportInput{
		UserID: uuid.NewString(), KBID: uuid.NewString(), OAuthAccountID: uuid.NewString(), URL: "https://evil.example/docx/token",
	})
	if err == nil {
		t.Fatal("Import() error = nil, want unsupported URL")
	}
	if repo.createCalls != 0 || queue.calls != 0 {
		t.Fatalf("invalid URL caused create/enqueue calls = %d/%d", repo.createCalls, queue.calls)
	}
}

func TestFeishuImportCreatesOwnedOAuthBoundDocumentAndEnqueues(t *testing.T) {
	docID := uuid.New()
	repo := &fakeFeishuImportRepository{created: generated.Document{ID: docID}}
	queue := &fakeFeishuSyncEnqueuer{}
	svc := NewFeishuImport(feishu.NewURLResolver(), repo, queue)
	userID, kbID, accountID := uuid.NewString(), uuid.NewString(), uuid.NewString()

	doc, err := svc.Import(context.Background(), FeishuImportInput{
		UserID: userID, KBID: kbID, OAuthAccountID: accountID,
		URL: "https://acme.feishu.cn/docx/DocToken_123",
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if doc.ID != docID.String() {
		t.Fatalf("document ID = %q, want %q", doc.ID, docID)
	}
	if repo.createdInput.OwnerUserID.String() != userID || repo.createdInput.KBID.String() != kbID || repo.createdInput.OAuthAccountID.String() != accountID {
		t.Fatalf("ownership binding = %+v", repo.createdInput)
	}
	if repo.createdInput.SourceType != "feishu-docx" || repo.createdInput.SourceRef != "feishu://feishu.cn/docx/DocToken_123" {
		t.Fatalf("remote identity = %q/%q", repo.createdInput.SourceType, repo.createdInput.SourceRef)
	}
	if repo.createdInput.SourceURL != "https://acme.feishu.cn/docx/DocToken_123" {
		t.Fatalf("source URL = %q", repo.createdInput.SourceURL)
	}
	if queue.calls != 1 || queue.documentID != docID.String() || queue.requestedRevision != InitialFeishuRevision {
		t.Fatalf("enqueue = calls:%d doc:%q revision:%q", queue.calls, queue.documentID, queue.requestedRevision)
	}
}

func TestFeishuImportMapsUniqueRaceToAlreadyImported(t *testing.T) {
	repo := &fakeFeishuImportRepository{createErr: &pgconn.PgError{Code: "23505", ConstraintName: "uniq_docs_kb_source"}}
	queue := &fakeFeishuSyncEnqueuer{}
	svc := NewFeishuImport(feishu.NewURLResolver(), repo, queue)

	_, err := svc.Import(context.Background(), FeishuImportInput{
		UserID: uuid.NewString(), KBID: uuid.NewString(), OAuthAccountID: uuid.NewString(),
		URL: "https://acme.feishu.cn/docx/DocToken_123",
	})
	var duplicate *ErrFeishuAlreadyImported
	if !errors.As(err, &duplicate) {
		t.Fatalf("Import() error = %v, want ErrFeishuAlreadyImported", err)
	}
	if queue.calls != 0 {
		t.Fatalf("duplicate import enqueue calls = %d, want 0", queue.calls)
	}
}

func TestFeishuImportRejectsMissingOwnedKBOrOAuthAccount(t *testing.T) {
	repo := &fakeFeishuImportRepository{createErr: pgx.ErrNoRows}
	svc := NewFeishuImport(feishu.NewURLResolver(), repo, &fakeFeishuSyncEnqueuer{})

	_, err := svc.Import(context.Background(), FeishuImportInput{
		UserID: uuid.NewString(), KBID: uuid.NewString(), OAuthAccountID: uuid.NewString(),
		URL: "https://acme.feishu.cn/docx/DocToken_123",
	})
	var ownership *ErrFeishuImportOwnership
	if !errors.As(err, &ownership) {
		t.Fatalf("Import() error = %v, want ErrFeishuImportOwnership", err)
	}
}

func TestFeishuImportEnqueueFailureMarksFirstImportFailedWithoutLeakingCause(t *testing.T) {
	docID := uuid.New()
	repo := &fakeFeishuImportRepository{created: generated.Document{ID: docID}}
	queue := &fakeFeishuSyncEnqueuer{err: errors.New("queue SECRET body")}
	svc := NewFeishuImport(feishu.NewURLResolver(), repo, queue)

	_, err := svc.Import(context.Background(), FeishuImportInput{
		UserID: uuid.NewString(), KBID: uuid.NewString(), OAuthAccountID: uuid.NewString(),
		URL: "https://acme.feishu.cn/docx/DocToken_123",
	})
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("Import() error leaked queue cause: %v", err)
	}
	if repo.failedDocumentID != docID || repo.safeError != "sync enqueue failed" {
		t.Fatalf("failed import state = %s/%q", repo.failedDocumentID, repo.safeError)
	}
}

type fakeFeishuImportRepository struct {
	created          generated.Document
	createErr        error
	createCalls      int
	createdInput     CreateFeishuDocumentInput
	failedDocumentID uuid.UUID
	safeError        string
}

func (f *fakeFeishuImportRepository) CreateFeishuDocument(_ context.Context, input CreateFeishuDocumentInput) (generated.Document, error) {
	f.createCalls++
	f.createdInput = input
	return f.created, f.createErr
}

func (f *fakeFeishuImportRepository) FailFeishuImportEnqueue(_ context.Context, documentID uuid.UUID, safeError string) (bool, error) {
	f.failedDocumentID, f.safeError = documentID, safeError
	return true, nil
}

type fakeFeishuSyncEnqueuer struct {
	calls             int
	documentID        string
	requestedRevision string
	err               error
}

func (f *fakeFeishuSyncEnqueuer) EnqueueFeishuSync(_ context.Context, documentID, requestedRevision string) error {
	f.calls++
	f.documentID = documentID
	f.requestedRevision = requestedRevision
	return f.err
}
