package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ToolLogStore persists tool call records to the tool_logs table.
type ToolLogStore struct {
	pool *pgxpool.Pool
}

func NewToolLogStore(pool *pgxpool.Pool) *ToolLogStore {
	return &ToolLogStore{pool: pool}
}

// Log inserts a tool_logs row. input and output are serialised to JSONB.
func (s *ToolLogStore) Log(
	ctx context.Context,
	taskID, stepID uuid.UUID,
	toolName string,
	input map[string]any,
	output string,
	durationMs int,
) error {
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("marshal tool input: %w", err)
	}
	outputJSON, err := json.Marshal(map[string]string{"content": output})
	if err != nil {
		return fmt.Errorf("marshal tool output: %w", err)
	}

	var taskPtr, stepPtr *uuid.UUID
	if taskID != uuid.Nil {
		taskPtr = &taskID
	}
	if stepID != uuid.Nil {
		stepPtr = &stepID
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO tool_logs (task_id, step_id, tool_name, input_json, output_json, duration_ms)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		taskPtr, stepPtr, toolName, inputJSON, outputJSON, durationMs,
	)
	if err != nil {
		return fmt.Errorf("insert tool log: %w", err)
	}
	return nil
}
