package vectorstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
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

	if err := replaceChunksInTx(ctx, tx, documentID, items); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (v *Pgvector) ReplaceChunksAndPromote(ctx context.Context, promotion ports.PendingDocumentPromotion, items []domain.ChunkWithEmbedding) error {
	tx, err := v.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := replaceChunksInTx(ctx, tx, promotion.DocumentID.String(), items); err != nil {
		return err
	}
	if err := promoteDocumentInTx(ctx, tx, promotion); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (v *Pgvector) PatchChunkMetadataAndPromote(ctx context.Context, promotion ports.PendingDocumentPromotion, patch ports.ChunkMetadataPatcher) error {
	tx, err := v.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	rows, err := tx.Query(ctx, `SELECT id, metadata FROM chunks WHERE document_id = $1 FOR UPDATE`, promotion.DocumentID)
	if err != nil {
		return fmt.Errorf("lock chunk metadata: %w", err)
	}
	type metadataUpdate struct {
		id       uuid.UUID
		metadata []byte
	}
	updates := make([]metadataUpdate, 0)
	for rows.Next() {
		var id uuid.UUID
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return fmt.Errorf("scan chunk metadata: %w", err)
		}
		existing := decodeMetadata(raw)
		patched, err := json.Marshal(patch(existing))
		if err != nil {
			rows.Close()
			return fmt.Errorf("marshal patched chunk metadata: %w", err)
		}
		updates = append(updates, metadataUpdate{id: id, metadata: patched})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate chunk metadata: %w", err)
	}
	rows.Close()
	for _, update := range updates {
		if _, err := tx.Exec(ctx, `UPDATE chunks SET metadata = $2 WHERE id = $1`, update.id, update.metadata); err != nil {
			return fmt.Errorf("patch chunk metadata: %w", err)
		}
	}
	if err := promoteDocumentInTx(ctx, tx, promotion); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func promoteDocumentInTx(ctx context.Context, tx pgx.Tx, promotion ports.PendingDocumentPromotion) error {
	contentRef, checksum, revision := promotion.ContentRef, promotion.Checksum, promotion.RemoteRevision
	rows, err := generated.New(tx).PromoteFeishuSnapshot(ctx, generated.PromoteFeishuSnapshotParams{
		ID: promotion.DocumentID, PendingContentRef: &contentRef, PendingChecksum: &checksum, PendingRemoteRevision: &revision,
		ClaimToken: promotion.ClaimToken,
	})
	if err != nil {
		return fmt.Errorf("promote staged document: %w", err)
	}
	if rows != 1 {
		return ports.ErrStaleDocumentPromotion
	}
	return nil
}

func replaceChunksInTx(ctx context.Context, tx pgx.Tx, documentID string, items []domain.ChunkWithEmbedding) error {
	if _, err := tx.Exec(ctx, `DELETE FROM chunks WHERE document_id = $1`, documentID); err != nil {
		return fmt.Errorf("delete old chunks: %w", err)
	}
	if len(items) == 0 {
		return nil
	}
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
	return nil
}

// DeleteByDocument 删除文档的全部 chunks, 供重新摄入前清理旧数据。
func (v *Pgvector) DeleteByDocument(ctx context.Context, documentID string) error {
	if _, err := v.pool.Exec(ctx, `DELETE FROM chunks WHERE document_id = $1`, documentID); err != nil {
		return fmt.Errorf("delete chunks by document: %w", err)
	}
	return nil
}

func (v *Pgvector) ListByDocument(ctx context.Context, userID, docID string, limit, offset int) ([]domain.Chunk, int, error) {
	var total int
	if err := v.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM chunks c
		 JOIN documents d ON d.id = c.document_id
		 JOIN knowledge_bases kb ON kb.id = d.kb_id
		 WHERE c.document_id = $1 AND kb.owner_user_id = $2`, docID, userID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count chunks: %w", err)
	}

	rows, err := v.pool.Query(ctx,
		`SELECT c.id, c.kb_id, c.document_id, c.seq, c.content, c.token_count, c.metadata, c.created_at
		 FROM chunks c
		 JOIN documents d ON d.id = c.document_id
		 JOIN knowledge_bases kb ON kb.id = d.kb_id
		 WHERE c.document_id = $1 AND kb.owner_user_id = $2
		 ORDER BY c.seq ASC LIMIT $3 OFFSET $4`,
		docID, userID, limit, offset,
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

// Search is retained for trusted internal callers. User-facing retrieval must use SearchForOwner.
func (v *Pgvector) Search(ctx context.Context, kbID string, query []float32, opts ports.VectorSearchOptions) ([]ports.VectorSearchHit, error) {
	topK := opts.TopK
	if topK < 1 {
		topK = 8
	}
	tx, err := v.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin search tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `SET LOCAL ivfflat.probes = 10`); err != nil {
		return nil, fmt.Errorf("set ivfflat probes: %w", err)
	}
	rows, err := tx.Query(ctx,
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

func (v *Pgvector) SearchForOwner(ctx context.Context, userID, kbID string, query []float32, opts ports.VectorSearchOptions) ([]ports.VectorSearchHit, error) {
	topK := opts.TopK
	if topK < 1 {
		topK = 8
	}

	tx, err := v.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin search tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// idx_chunks_ivfflat 在建表迁移时创建, 彼时表为空, 质心退化; 加上数据量
	// 远小于 lists=100, 默认 probes=1 会把召回压到 0(2026-07-08 评测实测)。
	// 按 sqrt(lists) 经验值取 10; SET LOCAL 仅影响本事务。
	if _, err := tx.Exec(ctx, `SET LOCAL ivfflat.probes = 10`); err != nil {
		return nil, fmt.Errorf("set ivfflat probes: %w", err)
	}

	rows, err := tx.Query(ctx,
		`SELECT c.id, c.kb_id, c.document_id, d.title, c.seq, c.content,
		        1 - (c.embedding <=> $3) AS score, c.metadata
		   FROM chunks c
		   JOIN documents d ON d.id = c.document_id
		   JOIN knowledge_bases kb ON kb.id = d.kb_id
		  WHERE c.kb_id = $1
		    AND kb.owner_user_id = $2
		    AND d.status = 'ready'
		  ORDER BY c.embedding <=> $3
		  LIMIT $4`,
		kbID, userID, pgvector.NewVector(query), topK,
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

func (v *Pgvector) GetChunk(ctx context.Context, userID, kbID, chunkID string) (*domain.Chunk, error) {
	var c domain.Chunk
	var metaJSON []byte
	err := v.pool.QueryRow(ctx,
		`SELECT c.id, c.kb_id, c.document_id, c.seq, c.content, c.token_count, c.metadata, c.created_at
		   FROM chunks c
		   JOIN knowledge_bases kb ON kb.id = c.kb_id
		  WHERE c.kb_id = $1 AND c.id = $2 AND kb.owner_user_id = $3`,
		kbID, chunkID, userID,
	).Scan(&c.ID, &c.KBID, &c.DocumentID, &c.Seq, &c.Content, &c.TokenCount, &metaJSON, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get chunk: %w", err)
	}
	c.Metadata = decodeMetadata(metaJSON)
	return &c, nil
}

func (v *Pgvector) ListNeighbors(ctx context.Context, userID, kbID, documentID string, seq, window int) ([]domain.Chunk, error) {
	if window < 0 {
		window = 0
	}

	rows, err := v.pool.Query(ctx,
		`SELECT c.id, c.kb_id, c.document_id, c.seq, c.content, c.token_count, c.metadata, c.created_at
		   FROM chunks c
		   JOIN knowledge_bases kb ON kb.id = c.kb_id
		  WHERE c.kb_id = $1
		    AND c.document_id = $2
		    AND kb.owner_user_id = $3
		    AND c.seq BETWEEN $4 AND $5
		  ORDER BY c.seq ASC`,
		kbID, documentID, userID, seq-window, seq+window,
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
