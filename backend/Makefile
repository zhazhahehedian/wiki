# backend/Makefile
# Run from backend/: make <target>
.PHONY: help build run test tidy sqlc-gen migrate-up migrate-down migrate-status migrate-reset tools

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?##/ { printf "  %-20s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

tools: ## Install local dev tools (sqlc)
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

build: ## Build server binary
	go build -o bin/server ./cmd/server

run: ## Run server locally (needs .env loaded)
	go run ./cmd/server

test: ## Run all tests
	go test ./...

tidy: ## go mod tidy
	go mod tidy

sqlc-gen: ## Generate Go code from queries + migrations
	sqlc generate

migrate-up: ## Apply all migrations (uses cmd/migrate, supports Go migrations)
	go run ./cmd/migrate up

migrate-down: ## Rollback one migration
	go run ./cmd/migrate down

migrate-status: ## Show migration status
	go run ./cmd/migrate status

migrate-reset: ## DESTRUCTIVE: rollback all migrations
	go run ./cmd/migrate reset
