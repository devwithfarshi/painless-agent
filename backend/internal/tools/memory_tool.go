package tools

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/devwithfarshi/painless-agent/internal/memory"
)

// MemoryTool lets the agent proactively store information for future retrieval.
type MemoryTool struct {
	store memory.MemoryStore
}

func NewMemoryTool(store memory.MemoryStore) *MemoryTool {
	return &MemoryTool{store: store}
}

func (t *MemoryTool) Name() string { return "memory_store" }

func (t *MemoryTool) Description() string {
	return "Save a piece of information to long-term memory for future retrieval."
}

func (t *MemoryTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "Information to store in memory",
			},
			"tags": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Optional tags for categorising the memory",
			},
		},
		"required": []string{"content"},
	}
}

func (t *MemoryTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	content, _ := input["content"].(string)
	if content == "" {
		return ToolResult{IsError: true, Content: "content is required"}, nil
	}

	var tags []string
	if rawTags, ok := input["tags"].([]any); ok {
		for _, v := range rawTags {
			if s, ok := v.(string); ok {
				tags = append(tags, s)
			}
		}
	}

	if err := t.store.Store(ctx, content, tags, uuid.Nil); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("store memory: %s", err)}, nil
	}
	return ToolResult{Content: fmt.Sprintf("stored %d chars to memory", len(content))}, nil
}
