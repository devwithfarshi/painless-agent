package types

import (
	"time"

	"github.com/google/uuid"
)

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusRunning   StepStatus = "running"
	StepStatusCompleted StepStatus = "completed"
	StepStatusFailed    StepStatus = "failed"
)

type Task struct {
	ID        uuid.UUID
	Goal      string
	Status    TaskStatus
	Plan      []PlanStep
	CreatedAt time.Time
	UpdatedAt time.Time
}

type TaskStep struct {
	ID          uuid.UUID
	TaskID      uuid.UUID
	Description string
	Status      StepStatus
	Output      string
	ToolUsed    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PlanStep is one element of the structured plan stored in tasks.plan_json.
type PlanStep struct {
	Description string `json:"description"`
	ToolHint    string `json:"tool_hint,omitempty"`
}

// Skill is a reusable workflow learned by the reflection system (Order 5).
type Skill struct {
	ID          uuid.UUID
	Name        string
	Description string
	Steps       []SkillStep
	UsageCount  int
	CreatedAt   time.Time
}

type SkillStep struct {
	Description string `json:"description"`
	ToolHint    string `json:"tool_hint,omitempty"`
}
