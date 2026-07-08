package vectorstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type pgxDB interface {
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type Pgvector struct {
	pool pgxDB
}

func New(pool *pgxpool.Pool) *Pgvector {
	return &Pgvector{pool: pool}
}

// ReplaceChunks 在单个事务内删除文档旧 chunks 并插入新 chunks,
// 避免进程在"已删旧、未插新"之间中断时留下零 chunks 的文档。
func (v *Pgvector) ReplaceChunks(ctx context.Context, documentID string, items []domain.ChunkWithEmbedding) error {
	tx, err := v.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `DELETE FROM chunks WHERE document_id = $1`, documentID); err != nil {
		return fmt.Errorf("delete old chunks: %w", err)
	}

	if len(items) > 0 {
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
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// DeleteByDocument 删除文档的全部 chunks, 供重新摄入前清理旧数据。
func (v *Pgvector) DeleteByDocument(ctx context.Context, documentID string) error {
	if _, err := v.pool.Exec(ctx, `DELETE FROM chunks WHERE document_id = $1`, documentID); err != nil {
		return fmt.Errorf("delete chunks by document: %w", err)
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

func (v *Pgvector) Search(ctx context.Context, kbID string, query []float32, opts ports.VectorSearchOptions) ([]ports.VectorSearchHit, error) {
	topK := opts.TopK
	if topK < 1 {
		topK = 8
	}

	rows, err := v.pool.Query(ctx,
		`SELECT c.id, c.kb_id, c.document_id, d.title, c.seq, c.content,
		        1 - (c.embedding <=> $2) AS score, c.metadata
		   FROM chunks c
		   JOIN documents d ON d.id = c.document_id
		  WHERE c.kb_id = $1
		    AND d.status = 'ready'
		  ORDER BY c.embedding <=> $2
		  LIMIT $3`,
		kbID, pgvector.NewVector(query), topK,
	)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	var hits []ports.VectorSearchHit
	for rows.Next() {
		var hit ports.VectorSearchHit
		var metaJSON []byte
		if err := rows.Scan(&hit.ChunkID, &hit.KBID, &hit.DocumentID, &hit.DocumentTitle, &hit.Seq, &hit.Content, &hit.Score, &metaJSON); err != nil {
			return nil, fmt.Errorf("scan vector hit: %w", err)
		}
		hit.Metadata = decodeMetadata(metaJSON)
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vector hits: %w", err)
	}
	return hits, nil
}

func (v *Pgvector) GetChunk(ctx context.Context, kbID, chunkID string) (*domain.Chunk, error) {
	var c domain.Chunk
	var metaJSON []byte
	err := v.pool.QueryRow(ctx,
		`SELECT id, kb_id, document_id, seq, content, token_count, metadata, created_at
		   FROM chunks
		  WHERE kb_id = $1 AND id = $2`,
		kbID, chunkID,
	).Scan(&c.ID, &c.KBID, &c.DocumentID, &c.Seq, &c.Content, &c.TokenCount, &metaJSON, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get chunk: %w", err)
	}
	c.Metadata = decodeMetadata(metaJSON)
	return &c, nil
}

func (v *Pgvector) ListNeighbors(ctx context.Context, kbID, documentID string, seq, window int) ([]domain.Chunk, error) {
	if window < 0 {
		window = 0
	}

	rows, err := v.pool.Query(ctx,
		`SELECT id, kb_id, document_id, seq, content, token_count, metadata, created_at
		   FROM chunks
		  WHERE kb_id = $1
		    AND document_id = $2
		    AND seq BETWEEN $3 AND $4
		  ORDER BY seq ASC`,
		kbID, documentID, seq-window, seq+window,
	)
	if err != nil {
		return nil, fmt.Errorf("list chunk neighbors: %w", err)
	}
	defer rows.Close()

	var out []domain.Chunk
	for rows.Next() {
		var c domain.Chunk
		var metaJSON []byte
		if err := rows.Scan(&c.ID, &c.KBID, &c.DocumentID, &c.Seq, &c.Content, &c.TokenCount, &metaJSON, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan neighbor chunk: %w", err)
		}
		c.Metadata = decodeMetadata(metaJSON)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate neighbor chunks: %w", err)
	}
	return out, nil
}

func decodeMetadata(metaJSON []byte) map[string]any {
	metadata := map[string]any{}
	if len(metaJSON) > 0 {
		_ = json.Unmarshal(metaJSON, &metadata)
	}
	return metadata
}
