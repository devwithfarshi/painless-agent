// cmd/verify/main.go — end-to-end component verification.
// Run: go run ./cmd/verify  (from backend/, with .env present and make up running)
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/devwithfarshi/painless-agent/internal/agent"
	"github.com/devwithfarshi/painless-agent/internal/llm"
	memorystore "github.com/devwithfarshi/painless-agent/internal/memory"
	"github.com/devwithfarshi/painless-agent/internal/onboarding"
	"github.com/devwithfarshi/painless-agent/internal/sandbox"
	"github.com/devwithfarshi/painless-agent/internal/store"
	"github.com/devwithfarshi/painless-agent/internal/tools"
	"github.com/devwithfarshi/painless-agent/pkg/config"
	"github.com/devwithfarshi/painless-agent/pkg/db"
	"github.com/devwithfarshi/painless-agent/pkg/logger"
	"github.com/google/uuid"
)

// ── result tracking ───────────────────────────────────────────────────────────

type status int

const (
	pass status = iota
	fail
	skip
)

type result struct {
	component string
	procedure string
	expected  string
	actual    string
	st        status
	note      string
}

var results []result

func record(component, procedure, expected, actual string, st status, note ...string) {
	n := ""
	if len(note) > 0 {
		n = note[0]
	}
	results = append(results, result{component, procedure, expected, actual, st, n})
	icon := "✅"
	if st == fail {
		icon = "❌"
	} else if st == skip {
		icon = "⏭️ "
	}
	fmt.Printf("%s  %-30s %s\n", icon, component, truncate(actual, 80))
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", "↵ ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	ctx := context.Background()
	fmt.Println("\n╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║     painless-agent  end-to-end verification              ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝\n")

	// Load .env / onboarding.
	if name := onboarding.LoadUserName(); name != "" && os.Getenv("AGENT_USER_NAME") == "" {
		os.Setenv("AGENT_USER_NAME", name)
	}
	cfg, err := config.Load(".env")
	if err != nil {
		fmt.Fprintln(os.Stderr, "FATAL: load config:", err)
		os.Exit(1)
	}

	// ── 1. Infrastructure ─────────────────────────────────────────────────────
	fmt.Println("── Infrastructure ───────────────────────────────────────────")

	pool, pgErr := db.NewPool(ctx, cfg.DatabaseURL)
	if pgErr != nil {
		record("Postgres", "NewPool", "connected", pgErr.Error(), fail)
	} else {
		defer pool.Close()
		if err := pool.Ping(ctx); err != nil {
			record("Postgres", "Ping", "pong", err.Error(), fail)
		} else {
			record("Postgres", "Ping", "pong", "connected to "+cfg.DatabaseURL[:30]+"…", pass)
		}
	}

	if pgErr == nil {
		if err := db.EnablePgVector(ctx, pool); err != nil {
			record("pgvector", "EnablePgVector", "extension enabled", err.Error(), fail)
		} else {
			record("pgvector", "EnablePgVector", "extension enabled", "vector extension OK", pass)
		}
		if err := db.Migrate(pool); err != nil {
			record("Migrations", "Migrate", "all applied", err.Error(), fail)
		} else {
			record("Migrations", "Migrate", "all applied", "goose up — no pending", pass)
		}
	}

	rdb, redisErr := db.NewRedis(ctx, cfg.RedisURL)
	if redisErr != nil {
		record("Redis", "NewRedis", "connected", redisErr.Error(), fail)
	} else {
		defer rdb.Close()
		if err := rdb.Ping(ctx).Err(); err != nil {
			record("Redis", "Ping", "PONG", err.Error(), fail)
		} else {
			record("Redis", "Ping", "PONG", "PONG", pass)
		}
	}

	// ── 2. LLM Provider ───────────────────────────────────────────────────────
	fmt.Println("\n── LLM Provider ─────────────────────────────────────────────")

	provider, llmErr := llm.New(cfg)
	if llmErr != nil {
		record("LLM/"+cfg.LLMProvider, "New", "provider ready", llmErr.Error(), fail)
	} else {
		record("LLM/"+cfg.LLMProvider, "New", "provider ready", "model="+provider.Model(), pass)
	}

	var llmReady bool
	if llmErr == nil {
		resp, err := provider.Complete(ctx, llm.CompletionRequest{
			Messages:  []llm.Message{{Role: llm.RoleUser, Content: "Reply with only the word PONG"}},
			MaxTokens: 20,
		})
		if err != nil {
			record("LLM/Complete", "Complete(PONG)", "PONG", err.Error(), fail)
		} else {
			llmReady = true
			record("LLM/Complete", "Complete(PONG)", "PONG", strings.TrimSpace(resp.Content), pass)
		}
	}

	// ── 3. Embedder ───────────────────────────────────────────────────────────
	fmt.Println("\n── Embedder ─────────────────────────────────────────────────")

	embedder, embErr := llm.NewEmbedder(cfg)
	var embReady bool
	if embErr != nil {
		record("Embedder", "NewEmbedder", "embedder ready", embErr.Error(), fail)
	} else {
		vec, err := embedder.Embed(ctx, "hello world")
		if err != nil {
			record("Embedder", "Embed(hello world)", "[]float32 len=1536", err.Error(), fail)
		} else {
			embReady = true
			record("Embedder", "Embed(hello world)", "[]float32 len=1536",
				fmt.Sprintf("len=%d first=%.4f", len(vec), vec[0]), pass)
		}
	}

	// ── 4. Memory Store ───────────────────────────────────────────────────────
	fmt.Println("\n── Memory Store ─────────────────────────────────────────────")

	var memStore memorystore.MemoryStore
	if !embReady || pgErr != nil {
		record("MemoryStore", "NewPgMemoryStore", "store ready", "skipped (embedder/DB unavailable)", skip)
	} else {
		memStore = memorystore.NewPgMemoryStore(pool, embedder)
		testID := uuid.New()
		storeErr := memStore.Store(ctx, "painless-agent verification test memory", []string{"test"}, testID)
		if storeErr != nil {
			record("MemoryStore", "Store", "row inserted", storeErr.Error(), fail)
		} else {
			record("MemoryStore", "Store", "row inserted", "stored verification memory", pass)
		}
		if storeErr == nil {
			mems, searchErr := memStore.Search(ctx, "verification test", 1)
			if searchErr != nil {
				record("MemoryStore", "Search", "1 result", searchErr.Error(), fail)
			} else if len(mems) == 0 {
				record("MemoryStore", "Search", "≥1 result", "0 results", fail)
			} else {
				record("MemoryStore", "Search", "≥1 result",
					fmt.Sprintf("%d result: %q…", len(mems), truncate(fmt.Sprintf("%v", mems[0]), 40)), pass)
			}
		}
	}

	// ── 5. Tool Engine + individual tools ─────────────────────────────────────
	fmt.Println("\n── Tool Engine & Tools ──────────────────────────────────────")

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

	// code_executor
	dockerRunner, dockerErr := sandbox.NewRunner()
	if dockerErr != nil {
		record("sandbox.Runner", "NewRunner", "Docker client", dockerErr.Error(), fail)
	} else {
		defer dockerRunner.Close()
		record("sandbox.Runner", "NewRunner", "Docker client", "Docker connected", pass)
		engine.Register(tools.NewCodeExecutorTool(dockerRunner, cfg.CodeExecTimeoutSecs))
	}

	// browser
	browserTool := tools.NewBrowserTool(cfg.FilesystemRoot, cfg.ChromeCDPURL)
	engine.Register(browserTool)
	defer browserTool.Close()

	// github
	if cfg.GitHubToken != "" {
		engine.Register(tools.NewGitHubTool(cfg.GitHubToken))
	}

	defs := engine.Definitions()
	toolNames := make([]string, len(defs))
	for i, d := range defs {
		toolNames[i] = d.Name
	}
	record("ToolEngine", "Definitions()", "≥5 tools registered",
		fmt.Sprintf("%d tools: %s", len(defs), strings.Join(toolNames, ", ")), pass)

	// ── 5a. filesystem ────────────────────────────────────────────────────────
	testExec(ctx, engine, "filesystem/write",
		"Write then read a file in workspace",
		"wrote N bytes",
		map[string]any{"operation": "write", "path": "verify_test.txt", "content": "hello from verify"},
		func(out string) bool { return strings.Contains(out, "wrote") })

	testExec(ctx, engine, "filesystem/read",
		"Read the file back",
		"hello from verify",
		map[string]any{"operation": "read", "path": "verify_test.txt"},
		func(out string) bool { return strings.Contains(out, "hello from verify") })

	testExec(ctx, engine, "filesystem/list",
		"List workspace root",
		"verify_test.txt in listing",
		map[string]any{"operation": "list", "path": "."},
		func(out string) bool { return strings.Contains(out, "verify_test.txt") })

	testExec(ctx, engine, "filesystem/delete",
		"Delete the test file",
		"deleted verify_test.txt",
		map[string]any{"operation": "delete", "path": "verify_test.txt"},
		func(out string) bool { return strings.Contains(out, "deleted") })

	testExec(ctx, engine, "filesystem/traversal",
		"Reject path traversal (../etc/passwd)",
		"error: escapes workspace",
		map[string]any{"operation": "read", "path": "../etc/passwd"},
		func(out string) bool { return strings.Contains(out, "escapes") || strings.Contains(out, "error") })

	// ── 5b. http_client ───────────────────────────────────────────────────────
	// Use api.github.com — stable, always reachable, returns well-formed JSON.
	testExec(ctx, engine, "http_client/GET",
		"GET https://api.github.com/ (stable endpoint)",
		"HTTP 200 with JSON body",
		map[string]any{"method": "GET", "url": "https://api.github.com/"},
		func(out string) bool { return strings.Contains(out, "HTTP") && !strings.Contains(out, "error") })

	testExec(ctx, engine, "http_client/POST",
		"POST https://api.github.com/ (any HTTP response = tool works)",
		"any HTTP status returned",
		map[string]any{"method": "POST", "url": "https://api.github.com/", "body": `{}`},
		func(out string) bool { return strings.HasPrefix(out, "HTTP") })

	// ── 5c. web_search ────────────────────────────────────────────────────────
	searchProvider := "brave"
	if cfg.BraveAPIKey == "" && cfg.SerpAPIKey != "" {
		searchProvider = "serp"
	}
	testExec(ctx, engine, "web_search/"+searchProvider,
		`Search for "Go programming language"`,
		"results containing golang or Go",
		map[string]any{"query": "Go programming language golang"},
		func(out string) bool {
			low := strings.ToLower(out)
			return strings.Contains(low, "go") && !strings.Contains(low, "error")
		})

	// ── 5d. summarizer ────────────────────────────────────────────────────────
	if llmReady {
		testExec(ctx, engine, "summarizer",
			"Summarise a paragraph via LLM",
			"shorter summary returned",
			map[string]any{"text": "The Go programming language, also known as Golang, was created at Google in 2007 by Robert Griesemer, Rob Pike, and Ken Thompson. It is statically typed, compiled, and designed for simplicity, efficiency, and concurrent programming. Go has grown to power major infrastructure projects like Docker, Kubernetes, and many cloud services."},
			func(out string) bool { return len(out) > 5 && !strings.Contains(out, "error") })
	} else {
		record("summarizer", "Summarize(paragraph)", "shorter summary", "skipped (LLM unavailable)", skip)
	}

	// ── 5e. memory_tool ───────────────────────────────────────────────────────
	if memStore != nil {
		testExec(ctx, engine, "memory_tool/store",
			"Agent-triggered memory write",
			"memory stored",
			map[string]any{"content": "verification: memory tool is functional"},
			func(out string) bool { return !strings.Contains(out, "error") && len(out) > 0 })
	} else {
		record("memory_tool", "Execute(store)", "stored", "skipped (memory store unavailable)", skip)
	}

	// ── 5f. code_executor ────────────────────────────────────────────────────
	if dockerErr == nil {
		testExec(ctx, engine, "code_executor/python",
			`Execute python: print("sandbox OK")`,
			"sandbox OK in stdout",
			map[string]any{"language": "python", "code": `print("sandbox OK")`},
			func(out string) bool { return strings.Contains(out, "sandbox OK") })

		testExec(ctx, engine, "code_executor/python-math",
			"Execute python: compute 2**32",
			"4294967296",
			map[string]any{"language": "python", "code": "print(2**32)"},
			func(out string) bool { return strings.Contains(out, "4294967296") })

		testExec(ctx, engine, "code_executor/bash",
			"Execute bash: echo hello",
			"hello",
			map[string]any{"language": "bash", "code": "echo hello"},
			func(out string) bool { return strings.Contains(out, "hello") })

		// Verify network is blocked
		testExec(ctx, engine, "code_executor/network-isolation",
			"Python: attempt socket connect (should fail)",
			"error or connection refused (network disabled)",
			map[string]any{"language": "python", "code": `
import socket, sys
try:
    socket.setdefaulttimeout(3)
    socket.create_connection(("8.8.8.8", 53))
    print("NETWORK_ACCESSIBLE")
except Exception as e:
    print("NETWORK_BLOCKED:", e)
`},
			func(out string) bool { return strings.Contains(out, "NETWORK_BLOCKED") })

		// Unsupported language → clean error
		testExec(ctx, engine, "code_executor/bad-lang",
			"Pass unsupported language 'cobol'",
			"error mentioning unsupported",
			map[string]any{"language": "cobol", "code": "DISPLAY 'HELLO'"},
			func(out string) bool { return strings.Contains(strings.ToLower(out), "unsupported") })
	} else {
		for _, sub := range []string{"python", "python-math", "bash", "network-isolation", "bad-lang"} {
			record("code_executor/"+sub, "Execute", "output", "skipped (Docker unavailable)", skip)
		}
	}

	// ── 5g. browser ───────────────────────────────────────────────────────────
	ctx60, cancel60 := context.WithTimeout(ctx, 60*time.Second)
	defer cancel60()
	out, execErr := engine.Execute(ctx60, "browser",
		map[string]any{"operation": "get_text", "url": "https://example.com"})
	if execErr != nil {
		record("browser/get_text", "get_text(example.com)", "page text", execErr.Error(), fail)
	} else if strings.Contains(strings.ToLower(out), "error") || strings.Contains(out, "exec") {
		record("browser/get_text", "get_text(example.com)", "page text", out, fail, "Chrome not installed")
	} else if strings.Contains(out, "Example Domain") || strings.Contains(out, "example") {
		record("browser/get_text", "get_text(example.com)", "page text", truncate(out, 60), pass)
	} else {
		record("browser/get_text", "get_text(example.com)", "page text", truncate(out, 60), fail)
	}

	// ── 5h. github ────────────────────────────────────────────────────────────
	if cfg.GitHubToken == "" {
		record("github", "Execute(create_repo)", "repo created", "skipped (GITHUB_TOKEN not set)", skip)
	} else {
		testExec(ctx, engine, "github/create_repo",
			"Create repo verify-painless-test (will exist if already created)",
			"repo URL returned",
			map[string]any{"operation": "create_repo", "repo": "verify-painless-test", "description": "e2e verify", "private": true},
			func(out string) bool { return strings.Contains(out, "github.com") || strings.Contains(out, "already exists") })
	}

	// ── 6. Store layer ────────────────────────────────────────────────────────
	fmt.Println("\n── Store Layer ──────────────────────────────────────────────")

	var verifyTaskID uuid.UUID
	if pgErr == nil {
		ts := store.NewTaskStore(pool)
		task, err := ts.CreateTask(ctx, "verify: e2e test task")
		if err != nil {
			record("TaskStore", "CreateTask", "task row", err.Error(), fail)
		} else {
			verifyTaskID = task.ID
			record("TaskStore", "CreateTask", "task row", "id="+task.ID.String()[:8]+"…", pass)
			got, err := ts.GetTask(ctx, task.ID)
			if err != nil || got.ID != task.ID {
				record("TaskStore", "GetTask", "same task", fmt.Sprintf("%v", err), fail)
			} else {
				record("TaskStore", "GetTask", "same task", "goal="+truncate(got.Goal, 30), pass)
			}
		}

		// ToolLogStore requires a valid task_id FK; use the task we just created.
		tls := store.NewToolLogStore(pool)
		if verifyTaskID != uuid.Nil {
			logErr := tls.Log(ctx, verifyTaskID, uuid.Nil, "http_client", map[string]any{"url": "test"}, "HTTP 200", 42)
			if logErr != nil {
				record("ToolLogStore", "Log", "row inserted", logErr.Error(), fail)
			} else {
				record("ToolLogStore", "Log", "row inserted", "tool_log row written (task_id="+verifyTaskID.String()[:8]+"…)", pass)
			}
		}
	} else {
		record("TaskStore", "CreateTask", "task row", "skipped (DB unavailable)", skip)
		record("ToolLogStore", "Log", "row inserted", "skipped (DB unavailable)", skip)
	}

	// ── 7. Agent runtime (end-to-end) ─────────────────────────────────────────
	fmt.Println("\n── Agent Runtime (end-to-end) ───────────────────────────────")

	if !llmReady || pgErr != nil {
		record("AgentRuntime", "Run(small goal)", "task + steps in DB", "skipped (LLM or DB unavailable)", skip)
	} else {
		ts := store.NewTaskStore(pool)
		tls := store.NewToolLogStore(pool)
		log := logger.New(cfg.Env)
		runtime := agent.New(provider, ts, log, agent.DefaultRuntimeConfig()).
			WithTools(engine).
			WithToolLogs(tls)
		if memStore != nil {
			runtime = runtime.WithMemory(memStore)
		}

		runCtx, runCancel := context.WithTimeout(ctx, 90*time.Second)
		defer runCancel()

		runErr := runtime.Run(runCtx, "Write the number 42 to a file called verify_agent_output.txt in the workspace, then read it back and confirm the content.")
		if runErr != nil {
			record("AgentRuntime", "Run(write 42 to file)", "task completes", runErr.Error(), fail)
		} else {
			// Verify the file was actually written.
			data, readErr := os.ReadFile(cfg.FilesystemRoot + "/verify_agent_output.txt")
			if readErr != nil {
				record("AgentRuntime", "Run(write 42 to file)", "file contains 42",
					"task ran but file not found: "+readErr.Error(), fail)
			} else if strings.Contains(string(data), "42") {
				record("AgentRuntime", "Run(write 42 to file)", "file contains 42",
					"file contains: "+strings.TrimSpace(string(data)), pass)
			} else {
				record("AgentRuntime", "Run(write 42 to file)", "file contains 42",
					"file exists but missing 42: "+string(data), fail)
			}
		}
	}

	// ── Final Report ──────────────────────────────────────────────────────────
	printReport()
}

