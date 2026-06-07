package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/devwithfarshi/painless-agent/internal/types"
)

// TaskStore handles persistence for tasks and task_steps.
type TaskStore struct {
	pool *pgxpool.Pool
}

func NewTaskStore(pool *pgxpool.Pool) *TaskStore {
	return &TaskStore{pool: pool}
}

func (s *TaskStore) CreateTask(ctx context.Context, goal string) (types.Task, error) {
	var t types.Task
	err := s.pool.QueryRow(ctx,
		`INSERT INTO tasks (goal) VALUES ($1)
		 RETURNING id, goal, status, created_at, updated_at`,
		goal,
	).Scan(&t.ID, &t.Goal, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return t, fmt.Errorf("create task: %w", err)
	}
	return t, nil
}

func (s *TaskStore) GetTask(ctx context.Context, id uuid.UUID) (types.Task, error) {
	var t types.Task
	var planJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, goal, status, plan_json, created_at, updated_at FROM tasks WHERE id = $1`,
		id,
	).Scan(&t.ID, &t.Goal, &t.Status, &planJSON, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return t, fmt.Errorf("get task %s: %w", id, err)
	}
	if len(planJSON) > 0 {
		if err := json.Unmarshal(planJSON, &t.Plan); err != nil {
			return t, fmt.Errorf("unmarshal plan for task %s: %w", id, err)
		}
	}
	return t, nil
}

func (s *TaskStore) ListTasks(ctx context.Context) ([]types.Task, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, goal, status, plan_json, created_at, updated_at FROM tasks ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []types.Task
	for rows.Next() {
		var t types.Task
		var planJSON []byte
		if err := rows.Scan(&t.ID, &t.Goal, &t.Status, &planJSON, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan task row: %w", err)
		}
		if len(planJSON) > 0 {
			_ = json.Unmarshal(planJSON, &t.Plan)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (s *TaskStore) UpdateStatus(ctx context.Context, id uuid.UUID, status types.TaskStatus) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE tasks SET status = $1 WHERE id = $2`,
		status, id,
	)
	if err != nil {
		return fmt.Errorf("update task status: %w", err)
	}
	return nil
}

func (s *TaskStore) SetPlan(ctx context.Context, id uuid.UUID, plan []types.PlanStep) error {
	b, err := json.Marshal(plan)
	if err != nil {
		return fmt.Errorf("marshal plan: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE tasks SET plan_json = $1 WHERE id = $2`,
		b, id,
	)
	if err != nil {
		return fmt.Errorf("set plan: %w", err)
	}
	return nil
}

func (s *TaskStore) CreateStep(ctx context.Context, taskID uuid.UUID, description string) (types.TaskStep, error) {
	var st types.TaskStep
	err := s.pool.QueryRow(ctx,
		`INSERT INTO task_steps (task_id, description) VALUES ($1, $2)
		 RETURNING id, task_id, description, status, created_at, updated_at`,
		taskID, description,
	).Scan(&st.ID, &st.TaskID, &st.Description, &st.Status, &st.CreatedAt, &st.UpdatedAt)
	if err != nil {
		return st, fmt.Errorf("create step: %w", err)
	}
	return st, nil
}

func (s *TaskStore) UpdateStep(ctx context.Context, id uuid.UUID, status types.StepStatus, output string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE task_steps SET status = $1, output = $2 WHERE id = $3`,
		status, output, id,
	)
	if err != nil {
		return fmt.Errorf("update step: %w", err)
	}
	return nil
}

func (s *TaskStore) UpdateStepTool(ctx context.Context, id uuid.UUID, toolUsed string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE task_steps SET tool_used = $1 WHERE id = $2`,
		toolUsed, id,
	)
	if err != nil {
		return fmt.Errorf("update step tool: %w", err)
	}
	return nil
}

func (s *TaskStore) ListSteps(ctx context.Context, taskID uuid.UUID) ([]types.TaskStep, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, task_id, description, status, COALESCE(output,''), COALESCE(tool_used,''), created_at, updated_at
		 FROM task_steps WHERE task_id = $1 ORDER BY created_at`,
		taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("list steps: %w", err)
	}
	defer rows.Close()

	var steps []types.TaskStep
	for rows.Next() {
		var st types.TaskStep
		if err := rows.Scan(&st.ID, &st.TaskID, &st.Description, &st.Status,
			&st.Output, &st.ToolUsed, &st.CreatedAt, &st.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan step row: %w", err)
		}
		steps = append(steps, st)
	}
	return steps, rows.Err()
}
