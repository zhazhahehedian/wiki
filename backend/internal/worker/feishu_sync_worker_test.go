package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

func TestSQLFeishuSyncRepositoryStagesAndEnqueuesInSameTransaction(t *testing.T) {
	tx := &syncRepoFakeTx{tag: pgconn.NewCommandTag("UPDATE 1")}
	repo := NewSQLFeishuSyncRepository(&syncRepoFakeBeginner{tx: tx})
	queue := &fakeStagedIngestionQueue{}
	claimToken := time.Now().UTC()
	input := StageFeishuSnapshotInput{
		Expected: FeishuSyncExpectation{DocumentID: uuid.New(), Checksum: "old", ClaimToken: claimToken},
		Snapshot: PendingFeishuSnapshot{DocumentID: uuid.New(), ContentRef: "new.md", Checksum: "new", RemoteRevision: "r2", ClaimToken: claimToken},
	}

	staged, err := repo.StageFeishuSnapshotAndEnqueue(context.Background(), input, queue)
	if err != nil || !staged {
		t.Fatalf("StageFeishuSnapshotAndEnqueue() = %v, %v", staged, err)
	}
	if queue.tx != tx || !tx.committed || tx.rolledBack {
		t.Fatalf("transaction queue/commit/rollback = %p/%v/%v", queue.tx, tx.committed, tx.rolledBack)
	}
	if len(tx.ops) != 2 || !strings.Contains(tx.ops[0], "UPDATE documents") || tx.ops[1] != "commit" {
		t.Fatalf("transaction operations = %#v", tx.ops)
	}
}

func TestSQLFeishuSyncRepositoryRollsBackStageWhenTransactionalEnqueueFails(t *testing.T) {
	tx := &syncRepoFakeTx{tag: pgconn.NewCommandTag("UPDATE 1")}
	repo := NewSQLFeishuSyncRepository(&syncRepoFakeBeginner{tx: tx})
	queue := &fakeStagedIngestionQueue{err: errors.New("queue failed")}
	claimToken := time.Now().UTC()
	input := StageFeishuSnapshotInput{
		Expected: FeishuSyncExpectation{DocumentID: uuid.New(), Checksum: "old", ClaimToken: claimToken},
		Snapshot: PendingFeishuSnapshot{DocumentID: uuid.New(), ContentRef: "new.md", Checksum: "new", RemoteRevision: "r2", ClaimToken: claimToken},
	}

	if staged, err := repo.StageFeishuSnapshotAndEnqueue(context.Background(), input, queue); err == nil || staged || !errors.Is(err, errSnapshotStageRolledBack) {
		t.Fatalf("StageFeishuSnapshotAndEnqueue() = %v, %v, want rollback error", staged, err)
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("transaction committed/rolledBack = %v/%v", tx.committed, tx.rolledBack)
	}
}

