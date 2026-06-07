-- +goose Up
CREATE TABLE memory (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  content TEXT NOT NULL,
  embedding VECTOR(1536),
  tags TEXT[] DEFAULT '{}',
  task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_memory_task_id ON memory(task_id);
CREATE INDEX idx_memory_tags ON memory USING GIN(tags);

CREATE INDEX idx_memory_embedding
ON memory
USING ivfflat (embedding vector_cosine_ops)
WITH (lists = 100);

-- +goose Down
DROP INDEX IF EXISTS idx_memory_embedding;
DROP INDEX IF EXISTS idx_memory_tags;
DROP INDEX IF EXISTS idx_memory_task_id;
DROP TABLE IF EXISTS memory;