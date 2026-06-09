package main

import (
	"context"
	"os"

	"github.com/devwithfarshi/painless-agent/internal/agent"
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

	if onboarding.IsFirstRun() {
		if err := onboarding.Run(ctx, ".env"); err != nil {
			os.Stderr.WriteString("setup failed: " + err.Error() + "\n")
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
		os.Stderr.WriteString("load config: " + err.Error() + "\n")
		os.Exit(1)
	}

	log := logger.New(cfg.Env)
	log.Info("starting painless-agent worker", "env", cfg.Env)

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("connect to postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.EnablePgVector(ctx, pool); err != nil {
		log.Error("enable pgvector extension", "error", err)
		os.Exit(1)
	}
	if err := db.Migrate(pool); err != nil {
		log.Error("run migrations", "error", err)
		os.Exit(1)
	}
	log.Info("postgres ready")

	rdb, err := db.NewRedis(ctx, cfg.RedisURL)
	if err != nil {
		log.Error("connect to redis", "error", err)
		os.Exit(1)
	}
	defer rdb.Close()
	log.Info("redis ready")

	// Event emitter: publishes to Redis so the HTTP server can deliver SSE to clients.
	emitter := streaming.New(rdb)

	provider, err := llm.New(cfg)
	if err != nil {
		log.Error("init llm provider", "error", err)
		os.Exit(1)
	}
	log.Info("llm provider ready", "provider", cfg.LLMProvider, "model", provider.Model())

	var pgMem memorystore.MemoryStore
	embedder, embErr := llm.NewEmbedder(cfg)
	if embErr != nil {
		log.Warn("embedder unavailable — memory + skills disabled", "error", embErr)
	} else {
		log.Info("embedder ready", "model", embedder.Model())
		pgMem = memorystore.NewPgMemoryStore(pool, embedder)
	}

	taskStore := store.NewTaskStore(pool)
	toolLogStore := store.NewToolLogStore(pool)

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

	dockerRunner, dockerErr := sandbox.NewRunner()
	if dockerErr != nil {
		log.Warn("docker unavailable — code_executor tool disabled", "error", dockerErr)
	} else {
		toolEngine.Register(tools.NewCodeExecutorTool(dockerRunner, cfg.CodeExecTimeoutSecs))
		log.Info("code_executor tool registered")
	}

	browserTool := tools.NewBrowserTool(cfg.FilesystemRoot, cfg.ChromeCDPURL)
	toolEngine.Register(browserTool)
	defer browserTool.Close()

	if cfg.GitHubToken != "" {
		toolEngine.Register(tools.NewGitHubTool(cfg.GitHubToken))
		log.Info("github tool registered")
	}

	log.Info("tool engine ready", "tools", len(toolEngine.Definitions()))

	var skillStore skills.SkillStore
	if embedder != nil {
		skillStore = skills.NewPgSkillStore(pool, embedder, cfg.SkillMatchThreshold)
		log.Info("skill store ready", "match_threshold", cfg.SkillMatchThreshold)
	}

	reflectionStore := store.NewReflectionStore(pool)
	var reflector agent.Reflector
	if skillStore != nil {
		reflector = reflection.New(provider, reflectionStore, skillStore, cfg.ReflectionRatingThreshold)
		log.Info("reflector ready", "rating_threshold", cfg.ReflectionRatingThreshold)
	}

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

	srv, err := scheduler.NewServer(cfg.RedisURL, runtime, cfg.QueueConcurrency)
	if err != nil {
		log.Error("init scheduler server", "error", err)
		os.Exit(1)
	}

	log.Info("worker ready — listening for agent:run tasks", "concurrency", cfg.QueueConcurrency)

	if err := srv.Start(); err != nil {
		log.Error("worker stopped", "error", err)
		os.Exit(1)
	}

	if dockerRunner != nil {
		_ = dockerRunner.Close()
	}
}
