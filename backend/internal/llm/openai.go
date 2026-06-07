package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// OpenAIProvider implements LLMProvider using the official openai-go SDK.
type OpenAIProvider struct {
	client openai.Client // value type — NewClient returns Client, not *Client
	model  string
}

func NewOpenAI(apiKey, model string) *OpenAIProvider {
	return &OpenAIProvider{
		client: openai.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
	}
}

func (p *OpenAIProvider) Model() string { return p.model }

func (p *OpenAIProvider) Complete(ctx context.Context, req CompletionRequest) (*Response, error) {
	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(p.model),
		Messages: p.buildMessages(req.Messages),
	}
	if req.Temperature != 0 {
		params.Temperature = openai.Float(float64(req.Temperature))
	}
	if req.MaxTokens != 0 {
		params.MaxTokens = openai.Int(int64(req.MaxTokens))
	}
	if len(req.Tools) > 0 {
		params.Tools = p.buildTools(req.Tools)
	}

	chat, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai complete: %w", err)
	}
	if len(chat.Choices) == 0 {
		return &Response{}, nil
	}

	choice := chat.Choices[0]
	resp := &Response{
		Content: choice.Message.Content,
		Usage: Usage{
			InputTokens:  int(chat.Usage.PromptTokens),
			OutputTokens: int(chat.Usage.CompletionTokens),
		},
	}
	for _, tc := range choice.Message.ToolCalls {
		if tc.Type != "function" {
			continue
		}
		var input map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
			return nil, fmt.Errorf("unmarshal tool call args for %q: %w", tc.Function.Name, err)
		}
		resp.ToolCalls = append(resp.ToolCalls, ToolCall{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}
	return resp, nil
}

func (p *OpenAIProvider) Stream(ctx context.Context, req CompletionRequest) (<-chan Delta, error) {
	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(p.model),
		Messages: p.buildMessages(req.Messages),
	}
	if req.Temperature != 0 {
		params.Temperature = openai.Float(float64(req.Temperature))
	}
	if req.MaxTokens != 0 {
		params.MaxTokens = openai.Int(int64(req.MaxTokens))
	}
	if len(req.Tools) > 0 {
		params.Tools = p.buildTools(req.Tools)
	}

	stream := p.client.Chat.Completions.NewStreaming(ctx, params)
	ch := make(chan Delta, 32)
	go func() {
		defer close(ch)
		for stream.Next() {
			chunk := stream.Current()
			if len(chunk.Choices) == 0 {
				continue
			}
			if content := chunk.Choices[0].Delta.Content; content != "" {
				ch <- Delta{Content: content}
			}
		}
		if err := stream.Err(); err != nil {
			ch <- Delta{Err: fmt.Errorf("openai stream: %w", err), Done: true}
			return
		}
		ch <- Delta{Done: true}
	}()
	return ch, nil
}

func (p *OpenAIProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	resp, err := p.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model: openai.EmbeddingModelTextEmbedding3Small,
		Input: openai.EmbeddingNewParamsInputUnion{OfString: openai.String(text)},
	})
	if err != nil {
		return nil, fmt.Errorf("openai embed: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("openai embed: empty response")
	}
	f64 := resp.Data[0].Embedding
	out := make([]float32, len(f64))
	for i, v := range f64 {
		out[i] = float32(v)
	}
	return out, nil
}

func (p *OpenAIProvider) buildMessages(msgs []Message) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case RoleSystem:
			out = append(out, openai.SystemMessage(m.Content))
		case RoleUser:
			out = append(out, openai.UserMessage(m.Content))
		case RoleAssistant:
			if len(m.ToolCalls) > 0 {
				tcs := make([]openai.ChatCompletionMessageToolCallUnionParam, len(m.ToolCalls))
				for i, tc := range m.ToolCalls {
					args, _ := json.Marshal(tc.Input)
					tcs[i] = openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: tc.ID,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      tc.Name,
								Arguments: string(args),
							},
						},
					}
				}
				out = append(out, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &openai.ChatCompletionAssistantMessageParam{
						ToolCalls: tcs,
					},
				})
			} else {
				out = append(out, openai.AssistantMessage(m.Content))
			}
		case RoleTool:
			// openai.ToolMessage(content, toolCallID)
			out = append(out, openai.ToolMessage(m.Content, m.ToolCallID))
		}
	}
	return out
}

func (p *OpenAIProvider) buildTools(tools []ToolDefinition) []openai.ChatCompletionToolUnionParam {
	out := make([]openai.ChatCompletionToolUnionParam, len(tools))
	for i, t := range tools {
		out[i] = openai.ChatCompletionToolUnionParam{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: openai.FunctionDefinitionParam{
					Name:        t.Name,
					Description: openai.String(t.Description),
					Parameters:  openai.FunctionParameters(t.Parameters),
				},
			},
		}
	}
	return out
}
