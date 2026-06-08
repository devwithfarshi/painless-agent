package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/devwithfarshi/painless-agent/internal/agent"
	"github.com/devwithfarshi/painless-agent/internal/llm"
	"github.com/devwithfarshi/painless-agent/internal/onboarding"
	"github.com/devwithfarshi/painless-agent/internal/store"
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

	// LLM provider.
	provider, err := llm.New(cfg)
	if err != nil {
		log.Error("init llm provider", "error", err)
		os.Exit(1)
	}
	if cfg.UserName != "" {
		log.Info("llm provider ready", "provider", cfg.LLMProvider, "model", provider.Model(), "user", cfg.UserName)
	} else {
		log.Info("llm provider ready", "provider", cfg.LLMProvider, "model", provider.Model())
	}

	// Task store.
	taskStore := store.NewTaskStore(pool)

	// Agent runtime.
	runtime := agent.New(provider, taskStore, log, agent.DefaultRuntimeConfig())

	// Debug path: set AGENT_GOAL to run a single task synchronously and exit.
	if goal := os.Getenv("AGENT_GOAL"); goal != "" {
		log.Info("running goal", "goal", goal)
		if err := runtime.Run(ctx, goal); err != nil {
			log.Error("agent run failed", "error", err)
			os.Exit(1)
		}
		log.Info("agent run complete")
		return
	}

	log.Info("infrastructure ready — set AGENT_GOAL=<goal> to run a task (HTTP server arrives in Order 6)")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Info("shutting down")
}
