package queries

import (
	"os"
	"strings"
	"testing"
)

func TestAuthSessionQueriesUseOnlyCredentialHashesAndCleanupExpiry(t *testing.T) {
	raw, err := os.ReadFile("auth.sql")
	if err != nil {
		t.Fatalf("read auth.sql: %v", err)
	}
	sql := string(raw)

	create := strings.Join(strings.Fields(queryNamed(t, sql, "CreateUserSession")), " ")
	for _, want := range []string{"token_hash", "csrf_token_hash", "expires_at"} {
		if !strings.Contains(create, want) {
			t.Fatalf("CreateUserSession missing %q: %s", want, create)
		}
	}
	for _, queryName := range []string{"GetUserSessionByTokenHash", "DeleteUserSessionByTokenHash"} {
		query := strings.Join(strings.Fields(queryNamed(t, sql, queryName)), " ")
		if !strings.Contains(query, "token_hash = $1") {
			t.Fatalf("%s must address only the token hash: %s", queryName, query)
		}
	}
	cleanup := strings.Join(strings.Fields(queryNamed(t, sql, "DeleteExpiredUserSessions")), " ")
	if !strings.Contains(cleanup, "expires_at <= $1") {
		t.Fatalf("DeleteExpiredUserSessions contract = %s", cleanup)
	}
}

func TestOAuthIdentityQueryPersistsEncryptedTokenColumns(t *testing.T) {
	raw, err := os.ReadFile("auth.sql")
	if err != nil {
		t.Fatalf("read auth.sql: %v", err)
	}
	query := strings.Join(strings.Fields(queryNamed(t, string(raw), "UpsertOAuthIdentity")), " ")

	for _, want := range []string{
		"access_token_encrypted",
		"refresh_token_encrypted",
		"candidate_user_id",
		"ON CONFLICT (provider, provider_user_id)",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("UpsertOAuthIdentity missing %q: %s", want, query)
		}
	}
	if strings.Contains(query, " access_token,") || strings.Contains(query, " refresh_token,") {
		t.Fatalf("UpsertOAuthIdentity persists plaintext token columns: %s", query)
	}
}
