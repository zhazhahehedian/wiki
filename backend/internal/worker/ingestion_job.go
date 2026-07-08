package worker

import (
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type IngestionJobArgs struct {
	DocumentID string `json:"document_id"`
}

func (IngestionJobArgs) Kind() string { return "ingestion" }

func (IngestionJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 1,
		// 按 document_id 去重: 同一文档已有 pending/running 任务时再次投递为 no-op,
		// 防止并发双摄入交错 delete/insert 产生重复 chunks。
		// ByState 显式排除终态(completed/cancelled/discarded), 否则 river 默认
		// 包含 completed, 会在 cleaner 清理前阻止已完成文档的重新摄入。
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
