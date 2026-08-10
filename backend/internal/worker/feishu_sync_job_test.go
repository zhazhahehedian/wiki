package worker

import (
	"slices"
	"testing"

	"github.com/riverqueue/river/rivertype"
)

func TestFeishuSyncJobDedupesOnlyActiveAttempts(t *testing.T) {
	opts := (FeishuSyncJobArgs{}).InsertOpts()
	if opts.MaxAttempts != 1 || !opts.UniqueOpts.ByArgs {
		t.Fatalf("InsertOpts() = %+v", opts)
	}
	for _, state := range []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning,
		rivertype.JobStateScheduled, rivertype.JobStateRetryable,
	} {
		if !slices.Contains(opts.UniqueOpts.ByState, state) {
			t.Fatalf("active state %q missing from %v", state, opts.UniqueOpts.ByState)
		}
	}
	for _, state := range []rivertype.JobState{
		rivertype.JobStateCompleted, rivertype.JobStateCancelled, rivertype.JobStateDiscarded,
	} {
		if slices.Contains(opts.UniqueOpts.ByState, state) {
			t.Fatalf("terminal state %q present in %v", state, opts.UniqueOpts.ByState)
		}
	}
}
