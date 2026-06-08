package tools

import (
	"context"
	"fmt"

	"github.com/devwithfarshi/painless-agent/internal/llm"
)

const summarizerSystemPrompt = `You are a concise summarizer. Condense the provided text to its essential points.
Keep the output under 500 words. Preserve key facts, numbers, and conclusions.`

// SummarizerTool condenses long text using the LLM.
// It is both a callable tool (for the agent) and the output-truncation helper
// used by ToolEngine when a tool result exceeds maxOutput.
type SummarizerTool struct {
	provider llm.LLMProvider
}

func NewSummarizerTool(provider llm.LLMProvider) *SummarizerTool {
	return &SummarizerTool{provider: provider}
}

func (t *SummarizerTool) Name() string { return "summarizer" }

func (t *SummarizerTool) Description() string {
	return "Summarize a long piece of text into key points (under 500 words)."
}

func (t *SummarizerTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Text to summarize",
			},
		},
		"required": []string{"text"},
	}
}

func (t *SummarizerTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	text, _ := input["text"].(string)
	if text == "" {
		return ToolResult{IsError: true, Content: "text is required"}, nil
	}
	summary, err := t.Summarize(ctx, text)
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("summarize: %s", err)}, nil
	}
	return ToolResult{Content: summary}, nil
}

// Summarize condenses content to key points. Used internally by ToolEngine for truncation.
func (t *SummarizerTool) Summarize(ctx context.Context, content string) (string, error) {
	resp, err := t.provider.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: summarizerSystemPrompt},
			{Role: llm.RoleUser, Content: content},
		},
		Temperature: 0.2,
		MaxTokens:   1024,
	})
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}
	return resp.Content, nil
}
