# Repository Guidelines

## Current Direction

Capability Hub is an internal capability market plus knowledge service platform. Product facts, progress, invariants and the document index live in `CLAUDE.md`; current status and environment notes live in [the K1 handoff](docs/superpowers/plans/2026-09-19-knowledge-service-handoff.md).

## Scope and Reading

This file owns repository working rules. Follow current user instructions and applicable global rules; do not repeat approval already given. `CLAUDE.md` provides project facts and a topic index. Read only relevant spec sections, plan tasks, and dependencies; no session-wide document sweep is required.

Specs describe product decisions; plans describe a particular implementation effort. Historical plans do not automatically activate old branch, commit, environment, Skill, or cleanup instructions. Resolve design conflicts using the relevant decision and its explicit replacement, not date alone. Update the affected source when changing an accepted decision. Ordinary fixes and implementation choices do not require brainstorming or a new plan. Use design/planning for material changes to scope, architecture, external contracts, or dependencies.

External Skills mentioned in plans are optional workflow aids. Use an applicable available Skill or equivalent workflow; a missing named tool alone is not a blocker. Preserve task dependencies and meaningful verification.

## Project Structure and Style

`backend/` contains the Go service. Entrypoints are `cmd/server`, `cmd/migrate`, and `cmd/evalknowledge`; contracts live in `internal/domain/ports`, with implementations in HTTP, repo, infra, knowledge, registry, playground, skillbuilder, auth, and service (Feishu login) packages. Legacy it-wiki chat/RAG/ingestion code was removed; see git history at `c18e34e`. Use `gofmt`, lowercase package names, adjacent `*_test.go` tests, and `testdata/` fixtures. `internal/repo/generated` is sqlc output: never hand-edit it.

`frontend/` contains the Next.js app. Routes are in `app/`, shared Base UI/shadcn primitives in `components/ui`, feature components in feature folders, and API/hooks/schema helpers in `lib/`. TypeScript is strict; use `@/*` imports, PascalCase components, and camelCase functions. Inspect existing component exports and props before changing a primitive; check affected consumers of shared components. Use a project-compatible shadcn CLI only when adding components is needed.

## Commands and Preconditions

- Backend, from `backend/`: `go test ./...`, `go build ./cmd/server`, `sqlc generate`, and `go run ./cmd/migrate up`. The Makefile exposes corresponding targets. Use the project migration entrypoint because the repository includes Go migrations.
- Frontend, from `frontend/`: `pnpm install` when dependencies need installing, `pnpm dev`, `pnpm test`, `pnpm typecheck`, `pnpm build`, and `pnpm lint` (scripts are defined in `package.json`).
- Root: `make backend-build`, `make backend-test`, `make frontend-test`, `make frontend-typecheck`, and `make frontend-build`. GNU Make and `awk` are needed for `make help`.
- `deploy/compose.yaml` orchestrates the production application and HTTPS proxy only; see `deploy/README.md`. Provision PostgreSQL/MinIO separately. Local development uses the commands above; obsolete root Compose targets remain removed.
- Versions and package-manager requirements come from `backend/go.mod`, `frontend/package.json`, and the lockfile. Do not upgrade dependencies incidentally. For major upgrades, identify affected APIs and validate the relevant build and critical flows; explain compatibility evidence.

## Database Changes

Schema changes require a new goose migration in `backend/internal/repo/migrations`; query-only changes do not. Update SQL in `internal/repo/queries` when needed. Regenerate sqlc output when its inputs change, and adjust affected consumers. Commit legitimate generated changes with their inputs when committing the task. Existing raw pgx vector boundaries (`internal/repo/knowledge_vectors.go`) are documented in project decisions; do not move SQL into services or introduce a second ORM casually.

Verify migrations against a disposable database, including rollback when applicable. Do not directly alter production schema. K1 restores embedding/pgvector usage: preserve embedding dimension validation and the accepted dimension-change migration policy, and verify index versions and data ownership before reuse. D12 supersedes knowledge-table retirement; retain existing knowledge data and use incremental migrations for the new authorization and indexing boundaries.

## Verification by Task Risk

| Change | Expected verification |
|---|---|
| Documentation/rules only | Check consistency, referenced paths/commands, and diff; no business tests or build required. |
| Local Go behavior | Run affected package tests; expand to consumers for shared contracts. Run `go test ./...` before backend PRs. |
| Frontend logic, hooks, API client | Run relevant Vitest tests and typecheck. Add build for routing, server/client boundaries, dependency/config changes, or release integration. |
| Visual changes | Inspect affected pages/states; expand for shared themes/components. Include appropriate screenshots for a visible PR change, not a fixed full-site gallery. |
| Schema, auth, synchronization, cross-layer changes | Verify relevant integration and failure paths, isolation, cancellation, and data consistency. Preserve real runtime acceptance for stage/release claims. |

Add focused regression tests for meaningful behavior changes, particularly query consumers, shared hooks, authentication, governance transitions, discovery filtering, and credential access/audit. Legacy parsing and synchronization still need regression coverage when maintained. Do not add tests that only mirror implementation or require full-suite checks for every small edit. Stage-wide acceptance applies to that stage, not every maintenance task. Run applicable checks independently when one environment failure would otherwise suppress useful checks. Report failures, skips, and prerequisites accurately; skipped checks are not passes.

## Completion, Commits, and Cleanup

Complete the authorized scope, necessary documentation, risk-appropriate verification, and final diff review. Continue already-authorized next steps without repeated confirmation for routine choices. Once checks pass, repeat or broaden them only for new changes, failures, or unresolved risks. If an external prerequisite blocks part of the work, finish unaffected work and distinguish implementation completion, blocked verification, and release acceptance. Do not stop at a proposal when implementation is authorized, or claim completion while required work remains.

Update `CLAUDE.md` progress only when phase status changes; update the relevant spec/plan when an accepted decision or tracked task changes. No mandatory per-session log update is needed for unrelated work.

Follow current task authorization for commits and publishing; a historical plan's commit checkbox is not an instruction to commit today. Group commits by coherent change and stage only task-related files after reviewing the diff. Use concise imperative messages with optional scopes. Do not amend pushed commits, force push, or bypass hooks. Keep attribution accurate. PR descriptions should state purpose, meaningful changes, verification results, and applicable issue/spec, migration, environment, or screenshot information. For every push, identify the included features in the commit and PR descriptions; keep a concise release note for a combined feature batch.

Cleanup covers this task's temporary files and directly affected remnants; preserve unrelated user changes. `migrate-reset` rolls back migrations and is not routine cleanup. Do not run destructive data operations without task-specific authorization. Do not require a branch-finishing Skill or another merge-choice round when the user has already specified the next step.

## Security and Product Invariants

Platform configuration secrets belong in environment configuration; product-managed MCP credentials and user Token Hub keys use encrypted storage per the new spec. Never commit or log secret values. When implementing the relevant capabilities, verify owner/admin authorization, discovery visibility, separate credential grants with audit, credential rotation, and atomic version publication. Preserve session/CSRF checks, cancellation for streams, and third-party notices in reused foundations. For K1 knowledge ingestion and retrieval, preserve checksum, immutable active-version and deletion/revocation consistency invariants while adapting the legacy Owner-only boundary; D12 does not authorize restoring the old chat product or retiring knowledge data. Durable background business jobs use River; document and test retry-policy changes. These protections are based on project risks, not on a particular model version.
