-- +goose Up
-- Existing knowledge and vectors remain private and untouched.
ALTER TABLE knowledge_bases
 ADD COLUMN visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','company','department','members')),
 ADD COLUMN department TEXT NOT NULL DEFAULT '',
 ADD COLUMN members TEXT[] NOT NULL DEFAULT '{}',
 ADD COLUMN application_shared BOOLEAN NOT NULL DEFAULT false,
 ADD COLUMN deleted_at TIMESTAMPTZ;
CREATE TABLE knowledge_versions (
 id UUID PRIMARY KEY, document_id UUID NOT NULL REFERENCES documents(id),
 title TEXT NOT NULL, object_key TEXT NOT NULL, checksum TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','ready','failed')),
 error_code TEXT NOT NULL DEFAULT '', preview JSONB NOT NULL DEFAULT '{"blocks":[],"diagnostics":[]}',
 embed_model TEXT NOT NULL, embed_dim INT NOT NULL CHECK (embed_dim>0),
 parser_version TEXT NOT NULL DEFAULT 'k1-1', created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(document_id,id)
);
ALTER TABLE documents ADD COLUMN active_version_id UUID,
 ADD COLUMN pending_version_id UUID, ADD COLUMN deleted_at TIMESTAMPTZ,
 ADD CONSTRAINT active_knowledge_version FOREIGN KEY(id,active_version_id) REFERENCES knowledge_versions(document_id,id),
 ADD CONSTRAINT pending_knowledge_version FOREIGN KEY(id,pending_version_id) REFERENCES knowledge_versions(document_id,id);
DROP INDEX uniq_docs_kb_local_checksum;
CREATE UNIQUE INDEX uniq_docs_kb_local_checksum ON documents(kb_id,checksum)
 WHERE source_type='local-upload' AND deleted_at IS NULL;
CREATE TABLE knowledge_chunks (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), version_id UUID NOT NULL REFERENCES knowledge_versions(id),
 seq INT NOT NULL, content TEXT NOT NULL, locator JSONB NOT NULL,
 embedding vector NOT NULL, UNIQUE(version_id,seq)
);
CREATE INDEX knowledge_chunks_version ON knowledge_chunks(version_id);
CREATE TABLE knowledge_applications (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 120),
 enabled BOOLEAN NOT NULL DEFAULT true, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE knowledge_credentials (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), application_id UUID NOT NULL REFERENCES knowledge_applications(id),
 digest BYTEA NOT NULL UNIQUE CHECK(octet_length(digest)=32), prefix TEXT NOT NULL,
 scopes TEXT[] NOT NULL DEFAULT '{knowledge:search}', expires_at TIMESTAMPTZ NOT NULL,
 revoked_at TIMESTAMPTZ, last_used_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE knowledge_grants (
 application_id UUID NOT NULL REFERENCES knowledge_applications(id),
 kb_id UUID NOT NULL REFERENCES knowledge_bases(id), PRIMARY KEY(application_id,kb_id)
);
CREATE TABLE knowledge_search_audit (
 id BIGSERIAL PRIMARY KEY, application_id UUID REFERENCES knowledge_applications(id), user_id UUID REFERENCES users(id),
 request_id TEXT NOT NULL, status TEXT NOT NULL, result_count INT NOT NULL, duration_ms BIGINT NOT NULL,
 knowledge_base_ids UUID[] NOT NULL DEFAULT '{}',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX knowledge_search_audit_time ON knowledge_search_audit(created_at DESC);

-- +goose Down
-- Refuse rollback rather than deleting data if K1 soft-deleted duplicates exist.
DROP INDEX uniq_docs_kb_local_checksum;
CREATE UNIQUE INDEX uniq_docs_kb_local_checksum ON documents(kb_id,checksum) WHERE source_type='local-upload';
DROP TABLE knowledge_search_audit, knowledge_grants, knowledge_credentials, knowledge_applications, knowledge_chunks;
ALTER TABLE documents DROP CONSTRAINT active_knowledge_version, DROP CONSTRAINT pending_knowledge_version,
 DROP COLUMN active_version_id, DROP COLUMN pending_version_id, DROP COLUMN deleted_at;
DROP TABLE knowledge_versions;
ALTER TABLE knowledge_bases DROP COLUMN visibility, DROP COLUMN department, DROP COLUMN members,
 DROP COLUMN application_shared, DROP COLUMN deleted_at;
