package vectorstore

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

func TestSearchDefaultsTopKAndMapsHits(t *testing.T) {
	tx := &fakeTx{
		rows: &fakeRows{values: [][]any{{
			"chunk-1",
			"kb-1",
			"doc-1",
			"Runbook",
			3,
			"Rotate the database password.",
			float32(0.91),
			[]byte(`{"path":"ops.md"}`),
		}}},
	}
	db := &fakeDB{tx: tx}
	store := &Pgvector{pool: db}

	hits, err := store.Search(context.Background(), "kb-1", []float32{0.1, 0.2}, ports.VectorSearchOptions{})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(hits) != 1 {
		t.Fatalf("len(hits) = %d, want 1", len(hits))
	}
	hit := hits[0]
	if hit.ChunkID != "chunk-1" || hit.KBID != "kb-1" || hit.DocumentID != "doc-1" || hit.DocumentTitle != "Runbook" || hit.Seq != 3 {
		t.Fatalf("Search() hit = %+v", hit)
	}
	if hit.Content != "Rotate the database password." {
		t.Fatalf("Content = %q", hit.Content)
	}
	if hit.Score != 0.91 {
		t.Fatalf("Score = %f, want 0.91", hit.Score)
	}
	if hit.Metadata["path"] != "ops.md" {
		t.Fatalf("Metadata[path] = %v, want ops.md", hit.Metadata["path"])
	}
	if !strings.Contains(tx.execSQL, "SET LOCAL ivfflat.probes") {
		t.Fatalf("Search must raise ivfflat.probes in the same tx, got exec: %q", tx.execSQL)
	}
	if !strings.Contains(tx.querySQL, "JOIN documents d ON d.id = c.document_id") {
		t.Fatalf("Search SQL missing documents join: %s", tx.querySQL)
	}
	if !strings.Contains(tx.querySQL, "d.status = 'ready'") {
		t.Fatalf("Search SQL missing ready filter: %s", tx.querySQL)
	}
	if !strings.Contains(tx.querySQL, "ORDER BY c.embedding <=> $2") {
		t.Fatalf("Search SQL missing distance ordering: %s", tx.querySQL)
	}
	if got := tx.queryArgs[0]; got != "kb-1" {
		t.Fatalf("Search kb arg = %v, want kb-1", got)
	}
	if got := tx.queryArgs[2]; got != 8 {
		t.Fatalf("Search topK arg = %v, want 8", got)
	}
}

