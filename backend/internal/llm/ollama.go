package llm

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

const ollamaDefaultBaseURL = "http://localhost:11434/v1"

// NewOllama creates an LLMProvider backed by a local Ollama instance.
// Ollama exposes an OpenAI-compatible API; no API key is required.
// baseURL defaults to http://localhost:11434/v1 if empty.
func NewOllama(baseURL, model string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = ollamaDefaultBaseURL
	}
	return &OpenAIProvider{
		client: openai.NewClient(
			option.WithAPIKey("ollama"), // Ollama ignores the key; placeholder required by SDK
			option.WithBaseURL(baseURL),
		),
		model: model,
	}
}
