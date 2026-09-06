# 飞书身份与 Agent 扩展实施计划

> **适用范围（2026-09-05 规则整理）：** 本文属于转向前的 it-wiki 历史计划，保留阶段实施步骤与验收记录，不是能力中心实施计划或所有任务的常驻规则。仅执行本阶段相关工作时读取对应任务和依赖；旧分支、提交限制、环境结论、逐步 commit、全仓清扫与截图要求仅属于原阶段上下文，当前执行遵循 [AGENTS.md](../../../AGENTS.md) 和本次授权。未完成验收不自动视为通过。

> **执行方式：** 可使用适用且可用的 Skill 或等效流程；Superpowers 执行 Skills 不是前置依赖。不因 Skill 缺失停止工作，也不因阅读此计划自动启动子代理、提交或合并。

**Goal:** 为单租户 wiki 增加飞书 OAuth、用户级云文档导入与手动同步，并预留多 Agent 和 LiveAgent UI 复用边界。

**Architecture:** 认证 middleware 建立当前用户上下文；Feishu sync worker 将 docx、sheet、bitable、wiki 统一转换为 Markdown 快照，再复用现有 River ingestion pipeline。Agent 运行时通过 AgentRunner、AgentResolver、ToolRegistry 隔离，首版只有 `knowledge-rag`。

**Tech Stack:** Go 1.22+, chi, PostgreSQL/sqlc/goose, River, MinIO, pgvector, Next.js 15, React Query, shadcn/ui, LiveAgent MIT UI patterns, bitguide Go learning code.

---

## Scope and order

数据库和 sqlc 先行；认证与所有权随后；Feishu URL/Loader 独立测试；同步 worker 接入现有摄入链；Agent seams 和前端最后接入。每个任务应能独立验证；提交按当前授权和逻辑变更组织，不要求每个任务单独提交。

### Task 1: Identity, ownership, and staged-document schema

**Files:**
- Create: `backend/internal/repo/migrations/0007_identity_and_ownership.sql`
- Create: `backend/internal/repo/migrations/0008_document_sync.sql`
- Create: `backend/internal/repo/migrations/0009_conversation_agent.sql`
- Modify: `backend/internal/repo/queries/knowledge_bases.sql`
- Modify: `backend/internal/repo/queries/documents.sql`
- Modify: `backend/internal/repo/queries/conversations.sql`
- Modify: `backend/internal/config/config.go`
- Test: `backend/internal/config/config_test.go`

- [ ] Add `users`, `oauth_accounts`, and `user_sessions` with UUID keys, single-provider uniqueness, encrypted token columns, token hash, expiry, and timestamps.
- [ ] Add nullable `owner_user_id` to knowledge bases/conversations and `agent_id TEXT NOT NULL DEFAULT 'knowledge-rag'` to conversations.
- [ ] Add document fields `content_ref`, `source_url`, `remote_revision`, `oauth_account_id`, `pending_content_ref`, `pending_checksum`, `pending_remote_revision`, `sync_status`, `last_sync_error`, and `last_synced_at`; backfill local rows with `content_ref = source_ref`.
- [ ] Add `UNIQUE (kb_id, source_type, source_ref)` and owner/status/sync indexes.
- [ ] Add sqlc queries with explicit owner filters; run `cd backend && sqlc generate` and never hand-edit generated files.
- [ ] Add Feishu/session configuration and `BOOTSTRAP_OWNER_FEISHU_OPEN_ID`; fail fast when Feishu is enabled but required values are missing.
- [ ] Run `cd backend && go test ./internal/config ./internal/repo/...`; verify migration up/down against disposable PostgreSQL.
- [ ] Commit `backend: add identity and staged document schema`.

### Task 2: Feishu OAuth token and local session services

