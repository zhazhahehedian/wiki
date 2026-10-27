-- name: KnowledgeListBases :many
SELECT * FROM knowledge_bases WHERE deleted_at IS NULL ORDER BY created_at DESC;

-- name: KnowledgeGetBase :one
SELECT * FROM knowledge_bases WHERE id=$1 AND deleted_at IS NULL;

-- name: KnowledgeLockBase :one
SELECT * FROM knowledge_bases WHERE id=$1 AND deleted_at IS NULL FOR UPDATE;

-- name: KnowledgeShareLockBase :one
SELECT * FROM knowledge_bases WHERE id=$1 AND deleted_at IS NULL FOR SHARE;

-- name: KnowledgeBaseReadiness :one
SELECT EXISTS(SELECT 1 FROM documents d WHERE d.kb_id=$1 AND d.deleted_at IS NULL)::boolean AS has_documents,
EXISTS(SELECT 1 FROM documents a WHERE a.kb_id=$1 AND a.deleted_at IS NULL AND a.active_version_id IS NOT NULL)::boolean AS has_active;

-- name: KnowledgeSaveBase :one
INSERT INTO knowledge_bases(id,name,description,owner_user_id,embed_model,embed_dim,visibility,department,members,application_shared,department_id)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT(id) DO UPDATE SET name=excluded.name,description=excluded.description,visibility=excluded.visibility,
department=excluded.department,department_id=excluded.department_id,members=excluded.members,application_shared=excluded.application_shared,updated_at=now()
RETURNING *;

-- name: KnowledgeDeleteBase :exec
UPDATE knowledge_bases SET deleted_at=now() WHERE id=$1;

-- name: KnowledgeDocuments :many
SELECT * FROM documents WHERE kb_id=$1 AND deleted_at IS NULL ORDER BY created_at DESC;

-- name: KnowledgeDocument :one
SELECT * FROM documents WHERE id=$1 AND deleted_at IS NULL;

-- name: KnowledgeActiveDocuments :many
SELECT d.id,d.active_version_id FROM documents d
JOIN knowledge_bases b ON b.id=d.kb_id
WHERE d.id=ANY(sqlc.arg(document_ids)::uuid[]) AND d.kb_id=ANY(sqlc.arg(kb_ids)::uuid[])
AND d.deleted_at IS NULL AND b.deleted_at IS NULL;

-- name: KnowledgeLockDocument :one
SELECT * FROM documents WHERE id=$1 AND deleted_at IS NULL FOR UPDATE;

-- name: KnowledgeCreateDocument :one
INSERT INTO documents(id,kb_id,source_type,source_ref,title,mime_type,bytes,checksum,status)
VALUES($1,$2,'local-upload',$3,$4,$5,$6,$7,'pending') RETURNING *;

-- name: KnowledgeCreateVersion :exec
INSERT INTO knowledge_versions(id,document_id,title,object_key,checksum,embed_model,embed_dim)
VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: KnowledgeDuplicateChecksum :one
SELECT EXISTS(SELECT 1 FROM documents d WHERE d.kb_id=$1 AND d.id<>$2 AND d.deleted_at IS NULL
AND (d.checksum=$3 OR EXISTS(SELECT 1 FROM knowledge_versions v WHERE v.id=d.pending_version_id AND v.checksum=$3)))::boolean;

-- name: KnowledgeStageVersion :exec
UPDATE documents SET pending_version_id=$2,status='pending',error_message=NULL,updated_at=now() WHERE id=$1;

-- name: KnowledgeRetryDocument :execrows
UPDATE documents SET status='pending',error_message=NULL,updated_at=now()
WHERE id=$1 AND pending_version_id=$2 AND status='failed' AND deleted_at IS NULL;

-- name: KnowledgeVersion :one
SELECT v.*,d.kb_id FROM knowledge_versions v JOIN documents d ON d.id=v.document_id
JOIN knowledge_bases k ON k.id=d.kb_id
WHERE v.id=$1 AND d.deleted_at IS NULL AND k.deleted_at IS NULL;

-- name: KnowledgePreview :exec
UPDATE knowledge_versions SET preview=$2,status='processing',error_code='' WHERE id=$1 AND status<>'ready';

