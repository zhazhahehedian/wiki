package knowledge

import (
	"context"
	"github.com/riverqueue/river"
	"time"
)

type Job struct {
	VersionID string `json:"version_id"`
}

func (Job) Kind() string                 { return "knowledge_index_v1" }
func (Job) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 3} }

type Worker struct {
	river.WorkerDefaults[Job]
	Service *Service
}

func (w *Worker) Work(ctx context.Context, j *river.Job[Job]) error {
	return w.Service.Process(ctx, j.Args.VersionID)
}
func (w *Worker) Timeout(*river.Job[Job]) time.Duration { return 10 * time.Minute }
