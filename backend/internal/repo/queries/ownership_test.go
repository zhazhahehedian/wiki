package queries

import (
	"os"
	"strings"
	"testing"
)

func TestOwnerMessageQueriesJoinConversationOwnership(t *testing.T) {
	raw, err := os.ReadFile("messages.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, name := range []string{"CreateMessageForOwner", "ListMessagesByConversationForOwner", "CountMessagesByConversationForOwner", "ListRecentMessagesByConversationForOwner"} {
		query := strings.Join(strings.Fields(queryNamed(t, sql, name)), " ")
		if !strings.Contains(query, "owner_user_id") {
			t.Fatalf("%s lacks owner filter: %s", name, query)
		}
	}
}

func TestBootstrapQueriesOnlyAssignNullOwnership(t *testing.T) {
	raw, err := os.ReadFile("ownership_bootstrap.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, name := range []string{"AssignOrphanKnowledgeBases", "AssignOrphanConversations"} {
		query := strings.Join(strings.Fields(queryNamed(t, sql, name)), " ")
		if !strings.Contains(query, "owner_user_id IS NULL") {
			t.Fatalf("%s could overwrite ownership: %s", name, query)
		}
	}
}

func TestFeishuSyncAuthorizationJoinsOwnerAndAccount(t *testing.T) {
	raw, err := os.ReadFile("documents.sql")
	if err != nil {
		t.Fatal(err)
	}
	query := strings.Join(strings.Fields(queryNamed(t, string(raw), "GetFeishuDocumentForOwnerAndAccount")), " ")
	for _, want := range []string{"kb.owner_user_id = sqlc.arg('owner_user_id')", "oa.id = sqlc.arg('oauth_account_id')", "oa.user_id = sqlc.arg('owner_user_id')"} {
		if !strings.Contains(query, want) {
			t.Fatalf("Feishu sync authorization missing %q: %s", want, query)
		}
	}
}