func TestSearchReturnsIterationErrors(t *testing.T) {
	db := &fakeDB{
		tx: &fakeTx{rows: &fakeRows{err: errors.New("cursor failed")}},
	}
	store := &Pgvector{pool: db}

	_, err := store.Search(context.Background(), "kb-1", []float32{0.1}, ports.VectorSearchOptions{TopK: 2})
	if err == nil {
		t.Fatal("Search() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "iterate vector hits") {
		t.Fatalf("Search() error = %v, want iterate vector hits", err)
	}
}

func TestGetChunkMapsMetadata(t *testing.T) {
	createdAt := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	db := &fakeDB{
		row: &fakeRow{values: []any{
			"chunk-1",
			"kb-1",
			"doc-1",
			7,
			"Neighbor content",
			42,
			[]byte(`{"heading":"Rotation"}`),
			createdAt,
		}},
	}
	store := &Pgvector{pool: db}

	chunk, err := store.GetChunk(context.Background(), "kb-1", "chunk-1")
	if err != nil {
		t.Fatalf("GetChunk() error = %v", err)
	}

	if chunk.ID != "chunk-1" || chunk.KBID != "kb-1" || chunk.DocumentID != "doc-1" || chunk.Seq != 7 {
		t.Fatalf("GetChunk() = %+v", chunk)
	}
	if chunk.TokenCount != 42 {
		t.Fatalf("TokenCount = %d, want 42", chunk.TokenCount)
	}
	if chunk.Metadata["heading"] != "Rotation" {
		t.Fatalf("Metadata[heading] = %v, want Rotation", chunk.Metadata["heading"])
	}
	if db.rowArgs[0] != "kb-1" || db.rowArgs[1] != "chunk-1" {
		t.Fatalf("GetChunk args = %v, want kb/chunk", db.rowArgs)
	}
}

func TestListNeighborsClampsNegativeWindow(t *testing.T) {
	createdAt := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	db := &fakeDB{
		rows: &fakeRows{values: [][]any{{
			"chunk-5",
			"kb-1",
			"doc-1",
			5,
			"Primary content",
			21,
			[]byte(`{}`),
			createdAt,
		}}},
	}
	store := &Pgvector{pool: db}

	chunks, err := store.ListNeighbors(context.Background(), "kb-1", "doc-1", 5, -3)
	if err != nil {
		t.Fatalf("ListNeighbors() error = %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("len(chunks) = %d, want 1", len(chunks))
	}
	if chunks[0].ID != "chunk-5" || chunks[0].Seq != 5 {
		t.Fatalf("ListNeighbors() chunk = %+v", chunks[0])
	}
	if db.queryArgs[2] != 5 || db.queryArgs[3] != 5 {
		t.Fatalf("neighbor bounds = %v, want seq to seq for negative window", db.queryArgs[2:4])
	}
}

func TestReplaceChunksDeletesThenInsertsInOneTx(t *testing.T) {
	tx := &fakeTx{}
	db := &fakeDB{tx: tx}
	store := &Pgvector{pool: db}

	items := []domain.ChunkWithEmbedding{{
		Chunk: domain.Chunk{
			KBID: "kb-1", DocumentID: "doc-1", Seq: 0,
			Content: "rotate password", TokenCount: 2,
		},
		Embedding: []float32{0.1, 0.2},
	}}
	if err := store.ReplaceChunks(context.Background(), "doc-1", items); err != nil {
		t.Fatalf("ReplaceChunks() error = %v", err)
	}

	if len(tx.ops) != 3 || !strings.HasPrefix(tx.ops[0], "exec:") || tx.ops[1] != "sendbatch" || tx.ops[2] != "commit" {
		t.Fatalf("tx ops = %#v, want [exec:delete sendbatch commit]", tx.ops)
	}
	if !strings.Contains(tx.execSQL, "DELETE FROM chunks WHERE document_id = $1") {
		t.Fatalf("tx exec SQL = %q, want chunks delete by document_id", tx.execSQL)
	}
	if len(tx.execArgs) != 1 || tx.execArgs[0] != "doc-1" {
		t.Fatalf("tx exec args = %#v, want [doc-1]", tx.execArgs)
	}
	if tx.batch == nil || tx.batch.Len() != 1 {
		t.Fatalf("batch = %#v, want 1 queued insert", tx.batch)
	}
	if !strings.Contains(tx.batch.QueuedQueries[0].SQL, "INSERT INTO chunks") {
		t.Fatalf("batch SQL = %q, want chunks insert", tx.batch.QueuedQueries[0].SQL)
	}
}

func TestReplaceChunksRollsBackWhenDeleteFails(t *testing.T) {
	tx := &fakeTx{execErr: errors.New("boom")}
	db := &fakeDB{tx: tx}
	store := &Pgvector{pool: db}

	err := store.ReplaceChunks(context.Background(), "doc-1", nil)
	if err == nil || !strings.Contains(err.Error(), "delete old chunks") {
		t.Fatalf("ReplaceChunks() error = %v, want wrapped delete error", err)
	}
	if tx.committed {
		t.Fatal("tx committed despite delete failure")
	}
	if !tx.rolledBack {
		t.Fatal("tx not rolled back after delete failure")
	}
}

func TestReplaceChunksAndPromoteUsesOneTransactionAndConditionalSQLCUpdate(t *testing.T) {
	tx := &fakeTx{execTags: []pgconn.CommandTag{pgconn.NewCommandTag("DELETE 2"), pgconn.NewCommandTag("UPDATE 1")}}
	store := &Pgvector{pool: &fakeDB{tx: tx}}
	promotion := ports.PendingDocumentPromotion{
		DocumentID: uuid.MustParse("74ecb70f-b708-4a8e-ad85-40a9026f38ad"),
		ContentRef: "feishu/74ec/snapshot.md", Checksum: "new-sum", RemoteRevision: "rev-2",
	}
	items := []domain.ChunkWithEmbedding{{Chunk: domain.Chunk{KBID: "kb-1", DocumentID: promotion.DocumentID.String(), Content: "new chunk"}, Embedding: []float32{0.1}}}

	if err := store.ReplaceChunksAndPromote(context.Background(), promotion, items); err != nil {
		t.Fatalf("ReplaceChunksAndPromote() error = %v", err)
	}
	if !tx.committed || tx.rolledBack {
		t.Fatalf("transaction committed/rolledBack = %v/%v", tx.committed, tx.rolledBack)
	}
	if len(tx.ops) != 4 || !strings.Contains(tx.ops[0], "DELETE FROM chunks") || tx.ops[1] != "sendbatch" || !strings.Contains(tx.ops[2], "UPDATE documents") || tx.ops[3] != "commit" {
		t.Fatalf("transaction operations = %#v", tx.ops)
	}
	if !strings.Contains(tx.execSQL, "pending_content_ref") || !strings.Contains(tx.execSQL, "sync_status = 'syncing'") {
		t.Fatalf("promotion SQL is not conditional: %s", tx.execSQL)
	}
}

func TestReplaceChunksAndPromoteRollsBackWhenSnapshotIsStale(t *testing.T) {
	tx := &fakeTx{execTags: []pgconn.CommandTag{pgconn.NewCommandTag("DELETE 2"), pgconn.NewCommandTag("UPDATE 0")}}
	store := &Pgvector{pool: &fakeDB{tx: tx}}
	promotion := ports.PendingDocumentPromotion{
		DocumentID: uuid.New(), ContentRef: "snapshot.md", Checksum: "sum", RemoteRevision: "rev",
	}

	err := store.ReplaceChunksAndPromote(context.Background(), promotion, nil)
	if !errors.Is(err, ports.ErrStaleDocumentPromotion) {
		t.Fatalf("ReplaceChunksAndPromote() error = %v, want stale promotion", err)
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("stale transaction committed/rolledBack = %v/%v", tx.committed, tx.rolledBack)
	}
}

func TestDeleteByDocumentIssuesDelete(t *testing.T) {
	db := &fakeDB{}
	store := &Pgvector{pool: db}

	if err := store.DeleteByDocument(context.Background(), "doc-1"); err != nil {
		t.Fatalf("DeleteByDocument() error = %v", err)
	}
	if !strings.Contains(db.execSQL, "DELETE FROM chunks WHERE document_id = $1") {
		t.Fatalf("exec SQL = %q, want chunks delete by document_id", db.execSQL)
	}
	if len(db.execArgs) != 1 || db.execArgs[0] != "doc-1" {
		t.Fatalf("exec args = %#v, want [doc-1]", db.execArgs)
	}
}

func TestDeleteByDocumentWrapsError(t *testing.T) {
	db := &fakeDB{execErr: errors.New("boom")}
	store := &Pgvector{pool: db}

	err := store.DeleteByDocument(context.Background(), "doc-1")
	if err == nil || !strings.Contains(err.Error(), "delete chunks by document") {
		t.Fatalf("DeleteByDocument() error = %v, want wrapped", err)
	}
}

type fakeDB struct {
	querySQL  string
	queryArgs []any
	rows      pgx.Rows
	queryErr  error

	rowSQL  string
	rowArgs []any
	row     pgx.Row

	execSQL  string
	execArgs []any
	execErr  error

	tx pgx.Tx
}

func (f *fakeDB) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	if f.tx != nil {
		return f.tx, nil
	}
	return nil, errors.New("BeginTx not implemented")
}

func (f *fakeDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	f.querySQL = sql
	f.queryArgs = args
	return f.rows, f.queryErr
}

func (f *fakeDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	f.rowSQL = sql
	f.rowArgs = args
	return f.row
}

func (f *fakeDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.execSQL = sql
	f.execArgs = args
	return pgconn.CommandTag{}, f.execErr
}

// fakeTx records the order of operations issued inside a transaction so tests
// can assert delete-then-insert atomicity.
type fakeTx struct {
	ops        []string
	execSQL    string
	execArgs   []any
	execErr    error
	execTags   []pgconn.CommandTag
	batch      *pgx.Batch
	committed  bool
	rolledBack bool

	querySQL  string
	queryArgs []any
	rows      pgx.Rows
	queryErr  error
}

func (t *fakeTx) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("nested Begin not implemented")
}

