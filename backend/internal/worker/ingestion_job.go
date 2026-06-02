package worker

import "github.com/riverqueue/river"

type IngestionJobArgs struct {
	DocumentID string `json:"document_id"`
}

func (IngestionJobArgs) Kind() string { return "ingestion" }

func (IngestionJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 1,
	}
}
