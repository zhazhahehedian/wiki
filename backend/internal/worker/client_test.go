package worker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRiverConfigRegistersPeriodicFeishuReconciliation(t *testing.T) {
	reconcile := NewFeishuReconcileWorker(FeishuReconcileWorkerDeps{})
	config, err := newRiverConfig(RiverClientConfig{
		IngestionWorker:      NewIngestionWorker(WorkerDeps{}),
		FeishuSyncWorker:     NewFeishuSyncWorker(FeishuSyncWorkerDeps{}),
		ReconcileWorker:      reconcile,
		MaxWorkers:           4,
		ReconcileInterval:    5 * time.Minute,
		RescueStuckJobsAfter: 50 * time.Minute,
	})
	if err != nil {
		t.Fatalf("newRiverConfig() error = %v", err)
	}
	if config.Workers == nil || len(config.PeriodicJobs) != 1 {
		t.Fatalf("workers/periodic jobs = %v/%d", config.Workers, len(config.PeriodicJobs))
	}
	if config.RescueStuckJobsAfter != 50*time.Minute {
		t.Fatalf("RescueStuckJobsAfter = %s", config.RescueStuckJobsAfter)
	}
	registration := feishuReconcilePeriodicRegistration(5 * time.Minute)
	if registration.ID != "feishu_sync_reconciliation" || registration.Interval != 5*time.Minute || !registration.RunOnStart {
		t.Fatalf("periodic registration = %+v", registration)
	}
}

func TestRiverConfigOmitsFeishuPeriodicJobWhenIntegrationDisabled(t *testing.T) {
	config, err := newRiverConfig(RiverClientConfig{IngestionWorker: NewIngestionWorker(WorkerDeps{}), MaxWorkers: 2})
	if err != nil {
		t.Fatalf("newRiverConfig() error = %v", err)
	}
	if len(config.PeriodicJobs) != 0 {
		t.Fatalf("periodic jobs = %d, want 0", len(config.PeriodicJobs))
	}
}

func TestWorkersExposeConfiguredJobTimeouts(t *testing.T) {
	ingestion := NewIngestionWorker(WorkerDeps{JobTimeout: 17 * time.Minute})
	syncWorker := NewFeishuSyncWorker(FeishuSyncWorkerDeps{JobTimeout: 11 * time.Minute})
	reconcile := NewFeishuReconcileWorker(FeishuReconcileWorkerDeps{JobTimeout: 3 * time.Minute})

	if got := ingestion.Timeout(nil); got != 17*time.Minute {
		t.Fatalf("ingestion Timeout() = %s", got)
	}
	if got := syncWorker.Timeout(nil); got != 11*time.Minute {
		t.Fatalf("sync Timeout() = %s", got)
	}
	if got := reconcile.Timeout(nil); got != 3*time.Minute {
		t.Fatalf("reconcile Timeout() = %s", got)
	}
}

func TestStagedIngestionEnqueuerFuncsForwardsReconciliationCalls(t *testing.T) {
	var gotDocumentID, gotRevision string
	var gotSnapshot, gotMetadataOnly PendingFeishuSnapshot
	forwarder := StagedIngestionEnqueuerFuncs{
		EnqueueFunc: func(_ context.Context, snapshot PendingFeishuSnapshot) error {
			gotSnapshot = snapshot
			return nil
		},
		EnqueueFeishuSyncFunc: func(_ context.Context, documentID, revision string) error {
			gotDocumentID, gotRevision = documentID, revision
			return nil
		},
		EnqueueMetadataOnlyFunc: func(_ context.Context, snapshot PendingFeishuSnapshot) error {
			gotMetadataOnly = snapshot
			return nil
		},
	}
	var queue FeishuRecoveryQueue = forwarder
	wantSnapshot := PendingFeishuSnapshot{DocumentID: uuid.New(), ContentRef: "pending.md"}
	if err := queue.EnqueueFeishuSync(context.Background(), "doc-id", "rev-2"); err != nil {
		t.Fatal(err)
	}
	if err := queue.EnqueueStagedIngestion(context.Background(), wantSnapshot); err != nil {
		t.Fatal(err)
	}
	if err := queue.EnqueueMetadataOnlyIngestion(context.Background(), wantSnapshot); err != nil {
		t.Fatal(err)
	}
	if gotDocumentID != "doc-id" || gotRevision != "rev-2" || gotSnapshot.DocumentID != wantSnapshot.DocumentID || gotMetadataOnly.DocumentID != wantSnapshot.DocumentID {
		t.Fatalf("forwarded sync/snapshots = %q/%q/%+v/%+v", gotDocumentID, gotRevision, gotSnapshot, gotMetadataOnly)
	}
}

func TestMetadataOnlyIngestionArgsPreserveGuardsAndDistinctRecoveryMode(t *testing.T) {
	snapshot := PendingFeishuSnapshot{
		DocumentID: uuid.New(), ContentRef: "pending.md", Checksum: "same-sum", RemoteRevision: "rev-2",
		Title: "Title", Bytes: 42, Metadata: []byte(`{"source_type":"feishu-docx"}`),
		ClaimToken: time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC),
	}
	normal := ingestionJobArgs(snapshot)
	metadataOnly := metadataOnlyIngestionJobArgs(snapshot)

	if normal.MetadataOnly || !metadataOnly.MetadataOnly {
		t.Fatalf("recovery modes = normal:%v metadata-only:%v", normal.MetadataOnly, metadataOnly.MetadataOnly)
	}
	if metadataOnly.DocumentID != normal.DocumentID || metadataOnly.PendingContentRef != normal.PendingContentRef ||
		metadataOnly.PendingChecksum != normal.PendingChecksum || metadataOnly.PendingRemoteRevision != normal.PendingRemoteRevision ||
		!metadataOnly.ClaimToken.Equal(normal.ClaimToken) {
		t.Fatalf("metadata-only guards differ: normal=%+v metadata-only=%+v", normal, metadataOnly)
	}
	if !metadataOnly.InsertOpts().UniqueOpts.ByArgs {
		t.Fatalf("metadata-only uniqueness = %+v", metadataOnly.InsertOpts().UniqueOpts)
	}
	normalJSON, err := json.Marshal(normal)
	if err != nil {
		t.Fatal(err)
	}
	metadataOnlyJSON, err := json.Marshal(metadataOnly)
	if err != nil {
		t.Fatal(err)
	}
	repeatedJSON, err := json.Marshal(metadataOnlyIngestionJobArgs(snapshot))
	if err != nil {
		t.Fatal(err)
	}
	if string(normalJSON) == string(metadataOnlyJSON) || string(metadataOnlyJSON) != string(repeatedJSON) {
		t.Fatalf("serialized recovery args = normal:%s metadata-only:%s repeated:%s", normalJSON, metadataOnlyJSON, repeatedJSON)
	}
}
