package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/devwithfarshi/painless-agent/internal/agent"
	"github.com/devwithfarshi/painless-agent/internal/api"
	"github.com/devwithfarshi/painless-agent/internal/api/handlers"
	"github.com/devwithfarshi/painless-agent/internal/llm"
	memorystore "github.com/devwithfarshi/painless-agent/internal/memory"
	"github.com/devwithfarshi/painless-agent/internal/onboarding"
	"github.com/devwithfarshi/painless-agent/internal/reflection"
	"github.com/devwithfarshi/painless-agent/internal/sandbox"
	"github.com/devwithfarshi/painless-agent/internal/scheduler"
	"github.com/devwithfarshi/painless-agent/internal/skills"
	"github.com/devwithfarshi/painless-agent/internal/store"
	"github.com/devwithfarshi/painless-agent/internal/streaming"
	"github.com/devwithfarshi/painless-agent/internal/tools"
	"github.com/devwithfarshi/painless-agent/pkg/config"
	"github.com/devwithfarshi/painless-agent/pkg/db"
	"github.com/devwithfarshi/painless-agent/pkg/logger"
)

func main() {
	ctx := context.Background()

	// First-run onboarding: collect provider, model, and API credentials interactively.
	if onboarding.IsFirstRun() {
		if err := onboarding.Run(ctx, ".env"); err != nil {
			os.Stderr.WriteString("setup failed: " + err.Error() + "\n")
			os.Exit(1)
		}
	}

	// Inject the saved user name so config.Load picks it up via AGENT_USER_NAME.
	if name := onboarding.LoadUserName(); name != "" {
		if os.Getenv("AGENT_USER_NAME") == "" {
			os.Setenv("AGENT_USER_NAME", name)
		}
	}

	cfg, err := config.Load(".env")
	if err != nil {
		os.Stderr.WriteString("load config: " + err.Error() + "\n")
		os.Exit(1)
	}

	log := logger.New(cfg.Env)
	log.Info("starting painless-agent", "env", cfg.Env)

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("connect to postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	log.Info("connected to postgres")

	if err := db.EnablePgVector(ctx, pool); err != nil {
		log.Error("enable pgvector extension", "error", err)
		os.Exit(1)
	}

	if err := db.Migrate(pool); err != nil {
		log.Error("run migrations", "error", err)
		os.Exit(1)
	}
	log.Info("migrations applied")

	rdb, err := db.NewRedis(ctx, cfg.RedisURL)
	if err != nil {
		log.Error("connect to redis", "error", err)
		os.Exit(1)
	}
	defer rdb.Close()
	log.Info("connected to redis")

	// Event emitter — wired into both the runtime and the SSE HTTP handler.
	emitter := streaming.New(rdb)

	// LLM chat provider — wrapped in SwappableProvider for runtime hot-swap via POST /api/config/provider.
	rawProvider, err := llm.New(cfg)
	if err != nil {
		log.Error("init llm provider", "error", err)
		os.Exit(1)
	}
	provider := llm.NewSwappable(rawProvider)
	if cfg.UserName != "" {
		log.Info("llm provider ready", "provider", cfg.LLMProvider, "model", provider.Model(), "user", cfg.UserName)
	} else {
		log.Info("llm provider ready", "provider", cfg.LLMProvider, "model", provider.Model())
	}

	// Embedder (always OpenAI for 1536-dim vectors).
	var pgMem memorystore.MemoryStore
	embedder, embErr := llm.NewEmbedder(cfg)
	if embErr != nil {
		log.Warn("embedder unavailable — memory store disabled", "error", embErr)
	} else {
		log.Info("embedder ready", "model", embedder.Model())
		pgMem = memorystore.NewPgMemoryStore(pool, embedder)
	}

	// Task and tool-log stores.
	taskStore := store.NewTaskStore(pool)
	toolLogStore := store.NewToolLogStore(pool)

	// Tool engine: summarizer + all registered tools.
	summarizer := tools.NewSummarizerTool(provider)
	maxOutput := cfg.ToolMaxOutputKB * 1024
	if maxOutput == 0 {
		maxOutput = 32 * 1024
	}
	toolEngine := tools.NewEngine(maxOutput, summarizer.Summarize)
	toolEngine.Register(tools.NewHTTPClientTool(cfg.ToolTimeoutSecs, cfg.HTTPMaxBodyKB))
	toolEngine.Register(tools.NewFilesystemTool(cfg.FilesystemRoot))
	toolEngine.Register(tools.NewWebSearchTool(cfg.BraveAPIKey, cfg.SerpAPIKey))
	toolEngine.Register(summarizer)
	if pgMem != nil {
		toolEngine.Register(tools.NewMemoryTool(pgMem))
	}

	// Code executor: requires a running Docker daemon.
	dockerRunner, dockerErr := sandbox.NewRunner()
	if dockerErr != nil {
		log.Warn("docker unavailable — code_executor tool disabled", "error", dockerErr)
	} else {
		toolEngine.Register(tools.NewCodeExecutorTool(dockerRunner, cfg.CodeExecTimeoutSecs))
		log.Info("code_executor tool registered")
	}

	// Browser tool.
	browserTool := tools.NewBrowserTool(cfg.FilesystemRoot, cfg.ChromeCDPURL)
	toolEngine.Register(browserTool)
	if cfg.ChromeCDPURL != "" {
		log.Info("browser tool registered", "mode", "remote", "cdp_url", cfg.ChromeCDPURL)
	} else {
		log.Info("browser tool registered", "mode", "local-exec")
	}

	// GitHub tool.
	if cfg.GitHubToken != "" {
		toolEngine.Register(tools.NewGitHubTool(cfg.GitHubToken))
		log.Info("github tool registered")
	} else {
		log.Info("GITHUB_TOKEN not set — github tool disabled")
	}

	log.Info("tool engine ready", "tools", len(toolEngine.Definitions()))

	// Skill store.
	var skillStore skills.SkillStore
	if embedder != nil {
		skillStore = skills.NewPgSkillStore(pool, embedder, cfg.SkillMatchThreshold)
		log.Info("skill store ready", "match_threshold", cfg.SkillMatchThreshold)
	} else {
		log.Warn("embedder unavailable — skill store disabled")
	}

	// Reflection store + reflector.
	reflectionStore := store.NewReflectionStore(pool)
	var reflector agent.Reflector
	if skillStore != nil {
		reflector = reflection.New(provider, reflectionStore, skillStore, cfg.ReflectionRatingThreshold)
		log.Info("reflector ready", "rating_threshold", cfg.ReflectionRatingThreshold)
	}

	// Agent runtime — wire all dependencies including the event emitter.
	runtime := agent.New(provider, taskStore, log, agent.DefaultRuntimeConfig()).
		WithTools(toolEngine).
		WithToolLogs(toolLogStore).
		WithEmitter(emitter)
	if pgMem != nil {
		runtime = runtime.WithMemory(pgMem)
	}
	if skillStore != nil {
		runtime = runtime.WithSkills(skillStore)
	}
	if reflector != nil {
		runtime = runtime.WithReflector(reflector)
	}

	// Scheduler client.
	schedClient, schedErr := scheduler.NewClient(cfg.RedisURL)
	if schedErr != nil {
		log.Warn("scheduler client unavailable — tasks will run inline", "error", schedErr)
	} else {
		defer schedClient.Close()
		log.Info("scheduler client ready")
	}

	// HTTP API server.
	h := &handlers.Handlers{
		Tasks:     taskStore,
		Skills:    skillStore,
		Memory:    pgMem,
		Scheduler: schedClient,
		Emitter:   emitter,
		Pool:      pool,
		RDB:       rdb,
		Provider:  provider,
		Cfg:       cfg,
	}
	router := api.NewRouter(cfg, h, log)
	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // 0 = no timeout (needed for SSE streams)
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server error", "error", err)
		}
	}()

	// Debug path: set AGENT_GOAL to run a single task on startup.
	if goal := os.Getenv("AGENT_GOAL"); goal != "" {
		log.Info("running goal", "goal", goal)
		if schedClient != nil {
			info, err := schedClient.Enqueue(ctx, goal)
			if err != nil {
				log.Warn("enqueue failed — falling back to inline run", "error", err)
				if err := runtime.Run(ctx, goal); err != nil {
					log.Error("agent run failed", "error", err)
				}
			} else {
				log.Info("goal enqueued — run 'make worker' to process it", "task_id", info.ID)
			}
		} else {
			if err := runtime.Run(ctx, goal); err != nil {
				log.Error("agent run failed", "error", err)
			}
		}
	}

	// Graceful shutdown.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Info("shutting down")

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)

	browserTool.Close()
	if dockerRunner != nil {
		_ = dockerRunner.Close()
	}
}
