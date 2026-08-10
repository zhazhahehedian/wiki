-- name: CountOrphanOwnership :one
SELECT (SELECT COUNT(*) FROM knowledge_bases WHERE owner_user_id IS NULL)
     + (SELECT COUNT(*) FROM conversations WHERE owner_user_id IS NULL);

-- name: GetOAuthUserIDByProviderIdentity :one
SELECT user_id FROM oauth_accounts
WHERE provider = $1 AND provider_user_id = $2;

-- name: UpsertBootstrapUser :exec
INSERT INTO users (id, display_name)
VALUES ($1, $2)
ON CONFLICT (id) DO NOTHING;

-- name: AssignOrphanKnowledgeBases :exec
UPDATE knowledge_bases SET owner_user_id = $1 WHERE owner_user_id IS NULL;

-- name: AssignOrphanConversations :exec
UPDATE conversations SET owner_user_id = $1 WHERE owner_user_id IS NULL;
