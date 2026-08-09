package worker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/feishu"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

func TestFeishuSyncJobIsUniqueByDocumentAndRequestedRevision(t *testing.T) {
	opts := (FeishuSyncJobArgs{DocumentID: uuid.NewString(), RequestedRevision: "rev-2"}).InsertOpts()
	if opts.MaxAttempts != 1 || !opts.UniqueOpts.ByArgs {
		t.Fatalf("InsertOpts() = %+v", opts)
	}
	wantStates := []rivertype.JobState{rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateScheduled, rivertype.JobStateRetryable}
	if len(opts.UniqueOpts.ByState) != len(wantStates) {
		t.Fatalf("unique states = %v", opts.UniqueOpts.ByState)
	}
}

func TestFeishuSyncUnchangedRevisionUpdatesSyncOnly(t *testing.T) {
	state := newSyncState("rev-1", "sum-1", "active.md")
	storage := newMemoryStorage()
	queue := &fakeStagedIngestionQueue{}
	worker := newTestFeishuWorker(t, state, canonicalDoc(t, "rev-1", "same body"), storage, queue)

	if err := worker.Work(context.Background(), syncJob(state.doc.ID, "rev-1")); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if state.doc.SyncStatus != "idle" || !state.doc.LastSyncedAt.Valid {
		t.Fatalf("sync state = %q at=%+v", state.doc.SyncStatus, state.doc.LastSyncedAt)
	}
	if len(storage.objects) != 0 || queue.calls != 0 {
		t.Fatalf("unchanged revision wrote/enqueued = %d/%d", len(storage.objects), queue.calls)
	}
	assertActive(t, state.doc, "rev-1", "sum-1", "active.md")
}

func TestFeishuSyncChangedRevisionSameChecksumPromotesSnapshotWithoutIngestion(t *testing.T) {
	markdown := "same canonical body"
	checksum := sha256Hex([]byte(markdown))
	state := newSyncState("rev-1", checksum, "active.md")
	storage := newMemoryStorage()
	queue := &fakeStagedIngestionQueue{}
	worker := newTestFeishuWorker(t, state, canonicalDoc(t, "unsafe/rev?2", markdown), storage, queue)

	if err := worker.Work(context.Background(), syncJob(state.doc.ID, "rev-1")); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if queue.calls != 0 || len(storage.objects) != 1 {
		t.Fatalf("same checksum enqueue/objects = %d/%d", queue.calls, len(storage.objects))
	}
	var key string
	for key = range storage.objects {
	}
	if strings.Contains(key, "DocToken_123") || strings.Contains(key, "unsafe") || strings.Contains(key, "Runbook") {
		t.Fatalf("snapshot key contains unsafe remote data: %q", key)
	}
	assertActive(t, state.doc, "unsafe/rev?2", checksum, key)
	if state.doc.PendingContentRef != nil || state.doc.SyncStatus != "idle" {
		t.Fatalf("pending/sync after promotion = %v/%q", state.doc.PendingContentRef, state.doc.SyncStatus)
	}
}

func TestFeishuSyncChangedChecksumStagesAndEnqueues(t *testing.T) {
	state := newSyncState("rev-1", "old-sum", "active.md")
	storage := newMemoryStorage()
	queue := &fakeStagedIngestionQueue{}
	worker := newTestFeishuWorker(t, state, canonicalDoc(t, "rev-2", "changed body"), storage, queue)

	if err := worker.Work(context.Background(), syncJob(state.doc.ID, "rev-1")); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	assertActive(t, state.doc, "rev-1", "old-sum", "active.md")
	if state.doc.PendingContentRef == nil || state.doc.PendingChecksum == nil || state.doc.PendingRemoteRevision == nil {
		t.Fatalf("pending snapshot not staged: %+v", state.doc)
	}
	if queue.calls != 1 || queue.contentRef != *state.doc.PendingContentRef || queue.checksum != *state.doc.PendingChecksum || queue.revision != "rev-2" {
		t.Fatalf("staged enqueue = %+v", queue)
	}
	if state.doc.SyncStatus != "syncing" {
		t.Fatalf("sync status = %q, want syncing until ingestion promotion", state.doc.SyncStatus)
	}
}

func TestFeishuSyncFirstImportStagesWithoutPublishingActiveSnapshot(t *testing.T) {
	state := newSyncState("", "", "")
	state.doc.ContentRef, state.doc.RemoteRevision = nil, nil
	state.doc.Status = "pending"
	queue := &fakeStagedIngestionQueue{}
	worker := newTestFeishuWorker(t, state, canonicalDoc(t, "rev-1", "first body"), newMemoryStorage(), queue)

	if err := worker.Work(context.Background(), syncJob(state.doc.ID, InitialRevisionForTest)); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if state.doc.ContentRef != nil || state.doc.RemoteRevision != nil || state.doc.Checksum != "" {
		t.Fatalf("first import published active state before ingestion: %+v", state.doc)
	}
	if state.doc.PendingContentRef == nil || queue.calls != 1 || state.doc.SyncStatus != "syncing" {
		t.Fatalf("first import was not staged: doc=%+v queue=%+v", state.doc, queue)
	}
}

