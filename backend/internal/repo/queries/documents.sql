-- name: CreateDocument :one
INSERT INTO documents (
    kb_id, source_type, source_ref, content_ref, title, mime_type,
    bytes, checksum, status, metadata
)
VALUES ($1, $2, $3, CASE WHEN $2 = 'local-upload' THEN $3 ELSE NULL END, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: CreateDocumentForOwner :one
INSERT INTO documents (
    kb_id, source_type, source_ref, content_ref, title, mime_type,
    bytes, checksum, status, metadata
)
SELECT kb.id, sqlc.arg('source_type'), sqlc.arg('source_ref'),
       CASE WHEN sqlc.arg('source_type') = 'local-upload'
            THEN sqlc.arg('source_ref') ELSE NULL END,
       sqlc.arg('title'), sqlc.arg('mime_type'),
       sqlc.arg('bytes'), sqlc.arg('checksum'), sqlc.arg('status'),
       sqlc.arg('metadata')
FROM knowledge_bases AS kb
WHERE kb.id = sqlc.arg('kb_id')
  AND kb.owner_user_id = sqlc.arg('owner_user_id')
RETURNING documents.*;

-- name: CreateFeishuDocumentForOwner :one
INSERT INTO documents (
    kb_id, source_type, source_ref, source_url, oauth_account_id,
    title, mime_type, bytes, checksum, status, metadata
)
SELECT kb.id, sqlc.arg('source_type'), sqlc.arg('source_ref'),
       sqlc.arg('source_url'), oa.id,
       sqlc.arg('title'), 'text/markdown', 0, '', 'pending', '{}'::jsonb
FROM knowledge_bases AS kb
JOIN oauth_accounts AS oa
  ON oa.id = sqlc.arg('oauth_account_id')
 AND oa.user_id = sqlc.arg('owner_user_id')
 AND oa.provider = 'feishu'
WHERE kb.id = sqlc.arg('kb_id')
  AND kb.owner_user_id = sqlc.arg('owner_user_id')
RETURNING documents.*;

-- name: GetDocument :one
SELECT * FROM documents WHERE id = $1;

-- name: GetDocumentForOwner :one
SELECT d.* FROM documents AS d
JOIN knowledge_bases AS kb ON kb.id = d.kb_id
WHERE d.id = $1 AND kb.owner_user_id = $2;

-- name: GetFeishuDocumentForOwnerAndAccount :one
SELECT d.* FROM documents AS d
JOIN knowledge_bases AS kb ON kb.id = d.kb_id
JOIN oauth_accounts AS oa ON oa.id = d.oauth_account_id
WHERE d.id = sqlc.arg('id')
  AND d.source_type LIKE 'feishu-%'
  AND kb.owner_user_id = sqlc.arg('owner_user_id')
  AND oa.id = sqlc.arg('oauth_account_id')
  AND oa.user_id = sqlc.arg('owner_user_id');

-- name: FindDocumentByChecksum :one
SELECT * FROM documents
WHERE kb_id = $1 AND checksum = $2 AND source_type = 'local-upload';

-- name: FindDocumentByChecksumForOwner :one
SELECT d.* FROM documents AS d
JOIN knowledge_bases AS kb ON kb.id = d.kb_id
WHERE d.kb_id = $1
  AND d.checksum = $2
  AND d.source_type = 'local-upload'
  AND kb.owner_user_id = $3;

-- name: ListDocumentsByKB :many
SELECT * FROM documents
WHERE kb_id = $1
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListDocumentsByKBForOwner :many
SELECT d.* FROM documents AS d
JOIN knowledge_bases AS kb ON kb.id = d.kb_id
WHERE d.kb_id = sqlc.arg('kb_id')
  AND kb.owner_user_id = sqlc.arg('owner_user_id')
  AND (sqlc.narg('status')::text IS NULL OR d.status = sqlc.narg('status')::text)
ORDER BY d.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountDocumentsByKB :one
SELECT COUNT(*) FROM documents
WHERE kb_id = $1
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);

