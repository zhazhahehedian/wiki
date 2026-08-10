package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

func TestFeishuReconcileJobIsSingletonAndSingleAttempt(t *testing.T) {
	opts := (FeishuReconcileJobArgs{}).InsertOpts()
	if opts.MaxAttempts != 1 || !opts.UniqueOpts.ByArgs {
		t.Fatalf("InsertOpts() = %+v", opts)
	}
	wantStates := []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning,
		rivertype.JobStateScheduled, rivertype.JobStateRetryable,
	}
	if len(opts.UniqueOpts.ByState) != len(wantStates) {
		t.Fatalf("unique states = %v", opts.UniqueOpts.ByState)
	}
	for i := range wantStates {
		if opts.UniqueOpts.ByState[i] != wantStates[i] {
			t.Fatalf("unique states = %v, want %v", opts.UniqueOpts.ByState, wantStates)
		}
	}
}

func TestFeishuReconcilerReenqueuesLostSyncAndIngestion(t *testing.T) {
	claim := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	syncDoc := generated.Document{ID: uuid.New(), SourceType: "feishu-docx", SyncStatus: "syncing", RemoteRevision: ptr("rev-1"), UpdatedAt: claim}
	ingestDoc := generated.Document{ID: uuid.New(), SourceType: "feishu-sheet", SyncStatus: "syncing", UpdatedAt: claim.Add(time.Second)}
	setPendingDocument(&ingestDoc, PendingFeishuSnapshot{
		ContentRef: "feishu/snapshot.md", Checksum: "sum-2", RemoteRevision: "rev-2",
		Title: "Sheet", Bytes: 42, Metadata: validReconcileMetadata(t),
	})
	repo := &fakeReconcileRepository{documents: []generated.Document{syncDoc, ingestDoc}}
	queue := &recordingRecoveryQueue{}
	worker := NewFeishuReconcileWorker(FeishuReconcileWorkerDeps{
		Repository: repo, Queue: queue, SyncLease: 45 * time.Minute, BatchSize: 10, MaxBatches: 2,
	})

	if err := worker.Work(context.Background(), &river.Job[FeishuReconcileJobArgs]{}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if len(queue.syncs) != 1 || queue.syncs[0].documentID != syncDoc.ID.String() || queue.syncs[0].revision != "rev-1" {
		t.Fatalf("sync recovery = %+v", queue.syncs)
	}
	if len(queue.ingestions) != 1 || len(queue.metadataOnly) != 0 {
		t.Fatalf("ingestion/metadata-only recovery = %+v/%+v", queue.ingestions, queue.metadataOnly)
	}
	got := queue.ingestions[0]
	if got.DocumentID != ingestDoc.ID || got.ContentRef != "feishu/snapshot.md" || got.Checksum != "sum-2" || got.RemoteRevision != "rev-2" || got.ClaimToken != ingestDoc.UpdatedAt {
		t.Fatalf("recovered snapshot guards = %+v", got)
	}
	if got.Title != "Sheet" || got.Bytes != 42 || string(got.Metadata) != string(ingestDoc.PendingMetadata) {
		t.Fatalf("recovered snapshot payload = %+v", got)
	}
}

func TestFeishuReconcilerReenqueuesSameChecksumPendingAsMetadataOnly(t *testing.T) {
	claim := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	document := generated.Document{
		ID: uuid.New(), SourceType: "feishu-docx", SyncStatus: "syncing", UpdatedAt: claim, Checksum: "same-sum",
	}
	setPendingDocument(&document, PendingFeishuSnapshot{
		ContentRef: "feishu/snapshot.md", Checksum: "same-sum", RemoteRevision: "rev-2",
		Title: "Updated title", Bytes: 42, Metadata: validReconcileMetadata(t),
	})
	queue := &recordingRecoveryQueue{}
	worker := NewFeishuReconcileWorker(FeishuReconcileWorkerDeps{
		Repository: &fakeReconcileRepository{documents: []generated.Document{document}},
		Queue:      queue, SyncLease: time.Hour, BatchSize: 10, MaxBatches: 1,
	})

	if err := worker.Work(context.Background(), &river.Job[FeishuReconcileJobArgs]{}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if len(queue.metadataOnly) != 1 || len(queue.ingestions) != 0 || len(queue.syncs) != 0 {
		t.Fatalf("metadata-only/ingestion/sync recovery = %+v/%+v/%+v", queue.metadataOnly, queue.ingestions, queue.syncs)
	}
	if got := queue.metadataOnly[0]; got.DocumentID != document.ID || got.Checksum != document.Checksum || got.ClaimToken != claim {
		t.Fatalf("metadata-only snapshot guards = %+v", got)
	}
}

func TestFeishuReconcilerFallsBackToFreshSyncForIncompleteOrInvalidPending(t *testing.T) {
	incomplete := generated.Document{
		ID: uuid.New(), SourceType: "feishu-docx", SyncStatus: "syncing", UpdatedAt: time.Now(),
		RemoteRevision: ptr("rev-1"), PendingContentRef: ptr("partial.md"),
	}
	invalid := generated.Document{ID: uuid.New(), SourceType: "feishu-sheet", SyncStatus: "syncing", UpdatedAt: time.Now().Add(time.Second), RemoteRevision: ptr("rev-2")}
	setPendingDocument(&invalid, PendingFeishuSnapshot{
		ContentRef: "invalid.md", Checksum: "sum", RemoteRevision: "rev-3", Title: "Invalid", Bytes: -1,
		Metadata: json.RawMessage(`{"source_type":"feishu-sheet","unknown_secret":"must-not-pass"}`),
	})
	queue := &recordingRecoveryQueue{}
	worker := NewFeishuReconcileWorker(FeishuReconcileWorkerDeps{
		Repository: &fakeReconcileRepository{documents: []generated.Document{incomplete, invalid}},
		Queue:      queue, SyncLease: time.Hour, BatchSize: 10, MaxBatches: 1,
	})

	if err := worker.Work(context.Background(), &river.Job[FeishuReconcileJobArgs]{}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if len(queue.syncs) != 2 || len(queue.ingestions) != 0 || len(queue.metadataOnly) != 0 {
		t.Fatalf("fallback syncs/ingestions/metadata-only = %+v/%+v/%+v", queue.syncs, queue.ingestions, queue.metadataOnly)
	}
}

func TestFeishuReconcilerRedactsRecoveryEnqueueFailures(t *testing.T) {
	claim := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	validPending := generated.Document{ID: uuid.New(), SourceType: "feishu-docx", SyncStatus: "syncing", UpdatedAt: claim}
	setPendingDocument(&validPending, PendingFeishuSnapshot{
		ContentRef: "pending.md", Checksum: "sum", RemoteRevision: "rev-2", Title: "Title", Bytes: 4,
		Metadata: validReconcileMetadata(t),
	})
	metadataOnlyPending := validPending
	metadataOnlyPending.Checksum = "sum"
	tests := []struct {
		name      string
		document  generated.Document
		queue     *recordingRecoveryQueue
		wantCalls int
	}{
		{
			name: "sync", document: generated.Document{ID: uuid.New(), SourceType: "feishu-docx", SyncStatus: "syncing", UpdatedAt: claim},
			queue: &recordingRecoveryQueue{syncErr: errors.New("database SECRET detail")}, wantCalls: 1,
		},
		{
			name: "ingestion", document: validPending,
			queue: &recordingRecoveryQueue{ingestionErr: errors.New("provider SECRET body")}, wantCalls: 1,
		},
		{
			name: "metadata-only ingestion", document: metadataOnlyPending,
			queue: &recordingRecoveryQueue{ingestionErr: errors.New("provider SECRET body")}, wantCalls: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker := NewFeishuReconcileWorker(FeishuReconcileWorkerDeps{
				Repository: &fakeReconcileRepository{documents: []generated.Document{tt.document}},
				Queue:      tt.queue, SyncLease: time.Hour, BatchSize: 10, MaxBatches: 1,
			})

			err := worker.Work(context.Background(), &river.Job[FeishuReconcileJobArgs]{})
			if !errors.Is(err, errFeishuRecoveryEnqueue) || strings.Contains(err.Error(), "SECRET") {
				t.Fatalf("Work() error = %v", err)
			}
			if len(tt.queue.syncs)+len(tt.queue.ingestions)+len(tt.queue.metadataOnly) != tt.wantCalls {
				t.Fatalf("recovery calls = %d", len(tt.queue.syncs)+len(tt.queue.ingestions)+len(tt.queue.metadataOnly))
			}
		})
	}
}

func TestFeishuReconcilerContinuesAfterEnqueueFailureAcrossPages(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	secretURL := "https://secret.example/docx/provider-token"
	documents := []generated.Document{
		{ID: uuid.New(), SourceType: "feishu-docx", SourceUrl: &secretURL, SyncStatus: "syncing", UpdatedAt: base},
		{ID: uuid.New(), SourceType: "feishu-docx", SyncStatus: "syncing", UpdatedAt: base.Add(time.Second)},
		{ID: uuid.New(), SourceType: "feishu-docx", SyncStatus: "syncing", UpdatedAt: base.Add(2 * time.Second)},
	}
	queue := &recordingRecoveryQueue{
		syncErrors: map[string]error{documents[0].ID.String(): errors.New("oldest row SECRET failure")},
	}
	var logOutput bytes.Buffer
	worker := NewFeishuReconcileWorker(FeishuReconcileWorkerDeps{
		Repository: &fakeReconcileRepository{documents: documents}, Queue: queue,
		SyncLease: time.Hour, BatchSize: 2, MaxBatches: 2,
		Logger: slog.New(slog.NewJSONHandler(&logOutput, nil)),
	})

	err := worker.Work(context.Background(), &river.Job[FeishuReconcileJobArgs]{})
	if !errors.Is(err, errFeishuRecoveryEnqueue) || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("Work() error = %v", err)
	}
	if len(queue.syncs) != len(documents) {
		t.Fatalf("sync recovery calls = %+v, want all %d documents", queue.syncs, len(documents))
	}
	var aggregate *FeishuReconcileError
	if !errors.As(err, &aggregate) || aggregate.Failures != 1 || aggregate.Sync != 1 || aggregate.Ingestion != 0 || aggregate.MetadataOnly != 0 {
		t.Fatalf("aggregate error = %#v", err)
	}
	var warning map[string]any
	if decodeErr := json.Unmarshal(logOutput.Bytes(), &warning); decodeErr != nil {
		t.Fatalf("decode warning = %v; output=%q", decodeErr, logOutput.String())
	}
	allowed := map[string]bool{
		"time": true, "level": true, "msg": true, "event": true, "error_code": true,
		"document_id": true, "recovery_type": true,
	}
	for field := range warning {
		if !allowed[field] {
			t.Fatalf("unexpected warning field %q in %+v", field, warning)
		}
	}
	if warning["level"] != "WARN" || warning["msg"] != "Feishu reconciliation enqueue failed" ||
		warning["event"] != "feishu_recovery_enqueue_failed" || warning["error_code"] != "recovery_enqueue_failed" ||
		warning["document_id"] != documents[0].ID.String() || warning["recovery_type"] != "sync" {
		t.Fatalf("reconciliation warning = %+v", warning)
	}
	for _, secret := range []string{"SECRET", secretURL, "secret.example", "provider-token", "oldest row"} {
		if strings.Contains(logOutput.String(), secret) || strings.Contains(err.Error(), secret) {
			t.Fatalf("reconciliation result leaked %q: log=%s error=%v", secret, logOutput.String(), err)
		}
	}
}

func TestFeishuReconcilerStopsImmediatelyWhenEnqueueCancelsContext(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	documents := []generated.Document{
		{ID: uuid.New(), SourceType: "feishu-docx", SyncStatus: "syncing", UpdatedAt: base},
		{ID: uuid.New(), SourceType: "feishu-docx", SyncStatus: "syncing", UpdatedAt: base.Add(time.Second)},
	}
	ctx, cancel := context.WithCancel(context.Background())
	queue := &recordingRecoveryQueue{
		syncErrors: map[string]error{documents[0].ID.String(): errors.New("provider failure")},
		onSync: func(documentID string) {
			if documentID == documents[0].ID.String() {
				cancel()
			}
		},
	}
	worker := NewFeishuReconcileWorker(FeishuReconcileWorkerDeps{
		Repository: &fakeReconcileRepository{documents: documents}, Queue: queue,
		SyncLease: time.Hour, BatchSize: 10, MaxBatches: 1,
	})

	err := worker.Work(ctx, &river.Job[FeishuReconcileJobArgs]{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Work() error = %v, want context cancellation", err)
	}
	if len(queue.syncs) != 1 {
		t.Fatalf("sync calls after cancellation = %+v", queue.syncs)
	}
}

func TestFeishuReconcilerStopsImmediatelyOnFatalScanError(t *testing.T) {
	repo := &fakeReconcileRepository{err: errors.New("database SECRET detail")}
	queue := &recordingRecoveryQueue{}
	worker := NewFeishuReconcileWorker(FeishuReconcileWorkerDeps{
		Repository: repo, Queue: queue, SyncLease: time.Hour, BatchSize: 10, MaxBatches: 1,
	})

	err := worker.Work(context.Background(), &river.Job[FeishuReconcileJobArgs]{})
	if !errors.Is(err, errFeishuReconcileScan) || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("Work() error = %v", err)
	}
	if len(repo.calls) != 1 || len(queue.syncs)+len(queue.ingestions)+len(queue.metadataOnly) != 0 {
		t.Fatalf("fatal scan continued: scans=%d queue=%+v/%+v/%+v", len(repo.calls), queue.syncs, queue.ingestions, queue.metadataOnly)
	}
}

func TestFeishuReconcilerAggregatesBoundedFailureCategories(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	syncDocument := generated.Document{ID: uuid.New(), SourceType: "feishu-docx", SyncStatus: "syncing", UpdatedAt: base}
	ingestionDocument := generated.Document{ID: uuid.New(), SourceType: "feishu-docx", SyncStatus: "syncing", UpdatedAt: base.Add(time.Second), Checksum: "old-sum"}
	setPendingDocument(&ingestionDocument, PendingFeishuSnapshot{
		ContentRef: "changed.md", Checksum: "new-sum", RemoteRevision: "rev-2", Title: "Changed", Bytes: 7,
		Metadata: validReconcileMetadata(t),
	})
	metadataOnlyDocument := generated.Document{ID: uuid.New(), SourceType: "feishu-docx", SyncStatus: "syncing", UpdatedAt: base.Add(2 * time.Second), Checksum: "same-sum"}
	setPendingDocument(&metadataOnlyDocument, PendingFeishuSnapshot{
		ContentRef: "same.md", Checksum: "same-sum", RemoteRevision: "rev-3", Title: "Metadata", Bytes: 7,
		Metadata: validReconcileMetadata(t),
	})
	queue := &recordingRecoveryQueue{
		syncErr: errors.New("sync failure"), ingestionErr: errors.New("ingestion failure"),
	}
	worker := NewFeishuReconcileWorker(FeishuReconcileWorkerDeps{
		Repository: &fakeReconcileRepository{documents: []generated.Document{syncDocument, ingestionDocument, metadataOnlyDocument}},
		Queue:      queue, SyncLease: time.Hour, BatchSize: 10, MaxBatches: 1,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	err := worker.Work(context.Background(), &river.Job[FeishuReconcileJobArgs]{})
	var aggregate *FeishuReconcileError
	if !errors.As(err, &aggregate) || aggregate.Failures != 3 || aggregate.Sync != 1 || aggregate.Ingestion != 1 || aggregate.MetadataOnly != 1 {
		t.Fatalf("aggregate error = %#v", err)
	}
	if len(queue.syncs) != 1 || len(queue.ingestions) != 1 || len(queue.metadataOnly) != 1 {
		t.Fatalf("recovery calls = sync:%+v ingestion:%+v metadata-only:%+v", queue.syncs, queue.ingestions, queue.metadataOnly)
	}
}

func TestFeishuReconcilerBoundsBatchAndKeysetPages(t *testing.T) {
	documents := make([]generated.Document, 7)
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	for i := range documents {
		documents[i] = generated.Document{ID: uuid.New(), SourceType: "feishu-docx", SyncStatus: "syncing", UpdatedAt: base.Add(time.Duration(i) * time.Second)}
	}
	repo := &fakeReconcileRepository{documents: documents}
	queue := &recordingRecoveryQueue{}
	worker := NewFeishuReconcileWorker(FeishuReconcileWorkerDeps{
		Repository: repo, Queue: queue, SyncLease: time.Hour, BatchSize: 2, MaxBatches: 3,
	})

	if err := worker.Work(context.Background(), &river.Job[FeishuReconcileJobArgs]{}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if len(repo.calls) != 3 || len(queue.syncs) != 6 {
		t.Fatalf("scan calls/recovered = %d/%d, want 3/6", len(repo.calls), len(queue.syncs))
	}
	for _, call := range repo.calls {
		if call.batchSize != 2 || call.lease != time.Hour {
			t.Fatalf("scan call = %+v", call)
		}
	}
	if repo.calls[1].afterID != documents[1].ID || repo.calls[2].afterID != documents[3].ID {
		t.Fatalf("keyset cursors = %+v", repo.calls)
	}
}

func validReconcileMetadata(t *testing.T) json.RawMessage {
	t.Helper()
	return mustCanonicalMetadata(t, canonicalDoc(t, "rev-2", "body"))
}

func mustCanonicalMetadata(t *testing.T, document domain.CanonicalDocument) json.RawMessage {
	t.Helper()
	metadata, err := canonicalMetadata(document)
	if err != nil {
		t.Fatal(err)
	}
	return metadata
}

type reconcileScanCall struct {
	lease          time.Duration
	afterUpdatedAt time.Time
	afterID        uuid.UUID
	batchSize      int32
}

type fakeReconcileRepository struct {
	documents []generated.Document
	calls     []reconcileScanCall
	err       error
}

func (f *fakeReconcileRepository) ListStaleFeishuSyncs(_ context.Context, lease time.Duration, afterUpdatedAt time.Time, afterID uuid.UUID, batchSize int32) ([]generated.Document, error) {
	f.calls = append(f.calls, reconcileScanCall{lease: lease, afterUpdatedAt: afterUpdatedAt, afterID: afterID, batchSize: batchSize})
	if f.err != nil {
		return nil, f.err
	}
	start := 0
	if !afterUpdatedAt.IsZero() || afterID != uuid.Nil {
		for start < len(f.documents) && (f.documents[start].UpdatedAt.Before(afterUpdatedAt) || f.documents[start].UpdatedAt.Equal(afterUpdatedAt) && f.documents[start].ID.String() <= afterID.String()) {
			start++
		}
	}
	end := start + int(batchSize)
	if end > len(f.documents) {
		end = len(f.documents)
	}
	return append([]generated.Document(nil), f.documents[start:end]...), nil
}

type recoverySyncCall struct {
	documentID string
	revision   string
}

type recordingRecoveryQueue struct {
	syncs        []recoverySyncCall
	ingestions   []PendingFeishuSnapshot
	metadataOnly []PendingFeishuSnapshot
	syncErr      error
	syncErrors   map[string]error
	ingestionErr error
	onSync       func(string)
}

func (q *recordingRecoveryQueue) EnqueueFeishuSync(_ context.Context, documentID, requestedRevision string) error {
	q.syncs = append(q.syncs, recoverySyncCall{documentID: documentID, revision: requestedRevision})
	if q.onSync != nil {
		q.onSync(documentID)
	}
	if err := q.syncErrors[documentID]; err != nil {
		return err
	}
	return q.syncErr
}

func (q *recordingRecoveryQueue) EnqueueStagedIngestion(_ context.Context, snapshot PendingFeishuSnapshot) error {
	q.ingestions = append(q.ingestions, snapshot)
	return q.ingestionErr
}

func (q *recordingRecoveryQueue) EnqueueMetadataOnlyIngestion(_ context.Context, snapshot PendingFeishuSnapshot) error {
	q.metadataOnly = append(q.metadataOnly, snapshot)
	return q.ingestionErr
}
