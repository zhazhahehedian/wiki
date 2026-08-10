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

func TestFeishuCreateBindsOwnedKnowledgeBaseAndOAuthAccount(t *testing.T) {
	raw, err := os.ReadFile("documents.sql")
	if err != nil {
		t.Fatal(err)
	}
	query := strings.Join(strings.Fields(queryNamed(t, string(raw), "CreateFeishuDocumentForOwner")), " ")
	for _, required := range []string{
		"JOIN oauth_accounts AS oa",
		"oa.user_id = sqlc.arg('owner_user_id')",
		"oa.provider = 'feishu'",
		"kb.owner_user_id = sqlc.arg('owner_user_id')",
	} {
		if !strings.Contains(query, required) {
			t.Fatalf("owner-scoped Feishu insert missing %q: %s", required, query)
		}
	}
}

func TestFeishuStateTransitionsUseExpectedActiveAndPendingFields(t *testing.T) {
	raw, err := os.ReadFile("documents.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	claim := strings.Join(strings.Fields(queryNamed(t, sql, "ClaimFeishuSync")), " ")
	for _, required := range []string{
		"remote_revision IS NOT DISTINCT FROM sqlc.narg('expected_remote_revision')::text",
		"sync_status IN ('idle', 'failed')",
		"sync_status = 'syncing' AND updated_at < now() - (sqlc.arg('lease_seconds')::bigint * interval '1 second')",
		"pending_title = CASE",
		"pending_bytes = CASE",
		"pending_metadata = CASE",
	} {
		if !strings.Contains(claim, required) {
			t.Fatalf("claim query missing %q: %s", required, claim)
		}
	}

	stage := strings.Join(strings.Fields(queryNamed(t, sql, "StageFeishuSnapshot")), " ")
	for _, required := range []string{
		"pending_title = sqlc.arg('pending_title')",
		"pending_bytes = sqlc.arg('pending_bytes')",
		"pending_metadata = sqlc.arg('pending_metadata')",
		"sync_status = 'syncing'",
		"updated_at = sqlc.arg('claim_token')",
		"remote_revision IS NOT DISTINCT FROM sqlc.narg('expected_remote_revision')::text",
		"checksum = sqlc.arg('expected_checksum')",
		"pending_content_ref IS NOT DISTINCT FROM sqlc.narg('expected_pending_content_ref')::text",
		"pending_checksum IS NOT DISTINCT FROM sqlc.narg('expected_pending_checksum')::text",
		"pending_remote_revision IS NOT DISTINCT FROM sqlc.narg('expected_pending_remote_revision')::text",
	} {
		if !strings.Contains(stage, required) {
			t.Fatalf("stage query missing %q: %s", required, stage)
		}
	}
	for _, forbidden := range []string{"title = sqlc.arg('title')", "bytes = sqlc.arg('bytes')", "metadata = sqlc.arg('metadata')", "updated_at = now()"} {
		if strings.Contains(stage, forbidden) {
			t.Fatalf("stage query mutates active field %q: %s", forbidden, stage)
		}
	}
	promote := strings.Join(strings.Fields(queryNamed(t, sql, "PromoteFeishuSnapshot")), " ")
	for _, required := range []string{"title = pending_title", "bytes = pending_bytes", "metadata = pending_metadata", "pending_title = NULL", "pending_bytes = NULL", "pending_metadata = NULL"} {
		if !strings.Contains(promote, required) {
			t.Fatalf("promotion query missing active metadata update %q: %s", required, promote)
		}
	}

	transitionGuards := map[string][]string{
		"CompleteUnchangedFeishuSync": {
			"pending_content_ref IS NOT DISTINCT FROM sqlc.narg('expected_pending_content_ref')::text",
			"pending_checksum IS NOT DISTINCT FROM sqlc.narg('expected_pending_checksum')::text",
			"pending_remote_revision IS NOT DISTINCT FROM sqlc.narg('expected_pending_remote_revision')::text",
		},
		"PromoteFeishuSnapshot": {
			"pending_content_ref = sqlc.arg('pending_content_ref')",
			"pending_checksum = sqlc.arg('pending_checksum')",
			"pending_remote_revision = sqlc.arg('pending_remote_revision')",
		},
		"FailFeishuSync": {
			"pending_content_ref IS NOT DISTINCT FROM sqlc.narg('pending_content_ref')::text",
			"pending_checksum IS NOT DISTINCT FROM sqlc.narg('pending_checksum')::text",
			"pending_remote_revision IS NOT DISTINCT FROM sqlc.narg('pending_remote_revision')::text",
		},
	}
	for name, guards := range transitionGuards {
		query := strings.Join(strings.Fields(queryNamed(t, sql, name)), " ")
		for _, required := range append([]string{"sync_status = 'syncing'", "updated_at = sqlc.arg('claim_token')"}, guards...) {
			if !strings.Contains(query, required) {
				t.Fatalf("%s missing stale-worker guard %q: %s", name, required, query)
			}
		}
	}

	fail := strings.Join(strings.Fields(queryNamed(t, sql, "FailFeishuSync")), " ")
	for _, forbidden := range []string{"pending_content_ref = NULL", "pending_checksum = NULL", "pending_remote_revision = NULL"} {
		if strings.Contains(fail, forbidden) {
			t.Fatalf("failure query clears retryable pending field %q: %s", forbidden, fail)
		}
	}
}

func TestListStaleFeishuSyncsUsesDBClockAndBoundedKeysetPagination(t *testing.T) {
	raw, err := os.ReadFile("documents.sql")
	if err != nil {
		t.Fatal(err)
	}
	query := strings.Join(strings.Fields(queryNamed(t, string(raw), "ListStaleFeishuSyncs")), " ")
	for _, required := range []string{
		"source_type LIKE 'feishu-%'",
		"sync_status = 'syncing'",
		"updated_at < now() - (sqlc.arg('lease_seconds')::bigint * interval '1 second')",
		"(updated_at, id) > (sqlc.arg('after_updated_at')::timestamptz, sqlc.arg('after_id')::uuid)",
		"ORDER BY updated_at, id",
		"LIMIT sqlc.arg('batch_size')",
	} {
		if !strings.Contains(query, required) {
			t.Fatalf("stale scan query missing %q: %s", required, query)
		}
	}
}

func TestGeneratedDocumentQueriesRetainSQLCProvenance(t *testing.T) {
	raw, err := os.ReadFile("../generated/documents.sql.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.HasPrefix(text, "// Code generated by sqlc. DO NOT EDIT.") {
		t.Fatal("generated document queries are missing sqlc provenance")
	}
	for _, method := range []string{"CreateFeishuDocumentForOwner", "ListStaleFeishuSyncs", "ClaimFeishuSync", "CompleteUnchangedFeishuSync", "StageFeishuSnapshot", "PromoteFeishuSnapshot", "FailFeishuSync"} {
		if !strings.Contains(text, "func (q *Queries) "+method) {
			t.Fatalf("generated query method %s missing", method)
		}
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