func (t *fakeTx) Commit(context.Context) error {
	t.ops = append(t.ops, "commit")
	t.committed = true
	return nil
}

func (t *fakeTx) Rollback(context.Context) error {
	if !t.committed {
		t.rolledBack = true
	}
	return nil
}

func (t *fakeTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("CopyFrom not implemented")
}

func (t *fakeTx) SendBatch(_ context.Context, b *pgx.Batch) pgx.BatchResults {
	t.ops = append(t.ops, "sendbatch")
	t.batch = b
	return &fakeBatchResults{remaining: b.Len()}
}

func (t *fakeTx) LargeObjects() pgx.LargeObjects { return pgx.LargeObjects{} }

func (t *fakeTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("Prepare not implemented")
}

func (t *fakeTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	t.ops = append(t.ops, "exec:"+sql)
	t.execSQL = sql
	t.execArgs = args
	if len(t.execTags) > 0 {
		tag := t.execTags[0]
		t.execTags = t.execTags[1:]
		return tag, t.execErr
	}
	return pgconn.CommandTag{}, t.execErr
}

func (t *fakeTx) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	t.ops = append(t.ops, "query")
	t.querySQL = sql
	t.queryArgs = args
	return t.rows, t.queryErr
}

func (t *fakeTx) QueryRow(context.Context, string, ...any) pgx.Row { return nil }