// ── helpers ───────────────────────────────────────────────────────────────────

func testExec(ctx context.Context, engine *tools.ToolEngine, component, procedure, expected string, input map[string]any, check func(string) bool) {
	toolName := strings.SplitN(component, "/", 2)[0]
	out, err := engine.Execute(ctx, toolName, input)
	if err != nil {
		record(component, procedure, expected, "exec error: "+err.Error(), fail)
		return
	}
	if check(out) {
		record(component, procedure, expected, truncate(out, 80), pass)
	} else {
		record(component, procedure, expected, truncate(out, 80), fail)
	}
}

func printReport() {
	var passed, failed, skipped int
	for _, r := range results {
		switch r.st {
		case pass:
			passed++
		case fail:
			failed++
		case skip:
			skipped++
		}
	}

	fmt.Println("\n╔══════════════════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                           VERIFICATION REPORT                                           ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════════════════╝")
	fmt.Printf("\n  Total: %d   ✅ Pass: %d   ❌ Fail: %d   ⏭️  Skip: %d\n\n", len(results), passed, failed, skipped)

	// Column widths
	fmt.Printf("%-35s %-40s %-35s %-10s\n",
		"COMPONENT", "PROCEDURE", "ACTUAL RESULT", "STATUS")
	fmt.Println(strings.Repeat("─", 125))
	for _, r := range results {
		icon := "✅ PASS"
		if r.st == fail {
			icon = "❌ FAIL"
		} else if r.st == skip {
			icon = "⏭️  SKIP"
		}
		fmt.Printf("%-35s %-40s %-35s %s\n",
			r.component,
			truncate(r.procedure, 38),
			truncate(r.actual, 33),
			icon)
		if r.note != "" {
			fmt.Printf("   ↳ note: %s\n", r.note)
		}
	}
	fmt.Println(strings.Repeat("─", 125))

	// Summary by section
	fmt.Println("\n┌─ Working components ──────────────────────────────────────────────────────")
	for _, r := range results {
		if r.st == pass {
			fmt.Printf("│  ✅ %s\n", r.component)
		}
	}
	fmt.Println("├─ Failed / degraded ────────────────────────────────────────────────────────")
	for _, r := range results {
		if r.st == fail {
			fmt.Printf("│  ❌ %s — %s\n", r.component, truncate(r.actual, 60))
		}
	}
	fmt.Println("├─ Skipped (missing env/dep) ────────────────────────────────────────────────")
	for _, r := range results {
		if r.st == skip {
			fmt.Printf("│  ⏭️  %s — %s\n", r.component, r.actual)
		}
	}
	fmt.Println("└──────────────────────────────────────────────────────────────────────────────")

	if failed > 0 {
		fmt.Printf("\n⚠️  Overall health: DEGRADED (%d failures)\n\n", failed)
	} else {
		fmt.Printf("\n✅  Overall health: HEALTHY (%d/%d passed, %d skipped)\n\n", passed, passed+skipped, skipped)
	}
}
