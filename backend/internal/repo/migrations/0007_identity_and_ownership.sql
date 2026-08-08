-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_name TEXT NOT NULL DEFAULT '',
    avatar_url   TEXT NOT NULL DEFAULT '',
    email        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE oauth_accounts (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider                 TEXT NOT NULL,
    provider_user_id         TEXT NOT NULL,
    tenant_key               TEXT NOT NULL,
    access_token_encrypted   BYTEA NOT NULL,
    refresh_token_encrypted  BYTEA,
    access_token_expires_at  TIMESTAMPTZ NOT NULL,
    refresh_token_expires_at TIMESTAMPTZ,
    scopes                   TEXT[] NOT NULL DEFAULT '{}',
    reauth_required          BOOLEAN NOT NULL DEFAULT false,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uniq_oauth_provider_user UNIQUE (provider, provider_user_id),
    CONSTRAINT uniq_oauth_user_provider UNIQUE (user_id, provider)
);
CREATE INDEX idx_oauth_accounts_tenant
    ON oauth_accounts (provider, tenant_key);

CREATE TABLE user_sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash      BYTEA NOT NULL UNIQUE,
    csrf_token_hash BYTEA NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_user_sessions_user_expires
    ON user_sessions (user_id, expires_at);
CREATE INDEX idx_user_sessions_expires
    ON user_sessions (expires_at);

ALTER TABLE knowledge_bases
    ADD COLUMN owner_user_id UUID REFERENCES users(id);
CREATE INDEX idx_kbs_owner_user_created
    ON knowledge_bases (owner_user_id, created_at DESC);

ALTER TABLE conversations
    ADD COLUMN owner_user_id UUID REFERENCES users(id);
CREATE INDEX idx_conv_owner_user_updated
    ON conversations (owner_user_id, updated_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_conv_owner_user_updated;
ALTER TABLE conversations DROP COLUMN IF EXISTS owner_user_id;

DROP INDEX IF EXISTS idx_kbs_owner_user_created;
ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS owner_user_id;

DROP TABLE IF EXISTS user_sessions;
DROP TABLE IF EXISTS oauth_accounts;
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
