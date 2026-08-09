package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

type Client struct {
	rc *river.Client[pgx.Tx]
}

type RiverClientConfig struct {
	IngestionWorker      *IngestionWorker
	FeishuSyncWorker     *FeishuSyncWorker
	ReconcileWorker      *FeishuReconcileWorker
	MaxWorkers           int
	ReconcileInterval    time.Duration
	RescueStuckJobsAfter time.Duration
}

type periodicRegistration struct {
	ID         string
	Interval   time.Duration
	RunOnStart bool
}

func NewClient(ctx context.Context, pool *pgxpool.Pool, w *IngestionWorker, maxWorkers int, syncWorkers ...*FeishuSyncWorker) (*Client, error) {
	config := RiverClientConfig{IngestionWorker: w, MaxWorkers: maxWorkers}
	if len(syncWorkers) > 0 {
		config.FeishuSyncWorker = syncWorkers[0]
	}
	return NewConfiguredClient(ctx, pool, config)
}

func NewConfiguredClient(_ context.Context, pool *pgxpool.Pool, config RiverClientConfig) (*Client, error) {
	riverConfig, err := newRiverConfig(config)
	if err != nil {
		return nil, err
	}
	rc, err := river.NewClient(riverpgxv5.New(pool), riverConfig)
	if err != nil {
		return nil, fmt.Errorf("new river client: %w", err)
	}
	return &Client{rc: rc}, nil
}

func newRiverConfig(config RiverClientConfig) (*river.Config, error) {
	if config.IngestionWorker == nil {
		return nil, errors.New("ingestion worker is required")
	}
	if config.MaxWorkers < 1 {
		return nil, errors.New("River max workers must be positive")
	}
	workers := river.NewWorkers()
	if err := river.AddWorkerSafely(workers, config.IngestionWorker); err != nil {
		return nil, fmt.Errorf("add worker: %w", err)
	}
	if config.FeishuSyncWorker != nil {
		if err := river.AddWorkerSafely(workers, config.FeishuSyncWorker); err != nil {
			return nil, fmt.Errorf("add Feishu sync worker: %w", err)
		}
	}
	periodicJobs := make([]*river.PeriodicJob, 0, 1)
	if config.ReconcileWorker != nil {
		if config.FeishuSyncWorker == nil || config.ReconcileInterval <= 0 {
			return nil, errors.New("Feishu reconciliation configuration is incomplete")
		}
		if err := river.AddWorkerSafely(workers, config.ReconcileWorker); err != nil {
			return nil, fmt.Errorf("add Feishu reconcile worker: %w", err)
		}
		registration := feishuReconcilePeriodicRegistration(config.ReconcileInterval)
		periodicJobs = append(periodicJobs, river.NewPeriodicJob(
			river.PeriodicInterval(registration.Interval),
			func() (river.JobArgs, *river.InsertOpts) {
				args := FeishuReconcileJobArgs{}
				opts := args.InsertOpts()
				return args, &opts
			},
			&river.PeriodicJobOpts{ID: registration.ID, RunOnStart: registration.RunOnStart},
		))
	}
	return &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: config.MaxWorkers},
		},
		Workers: workers, PeriodicJobs: periodicJobs, RescueStuckJobsAfter: config.RescueStuckJobsAfter,
	}, nil
}

func feishuReconcilePeriodicRegistration(interval time.Duration) periodicRegistration {
	return periodicRegistration{ID: "feishu_sync_reconciliation", Interval: interval, RunOnStart: true}
}

func (c *Client) Start(ctx context.Context) error {
	return c.rc.Start(ctx)
}

func (c *Client) Stop(ctx context.Context) error {
	return c.rc.Stop(ctx)
}

func (c *Client) EnqueueIngestion(ctx context.Context, docID string) error {
	_, err := c.rc.Insert(ctx, IngestionJobArgs{DocumentID: docID}, nil)
	if err != nil {
		return fmt.Errorf("insert ingestion job: %w", err)
	}
	return nil
}

func (c *Client) EnqueueStagedIngestion(ctx context.Context, snapshot PendingFeishuSnapshot) error {
	_, err := c.rc.Insert(ctx, ingestionJobArgs(snapshot), nil)
	if err != nil {
		return fmt.Errorf("insert staged ingestion job: %w", err)
	}
	return nil
}

func (c *Client) EnqueueMetadataOnlyIngestion(ctx context.Context, snapshot PendingFeishuSnapshot) error {
	_, err := c.rc.Insert(ctx, metadataOnlyIngestionJobArgs(snapshot), nil)
	if err != nil {
		return fmt.Errorf("insert metadata-only ingestion job: %w", err)
	}
	return nil
}

func (c *Client) EnqueueStagedIngestionTx(ctx context.Context, tx pgx.Tx, snapshot PendingFeishuSnapshot) error {
	_, err := c.rc.InsertTx(ctx, tx, ingestionJobArgs(snapshot), nil)
	if err != nil {
		return fmt.Errorf("insert staged ingestion job in transaction: %w", err)
	}
	return nil
}

func ingestionJobArgs(snapshot PendingFeishuSnapshot) IngestionJobArgs {
	return IngestionJobArgs{
		DocumentID: snapshot.DocumentID.String(), PendingContentRef: snapshot.ContentRef,
		PendingChecksum: snapshot.Checksum, PendingRemoteRevision: snapshot.RemoteRevision,
		Title: snapshot.Title, Bytes: snapshot.Bytes, Metadata: snapshot.Metadata, ClaimToken: snapshot.ClaimToken,
	}
}

func metadataOnlyIngestionJobArgs(snapshot PendingFeishuSnapshot) IngestionJobArgs {
	args := ingestionJobArgs(snapshot)
	args.MetadataOnly = true
	return args
}

func (c *Client) EnqueueFeishuSync(ctx context.Context, documentID, requestedRevision string) error {
	_, err := c.rc.Insert(ctx, FeishuSyncJobArgs{DocumentID: documentID, RequestedRevision: requestedRevision}, nil)
	if err != nil {
		return fmt.Errorf("insert Feishu sync job: %w", err)
	}
	return nil
}

func (c *Client) EnqueueFeishuSyncTx(ctx context.Context, tx pgx.Tx, documentID, requestedRevision string) error {
	_, err := c.rc.InsertTx(ctx, tx, FeishuSyncJobArgs{DocumentID: documentID, RequestedRevision: requestedRevision}, nil)
	if err != nil {
		return fmt.Errorf("insert Feishu sync job in transaction: %w", err)
	}
	return nil
}
