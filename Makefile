# it-wiki top-level Makefile
# Cross-platform note: Windows needs GNU Make (scoop install make) or git bash
.PHONY: help up down logs ps restart build clean tools backend-test frontend-typecheck

COMPOSE := docker compose -f deploy/docker-compose.yml --env-file .env

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?##/ { printf "  %-22s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

up: ## Start full stack (detached + build)
	$(COMPOSE) up -d --build

down: ## Stop all services
	$(COMPOSE) down

logs: ## Tail backend logs (other services: docker compose logs <svc>)
	$(COMPOSE) logs -f backend

ps: ## Show service status
	$(COMPOSE) ps

restart: ## Restart backend + frontend (DB and storage left alone)
	$(COMPOSE) restart backend frontend

build: ## Build images without starting
	$(COMPOSE) build

clean: ## Stop and remove volumes (DESTRUCTIVE: wipes DB and uploads)
	$(COMPOSE) down -v

tools: ## Install local dev toolchain
	$(MAKE) -C backend tools

backend-test: ## Run backend tests
	$(MAKE) -C backend test

frontend-typecheck: ## Frontend type check
	cd frontend && pnpm typecheck