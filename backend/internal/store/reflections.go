package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReflectionStore persists task reflections to the reflections table.
type ReflectionStore struct {
	pool *pgxpool.Pool
}

// Reflection is a record of lessons learned from a completed task.
type Reflection struct {
	ID        uuid.UUID
	TaskID    uuid.UUID
	Lessons   []string
	Rating    int
	CreatedAt time.Time
}

func NewReflectionStore(pool *pgxpool.Pool) *ReflectionStore {
	return &ReflectionStore{pool: pool}
}

// Save inserts a reflection row. lessons is serialised to JSONB.
func (s *ReflectionStore) Save(ctx context.Context, taskID uuid.UUID, lessons []string, rating int) error {
	if lessons == nil {
		lessons = []string{}
	}
	lessonsJSON, err := json.Marshal(lessons)
	if err != nil {
		return fmt.Errorf("marshal lessons: %w", err)
	}

	var taskPtr *uuid.UUID
	if taskID != uuid.Nil {
		taskPtr = &taskID
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO reflections (task_id, lessons_json, rating) VALUES ($1, $2, $3)`,
		taskPtr, lessonsJSON, rating,
	)
	if err != nil {
		return fmt.Errorf("insert reflection: %w", err)
	}
	return nil
}
