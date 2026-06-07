package llm

import "context"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is a single turn in a conversation.
type Message struct {
	Role       Role
	Content    string
	ToolCallID string     // populated on tool-result messages (RoleTool)
	ToolCalls  []ToolCall // populated on assistant messages that invoke tools
	Name       string     // tool name, for tool-result messages
}

// ToolDefinition describes a callable tool exposed to the LLM.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON schema object
}

// ToolCall is a single tool invocation requested by the LLM.
type ToolCall struct {
	ID    string
	Name  string
	Input map[string]any
}

// CompletionRequest is the provider-agnostic input for a chat completion.
type CompletionRequest struct {
	Messages    []Message
	Tools       []ToolDefinition
	Temperature float32
	MaxTokens   int
}

// Response is the non-streaming result from a completion call.
type Response struct {
	Content   string
	ToolCalls []ToolCall
	Usage     Usage
}

// Usage tracks token consumption for a completion.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Delta is one streamed chunk from a completion.
type Delta struct {
	Content   string
	ToolCalls []ToolCall
	Done      bool
	Err       error
}

// LLMProvider is the central interface every component calls.
// Nothing outside internal/llm imports provider SDKs directly.
type LLMProvider interface {
	Complete(ctx context.Context, req CompletionRequest) (*Response, error)
	Stream(ctx context.Context, req CompletionRequest) (<-chan Delta, error)
	// Embed returns a 1536-dim vector. Providers that don't support embeddings
	// must return an error — callers route embeddings to a dedicated embedder.
	Embed(ctx context.Context, text string) ([]float32, error)
	Model() string
}
