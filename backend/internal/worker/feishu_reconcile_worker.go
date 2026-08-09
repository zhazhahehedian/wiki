package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

const (
	defaultReconcileBatchSize  = 100
	defaultReconcileMaxBatches = 10
	maxReconcileBatchSize      = 500
	maxReconcileBatches        = 20
)

var (
	errFeishuReconcileScan   = errors.New("Feishu reconciliation scan failed")
	errFeishuRecoveryEnqueue = errors.New("Feishu recovery enqueue failed")
)

type feishuRecoveryType string

const (
	feishuRecoverySync         feishuRecoveryType = "sync"
	feishuRecoveryIngestion    feishuRecoveryType = "ingestion"
	feishuRecoveryMetadataOnly feishuRecoveryType = "metadata_only"
)

type FeishuReconcileError struct {
	Failures     int
	Sync         int
	Ingestion    int
	MetadataOnly int
}

func (e *FeishuReconcileError) Error() string {
	return fmt.Sprintf("Feishu reconciliation recovery failed (%d enqueue failures)", e.Failures)
}

func (e *FeishuReconcileError) Unwrap() error { return errFeishuRecoveryEnqueue }

func (e *FeishuReconcileError) add(recoveryType feishuRecoveryType) {
	e.Failures++
	switch recoveryType {
	case feishuRecoverySync:
		e.Sync++
	case feishuRecoveryIngestion:
		e.Ingestion++
	case feishuRecoveryMetadataOnly:
		e.MetadataOnly++
	}
}

func (e *FeishuReconcileError) result() error {
	if e.Failures == 0 {
		return nil
	}
	return e
}

type FeishuReconcileJobArgs struct{}

func (FeishuReconcileJobArgs) Kind() string { return "feishu_reconcile" }

func (FeishuReconcileJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 1,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRunning,
				rivertype.JobStateScheduled,
				rivertype.JobStateRetryable,
			},
		},
	}
}

type FeishuReconcileRepository interface {
	ListStaleFeishuSyncs(ctx context.Context, lease time.Duration, afterUpdatedAt time.Time, afterID uuid.UUID, batchSize int32) ([]generated.Document, error)
}

type FeishuRecoveryQueue interface {
	EnqueueFeishuSync(ctx context.Context, documentID, requestedRevision string) error
	EnqueueStagedIngestion(ctx context.Context, snapshot PendingFeishuSnapshot) error
	EnqueueMetadataOnlyIngestion(ctx context.Context, snapshot PendingFeishuSnapshot) error
}

type FeishuReconcileWorkerDeps struct {
	Repository FeishuReconcileRepository
	Queue      FeishuRecoveryQueue
	SyncLease  time.Duration
	JobTimeout time.Duration
	BatchSize  int
	MaxBatches int
	Logger     *slog.Logger
}

type FeishuReconcileWorker struct {
	river.WorkerDefaults[FeishuReconcileJobArgs]
	repository FeishuReconcileRepository
	queue      FeishuRecoveryQueue
	syncLease  time.Duration
	jobTimeout time.Duration
	batchSize  int32
	maxBatches int
	logger     *slog.Logger
}