**Files:**
- Create: `backend/internal/domain/user.go`
- Create: `backend/internal/domain/ports/auth.go`
- Create: `backend/internal/auth/crypto.go`
- Create: `backend/internal/auth/session_store.go`
- Create: `backend/internal/service/auth_service.go`
- Create: `backend/internal/service/auth_service_test.go`
- Create: `backend/internal/infra/feishu/oauth_client.go`
- Create: `backend/internal/infra/feishu/oauth_client_test.go`

- [ ] Define User, OAuthAccount, Session, OAuthState, and FeishuIdentity domain types.
- [ ] Define `OAuthClient`, `TokenProtector`, `SessionStore`, and `OAuthStateStore` ports.
- [ ] Implement AES-GCM token protection with random nonce; test round-trip, wrong key, tampering, and empty-key rejection.
- [ ] Implement opaque sessions with SHA-256 token hashes, expiry, revocation, and cleanup; never persist raw session tokens.
- [ ] Implement authorization-code exchange and user-info calls with an injected `http.Client`; reject non-2xx and malformed JSON.
- [ ] Implement one-time five-minute OAuth state; test replay, expiry, tenant mismatch, exchange errors, and concurrent refresh.
- [ ] Commit `backend: add feishu oauth and local sessions`.

### Task 3: Authentication middleware and protected routing

**Files:**
- Create: `backend/internal/http/auth_handler.go`
- Create: `backend/internal/http/auth_middleware.go`
- Modify: `backend/internal/http/router.go`
- Modify: `backend/internal/http/cors.go`
- Modify: `frontend/lib/api/client.ts`
- Create: `frontend/lib/api/auth.ts`
- Create: `frontend/lib/hooks/use-auth.ts`
- Test: `backend/internal/http/auth_handler_test.go`
- Test: `backend/internal/http/auth_middleware_test.go`
- Test: `frontend/lib/api/auth.test.ts`

- [ ] Add `/auth/feishu/start`, `/auth/feishu/callback`, `/auth/me`, and `/auth/logout`; callback may redirect only to configured frontend origin/path.
- [ ] Add middleware that loads the session cookie, resolves the user, and stores `user_id` in context; return `401 unauthenticated` for missing/expired sessions.
- [ ] Keep health and OAuth endpoints public; protect KB/document/chunk/chat endpoints.
- [ ] Replace wildcard CORS with configured origin and credentials; validate Origin for state-changing requests.
- [ ] Set `credentials: 'include'` in the frontend client and expose auth query/mutations.
- [ ] Run backend HTTP tests and `cd frontend && pnpm typecheck`.
- [ ] Commit `backend: protect api with feishu sessions`.

### Task 4: Agent seams and bitguide reuse

**Files:**
- Create: `backend/internal/domain/ports/agent.go`
- Create: `backend/internal/agent/registry.go`
- Create: `backend/internal/agent/factory.go`
- Create: `backend/internal/agent/tool_registry.go`
- Modify: `backend/internal/service/chat_service.go`
- Modify: `backend/cmd/server/main.go`
- Test: `backend/internal/agent/registry_test.go`
- Test: `backend/internal/agent/tool_registry_test.go`

- [ ] Define `AgentRunner`, `AgentResolver`, and `ToolRegistry`; default Agent ID is `knowledge-rag`.
- [ ] Port relevant Factory/Registry/filter-chain ideas from `E:/MyLearn/go-project/bitguide-agent-platform-go-main/pkg/agent`, `pkg/tool`, and `pkg/gateway`; preserve current RAG, sqlc, River, and chi implementations.
- [ ] Wrap existing chat/ReAct as the first runner; do not add etcd, Redis, Milvus, or hot reload.
- [ ] Replace `agentToolFactory` in `main.go` with `ToolRegistry.ToolsFor(ctx, agentID, kbID, callback)`.
- [ ] Resolve conversation `agent_id` and reject unknown agents with a typed error.
- [ ] Test default resolution, unknown Agent, per-Agent tool lists, and concurrent registry reads.
- [ ] Commit `backend: add agent resolver and tool registry seams`.

