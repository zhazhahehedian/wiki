-- name: RegistryOwner :one
SELECT provider_user_id FROM oauth_accounts WHERE user_id = $1 AND provider = 'feishu';

-- name: CreateCapability :one
INSERT INTO capabilities (slug, type, name, description, owner_open_id, department, visibility, department_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

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
UPDATE capabilities SET name=$2, description=$3, department=$4, visibility=$5, department_id=$6,
    revision=revision+1, updated_at=now()
WHERE id=$1 RETURNING *;

-- name: SetCapabilityDraft :one
UPDATE capabilities SET draft_version_id=$2 WHERE id=$1 RETURNING *;

-- name: CreateCapabilityVersion :one
INSERT INTO capability_versions (capability_id, version, changelog, created_by,
    mcp_endpoint, mcp_transport, mcp_auth_scheme, tools, bundle_key, skill_manifest,
    bundle_sha256, bundle_size, bundle_findings)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING *;

-- name: UpdateCapabilityVersion :one
UPDATE capability_versions SET changelog=$2, mcp_endpoint=$3, mcp_transport=$4,
    mcp_auth_scheme=$5, tools=$6, bundle_key=$7, skill_manifest=$8,
    bundle_sha256=$9, bundle_size=$10, bundle_findings=$11
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
SELECT EXISTS(SELECT 1 FROM capability_versions WHERE bundle_key = $1);

-- name: BundleUploadCreate :one
-- Caps unfinished sessions per user inside the insert to avoid a count race.
INSERT INTO capability_bundle_uploads (id, user_id, file_name, size, part_size, object_key, storage_upload_id, expires_at)
SELECT $1, $2, $3, $4, $5, $6, $7, $8
WHERE (SELECT count(*) FROM capability_bundle_uploads
       WHERE user_id = $2 AND completed_at IS NULL AND expires_at > now()) < sqlc.arg(max_active)::int
RETURNING *;

-- name: BundleUploadGet :one
SELECT * FROM capability_bundle_uploads WHERE id = $1 AND user_id = $2;

-- name: BundleUploadComplete :one
UPDATE capability_bundle_uploads
SET completed_at = now(), bundle_sha256 = $3, bundle_findings = $4
WHERE id = $1 AND user_id = $2 AND completed_at IS NULL
RETURNING *;

-- name: BundleUploadDelete :exec
DELETE FROM capability_bundle_uploads WHERE id = $1 AND user_id = $2;

-- name: BundleUploadsExpired :many
SELECT * FROM capability_bundle_uploads WHERE expires_at < $1 ORDER BY expires_at LIMIT 100;

-- name: BundleUploadDeleteExpired :exec
DELETE FROM capability_bundle_uploads WHERE id = $1 AND expires_at < $2;
