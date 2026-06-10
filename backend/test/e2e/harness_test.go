//go:build integration

// Package e2e contains end-to-end tests that run against real Postgres and Redis.
// Set the following env vars (or rely on .env defaults) before running:
//
//	DATABASE_URL=postgres://...
//	REDIS_URL=redis://...
//
// Run with: go test -tags integration ./test/e2e/...
package e2e

import (
	"context"
	"os"
	"testing"

	"github.com/devwithfarshi/painless-agent/pkg/db"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// harness holds live infrastructure for each test.
type harness struct {
	pool *pgxpool.Pool
	rdb  *redis.Client
}

// newHarness connects to Postgres + Redis, runs migrations, and returns a harness.
// It skips the test if the required env vars are not set.
func newHarness(t *testing.T) *harness {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	redisURL := os.Getenv("REDIS_URL")
	if dbURL == "" || redisURL == "" {
		// Try to load .env from backend root (two directories up from test/e2e/).
		if _, err := os.Stat("../../.env"); err == nil {
			_ = loadEnvFile("../../.env")
			dbURL = os.Getenv("DATABASE_URL")
			redisURL = os.Getenv("REDIS_URL")
		}
	}
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}
	if redisURL == "" {
		t.Skip("REDIS_URL not set — skipping integration test")
	}

	ctx := context.Background()

	pool, err := db.NewPool(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to postgres: %v", err)
	}

	if err := db.EnablePgVector(ctx, pool); err != nil {
		pool.Close()
		t.Fatalf("enable pgvector: %v", err)
	}
	if err := db.Migrate(pool); err != nil {
		pool.Close()
		t.Fatalf("run migrations: %v", err)
	}

	rdb, err := db.NewRedis(ctx, redisURL)
	if err != nil {
		pool.Close()
		t.Fatalf("connect to redis: %v", err)
	}

	h := &harness{pool: pool, rdb: rdb}
	t.Cleanup(func() {
		h.pool.Close()
		_ = h.rdb.Close()
	})
	return h
}

// loadEnvFile reads key=value pairs from a file into the process environment.
func loadEnvFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range splitLines(string(data)) {
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		for i, ch := range line {
			if ch == '=' {
				key := line[:i]
				val := line[i+1:]
				if os.Getenv(key) == "" {
					_ = os.Setenv(key, val)
				}
				break
			}
		}
	}
	return nil
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
