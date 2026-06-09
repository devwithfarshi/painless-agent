package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/devwithfarshi/painless-agent/internal/llm"
	"github.com/devwithfarshi/painless-agent/pkg/config"
)

// SetProvider hot-swaps the active LLM provider at runtime.
// POST /api/config/provider  body: {"provider":"gemini","model":"gemini-2.0-flash","api_key":"..."}
func (h *Handlers) SetProvider(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		APIKey   string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}

	// Build an ephemeral config just for provider construction.
	scratch := *h.Cfg
	scratch.LLMProvider = body.Provider
	if body.Model != "" {
		scratch.LLMModel = body.Model
	}
	if body.APIKey != "" {
		switch body.Provider {
		case "openai":
			scratch.OpenAIAPIKey = body.APIKey
		case "anthropic":
			scratch.AnthropicAPIKey = body.APIKey
		case "gemini":
			scratch.GeminiAPIKey = body.APIKey
		case "openrouter":
			scratch.OpenRouterAPIKey = body.APIKey
		}
	}

	newProvider, err := llm.New(&config.Config{
		LLMProvider:      scratch.LLMProvider,
		LLMModel:         scratch.LLMModel,
		OpenAIAPIKey:     scratch.OpenAIAPIKey,
		AnthropicAPIKey:  scratch.AnthropicAPIKey,
		GeminiAPIKey:     scratch.GeminiAPIKey,
		OllamaBaseURL:    scratch.OllamaBaseURL,
		OpenRouterAPIKey: scratch.OpenRouterAPIKey,
		LLMMaxRetries:    scratch.LLMMaxRetries,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid provider config: "+err.Error())
		return
	}

	if h.Provider != nil {
		h.Provider.Swap(newProvider)
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":   "ok",
		"provider": body.Provider,
		"model":    newProvider.Model(),
	})
}
