package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devwithfarshi/painless-agent/internal/llm"
	"github.com/devwithfarshi/painless-agent/internal/types"
)

const plannerSystemPrompt = `You are a planning agent. Given a goal, break it into clear, actionable steps.
Each step must be specific and achievable. Output the plan by calling the create_plan tool.
Do not respond with prose — always use the tool.`

// plannerTool is the tool definition sent to the LLM to elicit a structured plan.
var plannerTool = llm.ToolDefinition{
	Name:        "create_plan",
	Description: "Create a structured step-by-step plan for the given goal",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"steps": map[string]any{
				"type":        "array",
				"description": "Ordered list of steps to accomplish the goal",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"description": map[string]any{
							"type":        "string",
							"description": "Clear, specific description of this step",
						},
						"tool_hint": map[string]any{
							"type":        "string",
							"description": "Optional: name of the tool most likely needed (e.g. web_search, filesystem, code_executor)",
						},
					},
					"required": []string{"description"},
				},
			},
		},
		"required": []string{"steps"},
	},
}

type Planner struct {
	provider llm.LLMProvider
}

func NewPlanner(provider llm.LLMProvider) *Planner {
	return &Planner{provider: provider}
}

// Plan generates an ordered list of steps for goal. memCtx contains relevant
// memories (strings); skill is an optional matching workflow template.
func (p *Planner) Plan(ctx context.Context, goal string, memCtx []string, skill *types.Skill) ([]types.PlanStep, error) {
	userMsg := buildPlannerUserMessage(goal, memCtx, skill)

	resp, err := p.provider.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: plannerSystemPrompt},
			{Role: llm.RoleUser, Content: userMsg},
		},
		Tools:       []llm.ToolDefinition{plannerTool},
		Temperature: 0.3,
		MaxTokens:   2048,
	})
	if err != nil {
		return nil, fmt.Errorf("plan: %w", err)
	}

	if len(resp.ToolCalls) == 0 {
		return nil, fmt.Errorf("planner did not call create_plan; content: %s", resp.Content)
	}
	tc := resp.ToolCalls[0]
	if tc.Name != "create_plan" {
		return nil, fmt.Errorf("planner called unexpected tool %q", tc.Name)
	}
	return parsePlanInput(tc.Input)
}

func buildPlannerUserMessage(goal string, memCtx []string, skill *types.Skill) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Goal: %s", goal)

	if len(memCtx) > 0 {
		b.WriteString("\n\nRelevant context from memory:\n")
		for _, m := range memCtx {
			fmt.Fprintf(&b, "- %s\n", m)
		}
	}

	if skill != nil {
		fmt.Fprintf(&b, "\n\nMatching skill template (%s):\n", skill.Name)
		for i, s := range skill.Steps {
			fmt.Fprintf(&b, "%d. %s\n", i+1, s.Description)
		}
		b.WriteString("Adapt the skill template to the current goal as needed.\n")
	}

	b.WriteString("\n\nCreate a detailed step-by-step plan using the create_plan tool.")
	return b.String()
}

type rawPlanInput struct {
	Steps []struct {
		Description string `json:"description"`
		ToolHint    string `json:"tool_hint"`
	} `json:"steps"`
}

func parsePlanInput(input map[string]any) ([]types.PlanStep, error) {
	b, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal plan input: %w", err)
	}
	var raw rawPlanInput
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse plan input: %w", err)
	}
	if len(raw.Steps) == 0 {
		return nil, fmt.Errorf("planner returned an empty plan")
	}
	steps := make([]types.PlanStep, len(raw.Steps))
	for i, s := range raw.Steps {
		steps[i] = types.PlanStep{
			Description: s.Description,
			ToolHint:    s.ToolHint,
		}
	}
	return steps, nil
}
