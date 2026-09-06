-- name: GetModelConnection :one
SELECT * FROM user_llm_keys WHERE user_id = $1;

-- name: SaveModelConnection :one
INSERT INTO user_llm_keys (user_id, base_url, key_ciphertext, models, default_model, protocol)
SELECT sqlc.arg(user_id), sqlc.arg(base_url), sqlc.arg(key_ciphertext), sqlc.arg(models)::text[], sqlc.arg(default_model), sqlc.arg(protocol)
WHERE sqlc.arg(expected_version)::bigint = 0 OR EXISTS (
  SELECT 1 FROM user_llm_keys WHERE user_id = sqlc.arg(user_id)
)
ON CONFLICT (user_id) DO UPDATE SET
    base_url = EXCLUDED.base_url,
    protocol = EXCLUDED.protocol,
    key_ciphertext = EXCLUDED.key_ciphertext,
    models = EXCLUDED.models,
    default_model = EXCLUDED.default_model,
    version = user_llm_keys.version + 1,
    updated_at = now()
WHERE user_llm_keys.version = sqlc.arg(expected_version)::bigint
RETURNING *;

-- name: DeleteModelConnection :exec
DELETE FROM user_llm_keys WHERE user_id = $1;
