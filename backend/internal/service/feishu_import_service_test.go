package service

import (
	"context"
	"errors"
	"reflect"
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

func TestSQLFeishuImportRepositoryCreatesAndEnqueuesInSameTransaction(t *testing.T) {
	doc := generated.Document{ID: uuid.New(), KbID: uuid.New(), SourceType: "feishu-docx", SourceRef: "feishu://feishu.cn/docx/token"}
	tx := &importFakeTx{row: &importFakeRow{doc: doc}}
	repo := NewSQLFeishuImportRepository(&importFakeBeginner{tx: tx})
	queue := &fakeFeishuSyncEnqueuer{}
	input := CreateFeishuDocumentInput{OwnerUserID: uuid.New(), KBID: doc.KbID, OAuthAccountID: uuid.New(), SourceType: doc.SourceType, SourceRef: doc.SourceRef, SourceURL: "https://acme.feishu.cn/docx/token", Title: "Feishu docx"}

	got, err := repo.CreateFeishuDocumentAndEnqueue(context.Background(), input, queue)
	if err != nil || got.ID != doc.ID {
		t.Fatalf("CreateFeishuDocumentAndEnqueue() = %+v, %v", got, err)
	}
	if queue.tx != tx || !tx.committed || tx.rolledBack {
		t.Fatalf("transaction queue/commit/rollback = %p/%v/%v", queue.tx, tx.committed, tx.rolledBack)
	}
}

func TestSQLFeishuImportRepositoryRollsBackWhenTransactionalEnqueueFails(t *testing.T) {
	doc := generated.Document{ID: uuid.New(), KbID: uuid.New(), SourceType: "feishu-docx", SourceRef: "feishu://feishu.cn/docx/token"}
	tx := &importFakeTx{row: &importFakeRow{doc: doc}}
	repo := NewSQLFeishuImportRepository(&importFakeBeginner{tx: tx})
	queue := &fakeFeishuSyncEnqueuer{err: errors.New("queue failed")}
	input := CreateFeishuDocumentInput{OwnerUserID: uuid.New(), KBID: doc.KbID, OAuthAccountID: uuid.New(), SourceType: doc.SourceType, SourceRef: doc.SourceRef, SourceURL: "https://acme.feishu.cn/docx/token", Title: "Feishu docx"}

	_, err := repo.CreateFeishuDocumentAndEnqueue(context.Background(), input, queue)
	if !errors.Is(err, errFeishuSyncEnqueue) {
		t.Fatalf("CreateFeishuDocumentAndEnqueue() error = %v", err)
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("transaction committed/rolledBack = %v/%v", tx.committed, tx.rolledBack)
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

func TestFeishuImportEnqueueFailureRollsBackDocumentWithoutLeakingCause(t *testing.T) {
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
	if repo.persisted {
		t.Fatal("enqueue failure stranded unique remote document")
	}
}

func TestFeishuImportRetryAfterEnqueueRollbackIsNotAlreadyImported(t *testing.T) {
	docID := uuid.New()
	repo := &fakeFeishuImportRepository{created: generated.Document{ID: docID}}
	queue := &fakeFeishuSyncEnqueuer{err: errors.New("queue failed")}
	svc := NewFeishuImport(feishu.NewURLResolver(), repo, queue)
	input := FeishuImportInput{
		UserID: uuid.NewString(), KBID: uuid.NewString(), OAuthAccountID: uuid.NewString(),
		URL: "https://acme.feishu.cn/docx/DocToken_123",
	}

	if _, err := svc.Import(context.Background(), input); err == nil {
		t.Fatal("first Import() error = nil, want enqueue failure")
	}
	queue.err = nil
	doc, err := svc.Import(context.Background(), input)
	if err != nil {
		var duplicate *ErrFeishuAlreadyImported
		if errors.As(err, &duplicate) {
			t.Fatalf("retry was incorrectly treated as already imported: %v", err)
		}
		t.Fatalf("retry Import() error = %v", err)
	}
	if doc.ID != docID.String() || repo.createCalls != 2 || queue.calls != 2 || !repo.persisted {
		t.Fatalf("retry result = doc:%+v create:%d enqueue:%d persisted:%v", doc, repo.createCalls, queue.calls, repo.persisted)
	}
}

type fakeFeishuImportRepository struct {
	created      generated.Document
	createErr    error
	createCalls  int
	createdInput CreateFeishuDocumentInput
	persisted    bool
}

func (f *fakeFeishuImportRepository) CreateFeishuDocument(_ context.Context, input CreateFeishuDocumentInput) (generated.Document, error) {
	f.createCalls++
	f.createdInput = input
	if f.createErr == nil {
		f.persisted = true
	}
	return f.created, f.createErr
}

func (f *fakeFeishuImportRepository) CreateFeishuDocumentAndEnqueue(ctx context.Context, input CreateFeishuDocumentInput, enqueuer FeishuSyncEnqueuer) (generated.Document, error) {
	row, err := f.CreateFeishuDocument(ctx, input)
	if err != nil {
		return generated.Document{}, err
	}
	if err := enqueuer.EnqueueFeishuSync(ctx, row.ID.String(), InitialFeishuRevision); err != nil {
		f.persisted = false
		return generated.Document{}, errFeishuSyncEnqueue
	}
	return row, nil
}

type fakeFeishuSyncEnqueuer struct {
	calls             int
	documentID        string
	requestedRevision string
	err               error
	tx                pgx.Tx
}

func (f *fakeFeishuSyncEnqueuer) EnqueueFeishuSyncTx(ctx context.Context, tx pgx.Tx, documentID, requestedRevision string) error {
	f.tx = tx
	return f.EnqueueFeishuSync(ctx, documentID, requestedRevision)
}

type importFakeBeginner struct{ tx pgx.Tx }

func (f *importFakeBeginner) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	return f.tx, nil
}

type importFakeTx struct {
	row        pgx.Row
	committed  bool
	rolledBack bool
}

func (t *importFakeTx) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("not implemented")
}
func (t *importFakeTx) Commit(context.Context) error { t.committed = true; return nil }
func (t *importFakeTx) Rollback(context.Context) error {
	if !t.committed {
		t.rolledBack = true
	}
	return nil
}
func (t *importFakeTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("not implemented")
}
func (t *importFakeTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (t *importFakeTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (t *importFakeTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("not implemented")
}
func (t *importFakeTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (t *importFakeTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}
func (t *importFakeTx) QueryRow(context.Context, string, ...any) pgx.Row { return t.row }
func (t *importFakeTx) Conn() *pgx.Conn                                  { return nil }

type importFakeRow struct{ doc generated.Document }

func (r *importFakeRow) Scan(dest ...any) error {
	values := []any{
		r.doc.ID, r.doc.KbID, r.doc.SourceType, r.doc.SourceRef, r.doc.Title, r.doc.MimeType,
		r.doc.Bytes, r.doc.Checksum, r.doc.Status, r.doc.ErrorMessage, r.doc.Metadata, r.doc.CreatedAt,
		r.doc.UpdatedAt, r.doc.ContentRef, r.doc.SourceUrl, r.doc.RemoteRevision, r.doc.OauthAccountID,
		r.doc.PendingContentRef, r.doc.PendingChecksum, r.doc.PendingRemoteRevision, r.doc.SyncStatus,
		r.doc.LastSyncError, r.doc.LastSyncedAt, r.doc.PendingTitle, r.doc.PendingBytes, r.doc.PendingMetadata,
	}
	if len(dest) != len(values) {
		return errors.New("scan count mismatch")
	}
	for i := range values {
		target := reflect.ValueOf(dest[i]).Elem()
		value := reflect.ValueOf(values[i])
		if !value.IsValid() {
			target.Set(reflect.Zero(target.Type()))
			continue
		}
		target.Set(value)
	}
	return nil
}

func (f *fakeFeishuSyncEnqueuer) EnqueueFeishuSync(_ context.Context, documentID, requestedRevision string) error {
	f.calls++
	f.documentID = documentID
	f.requestedRevision = requestedRevision
	return f.err
}
