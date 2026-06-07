package main

import (
	"context"
	"os"

	"github.com/devwithfarshi/painless-agent/pkg/config"
	"github.com/devwithfarshi/painless-agent/pkg/db"
	"github.com/devwithfarshi/painless-agent/pkg/logger"
)

func main() {
	ctx := context.Background()

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

	log.Info("infrastructure ready")
}
