-- name: GovernanceIdentity :one
SELECT a.provider_user_id AS open_id, COALESCE(p.is_admin, false)::boolean AS is_admin,
    COALESCE(p.is_reviewer, false)::boolean AS is_reviewer,
    COALESCE(p.department, '')::text AS department, COALESCE(p.revision, 0)::bigint AS revision,
    CASE WHEN m.expires_at>now() AND NOT a.reauth_required
        AND a.scopes @> ARRAY['contact:contact.base:readonly','contact:department.base:readonly','contact:user.base:readonly','contact:user.department:readonly']::text[]
        THEN m.department_ids ELSE '{}'::text[] END AS department_ids
FROM oauth_accounts a LEFT JOIN platform_profiles p ON p.open_id=a.provider_user_id
LEFT JOIN feishu_memberships m ON m.account_id=a.id
WHERE a.user_id=$1 AND a.provider='feishu';

-- name: LockAdminBootstrap :exec
SELECT pg_advisory_xact_lock(684291573);

-- name: BootstrapPlatformAdmin :exec
WITH consumed AS (
    INSERT INTO platform_admin_bootstrap(singleton, open_id)
    VALUES (true, $1) ON CONFLICT DO NOTHING RETURNING open_id
)
INSERT INTO platform_profiles(open_id, is_admin)
SELECT open_id, true FROM consumed
WHERE NOT EXISTS (SELECT 1 FROM platform_profiles WHERE is_admin)
ON CONFLICT (open_id) DO UPDATE SET is_admin=true, revision=platform_profiles.revision+1, updated_at=now();

-- name: GetCapability :one
SELECT * FROM capabilities WHERE slug=$1;

-- name: LockCapability :one
SELECT * FROM capabilities WHERE slug=$1 FOR UPDATE;

-- name: ListReviewCapabilities :many
SELECT * FROM capabilities WHERE status='in_review'
AND (sqlc.arg(filter_type)::text='' OR type=sqlc.arg(filter_type))
AND (sqlc.arg(filter_department)::text='' OR department=sqlc.arg(filter_department))
AND (sqlc.arg(search)::text='' OR strpos(lower(name || ' ' || description || ' ' || slug),lower(sqlc.arg(search)))>0)
ORDER BY updated_at, id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListCatalogCapabilities :many
SELECT sqlc.embed(c), v.version AS published_version, COALESCE(u.display_name, '')::text AS publisher_name,
    (SELECT count(*) FROM audit_log l WHERE l.action='download' AND l.target_id=c.id::text)::bigint AS download_count
FROM capabilities c JOIN capability_versions v ON v.id=c.current_version_id
LEFT JOIN oauth_accounts a ON a.provider='feishu' AND a.provider_user_id=c.owner_open_id
LEFT JOIN users u ON u.id=a.user_id
WHERE c.is_live
AND (sqlc.arg(is_admin)::boolean OR c.owner_open_id=sqlc.arg(open_id)
    OR published_metadata->>'visibility'='org'
    OR (published_metadata->>'visibility'='department' AND (
        (COALESCE(published_metadata->>'department_id','')<>'' AND published_metadata->>'department_id'=ANY(sqlc.arg(department_ids)::text[]))
        OR (COALESCE(published_metadata->>'department_id','')='' AND sqlc.arg(department)::text<>'' AND published_metadata->>'department'=sqlc.arg(department))))
    OR (published_metadata->>'visibility'='allowlist' AND (published_metadata->'allowlist') @> to_jsonb(ARRAY[sqlc.arg(open_id)::text])))
AND (sqlc.arg(filter_type)::text='' OR c.type=sqlc.arg(filter_type))
AND (sqlc.arg(filter_status)::text='' OR sqlc.arg(filter_status)='published')
AND (sqlc.arg(filter_department)::text='' OR published_metadata->>'department'=sqlc.arg(filter_department))
AND (sqlc.arg(search)::text='' OR strpos(lower((published_metadata->>'name') || ' ' || (published_metadata->>'description') || ' ' || slug),lower(sqlc.arg(search)))>0)
ORDER BY c.updated_at DESC,c.id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: SetCapabilityGovernance :one
UPDATE capabilities SET status=$2, is_live=$3, current_version_id=$4, draft_version_id=$5,
    published_metadata=$6, review_reason=$7, revision=revision+1, updated_at=now()
WHERE id=$1 RETURNING *;

-- name: MarkVersionPublished :exec
UPDATE capability_versions SET published_at=COALESCE(published_at,now()) WHERE id=$1;

-- name: InsertGovernanceAudit :exec
INSERT INTO audit_log(actor_open_id,action,target_type,target_id,detail,request_id)
VALUES ($1,$2,$3,$4,$5,$6);

