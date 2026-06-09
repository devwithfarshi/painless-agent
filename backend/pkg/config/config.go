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

	// UserName is set during first-run onboarding and used for personalisation.
	UserName string

	// LLM chat provider. One of: "openai", "anthropic", "copilot".
	LLMProvider     string
	OpenAIAPIKey    string
	AnthropicAPIKey string
	LLMModel        string // defaults per provider if empty

	// Embedding provider. Always "openai" — Anthropic has no embeddings API.
	// Switching chat providers must NOT change this; re-embedding all stored
	// memories would be required if the embedding model/dims change.
	EmbeddingProvider string // reserved for future providers; currently only "openai"
	EmbeddingModel    string // defaults to "text-embedding-3-small" (1536 dims)

	// Tool configuration.
	BraveAPIKey       string // Brave Search API key (primary web search)
	SerpAPIKey        string // SerpAPI key (fallback web search)
	FilesystemRoot    string // allowed root for filesystem tool (default: current dir)
	ToolMaxOutputKB   int    // max tool output size in KB before summarisation (default: 32)
	ToolTimeoutSecs   int    // HTTP/tool execution timeout in seconds (default: 30)
	HTTPMaxBodyKB     int    // max HTTP response body in KB (default: 256)

	// Code executor (Docker sandbox).
	DockerHost          string // Docker daemon socket; empty = DOCKER_HOST env / default
	CodeExecTimeoutSecs int    // per-execution timeout for the sandbox (default: 30)

	// Browser tool.
	// When set, the browser tool connects to a remote Chrome via CDP instead of
	// spawning a local Chrome process. Use the docker-compose chrome service:
	//   CHROME_CDP_URL=ws://localhost:9222
	ChromeCDPURL string

	// GitHub tool.
	GitHubToken string // personal access token with repo scope
}

func Load(envFile string) (*Config, error) {
	if envFile != "" {
		// Tolerate a missing .env file — values can come from env vars or onboarding config.
		if err := godotenv.Load(envFile); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("load env file: %w", err)
		}
	}

	cfg := &Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    os.Getenv("REDIS_URL"),
		Env:         envOr("ENV", "development"),

		UserName: os.Getenv("AGENT_USER_NAME"),

		LLMProvider:     envOr("LLM_PROVIDER", "anthropic"),
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		LLMModel:        os.Getenv("LLM_MODEL"),

		EmbeddingProvider: envOr("EMBEDDING_PROVIDER", "openai"),
		EmbeddingModel:    envOr("EMBEDDING_MODEL", "text-embedding-3-small"),

		BraveAPIKey:     os.Getenv("BRAVE_API_KEY"),
		SerpAPIKey:      os.Getenv("SERP_API_KEY"),
		FilesystemRoot:  envOr("FILESYSTEM_ROOT", "workspace"),
		ToolMaxOutputKB: envInt("TOOL_MAX_OUTPUT_KB", 32),
		ToolTimeoutSecs: envInt("TOOL_TIMEOUT_SECS", 30),
		HTTPMaxBodyKB:   envInt("HTTP_MAX_BODY_KB", 256),

		DockerHost:          os.Getenv("DOCKER_HOST"), // empty = default socket
		CodeExecTimeoutSecs: envInt("CODE_EXEC_TIMEOUT_SECS", 30),

		ChromeCDPURL: os.Getenv("CHROME_CDP_URL"),

		GitHubToken: os.Getenv("GITHUB_TOKEN"),
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
	case "copilot":
		// GitHub token is resolved at provider init via COPILOT_GITHUB_TOKEN / GH_TOKEN /
		// GITHUB_TOKEN env vars, `gh auth token`, stored token, or device-code login.
	default:
		return nil, fmt.Errorf("unknown LLM_PROVIDER %q (supported: openai, anthropic, copilot)", cfg.LLMProvider)
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}
