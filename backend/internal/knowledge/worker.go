package knowledge

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// Queue is the River queue that indexes knowledge versions.
const Queue = "knowledge"

type Job struct {
	// VersionID is the only uniqueness key so a second enqueue for the same
	// version does not run beside the first. RequestID is for logs only.
	VersionID string `json:"version_id" river:"unique"`
	RequestID string `json:"request_id,omitempty"`
}

func (Job) Kind() string { return "knowledge_index_v1" }
func (Job) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       Queue,
		MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			// Completed jobs must not block a later retry of the same version.
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

type Worker struct {
	river.WorkerDefaults[Job]
	Service *Service
}

type indexRequestIDKey struct{}

// WithIndexRequestID attaches the HTTP request id to a River worker context.
func WithIndexRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, indexRequestIDKey{}, id)
}

func indexRequestID(ctx context.Context) string {
	id, _ := ctx.Value(indexRequestIDKey{}).(string)
	return id
}

func (w *Worker) Work(ctx context.Context, j *river.Job[Job]) error {
	ctx = WithIndexRequestID(ctx, j.Args.RequestID)
	slog.InfoContext(ctx, "knowledge index started", "version_id", j.Args.VersionID, "request_id", j.Args.RequestID)
	err := w.Service.Process(ctx, j.Args.VersionID)
	if err != nil {
		slog.ErrorContext(ctx, "knowledge index failed", "version_id", j.Args.VersionID, "request_id", j.Args.RequestID, "error", err.Error())
	}
	return err
}

// Image recognition calls a vision model per DOCX image, so allow more time.
func (w *Worker) Timeout(*river.Job[Job]) time.Duration { return 30 * time.Minute }
