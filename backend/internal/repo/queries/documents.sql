-- name: CreateDocument :one
INSERT INTO documents (
    kb_id, source_type, source_ref, content_ref, title, mime_type,
    bytes, checksum, status, metadata
)
VALUES ($1, $2, $3, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: CreateDocumentForOwner :one
INSERT INTO documents (
    kb_id, source_type, source_ref, content_ref, title, mime_type,
    bytes, checksum, status, metadata
)
SELECT kb.id, sqlc.arg('source_type'), sqlc.arg('source_ref'),
       sqlc.arg('source_ref'), sqlc.arg('title'), sqlc.arg('mime_type'),
       sqlc.arg('bytes'), sqlc.arg('checksum'), sqlc.arg('status'),
       sqlc.arg('metadata')
FROM knowledge_bases AS kb
WHERE kb.id = sqlc.arg('kb_id')
  AND kb.owner_user_id = sqlc.arg('owner_user_id')
RETURNING documents.*;

-- name: GetDocument :one
SELECT * FROM documents WHERE id = $1;

-- name: GetDocumentForOwner :one
SELECT d.* FROM documents AS d
JOIN knowledge_bases AS kb ON kb.id = d.kb_id
WHERE d.id = $1 AND kb.owner_user_id = $2;

-- name: FindDocumentByChecksum :one
SELECT * FROM documents WHERE kb_id = $1 AND checksum = $2;

-- name: FindDocumentByChecksumForOwner :one
SELECT d.* FROM documents AS d
JOIN knowledge_bases AS kb ON kb.id = d.kb_id
WHERE d.kb_id = $1
  AND d.checksum = $2
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
