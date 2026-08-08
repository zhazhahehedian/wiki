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
