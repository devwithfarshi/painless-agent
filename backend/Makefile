.PHONY: up down run repl worker build tidy

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
