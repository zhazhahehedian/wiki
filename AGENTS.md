# Repository Guidelines

## Project Structure & Module Organization

`backend/` contains the Go service. Entrypoints live in `cmd/server` and `cmd/migrate`; domain types and ports are in `internal/domain`; services, HTTP handlers, repositories, adapters, and workers live under `internal/service`, `internal/http`, `internal/repo`, `internal/infra`, and `internal/worker`. `internal/repo/generated` is sqlc output; do not hand-edit it.

`frontend/` contains the Next.js app. Routes are under `app/`, UI primitives under `components/ui`, feature components under `components/kb`, `components/docs`, and `components/chunks`, and API/hooks/schema helpers under `lib/`. Design docs and plans live in `docs/superpowers/{specs,plans}`. Go tests sit beside code as `*_test.go`; fixtures use `testdata/`.

## Build, Test, and Development Commands

- `make help` lists root commands.
- `make backend-test` runs backend `go test ./...`.
- `make frontend-typecheck` runs frontend TypeScript checks.
- `make up` / `make down` start or stop the Docker stack; they require `.env` and `deploy/docker-compose.yml`.
- `cd backend && make build|run|test|sqlc-gen|migrate-up` builds, runs, tests, regenerates sqlc code, or applies migrations.
- `cd frontend && pnpm install`, `pnpm dev`, `pnpm build`, `pnpm typecheck`, and `pnpm lint` install dependencies and run frontend checks.

## Coding Style & Naming Conventions

Use `gofmt` for Go and lowercase package names. Keep contracts in `internal/domain/ports` and implementation details in `internal/infra`. Database changes must add a migration in `backend/internal/repo/migrations`, update SQL in `internal/repo/queries`, then run `sqlc generate`. TypeScript is strict; use `@/*` imports, PascalCase components, camelCase functions, and shadcn/lucide conventions. Keep shadcn primitives in `components/ui`; place product components by feature.

## Testing Guidelines

Backend tests use Go's standard `testing` package; name tests `TestXxx` and keep fixtures in `testdata`. Run `cd backend && go test ./...` before backend PRs. Frontend currently relies on type and build checks; run `cd frontend && pnpm typecheck` and `pnpm build` for UI or API-client changes. Add focused tests for parsing, splitting, checksums, migrations, query consumers, and shared hooks.

## Commit & Pull Request Guidelines

Git history is unavailable in this checkout, so use concise imperative messages with optional scopes, such as `backend: add parser tests` or `docs: update phase plan`. Do not amend pushed commits, force push, or bypass hooks. PRs should include purpose, key changes, verification commands, related issue or spec links, migration or env changes, and screenshots for visible frontend changes. Update `CLAUDE.md` and relevant specs/plans when decisions or phase status change.

## Security & Configuration Tips

Copy `.env.example` to `.env`; never commit real API keys or credentials. Keep LLM, embedding, database, and S3 settings in environment variables.
