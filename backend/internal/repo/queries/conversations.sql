-- name: CreateConversation :one
INSERT INTO conversations (kb_id, title, mode, user_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetConversation :one
SELECT * FROM conversations WHERE id = $1;

-- name: GetConversationForUser :one
SELECT * FROM conversations
WHERE id = $1 AND user_id = $2;

-- name: ListConversationsByKB :many
SELECT * FROM conversations
WHERE kb_id = $1 AND user_id = $2
ORDER BY updated_at DESC, id DESC
LIMIT $3 OFFSET $4;

-- name: CountConversationsByKB :one
SELECT COUNT(*) FROM conversations
WHERE kb_id = $1 AND user_id = $2;

-- name: TouchConversation :exec
UPDATE conversations
SET updated_at = now()
WHERE id = $1;

-- name: TouchConversationForUser :exec
UPDATE conversations
SET updated_at = now()
WHERE id = $1 AND user_id = $2;

-- name: UpdateConversationMode :one
UPDATE conversations
SET mode = $2, updated_at = now()
WHERE id = $1
RETURNING *;
