package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/pressly/goose/v3"
	"os"
	"strconv"
)

func init() { goose.AddMigrationContext(upKnowledgeVectorDimension, downKnowledgeVectorDimension) }
func upKnowledgeVectorDimension(ctx context.Context, tx *sql.Tx) error {
	dim, e := strconv.Atoi(os.Getenv("EMBEDDING_DIM"))
	if e != nil || dim < 1 || dim > 16000 {
		return fmt.Errorf("EMBEDDING_DIM must be between 1 and 16000")
	}
	_, e = tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE knowledge_chunks ALTER COLUMN embedding TYPE vector(%d)", dim))
	return e
}
func downKnowledgeVectorDimension(ctx context.Context, tx *sql.Tx) error {
	_, e := tx.ExecContext(ctx, "ALTER TABLE knowledge_chunks ALTER COLUMN embedding TYPE vector")
	return e
}
