package memory

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"

	"github.com/devwithfarshi/painless-agent/internal/llm"
)

type pgMemoryStore struct {
	pool     *pgxpool.Pool
	embedder llm.LLMProvider
}

// NewPgMemoryStore returns a MemoryStore backed by Postgres + pgvector.
func NewPgMemoryStore(pool *pgxpool.Pool, embedder llm.LLMProvider) MemoryStore {
	return &pgMemoryStore{pool: pool, embedder: embedder}
}

func (s *pgMemoryStore) Store(ctx context.Context, content string, tags []string, taskID uuid.UUID) error {
	vec, err := s.embedder.Embed(ctx, content)
	if err != nil {
		return fmt.Errorf("embed content: %w", err)
	}
	if tags == nil {
		tags = []string{}
	}
	var taskPtr *uuid.UUID
	if taskID != uuid.Nil {
		taskPtr = &taskID
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO memory (content, embedding, tags, task_id) VALUES ($1, $2::vector, $3, $4)`,
		content, pgvector.NewVector(vec).String(), tags, taskPtr,
	)
	if err != nil {
		return fmt.Errorf("insert memory: %w", err)
	}
	return nil
}

func (s *pgMemoryStore) Search(ctx context.Context, query string, k int) ([]string, error) {
	vec, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	rows, err := s.pool.Query(ctx,
		`SELECT content FROM memory ORDER BY embedding <=> $1::vector LIMIT $2`,
		pgvector.NewVector(vec).String(), k,
	)
	if err != nil {
		return nil, fmt.Errorf("search memory: %w", err)
	}
	defer rows.Close()
	var results []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		results = append(results, c)
	}
	return results, rows.Err()
}

func (s *pgMemoryStore) RecentContext(ctx context.Context, taskID uuid.UUID, limit int) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT content FROM memory WHERE task_id = $1 ORDER BY created_at DESC LIMIT $2`,
		taskID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("recent context: %w", err)
	}
	defer rows.Close()
	var results []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		results = append(results, c)
	}
	return results, rows.Err()
}
