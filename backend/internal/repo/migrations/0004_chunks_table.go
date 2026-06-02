package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upChunksTable, downChunksTable)
}

func upChunksTable(ctx context.Context, tx *sql.Tx) error {
	dimStr := os.Getenv("EMBEDDING_DIM")
	if dimStr == "" {
		return fmt.Errorf("EMBEDDING_DIM env var is required for chunks migration")
	}
	dim, err := strconv.Atoi(dimStr)
	if err != nil || dim <= 0 {
		return fmt.Errorf("EMBEDDING_DIM must be positive int, got %q", dimStr)
	}

	stmt := fmt.Sprintf(`
CREATE TABLE chunks (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id         UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    document_id   UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    seq           INT  NOT NULL,
    content       TEXT NOT NULL,
    token_count   INT  NOT NULL,
    embedding     vector(%d),
    metadata      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_chunks_doc_seq ON chunks (kb_id, document_id, seq);
CREATE INDEX idx_chunks_ivfflat ON chunks USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
`, dim)
	_, err = tx.ExecContext(ctx, stmt)
	return err
}

func downChunksTable(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS chunks;`)
	return err
}
