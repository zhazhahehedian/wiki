-- name: CreateKnowledgeBase :one
INSERT INTO knowledge_bases (name, description, embed_model, embed_dim, settings)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: CreateKnowledgeBaseForOwner :one
INSERT INTO knowledge_bases (name, description, embed_model, embed_dim, settings, owner_user_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetKnowledgeBase :one
SELECT * FROM knowledge_bases WHERE id = $1;

-- name: GetKnowledgeBaseForOwner :one
SELECT * FROM knowledge_bases
WHERE id = $1 AND owner_user_id = $2;

-- name: ListKnowledgeBases :many
SELECT * FROM knowledge_bases
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListKnowledgeBasesForOwner :many
SELECT * FROM knowledge_bases
WHERE owner_user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountKnowledgeBases :one
SELECT COUNT(*) FROM knowledge_bases;

-- name: CountKnowledgeBasesForOwner :one
SELECT COUNT(*) FROM knowledge_bases
WHERE owner_user_id = $1;

-- name: DeleteKnowledgeBase :exec
DELETE FROM knowledge_bases WHERE id = $1;

-- name: DeleteKnowledgeBaseForOwner :exec
DELETE FROM knowledge_bases
WHERE id = $1 AND owner_user_id = $2;
