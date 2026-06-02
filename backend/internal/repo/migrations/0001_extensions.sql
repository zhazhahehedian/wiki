-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP EXTENSION IF EXISTS vector;
DROP EXTENSION IF EXISTS pgcrypto;
-- +goose StatementEnd
