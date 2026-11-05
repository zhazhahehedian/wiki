package main

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/zenith-wang/it-wiki/backend/internal/config"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/feishu"
	"github.com/zenith-wang/it-wiki/backend/internal/notify"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
	"github.com/zenith-wang/it-wiki/backend/internal/repo"
)

// buildNotifier wires governance events to Feishu bot messages. It returns nil
// when notifications are disabled, leaving governance transitions unchanged.
func buildNotifier(cfg *config.Config, pool *pgxpool.Pool, registryRepo *repo.RegistryRepository) (*river.Client[pgx.Tx], error) {
	if !cfg.FeishuNotify {
		return nil, nil
	}
	return newNotifier(pool, registryRepo, cfg.FrontendOrigin, feishu.NewBotClient("", cfg.FeishuAppID, cfg.FeishuAppSecret, nil))
}

func newNotifier(pool *pgxpool.Pool, registryRepo *repo.RegistryRepository, origin string, sender notify.Sender) (*river.Client[pgx.Tx], error) {
	var rc *river.Client[pgx.Tx]
	registryRepo.WithEvents(func(ctx context.Context, tx pgx.Tx, event registry.Event) error {
		_, err := rc.InsertTx(ctx, tx, notify.EventJob{Event: event}, nil)
		return err
	})
	workers := river.NewWorkers()
	river.AddWorker(workers, &notify.EventWorker{Recipients: registryRepo, Origin: origin, Enqueue: func(ctx context.Context, jobs []notify.SendJob) error {
		params := make([]river.InsertManyParams, 0, len(jobs))
		for _, job := range jobs {
			params = append(params, river.InsertManyParams{Args: job})
		}
		_, err := rc.InsertMany(ctx, params)
		return err
	}})
	river.AddWorker(workers, &notify.SendWorker{Sender: sender})
	var err error
	rc, err = river.NewClient(riverpgxv5.New(pool), &river.Config{Workers: workers, Queues: map[string]river.QueueConfig{notify.Queue: {MaxWorkers: 2}}})
	return rc, err
}
