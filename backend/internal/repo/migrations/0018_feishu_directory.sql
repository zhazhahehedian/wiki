-- +goose Up
ALTER TABLE capabilities ADD COLUMN department_id TEXT NOT NULL DEFAULT '';
ALTER TABLE knowledge_bases ADD COLUMN department_id TEXT NOT NULL DEFAULT '';
-- Membership is fetched for the authenticated subject, never from directory browsing.
CREATE TABLE feishu_memberships (
    account_id UUID PRIMARY KEY REFERENCES oauth_accounts(id) ON DELETE CASCADE,
    department_ids TEXT[] NOT NULL DEFAULT '{}',
    checked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE feishu_memberships;
ALTER TABLE capabilities DROP COLUMN department_id;
ALTER TABLE knowledge_bases DROP COLUMN department_id;
