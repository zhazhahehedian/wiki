-- +goose Up
-- +goose StatementBegin
ALTER TABLE documents
    ADD COLUMN pending_title TEXT,
    ADD COLUMN pending_bytes BIGINT,
    ADD COLUMN pending_metadata JSONB;

-- Pre-0010 pending rows do not contain enough payload to promote safely.
-- Leave them syncing without pending state so reconciliation reloads the source.
UPDATE documents
SET pending_content_ref = NULL,
    pending_checksum = NULL,
    pending_remote_revision = NULL
WHERE pending_content_ref IS NOT NULL
   OR pending_checksum IS NOT NULL
   OR pending_remote_revision IS NOT NULL;

ALTER TABLE documents
    ADD CONSTRAINT chk_documents_pending_snapshot_complete
        CHECK (
            (
                pending_content_ref IS NULL
                AND pending_checksum IS NULL
                AND pending_remote_revision IS NULL
                AND pending_title IS NULL
                AND pending_bytes IS NULL
                AND pending_metadata IS NULL
            )
            OR
            (
                pending_content_ref IS NOT NULL
                AND pending_checksum IS NOT NULL
                AND pending_remote_revision IS NOT NULL
                AND pending_title IS NOT NULL
                AND pending_bytes IS NOT NULL
                AND pending_bytes >= 0
                AND pending_metadata IS NOT NULL
            )
        );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE documents
    DROP CONSTRAINT IF EXISTS chk_documents_pending_snapshot_complete,
    DROP COLUMN IF EXISTS pending_metadata,
    DROP COLUMN IF EXISTS pending_bytes,
    DROP COLUMN IF EXISTS pending_title;
-- +goose StatementEnd
