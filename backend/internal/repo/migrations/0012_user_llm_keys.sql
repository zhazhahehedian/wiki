-- +goose Up
CREATE TABLE user_llm_keys (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    base_url TEXT NOT NULL,
    key_ciphertext BYTEA NOT NULL,
    models TEXT[] NOT NULL CHECK (cardinality(models) BETWEEN 1 AND 100),
    default_model TEXT NOT NULL CHECK (default_model = ANY(models)),
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE user_llm_keys;
