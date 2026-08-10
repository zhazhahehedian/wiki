-- name: CreateMessage :one
INSERT INTO messages (conversation_id, role, content, citations, tool_calls, token_usage)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: CreateMessageForOwner :one
INSERT INTO messages (conversation_id, role, content, citations, tool_calls, token_usage)
SELECT c.id, sqlc.arg('role'), sqlc.arg('content'), sqlc.arg('citations'),
       sqlc.arg('tool_calls'), sqlc.arg('token_usage')
FROM conversations AS c
WHERE c.id = sqlc.arg('conversation_id')
  AND c.owner_user_id = sqlc.arg('owner_user_id')
RETURNING messages.*;

-- name: ListMessagesByConversation :many
SELECT * FROM messages
WHERE conversation_id = $1
ORDER BY created_at ASC, id ASC
LIMIT $2 OFFSET $3;

-- name: ListMessagesByConversationForUser :many
SELECT m.* FROM messages m
JOIN conversations c ON c.id = m.conversation_id
WHERE m.conversation_id = $1 AND c.user_id = $2
ORDER BY m.created_at ASC, m.id ASC
LIMIT $3 OFFSET $4;

-- name: ListMessagesByConversationForOwner :many
SELECT m.* FROM messages m
JOIN conversations c ON c.id = m.conversation_id
WHERE m.conversation_id = $1 AND c.owner_user_id = $2
ORDER BY m.created_at ASC, m.id ASC
LIMIT $3 OFFSET $4;

-- name: CountMessagesByConversation :one
SELECT COUNT(*) FROM messages
WHERE conversation_id = $1;

-- name: CountMessagesByConversationForUser :one
SELECT COUNT(*) FROM messages m
JOIN conversations c ON c.id = m.conversation_id
WHERE m.conversation_id = $1 AND c.user_id = $2;

-- name: CountMessagesByConversationForOwner :one
SELECT COUNT(*) FROM messages m
JOIN conversations c ON c.id = m.conversation_id
WHERE m.conversation_id = $1 AND c.owner_user_id = $2;

-- name: ListRecentMessagesByConversation :many
SELECT * FROM (
  SELECT * FROM messages
  WHERE conversation_id = $1
  ORDER BY created_at DESC, id DESC
  LIMIT $2
) recent_messages
ORDER BY created_at ASC, id ASC;

-- name: ListRecentMessagesByConversationForUser :many
SELECT * FROM (
  SELECT m.* FROM messages m
  JOIN conversations c ON c.id = m.conversation_id
  WHERE m.conversation_id = $1 AND c.user_id = $2
  ORDER BY m.created_at DESC, m.id DESC
  LIMIT $3
) recent_messages
ORDER BY created_at ASC, id ASC;

-- name: ListRecentMessagesByConversationForOwner :many
SELECT * FROM (
  SELECT m.* FROM messages m
  JOIN conversations c ON c.id = m.conversation_id
  WHERE m.conversation_id = $1 AND c.owner_user_id = $2
  ORDER BY m.created_at DESC, m.id DESC
  LIMIT $3
) recent_messages
ORDER BY created_at ASC, id ASC;
