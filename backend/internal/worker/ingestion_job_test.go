package worker

import (
	"slices"
	"testing"

	"github.com/riverqueue/river/rivertype"
)

func TestIngestionJobDedupesConcurrentInsertsOfSameDoc(t *testing.T) {
	opts := IngestionJobArgs{}.InsertOpts()

	if opts.MaxAttempts != 1 {
		t.Fatalf("MaxAttempts = %d, want 1 (no auto retry per spec)", opts.MaxAttempts)
	}
	if !opts.UniqueOpts.ByArgs {
		t.Fatal("UniqueOpts.ByArgs = false, want true (unique per document)")
	}

	// 只在非终态去重: 已完成/已失败的任务不阻止重新摄入。
	wantStates := []rivertype.JobState{
		rivertype.JobStateAvailable,
		rivertype.JobStatePending,
		rivertype.JobStateRunning,
		rivertype.JobStateScheduled,
		rivertype.JobStateRetryable,
	}
	for _, s := range wantStates {
		if !slices.Contains(opts.UniqueOpts.ByState, s) {
			t.Fatalf("UniqueOpts.ByState missing %q: %v", s, opts.UniqueOpts.ByState)
		}
	}
	for _, s := range []rivertype.JobState{
		rivertype.JobStateCompleted,
		rivertype.JobStateCancelled,
		rivertype.JobStateDiscarded,
	} {
		if slices.Contains(opts.UniqueOpts.ByState, s) {
			t.Fatalf("UniqueOpts.ByState contains final state %q, would block re-ingestion", s)
		}
	}
}
