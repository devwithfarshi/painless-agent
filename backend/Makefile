.PHONY: up down run repl worker build tidy dev test test-integration loadtest

up:
	docker compose up -d

down:
	docker compose down

run:
	go run ./cmd/server

repl:
	go run ./cmd/repl

worker:
	go run ./cmd/worker

build:
	go build -o bin/server ./cmd/server && go build -o bin/worker ./cmd/worker

tidy:
	go mod tidy

# Start the Next.js frontend dev server (run from repo root: make dev).
dev:
	cd ../frontend && npm run dev

# Unit tests (no external deps).
test:
	go test ./...

# Integration tests — requires running Postgres + Redis (make up first).
test-integration:
	go test -tags integration -v ./test/e2e/...

# k6 load tests — requires a running server (make run) and k6 installed.
# Override BASE_URL to target a different host.
loadtest:
	BASE_URL=$${BASE_URL:-http://localhost:8080} k6 run ../infra/loadtest/api.js
