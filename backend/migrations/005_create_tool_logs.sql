-- +goose Up
CREATE TABLE tool_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
  step_id UUID REFERENCES task_steps(id) ON DELETE SET NULL,
  tool_name TEXT NOT NULL,
  input_json JSONB,
  output_json JSONB,
  duration_ms INTEGER,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tool_logs_task_id ON tool_logs(task_id);
CREATE INDEX idx_tool_logs_step_id ON tool_logs(step_id);
CREATE INDEX idx_tool_logs_tool_name ON tool_logs(tool_name);

-- +goose Down
DROP TABLE IF EXISTS tool_logs;