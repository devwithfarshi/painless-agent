package onboarding

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/devwithfarshi/painless-agent/internal/llm"
)

// profileCfg is persisted at ~/.painless-agent/config.json.
type profileCfg struct {
	UserName  string `json:"user_name"`
	SetupDone bool   `json:"setup_done"`
}

func profilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".painless-agent", "config.json"), nil
}

// IsFirstRun returns true when the user has not completed onboarding yet.
func IsFirstRun() bool {
	path, err := profilePath()
	if err != nil {
		return true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	var p profileCfg
	_ = json.Unmarshal(data, &p)
	return !p.SetupDone
}

// LoadUserName returns the name saved during onboarding, or "" if not set.
func LoadUserName() string {
	path, err := profilePath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var p profileCfg
	_ = json.Unmarshal(data, &p)
	return p.UserName
}

func saveProfile(name string) error {
	path, err := profilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(profileCfg{UserName: name, SetupDone: true}, "", "  ")
	return os.WriteFile(path, data, 0600)
}

// Run executes the interactive first-run wizard.
// envPath is the path to the .env file to update with the chosen settings.
func Run(ctx context.Context, envPath string) error {
	r := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("────────────────────────────────────────────────")
	fmt.Println("  Welcome to painless-agent!")
	fmt.Println("────────────────────────────────────────────────")
	fmt.Println("  First-run setup — takes about a minute.")
	fmt.Println()

	// Step 1: name
	fmt.Print("  Your name: ")
	name := readLine(r)
	if name == "" {
		name = "User"
	}

	// Step 2: provider
	fmt.Println()
	fmt.Println("  Available AI providers:")
	fmt.Println("    [1] OpenAI         — GPT-4o, o3-mini, etc.")
	fmt.Println("    [2] Anthropic      — Claude Sonnet, Opus, Haiku")
	fmt.Println("    [3] GitHub Copilot — GPT-4o via your Copilot subscription")
	fmt.Println("    [4] Google Gemini  — Gemini 2.0 Flash, Pro, etc.")
	fmt.Println("    [5] Ollama         — local models (llama3, mistral, phi3, ...)")
	fmt.Println("    [6] OpenRouter     — 300+ models via one API key")
	fmt.Println()
	providerIdx := promptInt(r, "  Select provider [1-6]: ", 1, 6)
	providers := []string{"openai", "anthropic", "copilot", "gemini", "ollama", "openrouter"}
	provider := providers[providerIdx-1]

	// Step 3: authentication
	envSettings := map[string]string{"LLM_PROVIDER": provider}
	var apiKey string

	switch provider {
	case "openai":
		fmt.Println()
		fmt.Println("  OpenAI setup")
		fmt.Println("  Get your API key at: https://platform.openai.com/api-keys")
		fmt.Print("  API key (sk-...): ")
		apiKey = readLine(r)
		if apiKey == "" {
			return fmt.Errorf("OpenAI API key is required")
		}
		envSettings["OPENAI_API_KEY"] = apiKey

	case "anthropic":
		fmt.Println()
		fmt.Println("  Anthropic setup")
		fmt.Println("  Get your API key at: https://console.anthropic.com/settings/keys")
		fmt.Print("  API key (sk-ant-...): ")
		apiKey = readLine(r)
		if apiKey == "" {
			return fmt.Errorf("Anthropic API key is required")
		}
		envSettings["ANTHROPIC_API_KEY"] = apiKey

	case "copilot":
		fmt.Println()
		fmt.Println("  GitHub Copilot setup")
		fmt.Println("  You need an active GitHub Copilot subscription.")
		fmt.Println("  Launching device authorization...")
		fmt.Println()
		githubToken, err := llm.ResolveGitHubToken(ctx)
		if err != nil {
			return fmt.Errorf("Copilot auth: %w", err)
		}
		// Make the token available immediately so llm.New() skips the device flow.
		_ = os.Setenv("COPILOT_GITHUB_TOKEN", githubToken)
		// Stash for model fetch below.
		apiKey = githubToken

	case "gemini":
		fmt.Println()
		fmt.Println("  Google Gemini setup")
		fmt.Println("  Get your API key at: https://aistudio.google.com/app/apikey")
		fmt.Print("  API key: ")
		apiKey = readLine(r)
		if apiKey == "" {
			return fmt.Errorf("Gemini API key is required")
		}
		envSettings["GEMINI_API_KEY"] = apiKey

	case "ollama":
		fmt.Println()
		fmt.Println("  Ollama setup (local)")
		fmt.Println("  Make sure Ollama is running: https://ollama.ai")
		fmt.Printf("  Base URL [http://localhost:11434/v1]: ")
		baseURL := readLine(r)
		if baseURL == "" {
			baseURL = "http://localhost:11434/v1"
		}
		envSettings["OLLAMA_BASE_URL"] = baseURL

	case "openrouter":
		fmt.Println()
		fmt.Println("  OpenRouter setup")
		fmt.Println("  Get your API key at: https://openrouter.ai/keys")
		fmt.Print("  API key (sk-or-...): ")
		apiKey = readLine(r)
		if apiKey == "" {
			return fmt.Errorf("OpenRouter API key is required")
		}
		envSettings["OPENROUTER_API_KEY"] = apiKey
	}

	// Step 4: fetch models
	fmt.Println()
	fmt.Print("  Fetching available models...")
	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	models, err := fetchModels(fetchCtx, provider, apiKey)
	if err != nil || len(models) == 0 {
		if err != nil {
			fmt.Printf(" (fetch failed: %v)\n", err)
		} else {
			fmt.Println()
		}
		fmt.Println("  Using default model list.")
		models = defaultModels(provider)
	} else {
		fmt.Println(" done.")
	}

	// Step 5: select model
	fmt.Println()
	fmt.Println("  Available models:")
	for i, m := range models {
		if i == 0 {
			fmt.Printf("    [%d] %s  (recommended)\n", i+1, m)
		} else {
			fmt.Printf("    [%d] %s\n", i+1, m)
		}
	}
	fmt.Println()
	modelIdx := promptInt(r, fmt.Sprintf("  Select model [1-%d]: ", len(models)), 1, len(models))
	selectedModel := models[modelIdx-1]
	envSettings["LLM_MODEL"] = selectedModel

	// Step 6: persist settings
	if err := updateEnvFile(envPath, envSettings); err != nil {
		return fmt.Errorf("save settings to %s: %w", envPath, err)
	}
	if err := saveProfile(name); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: couldn't save profile: %v\n", err)
	}

	// Step 7: summary
	providerDisplay := map[string]string{
		"openai":      "OpenAI",
		"anthropic":   "Anthropic",
		"copilot":     "GitHub Copilot",
		"gemini":      "Google Gemini",
		"ollama":      "Ollama (local)",
		"openrouter":  "OpenRouter",
	}[provider]
	fmt.Println()
	fmt.Println("  ✓ Configuration saved!")
	fmt.Printf("    Hello    : %s\n", name)
	fmt.Printf("    Provider : %s\n", providerDisplay)
	fmt.Printf("    Model    : %s\n", selectedModel)
	fmt.Println()
	fmt.Println("────────────────────────────────────────────────")
	fmt.Println("  Starting painless-agent...")
	fmt.Println()

	return nil
}

