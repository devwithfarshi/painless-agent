.PHONY: up down run build tidy

up:
	docker compose up -d

down:
	docker compose down

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

tidy:
	go mod tidy
