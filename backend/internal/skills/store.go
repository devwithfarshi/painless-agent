package skills

import (
	"context"

	"github.com/google/uuid"

	"github.com/devwithfarshi/painless-agent/internal/types"
)

// SkillStore manages reusable workflow templates learned by the reflection system.
// The agent.SkillStore interface (just Match) is a subset of this.
type SkillStore interface {
	// Match returns the most similar skill to goal, or nil if no close match exists.
	Match(ctx context.Context, goal string) (*types.Skill, error)
	// Save persists a new skill, embedding its description for future similarity search.
	Save(ctx context.Context, skill types.Skill) error
	// IncrementUsage bumps the usage counter for a skill.
	IncrementUsage(ctx context.Context, skillID uuid.UUID) error
	// List returns all skills ordered by usage_count desc.
	List(ctx context.Context) ([]types.Skill, error)
	// Get returns a single skill by ID.
	Get(ctx context.Context, id uuid.UUID) (*types.Skill, error)
	// Delete removes a skill.
	Delete(ctx context.Context, id uuid.UUID) error
}
