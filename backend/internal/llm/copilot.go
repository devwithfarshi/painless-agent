package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// package-level var so tests can override the chat endpoint
var copilotChatEndpoint = "https://api.githubcopilot.com/chat/completions"

// CopilotProvider implements LLMProvider against the GitHub Copilot API.
// The Copilot API is OpenAI-compatible; tokens are fetched via the Copilot
// internal endpoint and cached until 30 s before expiry.
type CopilotProvider struct {
	model       string
	githubToken string // GitHub OAuth token used to exchange for Copilot API tokens

	mu          sync.Mutex
	apiToken    string
	tokenExpiry time.Time
}

// NewCopilot creates a CopilotProvider. githubToken must be a validated GitHub
// OAuth token (gho_, github_pat_, or ghu_ prefix).
func NewCopilot(githubToken, model string) *CopilotProvider {
	return &CopilotProvider{
		model:       model,
		githubToken: githubToken,
	}
}

func (p *CopilotProvider) Model() string { return p.model }

func (p *CopilotProvider) Complete(ctx context.Context, req CompletionRequest) (*Response, error) {
	token, err := p.getAPIToken(ctx)
	if err != nil {
		return nil, err
	}

	body, err := p.buildRequestBody(req, false)
	if err != nil {
		return nil, fmt.Errorf("copilot complete: marshal request: %w", err)
	}

	httpResp, err := p.doRequest(ctx, token, body)
	if err != nil {
		return nil, fmt.Errorf("copilot complete: %w", err)
	}

	if httpResp.StatusCode == http.StatusUnauthorized {
		// Refresh the Copilot token and retry once.
		p.invalidateToken()
		httpResp.Body.Close()
		token, err = p.getAPIToken(ctx)
		if err != nil {
			return nil, fmt.Errorf("copilot complete: refresh token: %w", err)
		}
		httpResp, err = p.doRequest(ctx, token, body)
		if err != nil {
			return nil, fmt.Errorf("copilot complete (retry): %w", err)
		}
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("copilot complete: HTTP %d", httpResp.StatusCode)
	}

	var cr chatCompletionResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&cr); err != nil {
		return nil, fmt.Errorf("copilot complete: decode response: %w", err)
	}
	if cr.Error != nil {
		return nil, fmt.Errorf("copilot API error: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return &Response{}, nil
	}

	choice := cr.Choices[0]
	resp := &Response{
		Content: choice.Message.Content,
		Usage: Usage{
			InputTokens:  cr.Usage.PromptTokens,
			OutputTokens: cr.Usage.CompletionTokens,
		},
	}
	for _, tc := range choice.Message.ToolCalls {
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

func (p *CopilotProvider) Stream(ctx context.Context, req CompletionRequest) (<-chan Delta, error) {
	token, err := p.getAPIToken(ctx)
	if err != nil {
		return nil, err
	}

	body, err := p.buildRequestBody(req, true)
	if err != nil {
		return nil, fmt.Errorf("copilot stream: marshal request: %w", err)
	}

	httpResp, err := p.doRequest(ctx, token, body)
	if err != nil {
		return nil, fmt.Errorf("copilot stream: %w", err)
	}

	if httpResp.StatusCode == http.StatusUnauthorized {
		p.invalidateToken()
		httpResp.Body.Close()
		token, err = p.getAPIToken(ctx)
		if err != nil {
			return nil, fmt.Errorf("copilot stream: refresh token: %w", err)
		}
		httpResp, err = p.doRequest(ctx, token, body)
		if err != nil {
			return nil, fmt.Errorf("copilot stream (retry): %w", err)
		}
	}

	if httpResp.StatusCode != http.StatusOK {
		httpResp.Body.Close()
		return nil, fmt.Errorf("copilot stream: HTTP %d", httpResp.StatusCode)
	}

	ch := make(chan Delta, 32)
	go func() {
		defer close(ch)
		defer httpResp.Body.Close()

		scanner := bufio.NewScanner(httpResp.Body)
		scanner.Buffer(make([]byte, 1<<20), 1<<20)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				ch <- Delta{Done: true}
				return
			}
			var chunk streamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if chunk.Error != nil {
				ch <- Delta{Err: fmt.Errorf("copilot stream: %s", chunk.Error.Message), Done: true}
				return
			}
			if len(chunk.Choices) > 0 {
				if content := chunk.Choices[0].Delta.Content; content != "" {
					ch <- Delta{Content: content}
				}
			}
		}
		if err := scanner.Err(); err != nil {
			ch <- Delta{Err: fmt.Errorf("copilot stream: %w", err), Done: true}
			return
		}
		ch <- Delta{Done: true}
	}()
	return ch, nil
}