// --- helpers ---

func readLine(r *bufio.Reader) string {
	line, _ := r.ReadString('\n')
	return strings.TrimSpace(line)
}

func promptInt(r *bufio.Reader, prompt string, min, max int) int {
	for {
		fmt.Print(prompt)
		line := readLine(r)
		n, err := strconv.Atoi(line)
		if err == nil && n >= min && n <= max {
			return n
		}
		fmt.Printf("  Please enter a number between %d and %d.\n", min, max)
	}
}

// updateEnvFile writes key=value pairs into path, updating existing lines
// in-place and appending new ones, preserving all comments and formatting.
func updateEnvFile(path string, settings map[string]string) error {
	existing, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(existing), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}

	updated := make(map[string]bool)
	for i, line := range lines {
		for key, val := range settings {
			if strings.HasPrefix(line, key+"=") || strings.HasPrefix(line, "# "+key+"=") {
				// Replace or uncomment the line.
				lines[i] = key + "=" + val
				updated[key] = true
				break
			}
		}
	}
	for key, val := range settings {
		if !updated[key] {
			lines = append(lines, key+"="+val)
		}
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600)
}

// --- model fetching ---

func fetchModels(ctx context.Context, provider, apiKey string) ([]string, error) {
	switch provider {
	case "openai":
		return fetchOpenAIModels(ctx, apiKey)
	case "anthropic":
		return fetchAnthropicModels(ctx, apiKey)
	case "copilot":
		return llm.FetchCopilotModels(ctx, apiKey)
	default:
		return defaultModels(provider), nil
	}
}