func TestFeishuSyncFailurePreservesActiveAndRedactsError(t *testing.T) {
	state := newSyncState("rev-1", "old-sum", "active.md")
	loader := &fakeSourceLoader{err: errors.New("provider body SECRET_TOKEN https://host/path?secret=yes")}
	worker := NewFeishuSyncWorker(FeishuSyncWorkerDeps{
		Repository: state, Resolver: feishu.NewURLResolver(), Loaders: map[domain.ResourceType]ports.SourceLoader{domain.ResourceDocx: loader},
		Tokens: fakeTokenProvider{token: "SECRET_TOKEN"}, Storage: newMemoryStorage(), Ingestion: &fakeStagedIngestionQueue{},
	})

	err := worker.Work(context.Background(), syncJob(state.doc.ID, "rev-1"))
	if err == nil || strings.Contains(err.Error(), "SECRET_TOKEN") || strings.Contains(err.Error(), "secret=yes") {
		t.Fatalf("Work() error leaked sensitive text: %v", err)
	}
	assertActive(t, state.doc, "rev-1", "old-sum", "active.md")
	if state.doc.SyncStatus != "failed" || state.doc.LastSyncError == nil || *state.doc.LastSyncError != "source load failed" {
		t.Fatalf("failure state = %q/%v", state.doc.SyncStatus, state.doc.LastSyncError)
	}
}

func TestFeishuSyncStorageFailurePreservesActiveSnapshot(t *testing.T) {
	state := newSyncState("rev-1", "old-sum", "active.md")
	storage := newMemoryStorage()
	storage.putErr = errors.New("storage SECRET body")
	worker := newTestFeishuWorker(t, state, canonicalDoc(t, "rev-2", "changed"), storage, &fakeStagedIngestionQueue{})

	err := worker.Work(context.Background(), syncJob(state.doc.ID, "rev-1"))
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("Work() error = %v", err)
	}
	assertActive(t, state.doc, "rev-1", "old-sum", "active.md")
	if state.doc.SyncStatus != "failed" || state.doc.LastSyncError == nil || *state.doc.LastSyncError != "snapshot write failed" {
		t.Fatalf("storage failure state = %+v", state.doc)
	}
}

func TestFeishuSyncSameChecksumPromotionFailurePreservesActiveSnapshot(t *testing.T) {
	markdown := "same body"
	state := newSyncState("rev-1", sha256Hex([]byte(markdown)), "active.md")
	state.promoteErr = errors.New("database SECRET body")
	worker := newTestFeishuWorker(t, state, canonicalDoc(t, "rev-2", markdown), newMemoryStorage(), &fakeStagedIngestionQueue{})

	err := worker.Work(context.Background(), syncJob(state.doc.ID, "rev-1"))
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("Work() error = %v", err)
	}
	assertActive(t, state.doc, "rev-1", sha256Hex([]byte(markdown)), "active.md")
	if state.doc.PendingContentRef != nil || state.doc.SyncStatus != "failed" {
		t.Fatalf("promotion failure did not clear matching pending state: %+v", state.doc)
	}
}

func TestFeishuSyncEnqueueFailureClearsOnlyMatchingPendingSnapshot(t *testing.T) {
	state := newSyncState("rev-1", "old-sum", "active.md")
	oldMetadata := append([]byte(nil), state.doc.Metadata...)
	oldTitle, oldBytes := state.doc.Title, state.doc.Bytes
	queue := &fakeStagedIngestionQueue{err: errors.New("queue includes SECRET")}
	worker := newTestFeishuWorker(t, state, canonicalDoc(t, "rev-2", "changed body"), newMemoryStorage(), queue)

	err := worker.Work(context.Background(), syncJob(state.doc.ID, "rev-1"))
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("Work() error = %v", err)
	}
	assertActive(t, state.doc, "rev-1", "old-sum", "active.md")
	if state.doc.PendingContentRef != nil || state.doc.PendingChecksum != nil || state.doc.PendingRemoteRevision != nil {
		t.Fatalf("failed pending snapshot was not cleared: %+v", state.doc)
	}
	if state.doc.SyncStatus != "failed" || state.doc.LastSyncError == nil || *state.doc.LastSyncError != "ingestion enqueue failed" {
		t.Fatalf("failure state = %q/%v", state.doc.SyncStatus, state.doc.LastSyncError)
	}
	if state.doc.Title != oldTitle || state.doc.Bytes != oldBytes || !bytes.Equal(state.doc.Metadata, oldMetadata) {
		t.Fatalf("failed resync changed active metadata: title=%q bytes=%d metadata=%s", state.doc.Title, state.doc.Bytes, state.doc.Metadata)
	}
}

