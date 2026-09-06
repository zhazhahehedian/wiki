-- +goose Up
ALTER TABLE user_llm_keys ADD COLUMN protocol TEXT NOT NULL DEFAULT 'openai'
    CHECK (protocol IN ('openai', 'anthropic'));

-- +goose Down
ALTER TABLE user_llm_keys DROP COLUMN protocol;