-- name: CountDocumentsByKBForOwner :one
SELECT COUNT(*) FROM documents AS d
JOIN knowledge_bases AS kb ON kb.id = d.kb_id
WHERE d.kb_id = sqlc.arg('kb_id')
  AND kb.owner_user_id = sqlc.arg('owner_user_id')
  AND (sqlc.narg('status')::text IS NULL OR d.status = sqlc.narg('status')::text);

-- name: UpdateDocumentStatus :exec
UPDATE documents
SET status = $2, error_message = $3, updated_at = now()
WHERE id = $1;

-- name: UpdateDocumentStatusForOwner :exec
UPDATE documents AS d
SET status = sqlc.arg('status'),
    error_message = sqlc.narg('error_message'),
    updated_at = now()
WHERE d.id = sqlc.arg('id')
  AND EXISTS (
      SELECT 1
      FROM knowledge_bases AS kb
      WHERE kb.id = d.kb_id
        AND kb.owner_user_id = sqlc.arg('owner_user_id')
  );

-- name: ClaimFeishuSync :one
UPDATE documents
SET sync_status = 'syncing',
    last_sync_error = NULL,
    pending_content_ref = CASE
        WHEN pending_content_ref IS NOT NULL
         AND pending_checksum IS NOT NULL
         AND pending_remote_revision IS NOT NULL
         AND pending_title IS NOT NULL
         AND pending_bytes IS NOT NULL
         AND pending_metadata IS NOT NULL
        THEN pending_content_ref ELSE NULL END,
    pending_checksum = CASE
        WHEN pending_content_ref IS NOT NULL
         AND pending_checksum IS NOT NULL
         AND pending_remote_revision IS NOT NULL
         AND pending_title IS NOT NULL
         AND pending_bytes IS NOT NULL
         AND pending_metadata IS NOT NULL
        THEN pending_checksum ELSE NULL END,
    pending_remote_revision = CASE
        WHEN pending_content_ref IS NOT NULL
         AND pending_checksum IS NOT NULL
         AND pending_remote_revision IS NOT NULL
         AND pending_title IS NOT NULL
         AND pending_bytes IS NOT NULL
         AND pending_metadata IS NOT NULL
        THEN pending_remote_revision ELSE NULL END,
    pending_title = CASE
        WHEN pending_content_ref IS NOT NULL
         AND pending_checksum IS NOT NULL
         AND pending_remote_revision IS NOT NULL
         AND pending_title IS NOT NULL
         AND pending_bytes IS NOT NULL
         AND pending_metadata IS NOT NULL
        THEN pending_title ELSE NULL END,
    pending_bytes = CASE
        WHEN pending_content_ref IS NOT NULL
         AND pending_checksum IS NOT NULL
         AND pending_remote_revision IS NOT NULL
         AND pending_title IS NOT NULL
         AND pending_bytes IS NOT NULL
         AND pending_metadata IS NOT NULL
        THEN pending_bytes ELSE NULL END,
    pending_metadata = CASE
        WHEN pending_content_ref IS NOT NULL
         AND pending_checksum IS NOT NULL
         AND pending_remote_revision IS NOT NULL
         AND pending_title IS NOT NULL
         AND pending_bytes IS NOT NULL
         AND pending_metadata IS NOT NULL
        THEN pending_metadata ELSE NULL END,
    updated_at = now()
WHERE id = $1
  AND source_type LIKE 'feishu-%'
  AND remote_revision IS NOT DISTINCT FROM sqlc.narg('expected_remote_revision')::text
  AND (
      sync_status IN ('idle', 'failed')
      OR (sync_status = 'syncing' AND updated_at < now() - (sqlc.arg('lease_seconds')::bigint * interval '1 second'))
  )
RETURNING *;

-- name: ListStaleFeishuSyncs :many
SELECT *
FROM documents
WHERE source_type LIKE 'feishu-%'
  AND sync_status = 'syncing'
  AND updated_at < now() - (sqlc.arg('lease_seconds')::bigint * interval '1 second')
  AND (updated_at, id) > (sqlc.arg('after_updated_at')::timestamptz, sqlc.arg('after_id')::uuid)
ORDER BY updated_at, id
LIMIT sqlc.arg('batch_size');

