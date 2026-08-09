-- name: UpsertOAuthIdentity :one
WITH selected_user_id AS (
    SELECT oa.user_id AS id
    FROM oauth_accounts AS oa
    WHERE oa.provider = sqlc.arg(p_provider)
      AND oa.provider_user_id = sqlc.arg(p_provider_user_id)
    UNION ALL
    SELECT sqlc.arg(p_candidate_user_id)::uuid
    WHERE NOT EXISTS (
        SELECT 1
        FROM oauth_accounts AS oa
        WHERE oa.provider = sqlc.arg(p_provider)
          AND oa.provider_user_id = sqlc.arg(p_provider_user_id)
    )
    LIMIT 1
),
upsert_user AS (
    INSERT INTO users (id, display_name, avatar_url, email)
    SELECT id, sqlc.arg(p_display_name), sqlc.arg(p_avatar_url), NULLIF(sqlc.arg(p_email), '')
    FROM selected_user_id
    ON CONFLICT (id) DO UPDATE SET
        display_name = EXCLUDED.display_name,
        avatar_url = EXCLUDED.avatar_url,
        email = EXCLUDED.email,
        updated_at = now()
    RETURNING *
),
upsert_account AS (
    INSERT INTO oauth_accounts (
        user_id, provider, provider_user_id, tenant_key,
        access_token_encrypted, refresh_token_encrypted,
        access_token_expires_at, refresh_token_expires_at, scopes
    )
    SELECT
        id, sqlc.arg(p_provider), sqlc.arg(p_provider_user_id), sqlc.arg(p_tenant_key),
        sqlc.arg(p_access_token_encrypted), sqlc.arg(p_refresh_token_encrypted),
        sqlc.arg(p_access_token_expires_at), sqlc.narg(p_refresh_token_expires_at), sqlc.arg(p_scopes)
    FROM upsert_user
    ON CONFLICT (provider, provider_user_id) DO UPDATE SET
        tenant_key = EXCLUDED.tenant_key,
        access_token_encrypted = EXCLUDED.access_token_encrypted,
        refresh_token_encrypted = EXCLUDED.refresh_token_encrypted,
        access_token_expires_at = EXCLUDED.access_token_expires_at,
        refresh_token_expires_at = EXCLUDED.refresh_token_expires_at,
        scopes = EXCLUDED.scopes,
        reauth_required = false,
        updated_at = now()
    RETURNING *
)
SELECT
    u.id AS user_id,
    u.display_name,
    u.avatar_url,
    u.email,
    u.created_at AS user_created_at,
    u.updated_at AS user_updated_at,
    a.id AS account_id,
    a.provider,
    a.provider_user_id,
    a.tenant_key,
    a.access_token_encrypted,
    a.refresh_token_encrypted,
    a.access_token_expires_at,
    a.refresh_token_expires_at,
    a.scopes,
    a.reauth_required,
    a.created_at AS account_created_at,
    a.updated_at AS account_updated_at
FROM upsert_user u
JOIN upsert_account a ON a.user_id = u.id;

-- name: GetOAuthAccount :one
SELECT * FROM oauth_accounts AS oa WHERE id = $1;

-- name: GetOAuthAccountForUser :one
SELECT * FROM oauth_accounts AS oa
WHERE oa.user_id = $1 AND oa.provider = 'feishu';

-- name: UpdateOAuthAccountTokens :exec
UPDATE oauth_accounts
SET access_token_encrypted = sqlc.arg(p_access_token_encrypted),
    refresh_token_encrypted = sqlc.arg(p_refresh_token_encrypted),
    access_token_expires_at = sqlc.arg(p_access_token_expires_at),
    refresh_token_expires_at = sqlc.narg(p_refresh_token_expires_at),
    scopes = sqlc.arg(p_scopes),
    reauth_required = sqlc.arg(p_reauth_required),
    updated_at = now()
WHERE id = sqlc.arg(p_id);

-- name: MarkOAuthAccountReauthRequired :exec
UPDATE oauth_accounts
SET reauth_required = true, updated_at = now()
WHERE id = $1;

-- name: GetAuthUser :one
SELECT * FROM users WHERE id = $1;

-- name: CreateUserSession :one
INSERT INTO user_sessions (id, user_id, token_hash, csrf_token_hash, expires_at, created_at)
VALUES (
    sqlc.arg(p_id), sqlc.arg(p_user_id), sqlc.arg(p_token_hash),
    sqlc.arg(p_csrf_token_hash), sqlc.arg(p_expires_at), sqlc.arg(p_created_at)
)
RETURNING *;

-- name: GetUserSessionByTokenHash :one
SELECT * FROM user_sessions WHERE token_hash = $1;

-- name: DeleteUserSessionByTokenHash :execrows
DELETE FROM user_sessions WHERE token_hash = $1;

-- name: DeleteExpiredUserSessions :execrows
DELETE FROM user_sessions WHERE expires_at <= $1;