### Task 5: Feishu URL resolution and canonical resource model

**Files:**
- Create: `backend/internal/domain/source.go`
- Create: `backend/internal/domain/ports/source.go`
- Create: `backend/internal/infra/feishu/resource.go`
- Create: `backend/internal/infra/feishu/url_resolver.go`
- Test: `backend/internal/infra/feishu/url_resolver_test.go`

- [ ] Define resource types `docx`, `sheet`, `bitable`, and `wiki`, plus `ResourceRef` with tenant, token, optional table/view/sheet IDs, and original URL.
- [ ] Parse accepted Feishu hosts and canonical paths; reject unrelated URLs, empty tokens, and unsupported types with typed errors.
- [ ] Define `CanonicalDocument` with title, Markdown, remote revision, source metadata, and source URL.
- [ ] Test canonical URL variants, query strings, Wiki paths, malformed URLs, and stable resource identity.
- [ ] Commit `backend: add feishu resource resolver`.

### Task 6: Feishu loaders and Markdown normalization

**Files:**
- Create: `backend/internal/infra/feishu/client.go`
- Create: `backend/internal/infra/feishu/docx_loader.go`
- Create: `backend/internal/infra/feishu/sheet_loader.go`
- Create: `backend/internal/infra/feishu/bitable_loader.go`
- Create: `backend/internal/infra/feishu/wiki_loader.go`
- Create: `backend/internal/infra/feishu/markdown_normalizer.go`
- Create: `backend/internal/infra/feishu/testdata/*.json`
- Test: `backend/internal/infra/feishu/*_test.go`

- [ ] Define typed Feishu API calls with pagination, timeout, `Retry-After`, bounded 429/5xx retries, and redacted errors.
- [ ] Traverse docx blocks preserving headings, paragraphs, lists, quotes, code, tables, and children.
- [ ] Paginate sheets and create one Markdown section per sheet with sheet/row metadata.
- [ ] Paginate bitable table/view rows and deterministically flatten complex fields with configurable row/size limits.
- [ ] Resolve Wiki nodes to the underlying resource type and delegate to its loader.
- [ ] Normalize escaping, blank lines, heading levels, table delimiters, and unsupported images consistently.
- [ ] Test fixtures for every resource type, pagination, malformed blocks, Unicode, empty resources, and rate limits.
- [ ] Commit `backend: add feishu resource loaders`.

### Task 7: Feishu sync worker and snapshot promotion

**Files:**
- Create: `backend/internal/service/feishu_import_service.go`
- Create: `backend/internal/service/feishu_import_service_test.go`
- Create: `backend/internal/worker/feishu_sync_job.go`
- Create: `backend/internal/worker/feishu_sync_worker.go`
- Modify: `backend/internal/worker/ingestion_job.go`
- Modify: `backend/internal/worker/ingestion_worker.go`
- Test: `backend/internal/worker/feishu_sync_worker_test.go`

- [ ] Validate KB ownership and URL before creating a pending document; enforce remote uniqueness per KB.
- [ ] Add a River `feishu_sync` job unique by document ID and requested revision; set `sync_status=syncing` while running.
- [ ] Fetch, normalize, write a revision-keyed MinIO snapshot, and set pending fields before enqueueing ingestion.
- [ ] Make ingestion read pending content for remote documents, atomically replace chunks, promote pending fields, and clear sync errors.
- [ ] Preserve active snapshot/chunks when resync fails; set `sync_status=failed` with redacted error.
- [ ] Test first import, duplicate import, unchanged/changed sync, loader/embedding failure, concurrent sync, and old-chunk preservation.
- [ ] Commit `backend: add staged feishu synchronization`.

### Task 8: User ownership enforcement and bootstrap migration

