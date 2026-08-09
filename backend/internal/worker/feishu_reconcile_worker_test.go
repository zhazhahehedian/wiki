package worker

import (
	"context"
	"encoding/json"
	"errors"
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
	if len(queue.ingestions) != 1 {
		t.Fatalf("ingestion recovery = %+v", queue.ingestions)
	}
	got := queue.ingestions[0]
	if got.DocumentID != ingestDoc.ID || got.ContentRef != "feishu/snapshot.md" || got.Checksum != "sum-2" || got.RemoteRevision != "rev-2" || got.ClaimToken != ingestDoc.UpdatedAt {
		t.Fatalf("recovered snapshot guards = %+v", got)
	}
	if got.Title != "Sheet" || got.Bytes != 42 || string(got.Metadata) != string(ingestDoc.PendingMetadata) {
		t.Fatalf("recovered snapshot payload = %+v", got)
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
	if len(queue.syncs) != 2 || len(queue.ingestions) != 0 {
		t.Fatalf("fallback syncs/ingestions = %+v/%+v", queue.syncs, queue.ingestions)
	}
}

func TestFeishuReconcilerRedactsRecoveryEnqueueFailures(t *testing.T) {
	claim := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	validPending := generated.Document{ID: uuid.New(), SourceType: "feishu-docx", SyncStatus: "syncing", UpdatedAt: claim}
	setPendingDocument(&validPending, PendingFeishuSnapshot{
		ContentRef: "pending.md", Checksum: "sum", RemoteRevision: "rev-2", Title: "Title", Bytes: 4,
		Metadata: validReconcileMetadata(t),
	})
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
			if len(tt.queue.syncs)+len(tt.queue.ingestions) != tt.wantCalls {
				t.Fatalf("recovery calls = %d", len(tt.queue.syncs)+len(tt.queue.ingestions))
			}
		})
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
}

func (f *fakeReconcileRepository) ListStaleFeishuSyncs(_ context.Context, lease time.Duration, afterUpdatedAt time.Time, afterID uuid.UUID, batchSize int32) ([]generated.Document, error) {
	f.calls = append(f.calls, reconcileScanCall{lease: lease, afterUpdatedAt: afterUpdatedAt, afterID: afterID, batchSize: batchSize})
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
	syncErr      error
	ingestionErr error
}

func (q *recordingRecoveryQueue) EnqueueFeishuSync(_ context.Context, documentID, requestedRevision string) error {
	q.syncs = append(q.syncs, recoverySyncCall{documentID: documentID, revision: requestedRevision})
	return q.syncErr
}

func (q *recordingRecoveryQueue) EnqueueStagedIngestion(_ context.Context, snapshot PendingFeishuSnapshot) error {
	q.ingestions = append(q.ingestions, snapshot)
	return q.ingestionErr
}
