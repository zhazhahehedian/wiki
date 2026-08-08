-- +goose Up
-- +goose StatementBegin
ALTER TABLE documents
    ADD COLUMN content_ref TEXT,
    ADD COLUMN source_url TEXT,
    ADD COLUMN remote_revision TEXT,
    ADD COLUMN oauth_account_id UUID REFERENCES oauth_accounts(id) ON DELETE SET NULL,
    ADD COLUMN pending_content_ref TEXT,
    ADD COLUMN pending_checksum TEXT,
    ADD COLUMN pending_remote_revision TEXT,
    ADD COLUMN sync_status TEXT NOT NULL DEFAULT 'idle',
    ADD COLUMN last_sync_error TEXT,
    ADD COLUMN last_synced_at TIMESTAMPTZ,
    ADD CONSTRAINT chk_documents_sync_status
        CHECK (sync_status IN ('idle', 'syncing', 'failed'));

UPDATE documents
SET content_ref = source_ref
WHERE source_type = 'local-upload';

ALTER TABLE documents
    ADD CONSTRAINT uniq_docs_kb_source
        UNIQUE (kb_id, source_type, source_ref);

CREATE INDEX idx_docs_kb_sync_status
    ON documents (kb_id, sync_status);
CREATE INDEX idx_docs_oauth_sync_status
    ON documents (oauth_account_id, sync_status)
    WHERE oauth_account_id IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_docs_oauth_sync_status;
DROP INDEX IF EXISTS idx_docs_kb_sync_status;

ALTER TABLE documents
    DROP CONSTRAINT IF EXISTS uniq_docs_kb_source,
    DROP CONSTRAINT IF EXISTS chk_documents_sync_status,
    DROP COLUMN IF EXISTS last_synced_at,
    DROP COLUMN IF EXISTS last_sync_error,
    DROP COLUMN IF EXISTS sync_status,
    DROP COLUMN IF EXISTS pending_remote_revision,
    DROP COLUMN IF EXISTS pending_checksum,
    DROP COLUMN IF EXISTS pending_content_ref,
    DROP COLUMN IF EXISTS oauth_account_id,
    DROP COLUMN IF EXISTS remote_revision,
    DROP COLUMN IF EXISTS source_url,
    DROP COLUMN IF EXISTS content_ref;
-- +goose StatementEnd
