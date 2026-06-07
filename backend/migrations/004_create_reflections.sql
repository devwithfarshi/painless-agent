-- +goose Up
CREATE TABLE reflections (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  task_id UUID REFERENCES tasks(id) ON DELETE CASCADE,
  lessons_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  rating INTEGER CHECK (rating >= 1 AND rating <= 10),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_reflections_task_id ON reflections(task_id);
CREATE INDEX idx_reflections_rating ON reflections(rating);

-- +goose Down
DROP TABLE IF EXISTS reflections;