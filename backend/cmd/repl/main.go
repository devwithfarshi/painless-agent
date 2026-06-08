package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/devwithfarshi/painless-agent/internal/llm"
	memorystore "github.com/devwithfarshi/painless-agent/internal/memory"
	"github.com/devwithfarshi/painless-agent/internal/onboarding"
	"github.com/devwithfarshi/painless-agent/internal/store"
	"github.com/devwithfarshi/painless-agent/internal/tools"
	"github.com/devwithfarshi/painless-agent/pkg/config"
	"github.com/devwithfarshi/painless-agent/pkg/db"
	"github.com/devwithfarshi/painless-agent/pkg/logger"
)

const systemPrompt = `You are a helpful AI assistant with access to tools.
You can search the web, read and write files, make HTTP requests, and save information to memory.
Be concise and direct. Use tools when they provide better or more current information than you have.`

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if onboarding.IsFirstRun() {
		if err := onboarding.Run(ctx, ".env"); err != nil {
			fmt.Fprintln(os.Stderr, "setup failed:", err)
			os.Exit(1)
		}
	}
	if name := onboarding.LoadUserName(); name != "" {
		if os.Getenv("AGENT_USER_NAME") == "" {
			os.Setenv("AGENT_USER_NAME", name)
		}
	}

	cfg, err := config.Load(".env")
	if err != nil {
		fmt.Fprintln(os.Stderr, "load config:", err)
		os.Exit(1)
	}

	log := logger.New(cfg.Env)

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect to postgres:", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.EnablePgVector(ctx, pool); err != nil {
		fmt.Fprintln(os.Stderr, "enable pgvector:", err)
		os.Exit(1)
	}
	if err := db.Migrate(pool); err != nil {
		fmt.Fprintln(os.Stderr, "run migrations:", err)
		os.Exit(1)
	}

	rdb, err := db.NewRedis(ctx, cfg.RedisURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect to redis:", err)
		os.Exit(1)
	}
	defer rdb.Close()

	provider, err := llm.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "init llm provider:", err)
		os.Exit(1)
	}

	var memStore memorystore.MemoryStore
	if embedder, embErr := llm.NewEmbedder(cfg); embErr == nil {
		memStore = memorystore.NewPgMemoryStore(pool, embedder)
	} else {
		log.Warn("embedder unavailable — memory disabled", "error", embErr)
	}

	toolLogStore := store.NewToolLogStore(pool)

	summarizer := tools.NewSummarizerTool(provider)
	maxOutput := cfg.ToolMaxOutputKB * 1024
	if maxOutput == 0 {
		maxOutput = 32 * 1024
	}
	engine := tools.NewEngine(maxOutput, summarizer.Summarize)
	engine.Register(tools.NewHTTPClientTool(cfg.ToolTimeoutSecs, cfg.HTTPMaxBodyKB))
	engine.Register(tools.NewFilesystemTool(cfg.FilesystemRoot))
	engine.Register(tools.NewWebSearchTool(cfg.BraveAPIKey, cfg.SerpAPIKey))
	engine.Register(summarizer)
	if memStore != nil {
		engine.Register(tools.NewMemoryTool(memStore))
	}

	name := cfg.UserName
	if name == "" {
		name = "you"
	}

	fmt.Printf("\npainless-agent — %s (%s)\n", provider.Model(), cfg.LLMProvider)
	fmt.Println("type 'exit' or press Ctrl+C to quit")
	fmt.Println()

	runREPL(ctx, provider, engine, toolLogStore, memStore, name)
}

