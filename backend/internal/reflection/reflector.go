package reflection

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/devwithfarshi/painless-agent/internal/llm"
	"github.com/devwithfarshi/painless-agent/internal/skills"
	"github.com/devwithfarshi/painless-agent/internal/store"
	"github.com/devwithfarshi/painless-agent/internal/types"
)

const reflectionSystemPrompt = `You are a task reflection agent. Given a completed task and its execution steps, you extract lessons learned and rate the overall quality. Always call the extract_reflection tool — never respond with prose.`

var reflectTool = llm.ToolDefinition{
	Name:        "extract_reflection",
	Description: "Extract lessons learned from a completed task and rate its execution quality",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"lessons": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Key lessons learned from this task execution",
			},
			"rating": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"maximum":     10,
				"description": "Overall quality rating (1=poor, 10=excellent)",
			},
			"promote_to_skill": map[string]any{
				"type":        "boolean",
				"description": "Whether this workflow is general enough to save as a reusable skill",
			},
			"skill_name": map[string]any{
				"type":        "string",
				"description": "Short descriptive name for the skill (required when promote_to_skill is true)",
			},
			"skill_steps": map[string]any{
				"type":        "array",
				"description": "Generalised steps for the reusable skill (required when promote_to_skill is true)",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"description": map[string]any{"type": "string"},
						"tool_hint":   map[string]any{"type": "string", "description": "Optional tool name hint"},
					},
					"required": []string{"description"},
				},
			},
		},
		"required": []string{"lessons", "rating", "promote_to_skill"},
	},
}

type extractResult struct {
	Lessons        []string `json:"lessons"`
	Rating         int      `json:"rating"`
	PromoteToSkill bool     `json:"promote_to_skill"`
	SkillName      string   `json:"skill_name"`
	SkillSteps     []struct {
		Description string `json:"description"`
		ToolHint    string `json:"tool_hint,omitempty"`
	} `json:"skill_steps"`
}

// Reflector extracts lessons from completed tasks and promotes high-quality
// workflows to reusable skills.
type Reflector struct {
	provider         llm.LLMProvider
	reflectionStore  *store.ReflectionStore
	skillStore       skills.SkillStore
	ratingThreshold  int // minimum rating (1–10) to promote to skill
}

// New creates a Reflector. ratingThreshold is the minimum rating (inclusive)
// required before a workflow is promoted to a skill.
func New(
	provider llm.LLMProvider,
	reflectionStore *store.ReflectionStore,
	skillStore skills.SkillStore,
	ratingThreshold int,
) *Reflector {
	return &Reflector{
		provider:        provider,
		reflectionStore: reflectionStore,
		skillStore:      skillStore,
		ratingThreshold: ratingThreshold,
	}
}

// Reflect implements agent.Reflector. It extracts lessons from the completed task,
// saves a reflections row, and optionally promotes the workflow to a skill.
func (r *Reflector) Reflect(ctx context.Context, task types.Task, steps []types.TaskStep) error {
	result, err := r.extract(ctx, task, steps)
	if err != nil {
		return fmt.Errorf("extract reflection: %w", err)
	}

	if err := r.reflectionStore.Save(ctx, task.ID, result.Lessons, result.Rating); err != nil {
		return fmt.Errorf("save reflection: %w", err)
	}

	if result.Rating >= r.ratingThreshold && result.PromoteToSkill && result.SkillName != "" {
		skill := types.Skill{
			ID:          uuid.New(),
			Name:        result.SkillName,
			Description: task.Goal,
			Steps:       make([]types.SkillStep, len(result.SkillSteps)),
		}
		for i, s := range result.SkillSteps {
			skill.Steps[i] = types.SkillStep{
				Description: s.Description,
				ToolHint:    s.ToolHint,
			}
		}
		if err := r.skillStore.Save(ctx, skill); err != nil {
			return fmt.Errorf("save skill: %w", err)
		}
	}

	return nil
}

func (r *Reflector) extract(ctx context.Context, task types.Task, steps []types.TaskStep) (*extractResult, error) {
	msg := buildReflectionPrompt(task, steps)

	resp, err := r.provider.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: reflectionSystemPrompt},
			{Role: llm.RoleUser, Content: msg},
		},
		Tools:       []llm.ToolDefinition{reflectTool},
		Temperature: 0.3,
		MaxTokens:   1024,
	})
	if err != nil {
		return nil, fmt.Errorf("llm complete: %w", err)
	}

	if len(resp.ToolCalls) == 0 {
		return nil, fmt.Errorf("reflector did not call extract_reflection; content: %s", resp.Content)
	}

	b, err := json.Marshal(resp.ToolCalls[0].Input)
	if err != nil {
		return nil, fmt.Errorf("marshal reflection input: %w", err)
	}
	var result extractResult
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("parse reflection: %w", err)
	}
	return &result, nil
}

func buildReflectionPrompt(task types.Task, steps []types.TaskStep) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Task goal: %s\n\nExecution steps:\n", task.Goal)
	for i, s := range steps {
		fmt.Fprintf(&b, "%d. [%s] %s", i+1, s.Status, s.Description)
		if s.Output != "" {
			out := s.Output
			if len(out) > 500 {
				out = out[:500] + "…"
			}
			fmt.Fprintf(&b, "\n   Output: %s", out)
		}
		b.WriteByte('\n')
	}
	b.WriteString("\nAnalyse the task execution and call extract_reflection.")
	return b.String()
}