func TestSQLFeishuSyncRepositoryDoesNotClassifyCommitErrorAsKnownRollback(t *testing.T) {
	tx := &syncRepoFakeTx{tag: pgconn.NewCommandTag("UPDATE 1"), commitErr: errors.New("commit result unknown")}
	repo := NewSQLFeishuSyncRepository(&syncRepoFakeBeginner{tx: tx})
	claimToken := time.Now().UTC()
	input := StageFeishuSnapshotInput{
		Expected: FeishuSyncExpectation{DocumentID: uuid.New(), Checksum: "old", ClaimToken: claimToken},
		Snapshot: PendingFeishuSnapshot{DocumentID: uuid.New(), ContentRef: "new.md", Checksum: "new", RemoteRevision: "r2", ClaimToken: claimToken},
	}

	staged, err := repo.StageFeishuSnapshotAndEnqueue(context.Background(), input, &fakeStagedIngestionQueue{})
	if err == nil || staged {
		t.Fatalf("StageFeishuSnapshotAndEnqueue() = %v, %v, want ambiguous commit error", staged, err)
	}
	if errors.Is(err, errSnapshotStageRolledBack) {
		t.Fatalf("commit error %v was incorrectly classified as a known rollback", err)
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

func TestFeishuSyncSameChecksumPatchesChunkCitationsAndPromotesAtomically(t *testing.T) {
	markdown := "same canonical body"
	checksum := sha256Hex([]byte(markdown))
	state := newSyncState("rev-1", checksum, "active.md")
	embedderCalls := 0
	queue := &fakeStagedIngestionQueue{onEnqueue: func() { embedderCalls++ }}
	citations := &fakeStagedVectorStore{}
	worker := NewFeishuSyncWorker(FeishuSyncWorkerDeps{
		Repository: state, Resolver: feishu.NewURLResolver(),
		Loaders: map[domain.ResourceType]ports.SourceLoader{domain.ResourceDocx: &fakeSourceLoader{document: canonicalDoc(t, "rev-2", markdown)}},
		Tokens:  fakeTokenProvider{token: "token"}, Storage: newMemoryStorage(),
		Ingestion: queue, CitationStore: citations,
	})

	if err := worker.Work(context.Background(), syncJob(state.doc.ID, "rev-1")); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if citations.patchCalls != 1 {
		t.Fatalf("citation patch calls = %d, want 1", citations.patchCalls)
	}
	if state.promoteCalls != 0 {
		t.Fatalf("document-only promotion calls = %d, want 0", state.promoteCalls)
	}
	if queue.calls != 0 || embedderCalls != 0 {
		t.Fatalf("same-checksum path entered ingestion/embed pipeline: queue=%d embedder=%d", queue.calls, embedderCalls)
	}
}

func TestCitationMetadataPatcherUsesMatchingSectionAndPreservesUnrelatedMetadata(t *testing.T) {
	patch := citationMetadataPatcher(json.RawMessage(`{
		"source_type":"feishu-sheet",
		"source_url":"https://acme.feishu.cn/sheets/token",
		"remote_revision":"rev-2",
		"locations":[
			{"section_path":"Budget","sheet_name":"Budget","sheet_id":"sh_budget","row_start":2,"row_end":8},
			{"section_path":"Forecast","sheet_name":"Forecast","sheet_id":"sh_forecast","row_start":9,"row_end":12}
		]
	}`))

	got := patch(map[string]any{
		"section_path": "Forecast", "sheet_id": "old", "remote_revision": "rev-1",
		"content_hash": "preserve-me", "token_count": float64(42),
	})
	if got["sheet_id"] != "sh_forecast" || got["row_start"] != 9 || got["row_end"] != 12 {
		t.Fatalf("section-specific citation metadata = %+v", got)
	}
	if got["content_hash"] != "preserve-me" || got["token_count"] != float64(42) {
		t.Fatalf("unrelated chunk metadata was not preserved: %+v", got)
	}
	if _, ok := got["locations"]; ok {
		t.Fatalf("raw locations leaked into citation metadata: %+v", got)
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

func TestFeishuSyncStorageFailureDeletesPartialAttemptObject(t *testing.T) {
	state := newSyncState("rev-1", "old-sum", "active.md")
	storage := newMemoryStorage()
	storage.putErr = errors.New("write failed")
	storage.putBeforeError = true
	worker := newTestFeishuWorker(t, state, canonicalDoc(t, "rev-2", "changed"), storage, &fakeStagedIngestionQueue{})

	if err := worker.Work(context.Background(), syncJob(state.doc.ID, "rev-1")); err == nil {
		t.Fatal("Work() error = nil, want storage failure")
	}
	if len(storage.deleted) != 1 || len(storage.objects) != 0 {
		t.Fatalf("partial attempt cleanup = deleted:%v objects:%v", storage.deleted, storage.objects)
	}
}

func TestFeishuSyncInvalidMetadataDoesNotWriteSnapshot(t *testing.T) {
	state := newSyncState("rev-1", "old-sum", "active.md")
	canonical := canonicalDoc(t, "rev-2", "changed")
	canonical.SourceMetadata = domain.SourceMetadata{}
	storage := newMemoryStorage()
	worker := newTestFeishuWorker(t, state, canonical, storage, &fakeStagedIngestionQueue{})

	if err := worker.Work(context.Background(), syncJob(state.doc.ID, "rev-1")); err == nil || err.Error() != "source metadata invalid" {
		t.Fatalf("Work() error = %v, want metadata failure", err)
	}
	if len(storage.objects) != 0 {
		t.Fatalf("invalid metadata wrote snapshots: %v", storage.objects)
	}
}

func TestSnapshotKeyIsScopedToClaimAttempt(t *testing.T) {
	documentID := uuid.New()
	first := snapshotKey(documentID, "rev-2", "sum", time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC))
	second := snapshotKey(documentID, "rev-2", "sum", time.Date(2026, 8, 9, 12, 20, 0, 0, time.UTC))
	if first == second {
		t.Fatalf("snapshot keys collide across attempts: %q", first)
	}
}

func TestFeishuSyncSameChecksumPromotionFailurePreservesActiveSnapshot(t *testing.T) {
	markdown := "same body"
	state := newSyncState("rev-1", sha256Hex([]byte(markdown)), "active.md")
	state.promoteErr = errors.New("database SECRET body")
	storage := newMemoryStorage()
	worker := newTestFeishuWorker(t, state, canonicalDoc(t, "rev-2", markdown), storage, &fakeStagedIngestionQueue{})

	err := worker.Work(context.Background(), syncJob(state.doc.ID, "rev-1"))
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("Work() error = %v", err)
	}
	assertActive(t, state.doc, "rev-1", sha256Hex([]byte(markdown)), "active.md")
	if state.doc.PendingContentRef == nil || state.doc.SyncStatus != "failed" {
		t.Fatalf("promotion failure did not retain retryable pending state: %+v", state.doc)
	}
	if len(storage.deleted) != 0 {
		t.Fatalf("referenced pending snapshot was deleted: %v", storage.deleted)
	}
	if _, ok := storage.objects[*state.doc.PendingContentRef]; !ok {
		t.Fatalf("referenced pending snapshot %q is missing", *state.doc.PendingContentRef)
	}
}

func TestFeishuSyncAmbiguousSameChecksumStageRetainsPossiblyReferencedSnapshot(t *testing.T) {
	markdown := "same body"
	state := newSyncState("rev-1", sha256Hex([]byte(markdown)), "active.md")
	repo := &ambiguousSnapshotStageRepository{memorySyncRepository: state, err: errors.New("statement result unknown")}
	storage := newMemoryStorage()
	worker := NewFeishuSyncWorker(FeishuSyncWorkerDeps{
		Repository: repo, Resolver: feishu.NewURLResolver(),
		Loaders: map[domain.ResourceType]ports.SourceLoader{domain.ResourceDocx: &fakeSourceLoader{document: canonicalDoc(t, "rev-2", markdown)}},
		Tokens:  fakeTokenProvider{token: "token"}, Storage: storage, Ingestion: &fakeStagedIngestionQueue{},
		CitationStore: &fakeStagedVectorStore{state: state},
	})

	if err := worker.Work(context.Background(), syncJob(state.doc.ID, "rev-1")); err == nil {
		t.Fatal("Work() error = nil, want ambiguous stage failure")
	}
	if state.doc.PendingContentRef == nil || len(storage.deleted) != 0 {
		t.Fatalf("ambiguous stage snapshot reference/deletes = %v/%v", state.doc.PendingContentRef, storage.deleted)
	}
	if _, ok := storage.objects[*state.doc.PendingContentRef]; !ok {
		t.Fatalf("possibly referenced snapshot %q was removed", *state.doc.PendingContentRef)
	}
}

func TestFeishuSyncEnqueueFailureClearsOnlyMatchingPendingSnapshot(t *testing.T) {
	state := newSyncState("rev-1", "old-sum", "active.md")
	oldMetadata := append([]byte(nil), state.doc.Metadata...)
	oldTitle, oldBytes := state.doc.Title, state.doc.Bytes
	queue := &fakeStagedIngestionQueue{err: errors.New("queue includes SECRET")}
	storage := newMemoryStorage()
	worker := newTestFeishuWorker(t, state, canonicalDoc(t, "rev-2", "changed body"), storage, queue)

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
	if len(storage.deleted) != 1 {
		t.Fatalf("unreferenced snapshot deletes = %v, want one", storage.deleted)
	}
}

func TestFeishuSyncAmbiguousEnqueueCommitRetainsPossiblyReferencedSnapshot(t *testing.T) {
	state := newSyncState("rev-1", "old-sum", "active.md")
	repo := &ambiguousStageSyncRepository{memorySyncRepository: state, err: errors.New("commit result unknown")}
	storage := newMemoryStorage()
	worker := NewFeishuSyncWorker(FeishuSyncWorkerDeps{
		Repository: repo, Resolver: feishu.NewURLResolver(),
		Loaders: map[domain.ResourceType]ports.SourceLoader{domain.ResourceDocx: &fakeSourceLoader{document: canonicalDoc(t, "rev-2", "changed body")}},
		Tokens:  fakeTokenProvider{token: "token"}, Storage: storage, Ingestion: &fakeStagedIngestionQueue{},
		CitationStore: &fakeStagedVectorStore{state: state},
	})

	err := worker.Work(context.Background(), syncJob(state.doc.ID, "rev-1"))
	if err == nil {
		t.Fatal("Work() error = nil, want enqueue commit failure")
	}
	if state.doc.PendingContentRef == nil {
		t.Fatalf("ambiguous commit did not retain possible pending reference: %+v", state.doc)
	}
	if len(storage.deleted) != 0 {
		t.Fatalf("possibly referenced snapshots were deleted: %v", storage.deleted)
	}
	if _, ok := storage.objects[*state.doc.PendingContentRef]; !ok {
		t.Fatalf("possibly referenced snapshot %q was removed", *state.doc.PendingContentRef)
	}
}

func TestFeishuSyncSnapshotCleanupFailureIsBestEffort(t *testing.T) {
	state := newSyncState("rev-1", "old-sum", "active.md")
	storage := newMemoryStorage()
	storage.deleteErr = errors.New("delete failed")
	queue := &fakeStagedIngestionQueue{err: errors.New("queue failed")}
	worker := newTestFeishuWorker(t, state, canonicalDoc(t, "rev-2", "changed body"), storage, queue)

	err := worker.Work(context.Background(), syncJob(state.doc.ID, "rev-1"))
	if err == nil || err.Error() != "ingestion enqueue failed" {
		t.Fatalf("Work() error = %v, want redacted enqueue failure", err)
	}
	if len(storage.deleted) != 1 || len(storage.objects) != 1 {
		t.Fatalf("best-effort cleanup attempts/remaining objects = %v/%d", storage.deleted, len(storage.objects))
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

func TestFeishuSyncOldQueuedRevisionCannotClaimIdleNewerDocument(t *testing.T) {
	state := newSyncState("rev-2", "sum-2", "new.md")
	tokens := &countingTokenProvider{}
	worker := NewFeishuSyncWorker(FeishuSyncWorkerDeps{
		Repository: state, Resolver: feishu.NewURLResolver(), Loaders: map[domain.ResourceType]ports.SourceLoader{},
		Tokens: tokens, Storage: newMemoryStorage(), Ingestion: &fakeStagedIngestionQueue{},
	})

	if err := worker.Work(context.Background(), syncJob(state.doc.ID, "rev-1")); err != nil {
		t.Fatalf("old queued Work() error = %v", err)
	}
	if tokens.calls != 0 || state.doc.SyncStatus != "idle" {
		t.Fatalf("old queued job claimed newer document: token calls=%d status=%q", tokens.calls, state.doc.SyncStatus)
	}
}

func TestFeishuSyncCancellationUsesDetachedFailureCleanup(t *testing.T) {
	state := newSyncState("rev-1", "sum-1", "active.md")
	loader := &fakeSourceLoader{err: context.Canceled}
	worker := NewFeishuSyncWorker(FeishuSyncWorkerDeps{
		Repository: state, Resolver: feishu.NewURLResolver(),
		Loaders: map[domain.ResourceType]ports.SourceLoader{domain.ResourceDocx: loader},
		Tokens:  fakeTokenProvider{token: "token"}, Storage: newMemoryStorage(), Ingestion: &fakeStagedIngestionQueue{},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := worker.Work(ctx, syncJob(state.doc.ID, "rev-1")); err == nil {
		t.Fatal("Work() error = nil, want cancellation failure")
	}
	if state.failContextErr != nil {
		t.Fatalf("failure cleanup context = %v, want detached context", state.failContextErr)
	}
	if state.doc.SyncStatus != "failed" {
		t.Fatalf("sync status = %q, want failed", state.doc.SyncStatus)
	}
}

func TestFeishuSyncReclaimsExpiredLease(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	state := newSyncState("rev-1", "old-sum", "active.md")
	state.doc.SyncStatus = "syncing"
	state.doc.UpdatedAt = now.Add(-20 * time.Minute)
	state.claimNow = now
	queue := &fakeStagedIngestionQueue{}
	worker := NewFeishuSyncWorker(FeishuSyncWorkerDeps{
		Repository: state, Resolver: feishu.NewURLResolver(),
		Loaders: map[domain.ResourceType]ports.SourceLoader{domain.ResourceDocx: &fakeSourceLoader{document: canonicalDoc(t, "rev-2", "changed")}},
		Tokens:  fakeTokenProvider{token: "token"}, Storage: newMemoryStorage(), Ingestion: queue,
		Now: func() time.Time { return now }, SyncLease: 10 * time.Minute,
	})

	if err := worker.Work(context.Background(), syncJob(state.doc.ID, "rev-1")); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if queue.calls != 1 || state.doc.UpdatedAt != now {
		t.Fatalf("expired lease was not reclaimed: queue=%d updated_at=%v", queue.calls, state.doc.UpdatedAt)
	}
}

func TestFeishuSyncDoesNotReclaimLiveLease(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	state := newSyncState("rev-1", "old-sum", "active.md")
	state.doc.SyncStatus = "syncing"
	state.doc.UpdatedAt = now.Add(-time.Minute)
	state.claimNow = now
	tokens := &countingTokenProvider{}
	worker := NewFeishuSyncWorker(FeishuSyncWorkerDeps{
		Repository: state, Resolver: feishu.NewURLResolver(), Loaders: map[domain.ResourceType]ports.SourceLoader{},
		Tokens: tokens, Storage: newMemoryStorage(), Ingestion: &fakeStagedIngestionQueue{},
		Now: func() time.Time { return now }, SyncLease: 10 * time.Minute,
	})

	if err := worker.Work(context.Background(), syncJob(state.doc.ID, "rev-1")); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if tokens.calls != 0 || state.doc.SyncStatus != "syncing" {
		t.Fatalf("live lease was reclaimed: token calls=%d status=%q", tokens.calls, state.doc.SyncStatus)
	}
}

func TestFeishuSyncOldAttemptCannotStageAfterLeaseReclaim(t *testing.T) {
	t1 := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	t2 := t1.Add(20 * time.Minute)
	state := newSyncState("rev-1", "sum-1", "active.md")
	state.claimNow = t1
	first, err := state.ClaimFeishuSync(context.Background(), state.doc.ID, ptr("rev-1"), t1.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	state.claimNow = t2
	second, err := state.ClaimFeishuSync(context.Background(), state.doc.ID, ptr("rev-1"), t2.Add(-10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if first.UpdatedAt == second.UpdatedAt {
		t.Fatal("reclaim did not issue a new attempt token")
	}

	staged, err := state.StageFeishuSnapshot(context.Background(), StageFeishuSnapshotInput{
		Expected: FeishuSyncExpectation{DocumentID: state.doc.ID, RemoteRevision: ptr("rev-1"), Checksum: "sum-1", ClaimToken: first.UpdatedAt},
		Snapshot: PendingFeishuSnapshot{DocumentID: state.doc.ID, ContentRef: "stale.md", Checksum: "stale", RemoteRevision: "rev-old", ClaimToken: first.UpdatedAt},
	})
	if err != nil {
		t.Fatal(err)
	}
	if staged || state.doc.PendingContentRef != nil {
		t.Fatalf("old attempt staged after reclaim: staged=%v doc=%+v", staged, state.doc)
	}
}

func TestFeishuSyncReconcilesExpiredPendingSnapshotAndDeletesOnlySupersededObject(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	state := newSyncState("rev-1", "old-sum", "active.md")
	state.doc.SyncStatus = "syncing"
	state.doc.UpdatedAt = now.Add(-20 * time.Minute)
	state.doc.PendingContentRef, state.doc.PendingChecksum, state.doc.PendingRemoteRevision = ptr("orphan-pending.md"), ptr("orphan-sum"), ptr("rev-orphan")
	state.claimNow = now
	storage := newMemoryStorage()
	storage.objects["active.md"] = []byte("active")
	storage.objects["orphan-pending.md"] = []byte("orphan")
	queue := &fakeStagedIngestionQueue{}
	worker := NewFeishuSyncWorker(FeishuSyncWorkerDeps{
		Repository: state, Resolver: feishu.NewURLResolver(),
		Loaders: map[domain.ResourceType]ports.SourceLoader{domain.ResourceDocx: &fakeSourceLoader{document: canonicalDoc(t, "rev-2", "replacement")}},
		Tokens:  fakeTokenProvider{token: "token"}, Storage: storage, Ingestion: queue,
		Now: func() time.Time { return now }, SyncLease: 10 * time.Minute,
	})

	if err := worker.Work(context.Background(), syncJob(state.doc.ID, "rev-1")); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if queue.calls != 1 || state.doc.PendingContentRef == nil || *state.doc.PendingContentRef == "orphan-pending.md" {
		t.Fatalf("crash state not reconciled: queue=%d doc=%+v", queue.calls, state.doc)
	}
	if len(storage.deleted) != 1 || storage.deleted[0] != "orphan-pending.md" {
		t.Fatalf("deleted snapshots = %v, want only superseded pending", storage.deleted)
	}
	if _, ok := storage.objects["active.md"]; !ok {
		t.Fatal("active snapshot was deleted")
	}
}

func newTestFeishuWorker(t *testing.T, state *memorySyncRepository, canonical domain.CanonicalDocument, storage *memoryStorage, queue *fakeStagedIngestionQueue) *FeishuSyncWorker {
	t.Helper()
	return NewFeishuSyncWorker(FeishuSyncWorkerDeps{
		Repository: state, Resolver: feishu.NewURLResolver(),
		Loaders: map[domain.ResourceType]ports.SourceLoader{domain.ResourceDocx: &fakeSourceLoader{document: canonical}},
		Tokens:  fakeTokenProvider{token: "access-token"}, Storage: storage, Ingestion: queue,
		CitationStore: &fakeStagedVectorStore{state: state},
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
	tx         pgx.Tx
	onEnqueue  func()
}

func (f *fakeStagedIngestionQueue) EnqueueStagedIngestionTx(ctx context.Context, tx pgx.Tx, snapshot PendingFeishuSnapshot) error {
	f.tx = tx
	return f.EnqueueStagedIngestion(ctx, snapshot)
}

type syncRepoFakeBeginner struct{ tx pgx.Tx }

func (f *syncRepoFakeBeginner) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	return f.tx, nil
}

type syncRepoFakeTx struct {
	tag        pgconn.CommandTag
	ops        []string
	committed  bool
	rolledBack bool
	commitErr  error
}

func (t *syncRepoFakeTx) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("not implemented")
}
func (t *syncRepoFakeTx) Commit(context.Context) error {
	t.ops = append(t.ops, "commit")
	if t.commitErr == nil {
		t.committed = true
	}
	return t.commitErr
}
func (t *syncRepoFakeTx) Rollback(context.Context) error {
	if !t.committed {
		t.rolledBack = true
	}
	return nil
}
func (t *syncRepoFakeTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("not implemented")
}
func (t *syncRepoFakeTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (t *syncRepoFakeTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (t *syncRepoFakeTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("not implemented")
}
func (t *syncRepoFakeTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	t.ops = append(t.ops, "exec:"+sql)
	return t.tag, nil
}
func (t *syncRepoFakeTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}
func (t *syncRepoFakeTx) QueryRow(context.Context, string, ...any) pgx.Row { return nil }
func (t *syncRepoFakeTx) Conn() *pgx.Conn                                  { return nil }

func (f *fakeStagedIngestionQueue) EnqueueStagedIngestion(_ context.Context, snapshot PendingFeishuSnapshot) error {
	f.calls++
	if f.onEnqueue != nil {
		f.onEnqueue()
	}
	f.documentID, f.contentRef, f.checksum, f.revision = snapshot.DocumentID.String(), snapshot.ContentRef, snapshot.Checksum, snapshot.RemoteRevision
	return f.err
}

type memoryStorage struct {
	objects        map[string][]byte
	putErr         error
	putBeforeError bool
	deleteErr      error
	deleted        []string
}

const InitialRevisionForTest = "initial"

func newMemoryStorage() *memoryStorage { return &memoryStorage{objects: map[string][]byte{}} }
func (m *memoryStorage) Put(_ context.Context, key string, body io.Reader, _ int64, _ string) error {
	b, err := io.ReadAll(body)
	if err == nil && (m.putErr == nil || m.putBeforeError) {
		m.objects[key] = b
	}
	if m.putErr != nil {
		return m.putErr
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
	m.deleted = append(m.deleted, key)
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.objects, key)
	return nil
}

type ambiguousStageSyncRepository struct {
	*memorySyncRepository
	err error
}

type ambiguousSnapshotStageRepository struct {
	*memorySyncRepository
	err error
}

func (r *ambiguousSnapshotStageRepository) StageFeishuSnapshot(ctx context.Context, in StageFeishuSnapshotInput) (bool, error) {
	staged, err := r.memorySyncRepository.StageFeishuSnapshot(ctx, in)
	if err != nil || !staged {
		return staged, err
	}
	return false, r.err
}

func (r *ambiguousStageSyncRepository) StageFeishuSnapshotAndEnqueue(ctx context.Context, in StageFeishuSnapshotInput, _ StagedIngestionEnqueuer) (bool, error) {
	staged, err := r.StageFeishuSnapshot(ctx, in)
	if err != nil || !staged {
		return staged, err
	}
	return false, r.err
}

type memorySyncRepository struct {
	doc            generated.Document
	promoteErr     error
	failContextErr error
	claimNow       time.Time
	promoteCalls   int
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

func (m *memorySyncRepository) ClaimFeishuSync(_ context.Context, _ uuid.UUID, expectedRemoteRevision *string, staleBefore time.Time) (generated.Document, error) {
	if !sameString(m.doc.RemoteRevision, expectedRemoteRevision) || (m.doc.SyncStatus == "syncing" && !m.doc.UpdatedAt.Before(staleBefore)) {
		return generated.Document{}, pgx.ErrNoRows
	}
	m.doc.SyncStatus = "syncing"
	m.doc.LastSyncError = nil
	if !m.claimNow.IsZero() {
		m.doc.UpdatedAt = m.claimNow
	}
	return m.doc, nil
}
func (m *memorySyncRepository) CompleteUnchangedFeishuSync(_ context.Context, in FeishuSyncExpectation) (bool, error) {
	if m.doc.SyncStatus != "syncing" || m.doc.UpdatedAt != in.ClaimToken || !sameString(m.doc.RemoteRevision, in.RemoteRevision) || m.doc.Checksum != in.Checksum || !m.matchesExpectedPending(in.Pending) {
		return false, nil
	}
	m.doc.SyncStatus = "idle"
	m.doc.PendingContentRef, m.doc.PendingChecksum, m.doc.PendingRemoteRevision = nil, nil, nil
	m.doc.LastSyncedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	return true, nil
}
func (m *memorySyncRepository) StageFeishuSnapshot(_ context.Context, in StageFeishuSnapshotInput) (bool, error) {
	if m.doc.SyncStatus != "syncing" || m.doc.UpdatedAt != in.Expected.ClaimToken || !sameString(m.doc.RemoteRevision, in.Expected.RemoteRevision) || m.doc.Checksum != in.Expected.Checksum || !m.matchesExpectedPending(in.Expected.Pending) {
		return false, nil
	}
	m.doc.PendingContentRef, m.doc.PendingChecksum, m.doc.PendingRemoteRevision = ptr(in.Snapshot.ContentRef), ptr(in.Snapshot.Checksum), ptr(in.Snapshot.RemoteRevision)
	return true, nil
}

func (m *memorySyncRepository) StageFeishuSnapshotAndEnqueue(ctx context.Context, in StageFeishuSnapshotInput, enqueuer StagedIngestionEnqueuer) (bool, error) {
	before := m.doc
	staged, err := m.StageFeishuSnapshot(ctx, in)
	if err != nil || !staged {
		return staged, err
	}
	if err := enqueuer.EnqueueStagedIngestion(ctx, in.Snapshot); err != nil {
		m.doc = before
		return false, errors.Join(errSnapshotStageRolledBack, err)
	}
	return true, nil
}
func (m *memorySyncRepository) PromoteFeishuSnapshot(_ context.Context, in PendingFeishuSnapshot) (bool, error) {
	m.promoteCalls++
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
func (m *memorySyncRepository) FailFeishuSync(ctx context.Context, in FailFeishuSyncInput) (bool, error) {
	m.failContextErr = ctx.Err()
	if m.doc.SyncStatus != "syncing" || m.doc.UpdatedAt != in.ClaimToken || !m.matchesExpectedPending(in.Pending) {
		return false, nil
	}
	m.doc.SyncStatus, m.doc.LastSyncError = "failed", ptr(in.SafeError)
	if m.doc.ContentRef == nil {
		m.doc.Status = "failed"
	}
	return true, nil
}
func (m *memorySyncRepository) matchesPending(in PendingFeishuSnapshot) bool {
	return m.doc.SyncStatus == "syncing" && m.doc.UpdatedAt == in.ClaimToken && sameString(m.doc.PendingContentRef, &in.ContentRef) && sameString(m.doc.PendingChecksum, &in.Checksum) && sameString(m.doc.PendingRemoteRevision, &in.RemoteRevision)
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
