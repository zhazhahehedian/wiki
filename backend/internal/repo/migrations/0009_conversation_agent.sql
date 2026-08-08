-- +goose Up
-- +goose StatementBegin
ALTER TABLE conversations
    ADD COLUMN agent_id TEXT NOT NULL DEFAULT 'knowledge-rag';

CREATE INDEX idx_conv_owner_agent_updated
    ON conversations (owner_user_id, agent_id, updated_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_conv_owner_agent_updated;
ALTER TABLE conversations DROP COLUMN IF EXISTS agent_id;
-- +goose StatementEnd