-- name: KnowledgeDocumentProcessing :exec
UPDATE documents SET status='processing',error_message=NULL WHERE pending_version_id=$1 AND deleted_at IS NULL;

-- name: KnowledgeVersionReady :exec
UPDATE knowledge_versions SET status='ready',error_code='' WHERE id=$1;

-- name: KnowledgeActivate :execrows
UPDATE documents SET active_version_id=$2,pending_version_id=NULL,status='ready',error_message=NULL,
title=$3,checksum=$4,updated_at=now() WHERE id=$1 AND pending_version_id=$2 AND deleted_at IS NULL;

-- name: KnowledgeVersionFail :exec
UPDATE knowledge_versions SET status='failed',error_code=$2 WHERE id=$1 AND status<>'ready';

-- name: KnowledgeDocumentFail :exec
UPDATE documents SET status='failed',error_message=$2,updated_at=now() WHERE pending_version_id=$1 AND deleted_at IS NULL;

-- name: KnowledgeDeleteDocument :exec
UPDATE documents SET deleted_at=now(),pending_version_id=NULL WHERE id=$1;

-- name: KnowledgeLockApplication :one
SELECT id FROM knowledge_applications WHERE id=$1 FOR UPDATE;

-- name: KnowledgeApplications :many
SELECT * FROM knowledge_applications ORDER BY created_at DESC;

-- name: KnowledgeSaveApplication :one
INSERT INTO knowledge_applications(id,name,enabled) VALUES($1,$2,$3)
ON CONFLICT(id) DO UPDATE SET name=excluded.name,enabled=excluded.enabled RETURNING *;

-- name: KnowledgeGrants :many
SELECT kb_id FROM knowledge_grants WHERE application_id=$1;

-- name: KnowledgeClearGrants :exec
DELETE FROM knowledge_grants WHERE application_id=$1;

-- name: KnowledgeGrant :exec
INSERT INTO knowledge_grants(application_id,kb_id) VALUES($1,$2);

-- name: KnowledgeRevokeBaseGrants :exec
DELETE FROM knowledge_grants WHERE kb_id=$1;

-- name: KnowledgeRevoke :exec
UPDATE knowledge_credentials SET revoked_at=now() WHERE application_id=$1 AND revoked_at IS NULL;

-- name: KnowledgeIssue :exec
INSERT INTO knowledge_credentials(application_id,digest,prefix,expires_at) VALUES($1,$2,$3,$4);

-- name: KnowledgeCredential :one
SELECT c.* FROM knowledge_credentials c JOIN knowledge_applications a ON a.id=c.application_id
WHERE c.digest=$1 AND c.revoked_at IS NULL AND c.expires_at>now() AND a.enabled;

-- name: KnowledgeCredentialCurrent :one
SELECT EXISTS(SELECT 1 FROM knowledge_credentials c JOIN knowledge_applications a ON a.id=c.application_id
WHERE c.id=$1 AND c.application_id=$2 AND c.revoked_at IS NULL AND c.expires_at>now() AND a.enabled
AND 'knowledge:search'=ANY(c.scopes))::boolean;

-- name: KnowledgeCredentialUsed :exec
UPDATE knowledge_credentials SET last_used_at=now() WHERE id=$1;

-- name: KnowledgeLatestCredential :one
SELECT prefix,expires_at,last_used_at FROM knowledge_credentials
WHERE application_id=$1 AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 1;

-- name: KnowledgeAudit :exec
INSERT INTO knowledge_search_audit(application_id,user_id,request_id,status,result_count,duration_ms,knowledge_base_ids)
VALUES(sqlc.narg(application_id)::uuid,sqlc.narg(user_id)::uuid,sqlc.arg(request_id),sqlc.arg(status),sqlc.arg(result_count),sqlc.arg(duration_ms),sqlc.arg(knowledge_base_ids));

-- name: KnowledgeAuditEntries :many
SELECT request_id,application_id,status,result_count,duration_ms,created_at,knowledge_base_ids
FROM knowledge_search_audit ORDER BY id DESC LIMIT 100;
