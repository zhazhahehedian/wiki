-- +goose Up
ALTER TABLE capability_versions DROP CONSTRAINT capability_versions_check;
ALTER TABLE capability_versions DROP CONSTRAINT capability_versions_mcp_transport_check;
ALTER TABLE capability_versions ADD CONSTRAINT capability_versions_mcp_transport_check
    CHECK (mcp_transport IN ('streamable-http', 'stdio'));
-- skill_bundle_key is the historical name of the shared immutable bundle reference.
ALTER TABLE capability_versions ADD CONSTRAINT capability_versions_check CHECK (
    (mcp_transport IS NOT NULL AND mcp_auth_scheme IS NOT NULL AND tools IS NOT NULL
        AND skill_manifest IS NULL AND (mcp_endpoint IS NOT NULL OR skill_bundle_key IS NOT NULL)
        AND (mcp_transport <> 'stdio' OR (mcp_endpoint IS NULL AND mcp_auth_scheme = 'none')))
    OR (mcp_endpoint IS NULL AND mcp_transport IS NULL AND mcp_auth_scheme IS NULL
        AND tools IS NULL AND skill_bundle_key IS NOT NULL AND skill_manifest IS NOT NULL)
);

-- +goose Down
-- Never discard packages or rewrite published versions to make rollback succeed.
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM capability_versions WHERE
        (skill_manifest IS NULL AND skill_bundle_key IS NOT NULL) OR mcp_transport = 'stdio') THEN
        RAISE EXCEPTION 'Cannot roll back MCP packages: package/stdio versions must be preserved; keep migration 0021';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE capability_versions DROP CONSTRAINT capability_versions_check;
ALTER TABLE capability_versions DROP CONSTRAINT capability_versions_mcp_transport_check;
ALTER TABLE capability_versions ADD CONSTRAINT capability_versions_mcp_transport_check
    CHECK (mcp_transport = 'streamable-http');
ALTER TABLE capability_versions ADD CONSTRAINT capability_versions_check CHECK (
    (mcp_endpoint IS NOT NULL AND mcp_transport IS NOT NULL AND mcp_auth_scheme IS NOT NULL
        AND tools IS NOT NULL AND skill_bundle_key IS NULL AND skill_manifest IS NULL)
    OR (mcp_endpoint IS NULL AND mcp_transport IS NULL AND mcp_auth_scheme IS NULL
        AND tools IS NULL AND skill_bundle_key IS NOT NULL AND skill_manifest IS NOT NULL)
);
