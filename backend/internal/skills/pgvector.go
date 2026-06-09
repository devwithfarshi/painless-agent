package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"

	"github.com/devwithfarshi/painless-agent/internal/llm"
	"github.com/devwithfarshi/painless-agent/internal/types"
)

type pgSkillStore struct {
	pool      *pgxpool.Pool
	embedder  llm.LLMProvider
	threshold float64 // cosine distance; match accepted when dist <= threshold
}

// NewPgSkillStore returns a SkillStore backed by Postgres + pgvector.
// threshold is the cosine distance ceiling (0–1); skills further away are not returned.
func NewPgSkillStore(pool *pgxpool.Pool, embedder llm.LLMProvider, threshold float64) SkillStore {
	return &pgSkillStore{pool: pool, embedder: embedder, threshold: threshold}
}

func (s *pgSkillStore) Match(ctx context.Context, goal string) (*types.Skill, error) {
	vec, err := s.embedder.Embed(ctx, goal)
	if err != nil {
		return nil, fmt.Errorf("embed goal: %w", err)
	}

	row := s.pool.QueryRow(ctx,
		`SELECT id, name, description, steps_json, usage_count, created_at,
		        embedding <=> $1::vector AS dist
		 FROM skills
		 WHERE embedding IS NOT NULL
		 ORDER BY embedding <=> $1::vector
		 LIMIT 1`,
		pgvector.NewVector(vec).String(),
	)

	var sk types.Skill
	var stepsJSON []byte
	var dist float64
	err = row.Scan(&sk.ID, &sk.Name, &sk.Description, &stepsJSON, &sk.UsageCount, &sk.CreatedAt, &dist)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan skill: %w", err)
	}

	if dist > s.threshold {
		return nil, nil
	}

	if err := json.Unmarshal(stepsJSON, &sk.Steps); err != nil {
		return nil, fmt.Errorf("unmarshal skill steps: %w", err)
	}
	return &sk, nil
}

func (s *pgSkillStore) Save(ctx context.Context, skill types.Skill) error {
	vec, err := s.embedder.Embed(ctx, skill.Description)
	if err != nil {
		return fmt.Errorf("embed skill description: %w", err)
	}

	stepsJSON, err := json.Marshal(skill.Steps)
	if err != nil {
		return fmt.Errorf("marshal skill steps: %w", err)
	}

	if skill.ID == uuid.Nil {
		skill.ID = uuid.New()
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO skills (id, name, description, steps_json, embedding)
		 VALUES ($1, $2, $3, $4, $5::vector)
		 ON CONFLICT (name) DO UPDATE
		   SET description = EXCLUDED.description,
		       steps_json  = EXCLUDED.steps_json,
		       embedding   = EXCLUDED.embedding`,
		skill.ID, skill.Name, skill.Description, stepsJSON,
		pgvector.NewVector(vec).String(),
	)
	if err != nil {
		return fmt.Errorf("insert skill: %w", err)
	}
	return nil
}

func (s *pgSkillStore) IncrementUsage(ctx context.Context, skillID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE skills SET usage_count = usage_count + 1 WHERE id = $1`,
		skillID,
	)
	return err
}

func (s *pgSkillStore) List(ctx context.Context) ([]types.Skill, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, description, steps_json, usage_count, created_at
		 FROM skills ORDER BY usage_count DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()
	return scanSkills(rows)
}

func (s *pgSkillStore) Get(ctx context.Context, id uuid.UUID) (*types.Skill, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, description, steps_json, usage_count, created_at
		 FROM skills WHERE id = $1`,
		id,
	)
	var sk types.Skill
	var stepsJSON []byte
	err := row.Scan(&sk.ID, &sk.Name, &sk.Description, &stepsJSON, &sk.UsageCount, &sk.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get skill: %w", err)
	}
	if err := json.Unmarshal(stepsJSON, &sk.Steps); err != nil {
		return nil, fmt.Errorf("unmarshal skill steps: %w", err)
	}
	return &sk, nil
}

func (s *pgSkillStore) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM skills WHERE id = $1`, id)
	return err
}

func scanSkills(rows pgx.Rows) ([]types.Skill, error) {
	var result []types.Skill
	for rows.Next() {
		var sk types.Skill
		var stepsJSON []byte
		var createdAt time.Time
		if err := rows.Scan(&sk.ID, &sk.Name, &sk.Description, &stepsJSON, &sk.UsageCount, &createdAt); err != nil {
			return nil, err
		}
		sk.CreatedAt = createdAt
		if err := json.Unmarshal(stepsJSON, &sk.Steps); err != nil {
			return nil, fmt.Errorf("unmarshal skill steps: %w", err)
		}
		result = append(result, sk)
	}
	return result, rows.Err()
}
