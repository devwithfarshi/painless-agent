package llm

import (
	"context"
	"fmt"

	"github.com/devwithfarshi/painless-agent/pkg/config"
)

// New returns the chat LLM provider selected by cfg.LLMProvider, wrapped with retry.
func New(cfg *config.Config) (LLMProvider, error) {
	p, err := newProvider(cfg)
	if err != nil {
		return nil, err
	}
	return WithRetry(p, cfg.LLMMaxRetries), nil
}

// newProvider constructs the raw provider without retry wrapping.
func newProvider(cfg *config.Config) (LLMProvider, error) {
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

	case "copilot":
		model := cfg.LLMModel
		if model == "" {
			model = "gpt-4o"
		}
		githubToken, err := ResolveGitHubToken(context.Background())
		if err != nil {
			return nil, fmt.Errorf("copilot: resolve GitHub token: %w", err)
		}
		return NewCopilot(githubToken, model), nil

	case "gemini":
		if cfg.GeminiAPIKey == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY is required when LLM_PROVIDER=gemini")
		}
		model := cfg.LLMModel
		if model == "" {
			model = "gemini-2.0-flash"
		}
		return NewGemini(context.Background(), cfg.GeminiAPIKey, model)

	case "ollama":
		model := cfg.LLMModel
		if model == "" {
			model = "llama3.2"
		}
		return NewOllama(cfg.OllamaBaseURL, model), nil

	case "openrouter":
		if cfg.OpenRouterAPIKey == "" {
			return nil, fmt.Errorf("OPENROUTER_API_KEY is required when LLM_PROVIDER=openrouter")
		}
		model := cfg.LLMModel
		if model == "" {
			model = "meta-llama/llama-3.3-70b-instruct"
		}
		return NewOpenRouter(cfg.OpenRouterAPIKey, model), nil

	default:
		return nil, fmt.Errorf("unknown LLM_PROVIDER %q (supported: openai, anthropic, copilot, gemini, ollama, openrouter)", cfg.LLMProvider)
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
