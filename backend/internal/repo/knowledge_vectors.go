package repo

// pgvector operations use pgx directly because
// sqlc does not model runtime embedding dimensions. Business SQL stays in queries.
import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
	k "github.com/zenith-wang/it-wiki/backend/internal/knowledge"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

// Publish atomically stores all chunks of a version and switches the active
// pointer. Process has already validated block/vector pairing and vectors, and
// Stage guaranteed the version matches the base embedding model/dimension.
func (r *KnowledgeRepository) Publish(ctx context.Context, v k.Version, blocks []k.Block, vectors [][]float32) error {
	contents := make([]string, len(blocks))
	locators := make([]string, len(blocks))
	embeddings := make([]string, len(blocks))
	for i, b := range blocks {
		loc, e := json.Marshal(b.Locator)
		if e != nil {
			return e
		}
		contents[i], locators[i], embeddings[i] = b.Content, string(loc), pgvector.NewVector(vectors[i]).String()
	}
	return r.mutate(ctx, func(q *generated.Queries, tx pgx.Tx) error {
		d, e := q.KnowledgeLockDocument(ctx, ku(v.DocumentID))
		if e != nil {
			return ke(e)
		}
		if ks(d.PendingVersionID) != v.ID {
			return k.ErrConflict
		}
		if _, e = tx.Exec(ctx, "DELETE FROM knowledge_chunks WHERE version_id=$1", ku(v.ID)); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO knowledge_chunks(version_id,seq,content,locator,embedding)
 SELECT $1::uuid,(t.n-1)::int,t.content,t.locator::jsonb,t.embedding::vector
 FROM unnest($2::text[],$3::text[],$4::text[]) WITH ORDINALITY AS t(content,locator,embedding,n)`, ku(v.ID), contents, locators, embeddings); e != nil {
			return e
		}
		if e = q.KnowledgeVersionReady(ctx, ku(v.ID)); e != nil {
			return e
		}
		n, e := q.KnowledgeActivate(ctx, generated.KnowledgeActivateParams{ID: d.ID, ActiveVersionID: kn(v.ID), Title: v.Title, Checksum: v.Checksum})
		if e != nil {
			return e
		}
		if n != 1 {
			return k.ErrConflict
		}
		return nil
	})
}
func (r *KnowledgeRepository) Search(ctx context.Context, ids []string, vec []float32, top int, min float32) ([]k.Match, error) {
	kbIDs := make([]uuid.UUID, len(ids))
	for i, id := range ids {
		kbIDs[i] = ku(id)
	}
	rows, e := r.pool.Query(ctx, `WITH candidates AS MATERIALIZED (
 SELECT c.id,c.version_id,c.content,c.locator,c.embedding,d.id AS document_id,d.kb_id,v.title
 FROM knowledge_chunks c JOIN knowledge_versions v ON v.id=c.version_id
 JOIN documents d ON d.id=v.document_id AND d.active_version_id=v.id
 JOIN knowledge_bases b ON b.id=d.kb_id
 WHERE d.kb_id=ANY($1::uuid[]) AND d.deleted_at IS NULL AND b.deleted_at IS NULL AND v.status='ready'
 ) SELECT kb_id,document_id,version_id,id,title,content,locator,(1-(embedding <=> $2::vector))::real AS score
 FROM candidates WHERE 1-(embedding <=> $2::vector)>=$3 ORDER BY embedding <=> $2::vector,id LIMIT $4`, kbIDs, pgvector.NewVector(vec).String(), min, top)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []k.Match{}
	for rows.Next() {
		var m k.Match
		var loc []byte
		if e = rows.Scan(&m.KBID, &m.DocumentID, &m.VersionID, &m.ChunkID, &m.Title, &m.Content, &loc, &m.Score); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(loc, &m.Locator); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// LexicalSearch shares the vector retrieval boundary: only live active versions
// in explicitly authorized KBs are scored. No cross-library corpus statistics.
// Substring bigrams support Chinese without requiring a PostgreSQL extension.
func (r *KnowledgeRepository) LexicalSearch(ctx context.Context, ids []string, terms []string, top int) ([]k.Match, error) {
	if len(terms) == 0 {
		return []k.Match{}, nil
	}
	kbIDs := make([]uuid.UUID, len(ids))
	for i, id := range ids {
		kbIDs[i] = ku(id)
	}
	rows, e := r.pool.Query(ctx, `WITH candidates AS MATERIALIZED (
 SELECT c.id,c.version_id,c.content,c.locator,d.id AS document_id,d.kb_id,v.title,
 lower(v.title || ' ' || COALESCE(c.locator->>'heading','') || ' ' || c.content) AS text
 FROM knowledge_chunks c JOIN knowledge_versions v ON v.id=c.version_id
 JOIN documents d ON d.id=v.document_id AND d.active_version_id=v.id
 JOIN knowledge_bases b ON b.id=d.kb_id
 WHERE d.kb_id=ANY($1::uuid[]) AND d.deleted_at IS NULL AND b.deleted_at IS NULL AND v.status='ready'
 ), scored AS (
 SELECT *, (SELECT count(*)::real / cardinality($2::text[]) FROM unnest($2::text[]) term WHERE strpos(text,term)>0) AS score
 FROM candidates
 ) SELECT kb_id,document_id,version_id,id,title,content,locator,score
 FROM scored WHERE score>=0.5 ORDER BY score DESC,id LIMIT $3`, kbIDs, terms, top)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []k.Match{}
	for rows.Next() {
		var m k.Match
		var loc []byte
		if e = rows.Scan(&m.KBID, &m.DocumentID, &m.VersionID, &m.ChunkID, &m.Title, &m.Content, &loc, &m.Score); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(loc, &m.Locator); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
