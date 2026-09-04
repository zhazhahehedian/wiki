# Repository Guidelines

> ⚠️ **方向转变（2026-09-04）**：项目已从「知识库 Agent（it-wiki）」转向「内部 MCP / Skill 能力注册与分发平台（能力中心 / Capability Hub）」。权威设计见 [docs/superpowers/specs/2026-09-04-capability-hub-design.md](docs/superpowers/specs/2026-09-04-capability-hub-design.md) 与 `CLAUDE.md` §2。下面描述的目录结构（`components/kb`、`components/docs`、`components/chunks` 等）属于**待退役的 it-wiki 遗留实现**，当前代码仍是它；新平台复用其骨架（飞书 OAuth、MinIO、river、Go/chi/sqlc、Next.js/shadcn），KB/RAG 产品层将退役。命令、代码风格、测试、提交规范等指引仍然有效。

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
