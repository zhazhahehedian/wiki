-- +goose Up
-- Browser uploads of large MCP packages in parts. Rows are transient: a
-- completed upload is consumed by a capability save or expires; the stored
-- object is then owned by capability_versions or removed by the orphan sweep.
CREATE TABLE capability_bundle_uploads (
    id                UUID PRIMARY KEY,
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_name         TEXT NOT NULL CHECK (length(file_name) BETWEEN 5 AND 255),
    size              BIGINT NOT NULL CHECK (size > 0),
    part_size         BIGINT NOT NULL CHECK (part_size > 0),
    object_key        TEXT NOT NULL UNIQUE,
    storage_upload_id TEXT NOT NULL,
    bundle_sha256     TEXT CHECK (bundle_sha256 ~ '^[0-9a-f]{64}$'),
    bundle_findings   JSONB CHECK (jsonb_typeof(bundle_findings) = 'array'),
    completed_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at        TIMESTAMPTZ NOT NULL,
    CHECK ((completed_at IS NULL) = (bundle_sha256 IS NULL)),
    CHECK (bundle_findings IS NULL OR completed_at IS NOT NULL)
);
CREATE INDEX capability_bundle_uploads_user_idx ON capability_bundle_uploads (user_id, created_at);
CREATE INDEX capability_bundle_uploads_expiry_idx ON capability_bundle_uploads (expires_at);

-- +goose Down
-- Only in-progress upload sessions are lost; users re-upload. Unfinished
-- storage parts are aborted by MinIO's stale-upload cleanup, and completed but
-- unsaved objects by the registry orphan sweep.
DROP TABLE capability_bundle_uploads;
