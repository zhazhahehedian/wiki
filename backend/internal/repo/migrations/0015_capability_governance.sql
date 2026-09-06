-- +goose Up
-- Profiles are provisioned by trusted administrators, never OAuth browser input.
CREATE TABLE platform_profiles (
    open_id TEXT PRIMARY KEY CHECK (length(open_id) BETWEEN 1 AND 128),
    is_admin BOOLEAN NOT NULL DEFAULT false,
    department TEXT NOT NULL DEFAULT '' CHECK (length(department) <= 120),
    revision BIGINT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- A consumed bootstrap cannot silently regrant a subsequently revoked role.
CREATE TABLE platform_admin_bootstrap (
    singleton BOOLEAN PRIMARY KEY DEFAULT true CHECK (singleton),
    open_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE capabilities
    ADD COLUMN is_live BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN published_metadata JSONB,
    ADD COLUMN review_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE capability_versions ADD COLUMN published_at TIMESTAMPTZ;
UPDATE capabilities c SET is_live = true, published_metadata = jsonb_build_object(
    'name', c.name, 'description', c.description, 'department', c.department,
    'visibility', c.visibility, 'allowlist', COALESCE((SELECT jsonb_agg(a.open_id ORDER BY a.open_id)
        FROM capability_allowlist a WHERE a.capability_id=c.id), '[]'::jsonb))
WHERE c.status='published' AND c.current_version_id IS NOT NULL;
UPDATE capability_versions v SET published_at=c.updated_at FROM capabilities c WHERE c.current_version_id=v.id;
ALTER TABLE capabilities ADD CONSTRAINT capabilities_live_snapshot CHECK (
    NOT is_live OR (current_version_id IS NOT NULL AND published_metadata IS NOT NULL));
CREATE TABLE audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_open_id TEXT NOT NULL,
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    detail JSONB NOT NULL,
    request_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_time ON audit_log(created_at DESC, id DESC);
CREATE INDEX audit_log_target ON audit_log(target_id, created_at DESC, id DESC);
CREATE INDEX capabilities_review ON capabilities(updated_at, id) WHERE status='in_review';

-- +goose Down
DROP TABLE audit_log;
DROP INDEX capabilities_review;
ALTER TABLE capabilities DROP CONSTRAINT capabilities_live_snapshot;
ALTER TABLE capabilities DROP COLUMN is_live, DROP COLUMN published_metadata, DROP COLUMN review_reason;
ALTER TABLE capability_versions DROP COLUMN published_at;
DROP TABLE platform_admin_bootstrap;
DROP TABLE platform_profiles;
