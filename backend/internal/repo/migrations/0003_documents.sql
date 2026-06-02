-- +goose Up
-- +goose StatementBegin
CREATE TABLE documents (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id         UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    source_type   TEXT NOT NULL,
    source_ref    TEXT NOT NULL,
    title         TEXT NOT NULL,
    mime_type     TEXT NOT NULL,
    bytes         BIGINT NOT NULL,
    checksum      TEXT NOT NULL,
    status        TEXT NOT NULL,
    error_message TEXT,
    metadata      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_docs_kb_status ON documents (kb_id, status);
CREATE UNIQUE INDEX uniq_docs_kb_checksum ON documents (kb_id, checksum);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS documents;
-- +goose StatementEnd
