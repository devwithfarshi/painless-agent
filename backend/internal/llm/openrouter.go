package llm

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

const openRouterBaseURL = "https://openrouter.ai/api/v1"

// NewOpenRouter creates an LLMProvider backed by OpenRouter's OpenAI-compatible API.
// OpenRouter requires an HTTP-Referer header for attribution.
// Embeddings are not supported; use NewEmbedder (OpenAI) for vector storage.
func NewOpenRouter(apiKey, model string) *OpenAIProvider {
	return &OpenAIProvider{
		client: openai.NewClient(
			option.WithAPIKey(apiKey),
			option.WithBaseURL(openRouterBaseURL),
			option.WithHeader("HTTP-Referer", "https://github.com/devwithfarshi/painless-agent"),
			option.WithHeader("X-Title", "painless-agent"),
		),
		model: model,
	}
}
