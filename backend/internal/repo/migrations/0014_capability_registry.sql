-- +goose Up
CREATE TABLE capabilities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug TEXT NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(slug) <= 80),
    type TEXT NOT NULL CHECK (type IN ('mcp', 'skill')),
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    description TEXT NOT NULL,
    owner_open_id TEXT NOT NULL,
    department TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'in_review', 'published', 'offline')),
    visibility TEXT NOT NULL CHECK (visibility IN ('org', 'department', 'allowlist')),
    current_version_id UUID,
    draft_version_id UUID,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX capabilities_owner_updated ON capabilities(owner_open_id, updated_at DESC, id);
CREATE TABLE capability_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    capability_id UUID NOT NULL REFERENCES capabilities(id) ON DELETE CASCADE,
    version TEXT NOT NULL CHECK (length(version) BETWEEN 1 AND 64),
    changelog TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    mcp_endpoint TEXT,
    mcp_transport TEXT CHECK (mcp_transport = 'streamable-http'),
    mcp_auth_scheme TEXT,
    tools JSONB,
    skill_bundle_key TEXT,
    skill_manifest JSONB,
    UNIQUE(capability_id, version),
    UNIQUE(capability_id, id),
    CHECK ((mcp_endpoint IS NOT NULL AND mcp_transport IS NOT NULL AND mcp_auth_scheme IS NOT NULL AND tools IS NOT NULL AND skill_bundle_key IS NULL AND skill_manifest IS NULL)
        OR (mcp_endpoint IS NULL AND mcp_transport IS NULL AND mcp_auth_scheme IS NULL AND tools IS NULL AND skill_bundle_key IS NOT NULL AND skill_manifest IS NOT NULL))
);
ALTER TABLE capabilities ADD CONSTRAINT capabilities_current_version_fk
    FOREIGN KEY (id, current_version_id) REFERENCES capability_versions(capability_id, id);
ALTER TABLE capabilities ADD CONSTRAINT capabilities_draft_version_fk
    FOREIGN KEY (id, draft_version_id) REFERENCES capability_versions(capability_id, id);
CREATE TABLE capability_allowlist (
    capability_id UUID NOT NULL REFERENCES capabilities(id) ON DELETE CASCADE,
    open_id TEXT NOT NULL,
    PRIMARY KEY(capability_id, open_id)
);

-- +goose Down
ALTER TABLE capabilities DROP CONSTRAINT capabilities_draft_version_fk;
ALTER TABLE capabilities DROP CONSTRAINT capabilities_current_version_fk;
DROP TABLE capability_allowlist;
DROP TABLE capability_versions;
DROP TABLE capabilities;
