.PHONY: up down run repl worker build tidy dev

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
