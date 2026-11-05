-- +goose Up
-- The column now stores MCP and Skill bundles.
ALTER TABLE capability_versions RENAME COLUMN skill_bundle_key TO bundle_key;
-- Bundles uploaded before 0022 keep NULL digests and findings; they are not recomputed.
ALTER TABLE capability_versions
    ADD COLUMN bundle_sha256 TEXT CHECK (bundle_sha256 ~ '^[0-9a-f]{64}$'),
    ADD COLUMN bundle_size BIGINT CHECK (bundle_size > 0),
    ADD COLUMN bundle_findings JSONB CHECK (jsonb_typeof(bundle_findings) = 'array');
ALTER TABLE capability_versions ADD CONSTRAINT capability_versions_bundle_digest_check CHECK (
    (bundle_sha256 IS NULL) = (bundle_size IS NULL)
    AND (bundle_sha256 IS NULL OR bundle_key IS NOT NULL)
    AND (bundle_findings IS NULL OR bundle_key IS NOT NULL)
);

-- +goose Down
-- Digests and findings are derived from the stored bundle; packages are kept.
ALTER TABLE capability_versions DROP CONSTRAINT capability_versions_bundle_digest_check;
ALTER TABLE capability_versions DROP COLUMN bundle_findings, DROP COLUMN bundle_size, DROP COLUMN bundle_sha256;
ALTER TABLE capability_versions RENAME COLUMN bundle_key TO skill_bundle_key;
