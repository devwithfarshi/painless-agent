// Package handlers contains the HTTP request handlers for the agent API.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/devwithfarshi/painless-agent/internal/llm"
	"github.com/devwithfarshi/painless-agent/internal/memory"
	"github.com/devwithfarshi/painless-agent/internal/scheduler"
	"github.com/devwithfarshi/painless-agent/internal/skills"
	"github.com/devwithfarshi/painless-agent/internal/store"
	"github.com/devwithfarshi/painless-agent/internal/streaming"
	"github.com/devwithfarshi/painless-agent/pkg/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Handlers holds shared dependencies for all HTTP handlers.
type Handlers struct {
	Tasks    *store.TaskStore
	Skills   skills.SkillStore        // may be nil
	Memory   memory.MemoryStore       // may be nil
	Scheduler *scheduler.Client       // may be nil
	Emitter  *streaming.Emitter
	Pool     *pgxpool.Pool
	RDB      *redis.Client
	Provider *llm.SwappableProvider   // hot-swappable LLM provider
	Cfg      *config.Config           // live config reference for provider creation
}

// writeJSON encodes v as JSON and writes it with statusCode.
func writeJSON(w http.ResponseWriter, statusCode int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error envelope.
func writeError(w http.ResponseWriter, statusCode int, msg string) {
	writeJSON(w, statusCode, map[string]string{"error": msg})
}

// pingDB returns nil if the DB is reachable.
func pingDB(ctx context.Context, pool *pgxpool.Pool) error {
	return pool.Ping(ctx)
}

// pingRedis returns nil if Redis is reachable.
func pingRedis(ctx context.Context, rdb *redis.Client) error {
	return rdb.Ping(ctx).Err()
}
