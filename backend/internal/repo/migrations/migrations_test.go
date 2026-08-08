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
