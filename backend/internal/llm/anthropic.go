package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicProvider implements LLMProvider using the official anthropic-sdk-go.
// Embed always returns an error — embeddings must be routed to OpenAIProvider.
type AnthropicProvider struct {
	client anthropic.Client // value type — NewClient returns Client, not *Client
	model  string
}

func NewAnthropic(apiKey, model string) *AnthropicProvider {
	return &AnthropicProvider{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
	}
}

func (p *AnthropicProvider) Model() string { return p.model }

func (p *AnthropicProvider) Complete(ctx context.Context, req CompletionRequest) (*Response, error) {
	params := p.buildParams(req)
	msg, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic complete: %w", err)
	}

	resp := &Response{
		Usage: Usage{
			InputTokens:  int(msg.Usage.InputTokens),
			OutputTokens: int(msg.Usage.OutputTokens),
		},
	}
	for _, block := range msg.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			resp.Content += b.Text
		case anthropic.ToolUseBlock:
			var input map[string]any
			if err := json.Unmarshal(b.Input, &input); err != nil {
				return nil, fmt.Errorf("unmarshal tool input for %q: %w", b.Name, err)
			}
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{
				ID:    b.ID,
				Name:  b.Name,
				Input: input,
			})
		}
	}
	return resp, nil
}

func (p *AnthropicProvider) Stream(ctx context.Context, req CompletionRequest) (<-chan Delta, error) {
	params := p.buildParams(req)
	stream := p.client.Messages.NewStreaming(ctx, params)
	ch := make(chan Delta, 32)
	go func() {
		defer close(ch)
		for stream.Next() {
			event := stream.Current()
			switch e := event.AsAny().(type) {
			case anthropic.ContentBlockDeltaEvent:
				switch d := e.Delta.AsAny().(type) {
				case anthropic.TextDelta:
					if d.Text != "" {
						ch <- Delta{Content: d.Text}
					}
				}
			}
		}
		if err := stream.Err(); err != nil {
			ch <- Delta{Err: fmt.Errorf("anthropic stream: %w", err), Done: true}
			return
		}
		ch <- Delta{Done: true}
	}()
	return ch, nil
}

func (p *AnthropicProvider) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, fmt.Errorf("anthropic does not support embeddings; use EMBEDDING_PROVIDER=openai")
}

func (p *AnthropicProvider) buildParams(req CompletionRequest) anthropic.MessageNewParams {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(p.model),
		MaxTokens: 4096,
	}
	if req.MaxTokens != 0 {
		params.MaxTokens = int64(req.MaxTokens)
	}
	if req.Temperature != 0 {
		params.Temperature = anthropic.Float(float64(req.Temperature))
	}

	var msgs []anthropic.MessageParam
	for _, m := range req.Messages {
		switch m.Role {
		case RoleSystem:
			// Anthropic takes system messages as a separate field.
			params.System = append(params.System, anthropic.TextBlockParam{Text: m.Content})
		case RoleUser:
			msgs = append(msgs, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		case RoleAssistant:
			if len(m.ToolCalls) > 0 {
				blocks := make([]anthropic.ContentBlockParamUnion, 0, 1+len(m.ToolCalls))
				if m.Content != "" {
					blocks = append(blocks, anthropic.NewTextBlock(m.Content))
				}
				for _, tc := range m.ToolCalls {
					raw, _ := json.Marshal(tc.Input)
					blocks = append(blocks, anthropic.ContentBlockParamUnion{
						OfToolUse: &anthropic.ToolUseBlockParam{
							ID:    tc.ID,
							Name:  tc.Name,
							Input: raw,
						},
					})
				}
				msgs = append(msgs, anthropic.NewAssistantMessage(blocks...))
			} else {
				msgs = append(msgs, anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content)))
			}
		case RoleTool:
			msgs = append(msgs, anthropic.NewUserMessage(
				anthropic.ContentBlockParamUnion{
					OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: m.ToolCallID,
						Content: []anthropic.ToolResultBlockParamContentUnion{
							{OfText: &anthropic.TextBlockParam{Text: m.Content}},
						},
					},
				},
			))
		}
	}
	params.Messages = msgs

	if len(req.Tools) > 0 {
		tools := make([]anthropic.ToolUnionParam, len(req.Tools))
		for i, t := range req.Tools {
			props, required := extractSchemaComponents(t.Parameters)
			tools[i] = anthropic.ToolUnionParam{
				OfTool: &anthropic.ToolParam{
					Name:        t.Name,
					Description: anthropic.String(t.Description),
					InputSchema: anthropic.ToolInputSchemaParam{
						Properties: props,
						Required:   required,
					},
				},
			}
		}
		params.Tools = tools
	}

	return params
}

// extractSchemaComponents pulls properties and required from a JSON Schema object.
// t.Parameters is the whole schema ({"type":"object","properties":{...},"required":[...]}).
// Anthropic's ToolInputSchemaParam expects these as separate fields.
func extractSchemaComponents(schema map[string]any) (props any, required []string) {
	props = schema["properties"]
	if req, ok := schema["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				required = append(required, s)
			}
		}
	}
	return
}
