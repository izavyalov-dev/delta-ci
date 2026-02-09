.PHONY: build test lint up down serve worker dogfood

DATABASE_URL ?= postgres://delta:delta@localhost:5432/delta_ci?sslmode=disable

build:
	go build -o bin/ ./cmd/...
	go build -o bin/runner ./runner

test:
	go test ./...

lint:
	go vet ./...
	go fmt ./...

up:
	docker compose up -d

down:
	docker compose down

serve:
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/orchestrator serve

worker:
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/orchestrator worker

dogfood:
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/orchestrator dogfood