func NewFeishuReconcileWorker(deps FeishuReconcileWorkerDeps) *FeishuReconcileWorker {
	batchSize := deps.BatchSize
	if batchSize <= 0 {
		batchSize = defaultReconcileBatchSize
	}
	if batchSize > maxReconcileBatchSize {
		batchSize = maxReconcileBatchSize
	}
	maxBatches := deps.MaxBatches
	if maxBatches <= 0 {
		maxBatches = defaultReconcileMaxBatches
	}
	if maxBatches > maxReconcileBatches {
		maxBatches = maxReconcileBatches
	}
	if deps.JobTimeout <= 0 {
		deps.JobTimeout = 2 * time.Minute
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	return &FeishuReconcileWorker{
		repository: deps.Repository, queue: deps.Queue, syncLease: deps.SyncLease,
		jobTimeout: deps.JobTimeout, batchSize: int32(batchSize), maxBatches: maxBatches, logger: deps.Logger,
	}
}

func (w *FeishuReconcileWorker) Timeout(*river.Job[FeishuReconcileJobArgs]) time.Duration {
	return w.jobTimeout
}

func (w *FeishuReconcileWorker) Work(ctx context.Context, _ *river.Job[FeishuReconcileJobArgs]) error {
	afterUpdatedAt, afterID := time.Time{}, uuid.Nil
	failures := &FeishuReconcileError{}
	for page := 0; page < w.maxBatches; page++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		documents, err := w.repository.ListStaleFeishuSyncs(ctx, w.syncLease, afterUpdatedAt, afterID, w.batchSize)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return errFeishuReconcileScan
		}
		for _, document := range documents {
			if err := ctx.Err(); err != nil {
				return err
			}
			recoveryType, err := w.recover(ctx, document)
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if err != nil {
				failures.add(recoveryType)
				w.logger.WarnContext(ctx, "Feishu reconciliation enqueue failed",
					"event", "feishu_recovery_enqueue_failed",
					"error_code", "recovery_enqueue_failed",
					"document_id", document.ID.String(),
					"recovery_type", string(recoveryType),
				)
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(documents) < int(w.batchSize) {
			return failures.result()
		}
		last := documents[len(documents)-1]
		afterUpdatedAt, afterID = last.UpdatedAt, last.ID
	}
	return failures.result()
}

func (w *FeishuReconcileWorker) recover(ctx context.Context, document generated.Document) (feishuRecoveryType, error) {
	if snapshot, ok := recoverableSnapshotFromDocument(document); ok {
		enqueue := w.queue.EnqueueStagedIngestion
		recoveryType := feishuRecoveryIngestion
		if snapshot.Checksum == document.Checksum {
			enqueue = w.queue.EnqueueMetadataOnlyIngestion
			recoveryType = feishuRecoveryMetadataOnly
		}
		if err := enqueue(ctx, snapshot); err != nil {
			return recoveryType, errFeishuRecoveryEnqueue
		}
		return recoveryType, nil
	}
	requestedRevision := "initial"
	if document.RemoteRevision != nil {
		requestedRevision = *document.RemoteRevision
	}
	if err := w.queue.EnqueueFeishuSync(ctx, document.ID.String(), requestedRevision); err != nil {
		return feishuRecoverySync, errFeishuRecoveryEnqueue
	}
	return feishuRecoverySync, nil
}

func recoverableSnapshotFromDocument(document generated.Document) (PendingFeishuSnapshot, bool) {
	snapshot := snapshotFromDocument(document)
	if snapshot == nil || snapshot.Bytes < 0 || validateRecoverableMetadata(snapshot.Metadata) != nil {
		return PendingFeishuSnapshot{}, false
	}
	return *snapshot, true
}

type recoverableMetadata struct {
	SourceType     string                  `json:"source_type"`
	SourceURL      string                  `json:"source_url"`
	SectionPath    string                  `json:"section_path"`
	SheetName      string                  `json:"sheet_name"`
	TableID        string                  `json:"table_id"`
	ViewID         string                  `json:"view_id"`
	SheetID        string                  `json:"sheet_id"`
	RowStart       int                     `json:"row_start"`
	RowEnd         int                     `json:"row_end"`
	RemoteRevision string                  `json:"remote_revision"`
	ImageURL       domain.SafeURL          `json:"image_url"`
	SourceLocator  domain.SafeURL          `json:"source_locator"`
	Locations      []domain.SourceLocation `json:"locations"`
}

func validateRecoverableMetadata(raw json.RawMessage) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var metadata recoverableMetadata
	if err := decoder.Decode(&metadata); err != nil {
		return errors.New("invalid pending metadata")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("invalid pending metadata")
	}
	resourceType := domain.ResourceType(strings.TrimPrefix(metadata.SourceType, "feishu-"))
	if metadata.SourceType != "feishu-"+string(resourceType) {
		return errors.New("invalid pending metadata")
	}
	sourceURL, err := domain.NewSafeURL(metadata.SourceURL)
	if err != nil || sourceURL.String() != metadata.SourceLocator.String() {
		return errors.New("invalid pending metadata")
	}
	_, err = domain.NewSourceMetadata(domain.SourceMetadataInput{
		SourceType: resourceType, SectionPath: metadata.SectionPath, SheetName: metadata.SheetName,
		TableID: metadata.TableID, ViewID: metadata.ViewID, SheetID: metadata.SheetID,
		RowStart: metadata.RowStart, RowEnd: metadata.RowEnd, RemoteRevision: metadata.RemoteRevision,
		ImageURL: metadata.ImageURL, SourceLocator: metadata.SourceLocator, Locations: metadata.Locations,
	})
	if err != nil {
		return errors.New("invalid pending metadata")
	}
	return nil
}