func (t *fakeTx) Conn() *pgx.Conn { return nil }

type fakeBatchResults struct {
	remaining int
	execErr   error
}

func (r *fakeBatchResults) Exec() (pgconn.CommandTag, error) {
	r.remaining--
	return pgconn.CommandTag{}, r.execErr
}

func (r *fakeBatchResults) Query() (pgx.Rows, error) {
	return nil, errors.New("batch Query not implemented")
}

func (r *fakeBatchResults) QueryRow() pgx.Row { return nil }

func (r *fakeBatchResults) Close() error { return nil }

type fakeRows struct {
	values [][]any
	index  int
	err    error
	closed bool
}

func (r *fakeRows) Close() {
	r.closed = true
}

func (r *fakeRows) Err() error {
	return r.err
}

func (r *fakeRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}

func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func (r *fakeRows) Next() bool {
	if r.index >= len(r.values) {
		r.closed = true
		return false
	}
	r.index++
	return true
}

func (r *fakeRows) Scan(dest ...any) error {
	return scanValues(r.values[r.index-1], dest...)
}

func (r *fakeRows) Values() ([]any, error) {
	return r.values[r.index-1], nil
}

func (r *fakeRows) RawValues() [][]byte {
	return nil
}

func (r *fakeRows) Conn() *pgx.Conn {
	return nil
}

type fakeRow struct {
	values []any
	err    error
}

func (r *fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	return scanValues(r.values, dest...)
}

func scanValues(values []any, dest ...any) error {
	if len(values) != len(dest) {
		return errors.New("scan destination count mismatch")
	}
	for i := range values {
		target := reflect.ValueOf(dest[i])
		if target.Kind() != reflect.Ptr || target.IsNil() {
			return errors.New("scan destination must be a non-nil pointer")
		}
		value := reflect.ValueOf(values[i])
		elem := target.Elem()
		if value.Type().AssignableTo(elem.Type()) {
			elem.Set(value)
			continue
		}
		if value.Type().ConvertibleTo(elem.Type()) {
			elem.Set(value.Convert(elem.Type()))
			continue
		}
		return errors.New("scan value is not assignable")
	}
	return nil
}
