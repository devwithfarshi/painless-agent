package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL string
	RedisURL    string
	Env         string

	// LLM chat provider. One of: "openai", "anthropic".
	LLMProvider     string
	OpenAIAPIKey    string
	AnthropicAPIKey string
	LLMModel        string // defaults per provider if empty

	// Embedding provider. Always "openai" — Anthropic has no embeddings API.
	// Switching chat providers must NOT change this; re-embedding all stored
	// memories would be required if the embedding model/dims change.
	EmbeddingProvider string // reserved for future providers; currently only "openai"
	EmbeddingModel    string // defaults to "text-embedding-3-small" (1536 dims)
}

func Load(envFile string) (*Config, error) {
	if envFile != "" {
		if err := godotenv.Load(envFile); err != nil {
			return nil, fmt.Errorf("load env file: %w", err)
		}
	}

	cfg := &Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    os.Getenv("REDIS_URL"),
		Env:         envOr("ENV", "development"),

		LLMProvider:     envOr("LLM_PROVIDER", "anthropic"),
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		LLMModel:        os.Getenv("LLM_MODEL"),

		EmbeddingProvider: envOr("EMBEDDING_PROVIDER", "openai"),
		EmbeddingModel:    envOr("EMBEDDING_MODEL", "text-embedding-3-small"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.RedisURL == "" {
		return nil, fmt.Errorf("REDIS_URL is required")
	}

	// Validate the selected provider has the required key.
	switch cfg.LLMProvider {
	case "openai":
		if cfg.OpenAIAPIKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY is required when LLM_PROVIDER=openai")
		}
	case "anthropic":
		if cfg.AnthropicAPIKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY is required when LLM_PROVIDER=anthropic")
		}
	default:
		return nil, fmt.Errorf("unknown LLM_PROVIDER %q (supported: openai, anthropic)", cfg.LLMProvider)
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
