package migrations

import (
	"strings"
	"testing"
)

func TestDocumentSyncMigrationScopesChecksumUniquenessToLocalUploads(t *testing.T) {
	raw, err := EmbedMigrations.ReadFile("0008_document_sync.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.Join(strings.Fields(string(raw)), " ")
	parts := strings.SplitN(sql, "-- +goose Down", 2)
	if len(parts) != 2 {
		t.Fatal("migration is missing a goose Down section")
	}
	up, down := parts[0], parts[1]

	for _, statement := range []string{
		"DROP INDEX uniq_docs_kb_checksum;",
		"CREATE UNIQUE INDEX uniq_docs_kb_local_checksum ON documents (kb_id, checksum) WHERE source_type = 'local-upload';",
	} {
		if !strings.Contains(up, statement) {
			t.Errorf("Up migration missing %q", statement)
		}
	}
	for _, statement := range []string{
		"DROP INDEX IF EXISTS uniq_docs_kb_local_checksum;",
		"CREATE UNIQUE INDEX uniq_docs_kb_checksum ON documents (kb_id, checksum);",
	} {
		if !strings.Contains(down, statement) {
			t.Errorf("Down migration missing %q", statement)
		}
	}
}

func TestDocumentSyncDownMakesRemoteChecksumsLegacyUniqueBeforeRestoringIndex(t *testing.T) {
	raw, err := EmbedMigrations.ReadFile("0008_document_sync.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.Join(strings.Fields(string(raw)), " ")
	parts := strings.SplitN(sql, "-- +goose Down", 2)
	if len(parts) != 2 {
		t.Fatal("migration is missing a goose Down section")
	}
	down := parts[1]

	const (
		dropPartial = "DROP INDEX IF EXISTS uniq_docs_kb_local_checksum;"
		rewrite     = "UPDATE documents SET checksum = checksum || ':' || id::text WHERE source_type <> 'local-upload';"
		restore     = "CREATE UNIQUE INDEX uniq_docs_kb_checksum ON documents (kb_id, checksum);"
	)
	if !strings.Contains(strings.ToLower(down), "legacy schema cannot represent shared remote checksums") {
		t.Error("Down migration must explain why remote checksums are rewritten")
	}

	dropAt := strings.Index(down, dropPartial)
	rewriteAt := strings.Index(down, rewrite)
	restoreAt := strings.Index(down, restore)
	if dropAt < 0 {
		t.Fatalf("Down migration missing %q", dropPartial)
	}
	if rewriteAt < 0 {
		t.Fatalf("Down migration missing %q", rewrite)
	}
	if restoreAt < 0 {
		t.Fatalf("Down migration missing %q", restore)
	}
	if !(dropAt < rewriteAt && rewriteAt < restoreAt) {
		t.Fatalf("Down migration order must be drop, rewrite, restore; positions = %d, %d, %d", dropAt, rewriteAt, restoreAt)
	}
}

func TestPendingSnapshotPayloadMigrationIsAtomicAndReversible(t *testing.T) {
	raw, err := EmbedMigrations.ReadFile("0010_document_pending_snapshot_payload.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.Join(strings.Fields(string(raw)), " ")
	parts := strings.SplitN(sql, "-- +goose Down", 2)
	if len(parts) != 2 {
		t.Fatal("migration is missing a goose Down section")
	}
	up, down := parts[0], parts[1]
	for _, required := range []string{
		"ADD COLUMN pending_title TEXT",
		"ADD COLUMN pending_bytes BIGINT",
		"ADD COLUMN pending_metadata JSONB",
		"UPDATE documents SET pending_content_ref = NULL",
		"ADD CONSTRAINT chk_documents_pending_snapshot_complete",
		"pending_bytes >= 0",
	} {
		if !strings.Contains(up, required) {
			t.Errorf("Up migration missing %q", required)
		}
	}
	for _, required := range []string{
		"DROP CONSTRAINT IF EXISTS chk_documents_pending_snapshot_complete",
		"DROP COLUMN IF EXISTS pending_metadata",
		"DROP COLUMN IF EXISTS pending_bytes",
		"DROP COLUMN IF EXISTS pending_title",
	} {
		if !strings.Contains(down, required) {
			t.Errorf("Down migration missing %q", required)
		}
	}
}

func TestChunkDocumentKBConsistencyMigrationValidatesExistingRowsAndReverses(t *testing.T) {
	raw, err := EmbedMigrations.ReadFile("0011_chunk_document_kb_consistency.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.Join(strings.Fields(string(raw)), " ")
	parts := strings.SplitN(sql, "-- +goose Down", 2)
	if len(parts) != 2 {
		t.Fatal("migration is missing a goose Down section")
	}
	up, down := parts[0], parts[1]

	upStatements := []string{
		"ADD CONSTRAINT uq_documents_id_kb_id UNIQUE (id, kb_id)",
		"DROP CONSTRAINT IF EXISTS chunks_document_id_fkey",
		"ADD CONSTRAINT fk_chunks_document_kb FOREIGN KEY (document_id, kb_id) REFERENCES documents (id, kb_id) ON DELETE CASCADE NOT VALID",
		"VALIDATE CONSTRAINT fk_chunks_document_kb",
	}
	for _, statement := range upStatements {
		if !strings.Contains(up, statement) {
			t.Errorf("Up migration missing %q", statement)
		}
	}
	if addAt, validateAt := strings.Index(up, "NOT VALID"), strings.Index(up, "VALIDATE CONSTRAINT fk_chunks_document_kb"); addAt < 0 || validateAt <= addAt {
		t.Fatalf("existing rows are not explicitly validated after constraint creation: %s", up)
	}

	for _, statement := range []string{
		"DROP CONSTRAINT IF EXISTS fk_chunks_document_kb",
		"ADD CONSTRAINT chunks_document_id_fkey FOREIGN KEY (document_id) REFERENCES documents (id) ON DELETE CASCADE",
		"DROP CONSTRAINT IF EXISTS uq_documents_id_kb_id",
	} {
		if !strings.Contains(down, statement) {
			t.Errorf("Down migration missing %q", statement)
		}
	}
}
