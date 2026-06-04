package vectorstore

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

func TestSearchDefaultsTopKAndMapsHits(t *testing.T) {
	db := &fakeDB{
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
	if !strings.Contains(db.querySQL, "JOIN documents d ON d.id = c.document_id") {
		t.Fatalf("Search SQL missing documents join: %s", db.querySQL)
	}
	if !strings.Contains(db.querySQL, "d.status = 'ready'") {
		t.Fatalf("Search SQL missing ready filter: %s", db.querySQL)
	}
	if !strings.Contains(db.querySQL, "ORDER BY c.embedding <=> $2") {
		t.Fatalf("Search SQL missing distance ordering: %s", db.querySQL)
	}
	if got := db.queryArgs[0]; got != "kb-1" {
		t.Fatalf("Search kb arg = %v, want kb-1", got)
	}
	if got := db.queryArgs[2]; got != 8 {
		t.Fatalf("Search topK arg = %v, want 8", got)
	}
}

func TestSearchReturnsIterationErrors(t *testing.T) {
	db := &fakeDB{
		rows: &fakeRows{err: errors.New("cursor failed")},
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

type fakeDB struct {
	querySQL  string
	queryArgs []any
	rows      pgx.Rows
	queryErr  error

	rowSQL  string
	rowArgs []any
	row     pgx.Row
}

func (f *fakeDB) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
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