func defaultModels(provider string) []string {
	switch provider {
	case "openai":
		return []string{"gpt-4o", "gpt-4o-mini", "o1", "o3-mini", "gpt-4-turbo", "gpt-4", "gpt-3.5-turbo"}
	case "anthropic":
		return []string{
			"claude-opus-4-8-20251101",
			"claude-sonnet-4-6-20251001",
			"claude-haiku-4-5-20251001",
		}
	case "copilot":
		return []string{"gpt-4o", "gpt-4o-mini", "o3-mini", "gpt-4-turbo"}
	case "gemini":
		return []string{
			"gemini-2.0-flash",
			"gemini-2.5-pro",
			"gemini-2.5-flash",
			"gemini-1.5-pro",
			"gemini-1.5-flash",
		}
	case "ollama":
		return []string{
			"llama3.2",
			"llama3.1",
			"qwen2.5",
			"mistral",
			"phi3",
			"gemma3",
		}
	case "openrouter":
		return []string{
			"meta-llama/llama-3.3-70b-instruct",
			"anthropic/claude-3.5-sonnet",
			"openai/gpt-4o",
			"google/gemini-2.0-flash-001",
			"mistralai/mistral-7b-instruct",
			"deepseek/deepseek-chat",
		}
	default:
		return nil
	}
}

// openAI model priority for display order (lower = shown first).
var openAIModelPriority = map[string]int{
	"gpt-4o":        1,
	"gpt-4o-mini":   2,
	"o1":            3,
	"o3-mini":       4,
	"gpt-4-turbo":   5,
	"gpt-4":         6,
	"gpt-3.5-turbo": 7,
}

type openAIModelsResp struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func fetchOpenAIModels(ctx context.Context, apiKey string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var list openAIModelsResp
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}

	var models []string
	for _, m := range list.Data {
		if isOpenAIChatModel(m.ID) {
			models = append(models, m.ID)
		}
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("no chat models returned")
	}

	sort.Slice(models, func(i, j int) bool {
		pi, oki := openAIModelPriority[models[i]]
		pj, okj := openAIModelPriority[models[j]]
		if oki && okj {
			return pi < pj
		}
		if oki {
			return true
		}
		if okj {
			return false
		}
		return models[i] < models[j]
	})
	return models, nil
}

// chatModelPrefixes are the model ID prefixes we consider as chat-capable.
var chatModelPrefixes = []string{"gpt-4o", "gpt-4-turbo", "gpt-4", "gpt-3.5-turbo", "o1", "o3", "o4"}

// chatModelExcludes are suffix/substring patterns that indicate non-chat variants.
var chatModelExcludes = []string{"-instruct", "-0301", "-0314", "-base"}

func isOpenAIChatModel(id string) bool {
	for _, p := range chatModelPrefixes {
		if strings.HasPrefix(id, p) {
			for _, e := range chatModelExcludes {
				if strings.Contains(id, e) {
					return false
				}
			}
			return true
		}
	}
	return false
}

type anthropicModelsResp struct {
	Data []struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"data"`
}

func fetchAnthropicModels(ctx context.Context, apiKey string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.anthropic.com/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var list anthropicModelsResp
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}

	var models []string
	for _, m := range list.Data {
		if strings.HasPrefix(m.ID, "claude-") {
			models = append(models, m.ID)
		}
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("no models returned")
	}
	// Anthropic returns models newest-first; keep that order.
	return models, nil
}
