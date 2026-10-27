-- name: CreatePublishingCredential :one
INSERT INTO publishing_credentials(account_id,name,token_hash,token_prefix,scopes,expires_at)
SELECT id,sqlc.arg(name),sqlc.arg(token_hash),sqlc.arg(token_prefix),sqlc.arg(scopes),sqlc.arg(expires_at)
FROM oauth_accounts WHERE user_id=sqlc.arg(user_id) AND provider='feishu'
RETURNING *;

-- name: ListPublishingCredentials :many
SELECT c.* FROM publishing_credentials c JOIN oauth_accounts a ON a.id=c.account_id
WHERE a.user_id=$1 AND a.provider='feishu' ORDER BY c.created_at DESC,c.id DESC;

-- name: RevokePublishingCredential :one
UPDATE publishing_credentials c SET revoked_at=COALESCE(c.revoked_at,now())
FROM oauth_accounts a WHERE a.id=c.account_id AND a.user_id=$1 AND a.provider='feishu' AND c.id=$2
RETURNING c.*;

-- name: AuthenticatePublishingCredential :one
UPDATE publishing_credentials c SET last_used_at=now()
FROM oauth_accounts a WHERE a.id=c.account_id AND a.provider='feishu'
AND c.token_hash=$1 AND c.revoked_at IS NULL AND c.expires_at>now()
RETURNING c.id,a.user_id,c.scopes;
