.PHONY: up down run repl build tidy

up:
	docker compose up -d

down:
	docker compose down

run:
	go run ./cmd/server

repl:
	go run ./cmd/repl

build:
	go build -o bin/server ./cmd/server

tidy:
	go mod tidy
