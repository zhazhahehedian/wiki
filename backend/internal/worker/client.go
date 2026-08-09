package worker

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

type Client struct {
	rc *river.Client[pgx.Tx]
}

func NewClient(ctx context.Context, pool *pgxpool.Pool, w *IngestionWorker, maxWorkers int, syncWorkers ...*FeishuSyncWorker) (*Client, error) {
	workers := river.NewWorkers()
	if err := river.AddWorkerSafely(workers, w); err != nil {
		return nil, fmt.Errorf("add worker: %w", err)
	}
	for _, syncWorker := range syncWorkers {
		if syncWorker == nil {
			continue
		}
		if err := river.AddWorkerSafely(workers, syncWorker); err != nil {
			return nil, fmt.Errorf("add Feishu sync worker: %w", err)
		}
	}

	rc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: maxWorkers},
		},
		Workers: workers,
	})
	if err != nil {
		return nil, fmt.Errorf("new river client: %w", err)
	}

	return &Client{rc: rc}, nil
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
	_, err := c.rc.Insert(ctx, IngestionJobArgs{
		DocumentID: snapshot.DocumentID.String(), PendingContentRef: snapshot.ContentRef,
		PendingChecksum: snapshot.Checksum, PendingRemoteRevision: snapshot.RemoteRevision,
		Title: snapshot.Title, Bytes: snapshot.Bytes, Metadata: snapshot.Metadata,
	}, nil)
	if err != nil {
		return fmt.Errorf("insert staged ingestion job: %w", err)
	}
	return nil
}

func (c *Client) EnqueueFeishuSync(ctx context.Context, documentID, requestedRevision string) error {
	_, err := c.rc.Insert(ctx, FeishuSyncJobArgs{DocumentID: documentID, RequestedRevision: requestedRevision}, nil)
	if err != nil {
		return fmt.Errorf("insert Feishu sync job: %w", err)
	}
	return nil
}
