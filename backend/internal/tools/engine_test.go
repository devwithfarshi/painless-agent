package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// stubTool implements Tool for testing.
type stubTool struct {
	name    string
	content string
	isErr   bool
	execErr error
}

func (s *stubTool) Name() string              { return s.name }
func (s *stubTool) Description() string       { return "stub" }
func (s *stubTool) Schema() map[string]any    { return map[string]any{} }
func (s *stubTool) Execute(_ context.Context, _ map[string]any) (ToolResult, error) {
	return ToolResult{Content: s.content, IsError: s.isErr}, s.execErr
}

func TestEngine_UnknownTool(t *testing.T) {
	e := NewEngine(1024, nil)
	_, err := e.Execute(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestEngine_DuplicateRegisterPanics(t *testing.T) {
	e := NewEngine(1024, nil)
	e.Register(&stubTool{name: "myTool"})
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate tool registration")
		}
	}()
	e.Register(&stubTool{name: "myTool"})
}

func TestEngine_ExecuteSuccess(t *testing.T) {
	e := NewEngine(1024, nil)
	e.Register(&stubTool{name: "echo", content: "hello"})

	out, err := e.Execute(context.Background(), "echo", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hello" {
		t.Errorf("got %q, want %q", out, "hello")
	}
}

func TestEngine_OutputTruncated(t *testing.T) {
	maxOutput := 10
	e := NewEngine(maxOutput, nil)
	e.Register(&stubTool{name: "big", content: strings.Repeat("x", 50)})

	out, err := e.Execute(context.Background(), "big", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) > maxOutput+len("\n[output truncated]") {
		t.Errorf("output not truncated: len=%d", len(out))
	}
	if !strings.Contains(out, "[output truncated]") {
		t.Errorf("truncation marker missing: %q", out)
	}
}

func TestEngine_OutputSummarized(t *testing.T) {
	maxOutput := 10
	summarized := "summary"
	summarizeFn := func(_ context.Context, _ string) (string, error) {
		return summarized, nil
	}
	e := NewEngine(maxOutput, summarizeFn)
	e.Register(&stubTool{name: "big", content: strings.Repeat("x", 50)})

	out, err := e.Execute(context.Background(), "big", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != summarized {
		t.Errorf("got %q, want summarized %q", out, summarized)
	}
}

func TestEngine_SummarizerFallsBackToTruncation(t *testing.T) {
	maxOutput := 10
	summarizeFn := func(_ context.Context, _ string) (string, error) {
		return "", errors.New("summarizer unavailable")
	}
	e := NewEngine(maxOutput, summarizeFn)
	e.Register(&stubTool{name: "big", content: strings.Repeat("x", 50)})

	out, _ := e.Execute(context.Background(), "big", nil)
	if !strings.Contains(out, "[output truncated]") {
		t.Errorf("expected truncation fallback, got: %q", out)
	}
}

func TestEngine_ToolExecErrorReturnedAsContent(t *testing.T) {
	e := NewEngine(1024, nil)
	e.Register(&stubTool{name: "broken", content: "", execErr: errors.New("disk error")})

	out, err := e.Execute(context.Background(), "broken", nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !strings.Contains(out, "disk error") {
		t.Errorf("error message not in content: %q", out)
	}
}

func TestEngine_Definitions(t *testing.T) {
	e := NewEngine(1024, nil)
	e.Register(&stubTool{name: "toolA"})
	e.Register(&stubTool{name: "toolB"})

	defs := e.Definitions()
	if len(defs) != 2 {
		t.Errorf("got %d definitions, want 2", len(defs))
	}
	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	if !names["toolA"] || !names["toolB"] {
		t.Errorf("definitions missing expected tools: %v", names)
	}
}
