package worker

import (
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type FeishuSyncJobArgs struct {
	DocumentID        string `json:"document_id"`
	RequestedRevision string `json:"requested_revision"`
}

func (FeishuSyncJobArgs) Kind() string { return "feishu_sync" }

func (FeishuSyncJobArgs) InsertOpts() river.InsertOpts {
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