-- name: CompleteUnchangedFeishuSync :execrows
UPDATE documents
SET sync_status = 'idle',
    last_sync_error = NULL,
    last_synced_at = now(),
    pending_content_ref = NULL,
    pending_checksum = NULL,
    pending_remote_revision = NULL,
    pending_title = NULL,
    pending_bytes = NULL,
    pending_metadata = NULL,
    updated_at = now()
WHERE id = sqlc.arg('id')
  AND sync_status = 'syncing'
  AND updated_at = sqlc.arg('claim_token')
  AND remote_revision IS NOT DISTINCT FROM sqlc.narg('remote_revision')::text
  AND checksum = sqlc.arg('checksum')
  AND pending_content_ref IS NOT DISTINCT FROM sqlc.narg('expected_pending_content_ref')::text
  AND pending_checksum IS NOT DISTINCT FROM sqlc.narg('expected_pending_checksum')::text
  AND pending_remote_revision IS NOT DISTINCT FROM sqlc.narg('expected_pending_remote_revision')::text;

-- name: StageFeishuSnapshot :execrows
UPDATE documents
SET pending_content_ref = sqlc.arg('pending_content_ref'),
    pending_checksum = sqlc.arg('pending_checksum'),
    pending_remote_revision = sqlc.arg('pending_remote_revision'),
    pending_title = sqlc.arg('pending_title'),
    pending_bytes = sqlc.arg('pending_bytes'),
    pending_metadata = sqlc.arg('pending_metadata')
WHERE id = sqlc.arg('id')
  AND sync_status = 'syncing'
  AND updated_at = sqlc.arg('claim_token')
  AND remote_revision IS NOT DISTINCT FROM sqlc.narg('expected_remote_revision')::text
  AND checksum = sqlc.arg('expected_checksum')
  AND pending_content_ref IS NOT DISTINCT FROM sqlc.narg('expected_pending_content_ref')::text
  AND pending_checksum IS NOT DISTINCT FROM sqlc.narg('expected_pending_checksum')::text
  AND pending_remote_revision IS NOT DISTINCT FROM sqlc.narg('expected_pending_remote_revision')::text;

-- name: PromoteFeishuSnapshot :execrows
UPDATE documents
SET content_ref = pending_content_ref,
    checksum = pending_checksum,
    remote_revision = pending_remote_revision,
    title = pending_title,
    mime_type = 'text/markdown',
    bytes = pending_bytes,
    metadata = pending_metadata,
    pending_content_ref = NULL,
    pending_checksum = NULL,
    pending_remote_revision = NULL,
    pending_title = NULL,
    pending_bytes = NULL,
    pending_metadata = NULL,
    sync_status = 'idle',
    last_sync_error = NULL,
    last_synced_at = now(),
    status = 'ready',
    error_message = NULL,
    updated_at = now()
WHERE id = sqlc.arg('id')
  AND sync_status = 'syncing'
  AND updated_at = sqlc.arg('claim_token')
  AND pending_content_ref = sqlc.arg('pending_content_ref')
  AND pending_checksum = sqlc.arg('pending_checksum')
  AND pending_remote_revision = sqlc.arg('pending_remote_revision');

-- name: FailFeishuSync :execrows
UPDATE documents
SET sync_status = 'failed',
    last_sync_error = sqlc.arg('safe_error'),
    status = CASE WHEN content_ref IS NULL THEN 'failed' ELSE status END,
    error_message = CASE WHEN content_ref IS NULL THEN sqlc.arg('safe_error') ELSE error_message END,
    updated_at = now()
WHERE id = sqlc.arg('id')
  AND sync_status = 'syncing'
  AND updated_at = sqlc.arg('claim_token')
  AND pending_content_ref IS NOT DISTINCT FROM sqlc.narg('pending_content_ref')::text
  AND pending_checksum IS NOT DISTINCT FROM sqlc.narg('pending_checksum')::text
  AND pending_remote_revision IS NOT DISTINCT FROM sqlc.narg('pending_remote_revision')::text;

-- name: DeleteDocument :exec
DELETE FROM documents WHERE id = $1;

-- name: DeleteDocumentForOwner :exec
DELETE FROM documents AS d
WHERE d.id = $1
  AND EXISTS (
      SELECT 1
      FROM knowledge_bases AS kb
      WHERE kb.id = d.kb_id
        AND kb.owner_user_id = $2
  );
