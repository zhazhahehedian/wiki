-- +goose Up
-- +goose StatementBegin
CREATE TABLE conversations (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id         UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    title         TEXT NOT NULL DEFAULT '新对话',
    mode          TEXT NOT NULL,
    user_id       TEXT NOT NULL DEFAULT 'local-admin',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_conv_kb_user_updated ON conversations (kb_id, user_id, updated_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS conversations;
-- +goose StatementEnd
