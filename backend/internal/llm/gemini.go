package llm

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

// GeminiProvider implements LLMProvider using Google's Gemini generative AI.
type GeminiProvider struct {
	client *genai.Client
	model  string
}

// NewGemini creates a Gemini provider. apiKey must be a Gemini API key from ai.google.dev.
// model defaults to "gemini-2.0-flash" if empty.
func NewGemini(ctx context.Context, apiKey, model string) (*GeminiProvider, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini client: %w", err)
	}
	if model == "" {
		model = "gemini-2.0-flash"
	}
	return &GeminiProvider{client: client, model: model}, nil
}

func (p *GeminiProvider) Model() string { return p.model }

func (p *GeminiProvider) Complete(ctx context.Context, req CompletionRequest) (*Response, error) {
	contents, sysText := p.buildContents(req.Messages)

	cfg := &genai.GenerateContentConfig{}
	if sysText != "" {
		cfg.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: sysText}},
		}
	}
	if req.Temperature != 0 {
		t := req.Temperature
		cfg.Temperature = &t
	}
	if req.MaxTokens != 0 {
		cfg.MaxOutputTokens = int32(req.MaxTokens)
	}
	if len(req.Tools) > 0 {
		cfg.Tools = p.buildTools(req.Tools)
	}

	result, err := p.client.Models.GenerateContent(ctx, p.model, contents, cfg)
	if err != nil {
		return nil, fmt.Errorf("gemini complete: %w", err)
	}
	if len(result.Candidates) == 0 || result.Candidates[0].Content == nil {
		return &Response{}, nil
	}

	resp := &Response{}
	if result.UsageMetadata != nil {
		resp.Usage = Usage{
			InputTokens:  int(result.UsageMetadata.PromptTokenCount),
			OutputTokens: int(result.UsageMetadata.CandidatesTokenCount),
		}
	}
	for _, part := range result.Candidates[0].Content.Parts {
		if part.Text != "" {
			resp.Content += part.Text
		}
		if fc := part.FunctionCall; fc != nil {
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{
				ID:    fc.ID,
				Name:  fc.Name,
				Input: fc.Args,
			})
		}
	}
	return resp, nil
}

func (p *GeminiProvider) Stream(ctx context.Context, req CompletionRequest) (<-chan Delta, error) {
	contents, sysText := p.buildContents(req.Messages)

	cfg := &genai.GenerateContentConfig{}
	if sysText != "" {
		cfg.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: sysText}},
		}
	}
	if req.Temperature != 0 {
		t := req.Temperature
		cfg.Temperature = &t
	}
	if req.MaxTokens != 0 {
		cfg.MaxOutputTokens = int32(req.MaxTokens)
	}

	ch := make(chan Delta, 32)
	go func() {
		defer close(ch)
		for result, err := range p.client.Models.GenerateContentStream(ctx, p.model, contents, cfg) {
			if err != nil {
				ch <- Delta{Err: fmt.Errorf("gemini stream: %w", err), Done: true}
				return
			}
			if len(result.Candidates) == 0 || result.Candidates[0].Content == nil {
				continue
			}
			for _, part := range result.Candidates[0].Content.Parts {
				if part.Text != "" {
					ch <- Delta{Content: part.Text}
				}
			}
		}
		ch <- Delta{Done: true}
	}()
	return ch, nil
}

// Embed uses Gemini's text-embedding-004 model for embeddings.
// Note: the agent always uses the dedicated OpenAI embedder for vector storage;
// this method exists to satisfy the interface for completeness.
func (p *GeminiProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	contents := []*genai.Content{{
		Parts: []*genai.Part{{Text: text}},
		Role:  genai.RoleUser,
	}}
	resp, err := p.client.Models.EmbedContent(ctx, "text-embedding-004", contents, nil)
	if err != nil {
		return nil, fmt.Errorf("gemini embed: %w", err)
	}
	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("gemini embed: empty response")
	}
	return resp.Embeddings[0].Values, nil
}

// buildContents converts our message slice into Gemini's []*Content format,
// extracting the system prompt separately. Consecutive tool-result messages are
// batched into a single "user" Content with multiple FunctionResponse parts,
// as required by the Gemini API.
func (p *GeminiProvider) buildContents(msgs []Message) ([]*genai.Content, string) {
	var sysText string
	var contents []*genai.Content

	i := 0
	for i < len(msgs) {
		m := msgs[i]
		switch m.Role {
		case RoleSystem:
			sysText = m.Content
			i++

		case RoleUser:
			contents = append(contents, &genai.Content{
				Role:  genai.RoleUser,
				Parts: []*genai.Part{{Text: m.Content}},
			})
			i++

		case RoleAssistant:
			c := &genai.Content{Role: genai.RoleModel}
			if m.Content != "" {
				c.Parts = append(c.Parts, &genai.Part{Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				c.Parts = append(c.Parts, &genai.Part{
					FunctionCall: &genai.FunctionCall{
						ID:   tc.ID,
						Name: tc.Name,
						Args: tc.Input,
					},
				})
			}
			if len(c.Parts) == 0 {
				c.Parts = []*genai.Part{{Text: ""}}
			}
			contents = append(contents, c)
			i++

		case RoleTool:
			// Batch all consecutive tool results into one user Content.
			uc := &genai.Content{Role: genai.RoleUser}
			for i < len(msgs) && msgs[i].Role == RoleTool {
				uc.Parts = append(uc.Parts, &genai.Part{
					FunctionResponse: &genai.FunctionResponse{
						ID:       msgs[i].ToolCallID,
						Name:     msgs[i].Name,
						Response: map[string]any{"result": msgs[i].Content},
					},
				})
				i++
			}
			contents = append(contents, uc)

		default:
			i++
		}
	}
	return contents, sysText
}

func (p *GeminiProvider) buildTools(tools []ToolDefinition) []*genai.Tool {
	decls := make([]*genai.FunctionDeclaration, len(tools))
	for i, t := range tools {
		decls[i] = &genai.FunctionDeclaration{
			Name:                 t.Name,
			Description:          t.Description,
			ParametersJsonSchema: t.Parameters,
		}
	}
	return []*genai.Tool{{FunctionDeclarations: decls}}
}
