package memory

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// MemoryStore persists agent memories and supports semantic retrieval.
type MemoryStore interface {
	Store(ctx context.Context, content string, tags []string, taskID uuid.UUID) error
	Search(ctx context.Context, query string, k int) ([]string, error)
	RecentContext(ctx context.Context, taskID uuid.UUID, limit int) ([]string, error)
}

// Memory is a single persisted memory record.
type Memory struct {
	ID        uuid.UUID
	Content   string
	Tags      []string
	TaskID    *uuid.UUID
	CreatedAt time.Time
}
