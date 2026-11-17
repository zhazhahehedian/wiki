package knowledge

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
)

// Queue is the River queue that indexes knowledge versions.
const Queue = "knowledge"

type Job struct {
	VersionID string `json:"version_id"`
	RequestID string `json:"request_id,omitempty"`
}

func (Job) Kind() string { return "knowledge_index_v1" }
func (Job) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: Queue, MaxAttempts: 3}
}

type Worker struct {
	river.WorkerDefaults[Job]
	Service *Service
}

func (w *Worker) Work(ctx context.Context, j *river.Job[Job]) error {
	slog.InfoContext(ctx, "knowledge index started", "version_id", j.Args.VersionID, "request_id", j.Args.RequestID)
	err := w.Service.Process(ctx, j.Args.VersionID)
	if err != nil {
		slog.ErrorContext(ctx, "knowledge index failed", "version_id", j.Args.VersionID, "request_id", j.Args.RequestID, "error", err.Error())
	}
	return err
}

// Image recognition calls a vision model per DOCX image, so allow more time.
func (w *Worker) Timeout(*river.Job[Job]) time.Duration { return 30 * time.Minute }
