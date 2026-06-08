package tools

import (
	"context"
	"fmt"

	"github.com/devwithfarshi/painless-agent/internal/llm"
)

// Tool is the interface every agent tool must implement.
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any // JSON Schema object for parameters
	Execute(ctx context.Context, input map[string]any) (ToolResult, error)
}

// ToolResult is the output of a tool execution.
type ToolResult struct {
	Content string
	IsError bool
}

// ToolEngine is a registry of tools. It satisfies the agent.ToolEngine interface.
type ToolEngine struct {
	tools     map[string]Tool
	maxOutput int // max chars before summarisation/truncation
	summarize func(ctx context.Context, content string) (string, error) // set by NewEngine
}

// NewEngine creates an empty engine. Pass a summarizer func for output truncation.
func NewEngine(maxOutput int, summarize func(ctx context.Context, content string) (string, error)) *ToolEngine {
	return &ToolEngine{
		tools:     make(map[string]Tool),
		maxOutput: maxOutput,
		summarize: summarize,
	}
}

// Register adds a tool to the engine. Panics on duplicate name.
func (e *ToolEngine) Register(t Tool) {
	if _, ok := e.tools[t.Name()]; ok {
		panic(fmt.Sprintf("tool %q already registered", t.Name()))
	}
	e.tools[t.Name()] = t
}

// Definitions returns the list of tool schemas for LLM consumption.
func (e *ToolEngine) Definitions() []llm.ToolDefinition {
	defs := make([]llm.ToolDefinition, 0, len(e.tools))
	for _, t := range e.tools {
		defs = append(defs, llm.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Schema(),
		})
	}
	return defs
}

// Execute runs the named tool and returns its output, truncated/summarised if needed.
// Tool-level errors are returned as content (so the LLM can react); only system-level
// errors (unknown tool, engine misconfiguration) are returned as Go errors.
func (e *ToolEngine) Execute(ctx context.Context, name string, input map[string]any) (string, error) {
	t, ok := e.tools[name]
	if !ok {
		return "", fmt.Errorf("tool %q not registered", name)
	}

	result, err := t.Execute(ctx, input)
	content := result.Content
	if err != nil {
		content = fmt.Sprintf("tool error: %s", err)
	}

	if len(content) > e.maxOutput {
		if e.summarize != nil {
			if summarized, sumErr := e.summarize(ctx, content); sumErr == nil {
				return summarized, nil
			}
		}
		content = content[:e.maxOutput] + "\n[output truncated]"
	}

	return content, nil
}