func TestFeishuSyncStaleJobCannotClaimOrMutateNewerSync(t *testing.T) {
	state := newSyncState("rev-2", "sum-2", "new.md")
	state.doc.SyncStatus = "syncing"
	state.doc.PendingRemoteRevision = ptr("rev-3")
	tokens := &countingTokenProvider{}
	worker := NewFeishuSyncWorker(FeishuSyncWorkerDeps{
		Repository: state, Resolver: feishu.NewURLResolver(), Loaders: map[domain.ResourceType]ports.SourceLoader{},
		Tokens: tokens, Storage: newMemoryStorage(), Ingestion: &fakeStagedIngestionQueue{},
	})

	if err := worker.Work(context.Background(), syncJob(state.doc.ID, "rev-1")); err != nil {
		t.Fatalf("stale Work() error = %v", err)
	}
	if tokens.calls != 0 || state.doc.PendingRemoteRevision == nil || *state.doc.PendingRemoteRevision != "rev-3" {
		t.Fatalf("stale worker touched newer state: token calls=%d doc=%+v", tokens.calls, state.doc)
	}
}

func newTestFeishuWorker(t *testing.T, state *memorySyncRepository, canonical domain.CanonicalDocument, storage *memoryStorage, queue *fakeStagedIngestionQueue) *FeishuSyncWorker {
	t.Helper()
	return NewFeishuSyncWorker(FeishuSyncWorkerDeps{
		Repository: state, Resolver: feishu.NewURLResolver(),
		Loaders: map[domain.ResourceType]ports.SourceLoader{domain.ResourceDocx: &fakeSourceLoader{document: canonical}},
		Tokens:  fakeTokenProvider{token: "access-token"}, Storage: storage, Ingestion: queue,
	})
}

func syncJob(id uuid.UUID, requestedRevision string) *river.Job[FeishuSyncJobArgs] {
	return &river.Job[FeishuSyncJobArgs]{Args: FeishuSyncJobArgs{DocumentID: id.String(), RequestedRevision: requestedRevision}}
}

