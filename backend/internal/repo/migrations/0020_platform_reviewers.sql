-- +goose Up
ALTER TABLE platform_profiles ADD COLUMN is_reviewer BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE platform_profiles DROP COLUMN is_reviewer;
