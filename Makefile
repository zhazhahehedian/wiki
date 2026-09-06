# Capability Hub local development commands
# Windows needs GNU Make and awk (for example, through Git Bash or WSL).
.PHONY: help tools backend-build backend-test frontend-test frontend-typecheck frontend-build

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?##/ { printf "  %-22s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

tools: ## Install local dev toolchain
	$(MAKE) -C backend tools

backend-build: ## Build the backend server
	$(MAKE) -C backend build

backend-test: ## Run backend tests
	$(MAKE) -C backend test

frontend-test: ## Run frontend tests
	cd frontend && pnpm test

frontend-typecheck: ## Frontend type check
	cd frontend && pnpm typecheck

frontend-build: ## Build the frontend
	cd frontend && pnpm build
