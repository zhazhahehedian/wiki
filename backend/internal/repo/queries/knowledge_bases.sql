-- name: CreateKnowledgeBase :one
INSERT INTO knowledge_bases (name, description, embed_model, embed_dim, settings)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetKnowledgeBase :one
SELECT * FROM knowledge_bases WHERE id = $1;

-- name: ListKnowledgeBases :many
SELECT * FROM knowledge_bases
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountKnowledgeBases :one
SELECT COUNT(*) FROM knowledge_bases;

-- name: DeleteKnowledgeBase :exec
DELETE FROM knowledge_bases WHERE id = $1;
