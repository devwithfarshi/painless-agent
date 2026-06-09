-- +goose Up
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS error_msg TEXT;

-- +goose Down
ALTER TABLE tasks DROP COLUMN IF EXISTS error_msg;