func runREPL(
	ctx context.Context,
	provider llm.LLMProvider,
	engine *tools.ToolEngine,
	toolLogs *store.ToolLogStore,
	memStore memorystore.MemoryStore,
	userName string,
) {
	sessionID := uuid.New()
	history := []llm.Message{
		{Role: llm.RoleSystem, Content: systemPrompt},
	}

	scanner := bufio.NewScanner(os.Stdin)
	prompt := userName + "> "

	for {
		fmt.Print(prompt)

		if !scanner.Scan() {
			fmt.Println()
			break
		}

		select {
		case <-ctx.Done():
			fmt.Println("\ngoodbye")
			return
		default:
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			fmt.Println("goodbye")
			return
		}

		// Enrich with relevant memory before sending.
		if memStore != nil {
			if memories, err := memStore.Search(ctx, input, 3); err == nil && len(memories) > 0 {
				var sb strings.Builder
				sb.WriteString("[relevant memory]\n")
				for _, m := range memories {
					fmt.Fprintf(&sb, "- %s\n", m)
				}
				sb.WriteString("\n")
				sb.WriteString(input)
				input = sb.String()
			}
		}

		history = append(history, llm.Message{Role: llm.RoleUser, Content: input})

		response, updatedHistory, err := chat(ctx, provider, engine, toolLogs, history, sessionID)
		if err != nil {
			fmt.Println("\nerror:", err)
			history = history[:len(history)-1] // roll back the user message
			continue
		}

		history = updatedHistory
		fmt.Printf("\nagent> %s\n\n", response)

		// Store the exchange in memory for future sessions.
		if memStore != nil {
			_ = memStore.Store(ctx, response, nil, sessionID)
		}
	}
}

// chat runs the Think→Act→Observe inner loop and returns the final text response.
func chat(
	ctx context.Context,
	provider llm.LLMProvider,
	engine *tools.ToolEngine,
	toolLogs *store.ToolLogStore,
	history []llm.Message,
	sessionID uuid.UUID,
) (string, []llm.Message, error) {
	const maxIter = 10
	for i := 0; i < maxIter; i++ {
		resp, err := provider.Complete(ctx, llm.CompletionRequest{
			Messages:    history,
			Tools:       engine.Definitions(),
			Temperature: 0.7,
			MaxTokens:   2048,
		})
		if err != nil {
			return "", history, fmt.Errorf("llm: %w", err)
		}

		if len(resp.ToolCalls) == 0 {
			history = append(history, llm.Message{Role: llm.RoleAssistant, Content: resp.Content})
			return resp.Content, history, nil
		}

		history = append(history, llm.Message{
			Role:      llm.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		for _, tc := range resp.ToolCalls {
			fmt.Printf("  → %s\n", formatCall(tc))

			start := time.Now()
			result, execErr := engine.Execute(ctx, tc.Name, tc.Input)
			durationMs := int(time.Since(start).Milliseconds())
			if execErr != nil {
				result = "error: " + execErr.Error()
			}

			fmt.Printf("  ✓ %dms\n", durationMs)

			_ = toolLogs.Log(ctx, sessionID, uuid.Nil, tc.Name, tc.Input, result, durationMs)

			history = append(history, llm.Message{
				Role:       llm.RoleTool,
				Content:    result,
				ToolCallID: tc.ID,
				Name:       tc.Name,
			})
		}
	}
	return "", history, fmt.Errorf("exceeded %d tool iterations", maxIter)
}

// formatCall renders a tool invocation as a readable one-liner.
func formatCall(tc llm.ToolCall) string {
	switch tc.Name {
	case "web_search":
		if q, ok := tc.Input["query"].(string); ok {
			return fmt.Sprintf("web_search(%q)", truncate(q, 60))
		}
	case "filesystem":
		op, _ := tc.Input["operation"].(string)
		path, _ := tc.Input["path"].(string)
		return fmt.Sprintf("filesystem(%s %q)", op, path)
	case "http_client":
		method, _ := tc.Input["method"].(string)
		u, _ := tc.Input["url"].(string)
		return fmt.Sprintf("http_client(%s %s)", method, truncate(u, 60))
	case "summarizer":
		if t, ok := tc.Input["text"].(string); ok {
			return fmt.Sprintf("summarizer(%q…)", truncate(t, 40))
		}
	case "memory_store":
		if c, ok := tc.Input["content"].(string); ok {
			return fmt.Sprintf("memory_store(%q…)", truncate(c, 40))
		}
	}
	return tc.Name + "(...)"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