**Files:**
- Modify: `backend/internal/service/kb_service.go`
- Modify: `backend/internal/service/document_service.go`
- Modify: `backend/internal/service/chat_service.go`
- Modify: `backend/internal/http/kb_handler.go`
- Modify: `backend/internal/http/document_handler.go`
- Modify: `backend/internal/http/chunk_handler.go`
- Modify: `backend/internal/http/chat_handler.go`
- Modify: `backend/cmd/server/main.go`
- Test: `backend/internal/service/*_test.go`
- Test: `backend/internal/http/*_test.go`

- [ ] Change service methods to accept `userID` and filter KB/document/conversation access through owner joins.
- [ ] Ensure chunks and neighbors cannot be read by guessing IDs from another user's KB.
- [ ] Add bootstrap assignment using `BOOTSTRAP_OWNER_FEISHU_OPEN_ID`; fail startup when legacy rows exist and no owner is configured.
- [ ] Keep local uploads compatible by assigning them to the authenticated user's KB.
- [ ] Test cross-user reads, writes, deletes, chat streams, chunks, and bootstrap success/failure.
- [ ] Commit `backend: enforce user ownership across knowledge APIs`.

### Task 9: Login and Feishu import frontend

**Files:**
- Create: `frontend/app/login/page.tsx`
- Create: `frontend/components/auth/feishu-login-button.tsx`
- Create: `frontend/components/docs/feishu-import-dialog.tsx`
- Modify: `frontend/components/docs/doc-table.tsx`
- Modify: `frontend/app/kbs/[kbId]/docs/page.tsx`
- Modify: `frontend/app/kbs/[kbId]/docs/[docId]/page.tsx`
- Modify: `frontend/lib/schemas/index.ts`
- Create: `frontend/lib/hooks/use-feishu-import.ts`
- Test: `frontend/components/docs/feishu-import-dialog.test.tsx`
- Test: `frontend/components/auth/feishu-login-button.test.tsx`

- [ ] Add minimal login and redirect unauthenticated business pages to it.
- [ ] Add “上传文件” and “从飞书导入”; validate one URL before submitting.
- [ ] Display source type, sync status, remote URL, last sync time, and retry action.
- [ ] Keep failed resync rows visible and separate previous ready state from current sync failure.
- [ ] Add user menu/logout and auth loading/error states.
- [ ] Migrate selected LiveAgent tokens/components into `frontend/components/liveagent` and `frontend/styles/liveagent.css`; record source in `THIRD_PARTY_NOTICES.md`.
- [ ] Run `cd frontend && pnpm lint && pnpm typecheck && pnpm test`.
- [ ] Commit `feat(frontend): add feishu login and import workflow`.

### Task 10: End-to-end verification and documentation

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`
- Modify: `.env.example`
- Create: `docs/deploy-debug-feishu.md`
- Test: `backend/internal/http/feishu_e2e_test.go`

- [ ] Document Feishu app scopes, redirect URL, tenant restriction, bootstrap owner, cookie setup, and manual-sync semantics.
- [ ] Add fake OAuth and fake Feishu API e2e coverage for login, import, sync, and ownership rejection.
- [ ] Run `cd backend && go test ./...`.
- [ ] Run `cd frontend && pnpm lint && pnpm typecheck && pnpm test && pnpm build`.
- [ ] Run `git diff --check` and verify no credentials or raw Feishu content fixtures are staged; generated sqlc changes must come from generation and match their input changes.
- [ ] Commit `docs: document feishu integration and verification`.

## Self-review checklist

- [ ] Review unfinished items relevant to this delivery. Resolve blocking placeholders or record the reason and acceptance impact of deferral; examples and documented future work do not require zero search matches.
- [ ] Confirm every spec section maps to a task: OAuth/session (2-3), source loaders (5-7), staged ingestion (7), ownership (1/8), Agent seams (4), UI reuse (9), tests/migration (10).
- [ ] Confirm generated files under `backend/internal/repo/generated` are produced only by `sqlc generate`, never hand-edited.
