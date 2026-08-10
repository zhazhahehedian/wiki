-- +goose Up
-- +goose StatementBegin
ALTER TABLE documents
    ADD CONSTRAINT uq_documents_id_kb_id UNIQUE (id, kb_id);

ALTER TABLE chunks
    DROP CONSTRAINT IF EXISTS chunks_document_id_fkey;

-- NOT VALID keeps constraint installation short while blocking new mismatches.
-- VALIDATE then checks every existing chunk before the migration commits.
ALTER TABLE chunks
    ADD CONSTRAINT fk_chunks_document_kb
        FOREIGN KEY (document_id, kb_id)
        REFERENCES documents (id, kb_id)
        ON DELETE CASCADE
        NOT VALID;

ALTER TABLE chunks
    VALIDATE CONSTRAINT fk_chunks_document_kb;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE chunks
    DROP CONSTRAINT IF EXISTS fk_chunks_document_kb;

ALTER TABLE chunks
    ADD CONSTRAINT chunks_document_id_fkey
        FOREIGN KEY (document_id)
        REFERENCES documents (id)
        ON DELETE CASCADE;

ALTER TABLE documents
    DROP CONSTRAINT IF EXISTS uq_documents_id_kb_id;
-- +goose StatementEnd