-- name: ListGovernanceAudit :many
SELECT * FROM audit_log
WHERE (sqlc.arg(action)::text='' OR action=sqlc.arg(action))
AND (sqlc.arg(actor)::text='' OR actor_open_id=sqlc.arg(actor))
AND (sqlc.arg(target)::text='' OR target_id=sqlc.arg(target))
ORDER BY created_at DESC, id DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListPlatformProfiles :many
SELECT a.provider_user_id AS open_id, u.display_name,
    COALESCE(p.is_reviewer, false)::boolean AS is_reviewer,
    COALESCE(p.is_admin, false)::boolean AS is_admin, COALESCE(p.department,'')::text AS department,
    COALESCE(p.revision,0)::bigint AS revision
FROM oauth_accounts a JOIN users u ON u.id=a.user_id
LEFT JOIN platform_profiles p ON p.open_id=a.provider_user_id WHERE a.provider='feishu'
AND (sqlc.arg(search)::text='' OR strpos(lower(u.display_name || ' ' || a.provider_user_id),lower(sqlc.arg(search)))>0)
ORDER BY a.provider_user_id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: SetTrustedDepartment :one
INSERT INTO platform_profiles(open_id,department)
SELECT sqlc.arg(open_id),sqlc.arg(department) WHERE EXISTS (
    SELECT 1 FROM oauth_accounts WHERE provider='feishu' AND provider_user_id=sqlc.arg(open_id))
AND (sqlc.arg(revision)::bigint=0 OR EXISTS(SELECT 1 FROM platform_profiles WHERE open_id=sqlc.arg(open_id) AND revision=sqlc.arg(revision)))
ON CONFLICT(open_id) DO UPDATE SET department=EXCLUDED.department,revision=platform_profiles.revision+1,updated_at=now()
WHERE platform_profiles.revision=sqlc.arg(revision)
RETURNING *;

-- name: IsPlatformAdmin :one
SELECT EXISTS(SELECT 1 FROM platform_profiles WHERE open_id=$1 AND is_admin);

-- name: LockGovernanceRole :one
SELECT p.open_id FROM platform_profiles p
JOIN oauth_accounts a ON a.provider='feishu' AND a.provider_user_id=p.open_id
WHERE a.user_id=$1 FOR SHARE OF p;

-- name: LockGovernanceProfiles :many
SELECT open_id FROM platform_profiles WHERE open_id=ANY($1::text[])
ORDER BY open_id FOR UPDATE;

-- name: SetReviewer :one
INSERT INTO platform_profiles(open_id,is_reviewer)
SELECT sqlc.arg(open_id),sqlc.arg(is_reviewer) WHERE EXISTS (
    SELECT 1 FROM oauth_accounts WHERE provider='feishu' AND provider_user_id=sqlc.arg(open_id))
AND (sqlc.arg(revision)::bigint=0 OR EXISTS(SELECT 1 FROM platform_profiles WHERE open_id=sqlc.arg(open_id) AND revision=sqlc.arg(revision)))
ON CONFLICT(open_id) DO UPDATE SET is_reviewer=EXCLUDED.is_reviewer,revision=platform_profiles.revision+1,updated_at=now()
WHERE platform_profiles.revision=sqlc.arg(revision)
RETURNING *;

-- name: ListReviewers :many
SELECT p.open_id, u.display_name, p.is_admin
FROM platform_profiles p
JOIN oauth_accounts a ON a.provider='feishu' AND a.provider_user_id=p.open_id
JOIN users u ON u.id=a.user_id
WHERE (p.is_admin OR p.is_reviewer)
ORDER BY p.is_admin DESC, u.display_name, p.open_id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: SetCapabilityVisibility :one
UPDATE capabilities SET visibility=$2,department=$3,department_id=$4 WHERE id=$1 RETURNING *;

-- name: CountCapabilityDownloads :one
SELECT count(*)::bigint FROM audit_log WHERE action='download' AND target_id=$1;

-- name: ListCapabilityDownloaders :many
SELECT l.actor_open_id, COALESCE(MAX(u.display_name), '')::text AS display_name,
    count(*)::bigint AS downloads, MAX(l.created_at)::timestamptz AS last_downloaded_at,
    (array_agg(l.detail->>'version_id' ORDER BY l.created_at DESC))[1]::text AS last_version_id
FROM audit_log l
LEFT JOIN oauth_accounts a ON a.provider='feishu' AND a.provider_user_id=l.actor_open_id
LEFT JOIN users u ON u.id=a.user_id
WHERE l.action='download' AND l.target_id=sqlc.arg(target_id)
GROUP BY l.actor_open_id
ORDER BY last_downloaded_at DESC, l.actor_open_id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListDownloaderAccounts :many
SELECT DISTINCT a.user_id, l.actor_open_id
FROM audit_log l JOIN oauth_accounts a ON a.provider='feishu' AND a.provider_user_id=l.actor_open_id
WHERE l.action='download' AND l.target_id=$1;

-- name: ListNotifyReviewers :many
SELECT p.open_id FROM platform_profiles p
JOIN oauth_accounts a ON a.provider='feishu' AND a.provider_user_id=p.open_id
WHERE p.is_admin OR p.is_reviewer;
