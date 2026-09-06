-- name: RegistryOwner :one
SELECT provider_user_id FROM oauth_accounts WHERE user_id = $1 AND provider = 'feishu';

-- name: CreateCapability :one
INSERT INTO capabilities (slug, type, name, description, owner_open_id, department, visibility)
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: GetOwnedCapability :one
SELECT * FROM capabilities WHERE slug = $1 AND owner_open_id = $2;

-- name: LockOwnedCapability :one
SELECT * FROM capabilities WHERE slug = $1 AND owner_open_id = $2 FOR UPDATE;

-- name: ListOwnedCapabilities :many
SELECT * FROM capabilities
WHERE owner_open_id = sqlc.arg(owner_open_id)
AND (sqlc.arg(filter_type)::text = '' OR type = sqlc.arg(filter_type))
AND (sqlc.arg(filter_status)::text = '' OR status = sqlc.arg(filter_status))
AND (sqlc.arg(filter_department)::text = '' OR department = sqlc.arg(filter_department))
AND (sqlc.arg(search)::text = '' OR strpos(lower(name || ' ' || description || ' ' || slug), lower(sqlc.arg(search))) > 0)
ORDER BY updated_at DESC, id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: UpdateCapability :one
UPDATE capabilities SET name=$2, description=$3, department=$4, visibility=$5,
    revision=revision+1, updated_at=now()
WHERE id=$1 RETURNING *;

-- name: SetCapabilityDraft :one
UPDATE capabilities SET draft_version_id=$2 WHERE id=$1 RETURNING *;

-- name: CreateCapabilityVersion :one
INSERT INTO capability_versions (capability_id, version, changelog, created_by,
    mcp_endpoint, mcp_transport, mcp_auth_scheme, tools, skill_bundle_key, skill_manifest)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *;

-- name: UpdateCapabilityVersion :one
UPDATE capability_versions SET changelog=$2, mcp_endpoint=$3, mcp_transport=$4,
    mcp_auth_scheme=$5, tools=$6, skill_bundle_key=$7, skill_manifest=$8
WHERE id=$1 RETURNING *;

-- name: ListCapabilityVersions :many
SELECT * FROM capability_versions WHERE capability_id=$1 ORDER BY created_at DESC, id;

-- name: ClearCapabilityAllowlist :exec
DELETE FROM capability_allowlist WHERE capability_id=$1;

-- name: AddCapabilityAllowlist :exec
INSERT INTO capability_allowlist(capability_id,open_id) VALUES ($1,$2);

-- name: ListCapabilityAllowlist :many
SELECT open_id FROM capability_allowlist WHERE capability_id=$1 ORDER BY open_id;

-- name: RegistryBundleReferenced :one
SELECT EXISTS(SELECT 1 FROM capability_versions WHERE skill_bundle_key = $1);