func canonicalDoc(t *testing.T, revision, markdown string) domain.CanonicalDocument {
	t.Helper()
	ref, err := feishu.NewURLResolver().Resolve("https://acme.feishu.cn/docx/DocToken_123")
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := domain.NewSourceMetadata(domain.SourceMetadataInput{SourceType: domain.ResourceDocx, RemoteRevision: revision, SourceLocator: ref.CanonicalURL})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := domain.NewCanonicalDocument(domain.CanonicalDocumentInput{Title: "Runbook", Markdown: markdown, RemoteRevision: revision, SourceMetadata: metadata, SafeSourceURL: ref.CanonicalURL})
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

type fakeSourceLoader struct {
	document domain.CanonicalDocument
	err      error
}

func (f *fakeSourceLoader) Load(context.Context, domain.ResourceRef, string) (domain.CanonicalDocument, error) {
	return f.document, f.err
}

type fakeTokenProvider struct{ token string }

func (f fakeTokenProvider) AccessToken(context.Context, string) (string, error) { return f.token, nil }

type countingTokenProvider struct{ calls int }

func (f *countingTokenProvider) AccessToken(context.Context, string) (string, error) {
	f.calls++
	return "", nil
}

type fakeStagedIngestionQueue struct {
	calls      int
	documentID string
	contentRef string
	checksum   string
	revision   string
	err        error
}

func (f *fakeStagedIngestionQueue) EnqueueStagedIngestion(_ context.Context, snapshot PendingFeishuSnapshot) error {
	f.calls++
	f.documentID, f.contentRef, f.checksum, f.revision = snapshot.DocumentID.String(), snapshot.ContentRef, snapshot.Checksum, snapshot.RemoteRevision
	return f.err
}

type memoryStorage struct {
	objects map[string][]byte
	putErr  error
}

const InitialRevisionForTest = "initial"

func newMemoryStorage() *memoryStorage { return &memoryStorage{objects: map[string][]byte{}} }
func (m *memoryStorage) Put(_ context.Context, key string, body io.Reader, _ int64, _ string) error {
	if m.putErr != nil {
		return m.putErr
	}
	b, err := io.ReadAll(body)
	if err == nil {
		m.objects[key] = b
	}
	return err
}
func (m *memoryStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	b, ok := m.objects[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
func (m *memoryStorage) Delete(_ context.Context, key string) error {
	delete(m.objects, key)
	return nil
}

type memorySyncRepository struct {
	doc        generated.Document
	promoteErr error
}

func newSyncState(revision, checksum, contentRef string) *memorySyncRepository {
	accountID := uuid.New()
	sourceURL := "https://acme.feishu.cn/docx/DocToken_123"
	return &memorySyncRepository{doc: generated.Document{
		ID: uuid.New(), KbID: uuid.New(), SourceType: "feishu-docx", SourceRef: "feishu://feishu.cn/docx/DocToken_123",
		Title: "Runbook", MimeType: "text/markdown", Checksum: checksum, Status: "ready", ContentRef: ptr(contentRef),
		SourceUrl: &sourceURL, RemoteRevision: ptr(revision), OauthAccountID: pgtype.UUID{Bytes: accountID, Valid: true}, SyncStatus: "idle",
	}}
}

func (m *memorySyncRepository) ClaimFeishuSync(context.Context, uuid.UUID) (generated.Document, error) {
	if m.doc.SyncStatus == "syncing" {
		return generated.Document{}, pgx.ErrNoRows
	}
	m.doc.SyncStatus = "syncing"
	m.doc.LastSyncError = nil
	return m.doc, nil
}
func (m *memorySyncRepository) CompleteUnchangedFeishuSync(_ context.Context, in FeishuSyncExpectation) (bool, error) {
	if m.doc.SyncStatus != "syncing" || !sameString(m.doc.RemoteRevision, in.RemoteRevision) || m.doc.Checksum != in.Checksum {
		return false, nil
	}
	m.doc.SyncStatus = "idle"
	m.doc.LastSyncedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	return true, nil
}
func (m *memorySyncRepository) StageFeishuSnapshot(_ context.Context, in StageFeishuSnapshotInput) (bool, error) {
	if m.doc.SyncStatus != "syncing" || !sameString(m.doc.RemoteRevision, in.Expected.RemoteRevision) || m.doc.Checksum != in.Expected.Checksum || m.doc.PendingContentRef != nil {
		return false, nil
	}
	m.doc.PendingContentRef, m.doc.PendingChecksum, m.doc.PendingRemoteRevision = ptr(in.Snapshot.ContentRef), ptr(in.Snapshot.Checksum), ptr(in.Snapshot.RemoteRevision)
	return true, nil
}
func (m *memorySyncRepository) PromoteFeishuSnapshot(_ context.Context, in PendingFeishuSnapshot) (bool, error) {
	if m.promoteErr != nil {
		return false, m.promoteErr
	}
	if !m.matchesPending(in) {
		return false, nil
	}
	m.doc.ContentRef, m.doc.Checksum, m.doc.RemoteRevision = ptr(in.ContentRef), in.Checksum, ptr(in.RemoteRevision)
	m.doc.Title, m.doc.MimeType, m.doc.Bytes, m.doc.Metadata = in.Title, "text/markdown", in.Bytes, append([]byte(nil), in.Metadata...)
	m.doc.PendingContentRef, m.doc.PendingChecksum, m.doc.PendingRemoteRevision = nil, nil, nil
	m.doc.SyncStatus, m.doc.Status, m.doc.LastSyncError = "idle", "ready", nil
	m.doc.LastSyncedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	return true, nil
}
func (m *memorySyncRepository) FailFeishuSync(_ context.Context, in FailFeishuSyncInput) (bool, error) {
	if m.doc.SyncStatus != "syncing" || !m.matchesExpectedPending(in.Pending) {
		return false, nil
	}
	m.doc.PendingContentRef, m.doc.PendingChecksum, m.doc.PendingRemoteRevision = nil, nil, nil
	m.doc.SyncStatus, m.doc.LastSyncError = "failed", ptr(in.SafeError)
	if m.doc.ContentRef == nil {
		m.doc.Status = "failed"
	}
	return true, nil
}
func (m *memorySyncRepository) matchesPending(in PendingFeishuSnapshot) bool {
	return m.doc.SyncStatus == "syncing" && sameString(m.doc.PendingContentRef, &in.ContentRef) && sameString(m.doc.PendingChecksum, &in.Checksum) && sameString(m.doc.PendingRemoteRevision, &in.RemoteRevision)
}
func (m *memorySyncRepository) matchesExpectedPending(in *PendingFeishuSnapshot) bool {
	if in == nil {
		return m.doc.PendingContentRef == nil && m.doc.PendingChecksum == nil && m.doc.PendingRemoteRevision == nil
	}
	return m.matchesPending(*in)
}

func assertActive(t *testing.T, doc generated.Document, revision, checksum, contentRef string) {
	t.Helper()
	if doc.RemoteRevision == nil || *doc.RemoteRevision != revision || doc.Checksum != checksum || doc.ContentRef == nil || *doc.ContentRef != contentRef {
		t.Fatalf("active snapshot = revision:%v checksum:%q ref:%v", doc.RemoteRevision, doc.Checksum, doc.ContentRef)
	}
}
func sameString(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
func ptr(s string) *string { return &s }