func (p *CopilotProvider) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, fmt.Errorf("copilot does not support embeddings; use EMBEDDING_PROVIDER=openai")
}

// getAPIToken returns a cached Copilot API token, refreshing if it expires within 30 s.
func (p *CopilotProvider) getAPIToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.apiToken != "" && time.Now().Before(p.tokenExpiry.Add(-30*time.Second)) {
		return p.apiToken, nil
	}

	ct, err := ExchangeCopilotToken(ctx, p.githubToken)
	if err != nil {
		return "", fmt.Errorf("get copilot API token: %w", err)
	}
	p.apiToken = ct.Token
	p.tokenExpiry = time.Unix(ct.ExpiresAt, 0)
	return p.apiToken, nil
}

func (p *CopilotProvider) invalidateToken() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.apiToken = ""
	p.tokenExpiry = time.Time{}
}

func (p *CopilotProvider) doRequest(ctx context.Context, apiToken string, bodyBytes []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, copilotChatEndpoint,
		bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Editor-Version", "painless-agent/0.1.0")
	req.Header.Set("Copilot-Integration-Id", "vscode-chat")
	req.Header.Set("Openai-Intent", "conversation-panel")
	req.Header.Set("x-initiator", "user")

	return copilotHTTPClient.Do(req)
}

func (p *CopilotProvider) buildRequestBody(req CompletionRequest, stream bool) ([]byte, error) {
	r := chatCompletionRequest{
		Model:    p.model,
		Messages: buildCopilotMessages(req.Messages),
		Stream:   stream,
	}
	if req.Temperature != 0 {
		r.Temperature = req.Temperature
	}
	if req.MaxTokens != 0 {
		r.MaxTokens = req.MaxTokens
	}
	if len(req.Tools) > 0 {
		r.Tools = buildCopilotTools(req.Tools)
	}
	return json.Marshal(r)
}

// --- OpenAI-compatible request/response types for the Copilot API ---

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float32       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Tools       []chatTool    `json:"tools,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	Name       string         `json:"name,omitempty"`
}

type chatTool struct {
	Type     string          `json:"type"`
	Function chatFunctionDef `json:"function"`
}

type chatFunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type chatToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function chatFunctionCall `json:"function"`
}

type chatFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatCompletionResponse struct {
	Choices []chatChoice `json:"choices"`
	Usage   chatUsage    `json:"usage"`
	Error   *chatAPIErr  `json:"error,omitempty"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
	Delta   chatMessage `json:"delta"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type chatAPIErr struct {
	Message string `json:"message"`
}

type streamChunk struct {
	Choices []chatChoice `json:"choices"`
	Error   *chatAPIErr  `json:"error,omitempty"`
}

func buildCopilotMessages(msgs []Message) []chatMessage {
	out := make([]chatMessage, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case RoleSystem:
			out = append(out, chatMessage{Role: "system", Content: m.Content})
		case RoleUser:
			out = append(out, chatMessage{Role: "user", Content: m.Content})
		case RoleAssistant:
			cm := chatMessage{Role: "assistant", Content: m.Content}
			if len(m.ToolCalls) > 0 {
				cm.ToolCalls = make([]chatToolCall, len(m.ToolCalls))
				for i, tc := range m.ToolCalls {
					args, _ := json.Marshal(tc.Input)
					cm.ToolCalls[i] = chatToolCall{
						ID:   tc.ID,
						Type: "function",
						Function: chatFunctionCall{
							Name:      tc.Name,
							Arguments: string(args),
						},
					}
				}
			}
			out = append(out, cm)
		case RoleTool:
			out = append(out, chatMessage{
				Role:       "tool",
				Content:    m.Content,
				ToolCallID: m.ToolCallID,
				Name:       m.Name,
			})
		}
	}
	return out
}

func buildCopilotTools(tools []ToolDefinition) []chatTool {
	out := make([]chatTool, len(tools))
	for i, t := range tools {
		out[i] = chatTool{
			Type: "function",
			Function: chatFunctionDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}
	return out
}
