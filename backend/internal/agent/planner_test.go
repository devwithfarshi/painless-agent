package agent

import (
	"strings"
	"testing"

	"github.com/devwithfarshi/painless-agent/internal/llm"
	"github.com/devwithfarshi/painless-agent/internal/types"
)

func TestParsePlanInput_Valid(t *testing.T) {
	input := map[string]any{
		"steps": []any{
			map[string]any{"description": "Step one", "tool_hint": "web_search"},
			map[string]any{"description": "Step two"},
		},
	}
	steps, err := parsePlanInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("got %d steps, want 2", len(steps))
	}
	if steps[0].Description != "Step one" {
		t.Errorf("step[0] description: got %q", steps[0].Description)
	}
	if steps[0].ToolHint != "web_search" {
		t.Errorf("step[0] tool_hint: got %q", steps[0].ToolHint)
	}
	if steps[1].ToolHint != "" {
		t.Errorf("step[1] tool_hint: got %q, want empty", steps[1].ToolHint)
	}
}

func TestParsePlanInput_EmptySteps(t *testing.T) {
	_, err := parsePlanInput(map[string]any{"steps": []any{}})
	if err == nil {
		t.Fatal("expected error for empty steps")
	}
}

func TestParsePlanInput_MissingStepsKey(t *testing.T) {
	_, err := parsePlanInput(map[string]any{})
	if err == nil {
		t.Fatal("expected error when steps key is absent")
	}
}

func TestBuildPlannerUserMessage_ContainsGoal(t *testing.T) {
	msg := buildPlannerUserMessage("do the thing", nil, nil, nil)
	if !strings.Contains(msg, "do the thing") {
		t.Errorf("goal not in message: %q", msg)
	}
}

func TestBuildPlannerUserMessage_IncludesMemory(t *testing.T) {
	memCtx := []string{"remember A", "remember B"}
	msg := buildPlannerUserMessage("goal", memCtx, nil, nil)
	for _, m := range memCtx {
		if !strings.Contains(msg, m) {
			t.Errorf("memory %q not in message", m)
		}
	}
}

func TestBuildPlannerUserMessage_IncludesSkill(t *testing.T) {
	skill := &types.Skill{
		Name: "deploy-skill",
		Steps: []types.SkillStep{
			{Description: "build the binary"},
			{Description: "push to registry"},
		},
	}
	msg := buildPlannerUserMessage("deploy", nil, skill, nil)
	if !strings.Contains(msg, "deploy-skill") {
		t.Errorf("skill name not in message")
	}
	if !strings.Contains(msg, "build the binary") {
		t.Errorf("skill step 1 not in message")
	}
}

func TestBuildPlannerUserMessage_IncludesTools(t *testing.T) {
	tools := []llm.ToolDefinition{
		{Name: "web_search", Description: "search the web"},
		{Name: "filesystem", Description: "read and write files"},
	}
	msg := buildPlannerUserMessage("goal", nil, nil, tools)
	for _, td := range tools {
		if !strings.Contains(msg, td.Name) {
			t.Errorf("tool %q not in message", td.Name)
		}
	}
}
