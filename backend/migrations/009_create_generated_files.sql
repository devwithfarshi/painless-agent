-- +goose Up
CREATE TABLE IF NOT EXISTS generated_files (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id     UUID        NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    file_name   TEXT        NOT NULL,
    file_path   TEXT        NOT NULL,
    file_size   BIGINT      NOT NULL DEFAULT 0,
    mime_type   TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS generated_files_task_id_idx ON generated_files(task_id);

-- +goose Down
DROP TABLE IF EXISTS generated_files;
