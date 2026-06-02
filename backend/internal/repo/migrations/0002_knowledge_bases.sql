-- +goose Up
-- +goose StatementBegin
CREATE TABLE knowledge_bases (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    owner_id      TEXT NOT NULL DEFAULT 'local-admin',
    embed_model   TEXT NOT NULL,
    embed_dim     INT  NOT NULL,
    settings      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_kbs_owner ON knowledge_bases (owner_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS knowledge_bases;
-- +goose StatementEnd
