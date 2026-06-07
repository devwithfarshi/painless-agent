package llm

import (
	"fmt"

	"github.com/devwithfarshi/painless-agent/pkg/config"
)

// New returns the chat LLM provider selected by cfg.LLMProvider.
func New(cfg *config.Config) (LLMProvider, error) {
	switch cfg.LLMProvider {
	case "openai":
		if cfg.OpenAIAPIKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY is required when LLM_PROVIDER=openai")
		}
		model := cfg.LLMModel
		if model == "" {
			model = "gpt-4o"
		}
		return NewOpenAI(cfg.OpenAIAPIKey, model), nil

	case "anthropic":
		if cfg.AnthropicAPIKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY is required when LLM_PROVIDER=anthropic")
		}
		model := cfg.LLMModel
		if model == "" {
			model = "claude-sonnet-4-20250514"
		}
		return NewAnthropic(cfg.AnthropicAPIKey, model), nil

	default:
		return nil, fmt.Errorf("unknown LLM_PROVIDER %q (supported: openai, anthropic)", cfg.LLMProvider)
	}
}

// NewEmbedder returns an embedder provider.
// Embeddings always use OpenAI text-embedding-3-small (1536 dims) to match
// the memory.embedding VECTOR(1536) column. The embedding provider is
// intentionally decoupled from the chat provider: switching chat providers
// must NOT change embedding dimensions (re-embedding all memories would be
// required and is not supported automatically).
func NewEmbedder(cfg *config.Config) (LLMProvider, error) {
	if cfg.OpenAIAPIKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is required for embeddings (EMBEDDING_PROVIDER=openai)")
	}
	model := cfg.EmbeddingModel
	if model == "" {
		model = "text-embedding-3-small"
	}
	return NewOpenAI(cfg.OpenAIAPIKey, model), nil
}
