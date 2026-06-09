-- +goose Up
ALTER TABLE skills ADD COLUMN IF NOT EXISTS embedding vector(1536);
CREATE INDEX IF NOT EXISTS idx_skills_embedding ON skills USING ivfflat (embedding vector_cosine_ops) WITH (lists = 50);

-- +goose Down
DROP INDEX IF EXISTS idx_skills_embedding;
ALTER TABLE skills DROP COLUMN IF EXISTS embedding;
