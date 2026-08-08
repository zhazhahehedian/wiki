package queries

import (
	"os"
	"strings"
	"testing"
)

func TestDocumentCreateQueriesOnlySetContentRefForLocalUploads(t *testing.T) {
	raw, err := os.ReadFile("documents.sql")
	if err != nil {
		t.Fatalf("read documents.sql: %v", err)
	}
	sql := string(raw)

	tests := []struct {
		name      string
		queryName string
		want      string
	}{
		{
			name:      "legacy create",
			queryName: "CreateDocument",
			want:      "CASE WHEN $2 = 'local-upload' THEN $3 ELSE NULL END",
		},
		{
			name:      "owner create",
			queryName: "CreateDocumentForOwner",
			want:      "CASE WHEN sqlc.arg('source_type') = 'local-upload' THEN sqlc.arg('source_ref') ELSE NULL END",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := queryNamed(t, sql, tt.queryName)
			normalized := strings.Join(strings.Fields(query), " ")
			if !strings.Contains(normalized, tt.want) {
				t.Fatalf("%s must contain %q; query = %s", tt.queryName, tt.want, normalized)
			}
		})
	}
}

func TestChecksumQueriesOnlyDedupeLocalUploads(t *testing.T) {
	raw, err := os.ReadFile("documents.sql")
	if err != nil {
		t.Fatalf("read documents.sql: %v", err)
	}
	sql := string(raw)

	tests := []struct {
		queryName string
		want      string
	}{
		{queryName: "FindDocumentByChecksum", want: "source_type = 'local-upload'"},
		{queryName: "FindDocumentByChecksumForOwner", want: "d.source_type = 'local-upload'"},
	}

	for _, tt := range tests {
		t.Run(tt.queryName, func(t *testing.T) {
			query := queryNamed(t, sql, tt.queryName)
			normalized := strings.Join(strings.Fields(query), " ")
			if !strings.Contains(normalized, tt.want) {
				t.Fatalf("%s must contain %q; query = %s", tt.queryName, tt.want, normalized)
			}
		})
	}
}

func queryNamed(t *testing.T, sql, name string) string {
	t.Helper()
	startMarker := "-- name: " + name + " "
	start := strings.Index(sql, startMarker)
	if start == -1 {
		t.Fatalf("query %s not found", name)
	}
	rest := sql[start+len(startMarker):]
	if next := strings.Index(rest, "-- name: "); next >= 0 {
		rest = rest[:next]
	}
	return rest
}
