package vectorstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

type Pgvector struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Pgvector {
	return &Pgvector{pool: pool}
}

func (v *Pgvector) InsertChunks(ctx context.Context, items []domain.ChunkWithEmbedding) error {
	if len(items) == 0 {
		return nil
	}

	tx, err := v.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	batch := &pgx.Batch{}
	for _, it := range items {
		metaJSON, err := json.Marshal(it.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
		batch.Queue(
			`INSERT INTO chunks (kb_id, document_id, seq, content, token_count, embedding, metadata)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			it.KBID, it.DocumentID, it.Seq, it.Content, it.TokenCount,
			pgvector.NewVector(it.Embedding), metaJSON,
		)
	}

	br := tx.SendBatch(ctx, batch)
	for i := 0; i < len(items); i++ {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return fmt.Errorf("insert chunk %d: %w", i, err)
		}
	}
	if err := br.Close(); err != nil {
		return fmt.Errorf("close batch: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (v *Pgvector) ListByDocument(ctx context.Context, docID string, limit, offset int) ([]domain.Chunk, int, error) {
	var total int
	if err := v.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM chunks WHERE document_id = $1`, docID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count chunks: %w", err)
	}

	rows, err := v.pool.Query(ctx,
		`SELECT id, kb_id, document_id, seq, content, token_count, metadata, created_at
		 FROM chunks WHERE document_id = $1
		 ORDER BY seq ASC LIMIT $2 OFFSET $3`,
		docID, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query chunks: %w", err)
	}
	defer rows.Close()

	var out []domain.Chunk
	for rows.Next() {
		var c domain.Chunk
		var metaJSON []byte
		if err := rows.Scan(&c.ID, &c.KBID, &c.DocumentID, &c.Seq, &c.Content, &c.TokenCount, &metaJSON, &c.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan: %w", err)
		}
		if len(metaJSON) > 0 {
			_ = json.Unmarshal(metaJSON, &c.Metadata)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}
