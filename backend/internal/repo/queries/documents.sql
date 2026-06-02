-- name: CreateDocument :one
INSERT INTO documents (kb_id, source_type, source_ref, title, mime_type, bytes, checksum, status, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetDocument :one
SELECT * FROM documents WHERE id = $1;

-- name: FindDocumentByChecksum :one
SELECT * FROM documents WHERE kb_id = $1 AND checksum = $2;

-- name: ListDocumentsByKB :many
SELECT * FROM documents
WHERE kb_id = $1
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountDocumentsByKB :one
SELECT COUNT(*) FROM documents
WHERE kb_id = $1
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);

-- name: UpdateDocumentStatus :exec
UPDATE documents
SET status = $2, error_message = $3, updated_at = now()
WHERE id = $1;

-- name: DeleteDocument :exec
DELETE FROM documents WHERE id = $1;
