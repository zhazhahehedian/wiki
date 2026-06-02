# 阶段 1 · 文档摄入闭环 · 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现"上传文档 → 异步解析切片向量化 → 状态变 ready → 前端能看到 chunks"的完整闭环。验收：在前端新建 KB，上传一份 Markdown，30 秒内文档状态变 `ready`，能在文档详情页看到切片内容。

**Architecture:** 后端三层（HTTP handler → service → repo），异步摄入用 river worker pool 解耦上传与解析。所有 LLM/Embedding 接口在 `internal/domain/ports` 定义为裸 Go interface（Eino 延后到 Phase 2）。chunks.embedding 列维度从 `EMBEDDING_DIM` 在 goose Go-migration 中渲染。前端用 TanStack Query 管理服务端状态，上传后轮询 doc 状态直到 ready/failed。

**Tech Stack:** Go 1.22+ · chi · pgx/v5 · sqlc · pressly/goose（库 API + Go migration）· riverqueue/river · pgvector/pgvector-go · goldmark · ledongthuc/pdf · sajari/docconv · xuri/excelize · pkoukk/tiktoken-go · Next.js 15 · React 19 · TanStack Query v5 · zustand · shadcn/ui · Tailwind v4

---

## 重要约定

1. **不做 git commit**：沿用阶段 0 约定（私有公司库未就绪）。每个任务最后一步是"本地验证 + 在本文件勾选 checkbox"，**不**执行 `git add` / `git commit`。立项后用户一次性追溯入库。
2. **决策来源**：本计划严格遵循 [2026-05-19-it-wiki-agent-design.md](../specs/2026-05-19-it-wiki-agent-design.md)（spec）+ [2026-05-20-phase-1-decisions.md](../specs/2026-05-20-phase-1-decisions.md)（决策补充）。遇到歧义先查这两份。
3. **PowerShell 优先**：Windows + PowerShell 7。Docker / docker compose / go / pnpm 都跨 shell 通用。
4. **测试策略**：
   - **TDD**：parser 各实现、splitter 算法、checksum 工具、HTTP 错误信封 — 因为容易出错且无外部依赖
   - **集成测试**：vectorstore、worker、handler、service — 通过任务 17 的端到端 curl 测试覆盖
   - **不做** UI 自动化测试（vibe coding，手动 browser 验证）
5. **不扩展范围**：spec §7 阶段 1 之外的端点（对话、ReAct、retrieval search）一律不实现。看到 spec 提到 `chat_handler.go` / `react_agent.go` **不要**建文件。
6. **Eino 不引入**：本阶段 go.mod 不出现 `github.com/cloudwego/eino`。
7. **goose 迁移用库 API**：阶段 0 plan 写的 `make migrate-up` 调 CLI 的方式**仅适用于 SQL 迁移**。本阶段加入 Go migration（任务 3 的 chunks 表），迁移改由 backend 启动时通过 `goose.UpContext()` 库 API 跑。Makefile 的 `migrate-up` target 改成 `go run ./cmd/migrate up`。
8. **embedding 列写入绕开 sqlc**：sqlc 对 `vector` 类型支持需要额外 plugin，本阶段简化为：所有 chunk 的 INSERT（含 embedding）直接用 pgx，**只有 chunk INSERT 这一处**例外。其他 chunk 查询（SELECT/COUNT）仍走 sqlc 但不返回 embedding 列。

---

## 进度日志

> 每次 session 结束时由执行者更新这一节。下次 session 从"下一步"列出的任务开始。

### 2026-05-22 session 1

**已完成的任务（代码全部写好且 `go build` / `go vet` 干净）：**
- ✅ **任务 1** 后端依赖 + Config 扩展 + dbpool
- ✅ **任务 2** SQL migrations 0001-0003 + migrations 包 + cmd/migrate（Makefile 已切到库 API）
- ✅ **任务 3** Go migration 0004 chunks + SQL migrations 0005-0006
- ✅ **任务 4** sqlc queries + sqlc.yaml（文件已写好，**未跑 `sqlc generate`**）

**本机执行环境约束（影响后续 session）：**
- ❌ 本地 **没装 Docker**：所有 `docker compose up` / `goose up` 实际跑迁移 / curl 端到端等步骤一律跳过，标 `[~]`。运行时验证由用户在服务器上补做（见 `memory/feedback_no_local_docker.md`）。
- ❌ 本地 **没装 sqlc CLI**：任务 4 的 `sqlc generate` 跳过。用户准备 `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest` 后再开新 session。

**已识别的 `[~]` 跳过步骤（需补做）：**
- Step 2.8（postgres + 跑迁移验证）— 待 server / docker
- Step 3.5 / 3.6（迁移 up + down/up cycle 验证）— 待 server / docker
- Step 4.5 / 4.6（`sqlc generate` + 后续 build 验证）— 待装 sqlc CLI

### 2026-05-22 session 2

**用户已装好 sqlc CLI**（路径 `E:\GoProject\bin\sqlc.exe`，v1.30.0；当时未加 PATH，调用时用全路径）。

**本 session 完成的任务：**
- ✅ **任务 4 补完** `sqlc generate` + `go build` 验证（sqlc.yaml 删了 `rules:` 兼容 v1.30；加了 `rename: knowledge_basis: KnowledgeBase` 修正命名）
- ✅ **任务 5** Domain types + ports interfaces
- ✅ **任务 6** Parser dispatcher + 4 格式 (markdown/pdf/docx/xlsx) + TDD（3 测试 PASS）。docx 用 docconv v2，API 是 `text, meta, err := ConvertDocx(r)` 三返回值（plan 写的是两返回值，已适配）
- ✅ **任务 7** RecursiveChar splitter + tiktoken tokenizer + TDD（3 测试 PASS）
- ✅ **任务 8** OpenAI-compatible Embedder（只 build；外部 API 留到 server 阶段）
- ✅ **任务 9** pgvector VectorStore
- ✅ **任务 10** MinIO storage Put/Get/Delete + checksum sha256（测试 PASS）
- ✅ **任务 11** IngestionService
- ✅ **任务 12** River worker。泛型类型修正为 `*river.Client[pgx.Tx]`（plan 里写的 `[pgxpool.Pool]` 不对）
- ✅ **任务 13** HTTP errors / pagination / cors / router shell
- ✅ **任务 14** KB service + handler
- ✅ **任务 15** Document service + handler（含 multipart upload + 413/409 错误信封）
- ✅ **任务 16** Chunk handler + main.go 启动编排（goose migrate → pgxpool → river migrate → embedding dim check → minio bucket → tokenizer init → wire deps → river start → http listen）
- ✅ **任务 18-21** 前端代码全部写完（API client / hooks / KB UI / Doc UI / Chunk UI）

**本 session 全程后端 `go build ./...` 干净，`go test ./... -timeout 120s` 全部 PASS。**

**本机执行环境约束（新发现）：**
- ❌ 本地仍 **没装 Docker** — 所有 docker / curl 端到端步骤继续跳过（标 `[~]`）。
- ✅ 本地装了 **node 22 + corepack** — 但 **pnpm 不在 PATH**，通过 `corepack enable pnpm` + `corepack prepare pnpm@9.12.0 --activate` 激活。
- ⚠️ 前端 node_modules 是从旧路径 `E:\work\it-wiki` 拷贝过来的，虚拟商店符号链接全部失效；**必须先跑 `pnpm install`** 重新链接才能 typecheck / build。本 session 在后台启动了 `pnpm install`，但是否跑完未确认。
- ⚠️ shadcn 组件还没加（需要 `pnpm dlx shadcn@latest add input label table dialog badge sonner`），前端代码引用了这些 ui 组件。

**本 session `[~]` 跳过步骤（需在有 docker 的环境补做）：**
- 后端 Step 2.8 / 3.5 / 3.6（已记录在 session 1）
- 任务 17 整任务（docker 端到端 curl 全流程）
- 任务 18 Step 18.1 中的 shadcn add 部分
- 任务 18 Step 18.9（pnpm typecheck / pnpm build 验证）
- 任务 19-21 Step xx.5/.6（browser 验证）
- 任务 22 整任务（端到端验收）

### 2026-05-26 session 3

**本 session 完成的任务：**
- ✅ **前端依赖补齐**：`pnpm install` 重链完毕；安装缺失包 `@tanstack/react-query`、`@tanstack/react-query-devtools`、`zod`（之前未写入 package.json）
- ✅ **前端 typecheck 修复**（`pnpm typecheck` 全部通过）：
  - `@base-ui/react` 不支持 `asChild`，改为 `render={<Button />}` 模式（kb-create-dialog.tsx）
  - Zod v4 `z.record()` 需要两个参数，全部改为 `z.record(z.string(), z.unknown())`（lib/schemas/index.ts × 4 处）
- ✅ **前端 build 验证**：`Compiled successfully`，类型检查 / 静态页面生成全部通过；EPERM symlink 只出现在 standalone 文件复制阶段（Windows 已知问题，Docker Linux 环境不复现）
- ✅ **后端再次确认**：`go build ./...` 干净，`go vet ./...` 干净，`go test ./... -timeout 120s` 全部 PASS

**当前状态：本地可做的工作已全部完成。**

### 下次 session 的入口

**剩下的工作就是任务 17、22 的 docker 端到端验收**（必须在有 Docker 的服务器环境跑）。本地 session 没有更多代码可写。

**全程仍要遵循的环境约束：**
- 涉及 `docker compose` / `psql` / 跑 server 起 backend 容器 / curl 端到端的步骤，继续标 `[~]` 跳过。
- 涉及外部 API 的代码（embedder OpenAI 兼容调用）只做 `go build`，不真发请求。

---

## 文件清单

阶段 1 完成后新增/修改的文件（**仅列阶段 1 涉及**）：

```
it-wiki/
├── backend/
│   ├── go.mod / go.sum                                # 任务 1 加依赖
│   ├── Makefile                                       # 任务 2 改 migrate target
│   ├── sqlc.yaml                                      # 任务 4 加 override
│   ├── cmd/
│   │   ├── server/main.go                             # 任务 13 编排启动
│   │   └── migrate/main.go                            # 任务 2 新建
│   └── internal/
│       ├── config/config.go                           # 任务 1 加 env 字段
│       ├── repo/
│       │   ├── migrations/
│       │   │   ├── 0001_extensions.sql                # 任务 2
│       │   │   ├── 0002_knowledge_bases.sql           # 任务 2
│       │   │   ├── 0003_documents.sql                 # 任务 2
│       │   │   ├── 0004_chunks_table.go               # 任务 3 (Go migration)
│       │   │   ├── 0005_conversations.sql             # 任务 3
│       │   │   ├── 0006_messages.sql                  # 任务 3
│       │   │   └── migrations.go                      # 任务 2 embed.FS + 包注释
│       │   ├── queries/
│       │   │   ├── knowledge_bases.sql                # 任务 4
│       │   │   ├── documents.sql                      # 任务 4
│       │   │   └── chunks.sql                         # 任务 4
│       │   ├── generated/                             # 任务 4 sqlc 输出
│       │   └── dbpool.go                              # 任务 1 pgxpool 构造
│       ├── domain/
│       │   ├── kb.go                                  # 任务 5
│       │   ├── document.go                            # 任务 5
│       │   ├── chunk.go                               # 任务 5
│       │   └── ports/
│       │       ├── parser.go                          # 任务 5
│       │       ├── splitter.go                        # 任务 5
│       │       ├── embedder.go                        # 任务 5
│       │       ├── llm.go                             # 任务 5 (stub)
│       │       ├── vectorstore.go                     # 任务 5
│       │       └── objectstorage.go                   # 任务 5
│       ├── infra/
│       │   ├── parser/
│       │   │   ├── parser.go                          # 任务 6 dispatcher
│       │   │   ├── markdown.go                        # 任务 6
│       │   │   ├── pdf.go                             # 任务 6
│       │   │   ├── docx.go                            # 任务 6
│       │   │   └── xlsx.go                            # 任务 6
│       │   ├── splitter/recursive_char.go             # 任务 7
│       │   ├── tokenizer/tiktoken.go                  # 任务 7
│       │   ├── embedder/openai_compat.go              # 任务 8
│       │   ├── vectorstore/pgvector.go                # 任务 9
│       │   ├── storage/minio.go                       # 任务 10 扩展 Put/Get
│       │   └── checksum/sha256.go                     # 任务 11 工具
│       ├── service/
│       │   ├── kb_service.go                          # 任务 14
│       │   ├── ingestion_service.go                   # 任务 11
│       │   └── document_service.go                    # 任务 15
│       ├── worker/
│       │   ├── ingestion_job.go                       # 任务 12
│       │   ├── ingestion_worker.go                    # 任务 12
│       │   └── client.go                              # 任务 12
│       └── http/
│           ├── router.go                              # 任务 14 改路由
│           ├── errors.go                              # 任务 13
│           ├── pagination.go                          # 任务 13
│           ├── cors.go                                # 任务 13
│           ├── kb_handler.go                          # 任务 14
│           ├── document_handler.go                    # 任务 15
│           └── chunk_handler.go                       # 任务 16
└── frontend/
    ├── package.json                                   # 任务 18 加 deps
    ├── app/
    │   ├── layout.tsx                                 # 任务 18 wrap Provider
    │   ├── page.tsx                                   # 任务 19 KB 列表
    │   └── kbs/
    │       ├── new/page.tsx                           # 任务 19
    │       └── [kbId]/
    │           ├── docs/page.tsx                      # 任务 20
    │           └── docs/[docId]/page.tsx              # 任务 21
    ├── components/
    │   ├── providers.tsx                              # 任务 18 QueryClientProvider
    │   ├── kb/kb-card.tsx                             # 任务 19
    │   ├── kb/kb-create-dialog.tsx                    # 任务 19
    │   ├── docs/doc-table.tsx                         # 任务 20
    │   ├── docs/doc-uploader.tsx                      # 任务 20
    │   ├── docs/ingest-status-badge.tsx               # 任务 20
    │   └── chunks/chunk-list.tsx                      # 任务 21
    ├── lib/
    │   ├── api/client.ts                              # 任务 18 fetch wrapper
    │   ├── api/kb.ts                                  # 任务 18
    │   ├── api/docs.ts                                # 任务 18
    │   ├── api/chunks.ts                              # 任务 18
    │   ├── schemas/index.ts                           # 任务 18 zod schemas
    │   └── hooks/
    │       ├── use-kbs.ts                             # 任务 19
    │       ├── use-docs.ts                            # 任务 20 (polling)
    │       └── use-chunks.ts                          # 任务 21
```

---

## 端点清单（本阶段实现）

```
POST   /api/v1/kbs                      创建 KB
GET    /api/v1/kbs                      列 KB (limit/offset)
GET    /api/v1/kbs/:id                  KB 详情
DELETE /api/v1/kbs/:id                  删除 KB (级联删 docs/chunks)
POST   /api/v1/kbs/:id/docs             上传文档 (multipart)
GET    /api/v1/kbs/:id/docs             列文档 (limit/offset, ?status filter)
GET    /api/v1/docs/:id                 文档详情 (含 status)
DELETE /api/v1/docs/:id                 删文档 (级联删 chunks)
GET    /api/v1/docs/:id/chunks          列 chunks (limit/offset, 不返回 embedding)
GET    /healthz                         (阶段 0 已实现, 复用)
```

详细 schema 见决策补充 §6.5 + 各 handler 任务的代码块。

---

## 摄入流程

```
client → POST /api/v1/kbs/:id/docs (multipart)
   │
   ▼
ingestion_service.UploadAndEnqueue
   ├─ 读 multipart file
   ├─ 计算 sha256
   ├─ 查 (kb_id, checksum) 是否已存在 → 409 + existing_doc_id
   ├─ ObjectStorage.Put → s3://it-wiki-docs/<kb_id>/<doc_id>/<filename>
   ├─ documents INSERT status='pending'
   └─ river.Enqueue(IngestionJob{doc_id})
   → 201 + doc resource

river worker pool (并发 N=4)
   IngestionWorker.Work(IngestionJob)
   ├─ UPDATE documents SET status='parsing'
   ├─ ObjectStorage.Get → bytes
   ├─ Parser.Parse(mime) → plain text
   ├─ UPDATE documents SET status='chunking'
   ├─ Splitter.Split → []Chunk
   ├─ UPDATE documents SET status='embedding'
   ├─ Embedder.Embed (batch=64) → [][]float32
   ├─ VectorStore.InsertChunks (单事务)
   └─ UPDATE documents SET status='ready'

任何步骤 err: UPDATE documents SET status='failed', error_message=err.Error(), 不重试
```

---

## 任务 1：后端依赖 + Config 扩展 + dbpool

**Files:**
- Modify: `e:\GoProject\it-wiki\backend\go.mod`
- Modify: `e:\GoProject\it-wiki\backend\internal\config\config.go`
- Create: `e:\GoProject\it-wiki\backend\internal\repo\dbpool.go`

- [x] **Step 1.1：拉新依赖**

从 `e:\GoProject\it-wiki\backend`：

```powershell
go get github.com/jackc/pgx/v5@latest
go get github.com/jackc/pgx/v5/pgxpool@latest
go get github.com/pressly/goose/v3@latest
go get github.com/riverqueue/river@latest
go get github.com/riverqueue/river/riverdriver/riverpgxv5@latest
go get github.com/riverqueue/river/rivermigrate@latest
go get github.com/google/uuid@latest
go get github.com/yuin/goldmark@latest
go get github.com/ledongthuc/pdf@latest
go get code.sajari.com/docconv/v2@latest
go get github.com/xuri/excelize/v2@latest
go get github.com/pkoukk/tiktoken-go@latest
go get github.com/pgvector/pgvector-go@latest
go mod tidy
```

> ⚠️ docconv 包路径写本计划时为 `code.sajari.com/docconv/v2`，若 `go get` 失败请用 `github.com/sajari/docconv` 或 `code.sajari.com/docconv`（去掉 v2）。

- [x] **Step 1.2：扩展 `internal/config/config.go`**

在 `Config` struct 中加字段（追加在 `EmbeddingDim` 后）：

```go
	TokenizerEncoding string // 默认 "cl100k_base"
	ChunkSize         int    // 默认 800 tokens
	ChunkOverlap      int    // 默认 120 tokens
	EmbedBatchSize    int    // 默认 64
	UploadMaxBytes    int64  // 默认 50 * 1024 * 1024
	RiverMaxWorkers   int    // 默认 4
```

在 `Load()` 中加（追加在已有字段后）：

```go
	cfg.TokenizerEncoding = getEnv("TOKENIZER_ENCODING", "cl100k_base")
	cfg.ChunkSize, _ = strconv.Atoi(getEnv("CHUNK_SIZE", "800"))
	cfg.ChunkOverlap, _ = strconv.Atoi(getEnv("CHUNK_OVERLAP", "120"))
	cfg.EmbedBatchSize, _ = strconv.Atoi(getEnv("EMBED_BATCH_SIZE", "64"))
	maxMB, _ := strconv.ParseInt(getEnv("UPLOAD_MAX_MB", "50"), 10, 64)
	cfg.UploadMaxBytes = maxMB * 1024 * 1024
	cfg.RiverMaxWorkers, _ = strconv.Atoi(getEnv("RIVER_MAX_WORKERS", "4"))
```

同步追加到 `e:\GoProject\it-wiki\.env.example`（在 `# ----- Backend -----` 段落下）：

```env
# ----- Splitter / Embedder runtime -----
TOKENIZER_ENCODING=cl100k_base
CHUNK_SIZE=800
CHUNK_OVERLAP=120
EMBED_BATCH_SIZE=64
UPLOAD_MAX_MB=50
RIVER_MAX_WORKERS=4
```

同步追加到 `.env`（用户本地副本）。

- [x] **Step 1.3：写 `internal/repo/dbpool.go`**

```go
package repo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pgxpool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return pool, nil
}
```

- [x] **Step 1.4：验证编译**

```powershell
cd e:\GoProject\it-wiki\backend
go build ./...
```

期望：无报错。如果 docconv 拉取失败，调整路径见 1.1 备注，重新 `go mod tidy` + build。

- [x] **Step 1.5：完成检查**

勾选 1.1~1.5。

---

## 任务 2：SQL migrations 0001-0003 + migrations 包 + cmd/migrate

**Files:**
- Create: `e:\GoProject\it-wiki\backend\internal\repo\migrations\migrations.go`
- Create: `e:\GoProject\it-wiki\backend\internal\repo\migrations\0001_extensions.sql`
- Create: `e:\GoProject\it-wiki\backend\internal\repo\migrations\0002_knowledge_bases.sql`
- Create: `e:\GoProject\it-wiki\backend\internal\repo\migrations\0003_documents.sql`
- Create: `e:\GoProject\it-wiki\backend\cmd\migrate\main.go`
- Modify: `e:\GoProject\it-wiki\backend\Makefile`

- [x] **Step 2.1：删除旧 `.gitkeep`**

```powershell
Remove-Item "e:\GoProject\it-wiki\backend\internal\repo\migrations\.gitkeep" -ErrorAction SilentlyContinue
```

- [x] **Step 2.2：写 `migrations.go`**（embed.FS + 包注释）

```go
// Package migrations 包含 goose 迁移：SQL 文件用 embed.FS 加载，
// Go 文件通过 init() 注册到 goose registry。
//
// 启动顺序：cmd/server/main.go 调用 goose.UpContext(ctx, db, ".") 时
// 会同时读取 EmbedMigrations 中的 .sql 和已注册的 Go migrations。
package migrations

import "embed"

//go:embed *.sql
var EmbedMigrations embed.FS
```

- [x] **Step 2.3：写 `0001_extensions.sql`**

```sql
-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP EXTENSION IF EXISTS vector;
DROP EXTENSION IF EXISTS pgcrypto;
-- +goose StatementEnd
```

> 注：pgcrypto 提供 `gen_random_uuid()`。pgvector 镜像默认装好 vector，本迁移幂等。

- [x] **Step 2.4：写 `0002_knowledge_bases.sql`**

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE knowledge_bases (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    owner_id      TEXT NOT NULL DEFAULT 'local-admin',
    embed_model   TEXT NOT NULL,
    embed_dim     INT  NOT NULL,
    settings      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_kbs_owner ON knowledge_bases (owner_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS knowledge_bases;
-- +goose StatementEnd
```

- [x] **Step 2.5：写 `0003_documents.sql`**

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE documents (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id         UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    source_type   TEXT NOT NULL,
    source_ref    TEXT NOT NULL,
    title         TEXT NOT NULL,
    mime_type     TEXT NOT NULL,
    bytes         BIGINT NOT NULL,
    checksum      TEXT NOT NULL,
    status        TEXT NOT NULL,
    error_message TEXT,
    metadata      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_docs_kb_status ON documents (kb_id, status);
CREATE UNIQUE INDEX uniq_docs_kb_checksum ON documents (kb_id, checksum);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS documents;
-- +goose StatementEnd
```

- [x] **Step 2.6：写 `cmd/migrate/main.go`**

```go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/zenith-wang/it-wiki/backend/internal/config"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/migrations"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: migrate <up|down|status|reset>")
	}
	cmd := os.Args[1]

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	goose.SetBaseFS(migrations.EmbedMigrations)
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("set dialect: %v", err)
	}

	ctx := context.Background()
	switch cmd {
	case "up":
		if err := goose.UpContext(ctx, db, "."); err != nil {
			log.Fatalf("goose up: %v", err)
		}
	case "down":
		if err := goose.DownContext(ctx, db, "."); err != nil {
			log.Fatalf("goose down: %v", err)
		}
	case "status":
		if err := goose.StatusContext(ctx, db, "."); err != nil {
			log.Fatalf("goose status: %v", err)
		}
	case "reset":
		if err := goose.ResetContext(ctx, db, "."); err != nil {
			log.Fatalf("goose reset: %v", err)
		}
	default:
		log.Fatalf("unknown command: %s", cmd)
	}

	fmt.Println("done.")
}
```

- [x] **Step 2.7：改 `backend/Makefile` 的 migrate target**

把现有 `migrate-up` / `migrate-down` / `migrate-status` / `migrate-new` 4 个 target 替换为：

```makefile
migrate-up: ## Apply all migrations (uses cmd/migrate, supports Go migrations)
	go run ./cmd/migrate up

migrate-down: ## Rollback one migration
	go run ./cmd/migrate down

migrate-status: ## Show migration status
	go run ./cmd/migrate status

migrate-reset: ## DESTRUCTIVE: rollback all migrations
	go run ./cmd/migrate reset
```

> 删 `migrate-new`（goose CLI 还能用作生成器，但启动统一走 library API）。如果要生成新迁移文件，手动 `New-Item migrations\NNNN_xxx.sql` 或 `.go`。

- [~] **Step 2.8：起 postgres + 跑迁移验证（前 3 个）** — 已跳过（本地无 docker，待服务器部署阶段补做）

```powershell
cd e:\GoProject\it-wiki
docker compose -f deploy/docker-compose.yml --env-file .env up -d postgres
Start-Sleep 5

# 加载 env 后跑迁移
cd backend
Get-Content "..\.env" | ForEach-Object {
  if ($_ -match '^([A-Z_]+)=(.*)$') {
    [System.Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process')
  }
}
# 注意 .env 里的 DATABASE_URL 用的是 `postgres:5432` (容器内名), 本机跑要换成 localhost
$env:DATABASE_URL = "postgres://itwiki:itwiki_dev_pass@localhost:5432/itwiki?sslmode=disable"
go run ./cmd/migrate status
go run ./cmd/migrate up
go run ./cmd/migrate status
```

期望 `status` 输出三条 `Applied`（0001、0002、0003）。如果 0004（chunks）不存在是正常的——下一任务才加。

- [x] **Step 2.9：完成检查**

勾选 2.1~2.9（2.8 已跳过 — 本地无 docker）。

---

## 任务 3：Go migration 0004 chunks + SQL migrations 0005-0006

**Files:**
- Create: `e:\GoProject\it-wiki\backend\internal\repo\migrations\0004_chunks_table.go`
- Create: `e:\GoProject\it-wiki\backend\internal\repo\migrations\0005_conversations.sql`
- Create: `e:\GoProject\it-wiki\backend\internal\repo\migrations\0006_messages.sql`

- [x] **Step 3.1：写 `0004_chunks_table.go`**

```go
package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upChunksTable, downChunksTable)
}

func upChunksTable(ctx context.Context, tx *sql.Tx) error {
	dimStr := os.Getenv("EMBEDDING_DIM")
	if dimStr == "" {
		return fmt.Errorf("EMBEDDING_DIM env var is required for chunks migration")
	}
	dim, err := strconv.Atoi(dimStr)
	if err != nil || dim <= 0 {
		return fmt.Errorf("EMBEDDING_DIM must be positive int, got %q", dimStr)
	}

	stmt := fmt.Sprintf(`
CREATE TABLE chunks (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id         UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    document_id   UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    seq           INT  NOT NULL,
    content       TEXT NOT NULL,
    token_count   INT  NOT NULL,
    embedding     vector(%d),
    metadata      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_chunks_doc_seq ON chunks (kb_id, document_id, seq);
CREATE INDEX idx_chunks_ivfflat ON chunks USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
`, dim)
	_, err = tx.ExecContext(ctx, stmt)
	return err
}

func downChunksTable(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS chunks;`)
	return err
}
```

- [x] **Step 3.2：写 `0005_conversations.sql`**

虽然 Phase 1 无对话 API，但 spec §4.4 要求建表（Phase 2 用），趁迁移流程一并建好。

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE conversations (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id         UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    title         TEXT NOT NULL DEFAULT '新对话',
    mode          TEXT NOT NULL,
    user_id       TEXT NOT NULL DEFAULT 'local-admin',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_conv_kb_user_updated ON conversations (kb_id, user_id, updated_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS conversations;
-- +goose StatementEnd
```

- [x] **Step 3.3：写 `0006_messages.sql`**

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL,
    content         TEXT NOT NULL,
    citations       JSONB NOT NULL DEFAULT '[]'::jsonb,
    tool_calls      JSONB NOT NULL DEFAULT '[]'::jsonb,
    token_usage     JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_msg_conv_created ON messages (conversation_id, created_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS messages;
-- +goose StatementEnd
```

- [x] **Step 3.4：让 cmd/migrate 触发 init**

修改 `e:\GoProject\it-wiki\backend\cmd\migrate\main.go`，把 `migrations` 导入从 named 改为 blank import（已经导入为 named，触发 init 是自动的，但 Go migration 的 `init()` 副作用必须确保包被 import）。**当前已经导入了 `migrations` 包用于 `EmbedMigrations`，无需改动**——只要 0004_chunks_table.go 与 migrations.go 在同一个包下，`init()` 就会随包 import 自动触发。

但是 cmd/server/main.go 后续也会调 goose，必须同样 import migrations 包。任务 13 处理。

- [~] **Step 3.5：跑迁移验证** — 已跳过（本地无 docker，待服务器部署阶段补做）

```powershell
cd e:\GoProject\it-wiki\backend
$env:DATABASE_URL = "postgres://itwiki:itwiki_dev_pass@localhost:5432/itwiki?sslmode=disable"
$env:EMBEDDING_DIM = "1024"
go run ./cmd/migrate up
go run ./cmd/migrate status
```

期望：6 条迁移全部 Applied。

进 psql 验证 chunks 列维度：

```powershell
docker exec -it $(docker ps -q --filter "name=postgres") psql -U itwiki -d itwiki -c "\d chunks"
```

期望看到 `embedding | vector(1024)`。

- [~] **Step 3.6：down 测试** — 已跳过（本地无 docker，待服务器部署阶段补做）

```powershell
go run ./cmd/migrate down  # 回滚 0006
go run ./cmd/migrate down  # 回滚 0005
go run ./cmd/migrate down  # 回滚 0004 (Go migration)
go run ./cmd/migrate status
go run ./cmd/migrate up    # 再升回去
```

期望：down 三步成功，再 up 回 6 条 Applied。**这一步验证 Go migration 注册到位**。

- [x] **Step 3.7：完成检查**

勾选 3.1~3.7（3.5/3.6 已跳过 — 本地无 docker）。

---

## 任务 4：sqlc queries + sqlc generate

**Files:**
- Modify: `e:\GoProject\it-wiki\backend\sqlc.yaml`
- Create: `e:\GoProject\it-wiki\backend\internal\repo\queries\knowledge_bases.sql`
- Create: `e:\GoProject\it-wiki\backend\internal\repo\queries\documents.sql`
- Create: `e:\GoProject\it-wiki\backend\internal\repo\queries\chunks.sql`
- Create: `e:\GoProject\it-wiki\backend\internal\repo\generated\*.go`（sqlc 输出）

- [x] **Step 4.1：删除旧 `.gitkeep`**

```powershell
Remove-Item "e:\GoProject\it-wiki\backend\internal\repo\queries\.gitkeep" -ErrorAction SilentlyContinue
```

- [x] **Step 4.2：修改 `sqlc.yaml`**

替换整个文件：

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "./internal/repo/queries"
    schema:  "./internal/repo/migrations"
    gen:
      go:
        package: "generated"
        out: "./internal/repo/generated"
        sql_package: "pgx/v5"
        emit_json_tags: true
        emit_interface: true
        emit_empty_slices: true
        emit_pointers_for_null_types: true
        overrides:
          - db_type: "uuid"
            go_type: "github.com/google/uuid.UUID"
          - db_type: "timestamptz"
            go_type: "time.Time"
          - db_type: "jsonb"
            go_type: "encoding/json.RawMessage"
        rules:
          - sqlc/db-prepare
```

> 注意 sqlc 不解析 Go migration（0004_chunks_table.go），所以 schema 目录里 `chunks` 表对 sqlc 不可见。**chunks 表的查询不能用 sqlc 生成**——但我们 already 决定 chunk INSERT 走 raw pgx，list/count 也直接用 pgx。无影响。

- [x] **Step 4.3：写 `queries/knowledge_bases.sql`**

```sql
-- name: CreateKnowledgeBase :one
INSERT INTO knowledge_bases (name, description, embed_model, embed_dim, settings)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetKnowledgeBase :one
SELECT * FROM knowledge_bases WHERE id = $1;

-- name: ListKnowledgeBases :many
SELECT * FROM knowledge_bases
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountKnowledgeBases :one
SELECT COUNT(*) FROM knowledge_bases;

-- name: DeleteKnowledgeBase :exec
DELETE FROM knowledge_bases WHERE id = $1;
```

- [x] **Step 4.4：写 `queries/documents.sql`**

```sql
-- name: CreateDocument :one
INSERT INTO documents (kb_id, source_type, source_ref, title, mime_type, bytes, checksum, status, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetDocument :one
SELECT * FROM documents WHERE id = $1;

-- name: FindDocumentByChecksum :one
SELECT * FROM documents WHERE kb_id = $1 AND checksum = $2;

-- name: ListDocumentsByKB :many
SELECT * FROM documents
WHERE kb_id = $1
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountDocumentsByKB :one
SELECT COUNT(*) FROM documents
WHERE kb_id = $1
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);

-- name: UpdateDocumentStatus :exec
UPDATE documents
SET status = $2, error_message = $3, updated_at = now()
WHERE id = $1;

-- name: DeleteDocument :exec
DELETE FROM documents WHERE id = $1;
```

- [x] **Step 4.5：跑 sqlc generate**

> ⚠️ chunks 表在 Go migration 中，sqlc 读 schema 目录时会忽略 .go 文件——只看 .sql。所以 `queries/chunks.sql` 不能依赖 chunks 表存在。本任务**不**为 chunks 写 sqlc 查询，全部通过 raw pgx 在 `infra/vectorstore/pgvector.go` 实现（任务 9）。

不创建 `queries/chunks.sql`，跳过。

```powershell
cd e:\GoProject\it-wiki\backend
sqlc generate
```

期望：`internal/repo/generated/` 下出现 `models.go`、`db.go`、`knowledge_bases.sql.go`、`documents.sql.go`、`querier.go`。无报错。

如果 sqlc 报错说找不到表 `knowledge_bases`，**sqlc 读 schema 目录跟 goose 读不一样**——sqlc 会按文件名顺序拼所有 .sql 解析。0001-0003 都是 SQL 没问题；0004 是 .go 文件 sqlc 跳过；0005-0006 是 SQL 但 conversations/messages 不被任何 query 引用，OK。

- [x] **Step 4.6：编译验证**

```powershell
go build ./...
```

期望：无报错。

- [x] **Step 4.7：完成检查**

勾选 4.1~4.7。

---

## 任务 5：Domain types + ports interfaces

**Files:**
- Create: `e:\GoProject\it-wiki\backend\internal\domain\kb.go`
- Create: `e:\GoProject\it-wiki\backend\internal\domain\document.go`
- Create: `e:\GoProject\it-wiki\backend\internal\domain\chunk.go`
- Create: `e:\GoProject\it-wiki\backend\internal\domain\ports\parser.go`
- Create: `e:\GoProject\it-wiki\backend\internal\domain\ports\splitter.go`
- Create: `e:\GoProject\it-wiki\backend\internal\domain\ports\embedder.go`
- Create: `e:\GoProject\it-wiki\backend\internal\domain\ports\llm.go`
- Create: `e:\GoProject\it-wiki\backend\internal\domain\ports\vectorstore.go`
- Create: `e:\GoProject\it-wiki\backend\internal\domain\ports\objectstorage.go`

> Phase 1 用裸 Go interface（D1 决策）。signatures 简化：去掉 Phase 1 用不到的方法（VectorStore.Search/Delete 留到 Phase 2，LLMClient 全部 stub）。

- [x] **Step 5.1：写 `domain/kb.go`**

```go
package domain

import "time"

type KnowledgeBase struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	OwnerID     string    `json:"owner_id"`
	EmbedModel  string    `json:"embed_model"`
	EmbedDim    int       `json:"embed_dim"`
	Settings    map[string]any `json:"settings"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
```

- [x] **Step 5.2：写 `domain/document.go`**

```go
package domain

import "time"

type DocStatus string

const (
	StatusPending   DocStatus = "pending"
	StatusParsing   DocStatus = "parsing"
	StatusChunking  DocStatus = "chunking"
	StatusEmbedding DocStatus = "embedding"
	StatusReady     DocStatus = "ready"
	StatusFailed    DocStatus = "failed"
)

type Document struct {
	ID           string         `json:"id"`
	KBID         string         `json:"kb_id"`
	SourceType   string         `json:"source_type"`
	SourceRef    string         `json:"source_ref"`
	Title        string         `json:"title"`
	MimeType     string         `json:"mime_type"`
	Bytes        int64          `json:"bytes"`
	Checksum     string         `json:"checksum"`
	Status       DocStatus      `json:"status"`
	ErrorMessage *string        `json:"error_message,omitempty"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}
```

- [x] **Step 5.3：写 `domain/chunk.go`**

```go
package domain

import "time"

type Chunk struct {
	ID         string         `json:"id"`
	KBID       string         `json:"kb_id"`
	DocumentID string         `json:"document_id"`
	Seq        int            `json:"seq"`
	Content    string         `json:"content"`
	TokenCount int            `json:"token_count"`
	Metadata   map[string]any `json:"metadata"`
	CreatedAt  time.Time      `json:"created_at"`
}

// ChunkWithEmbedding 仅在 worker 内部传输用，不暴露给 HTTP 层。
type ChunkWithEmbedding struct {
	Chunk
	Embedding []float32
}
```

- [x] **Step 5.4：写 `ports/parser.go`**

```go
package ports

import (
	"context"
	"io"
)

type ParseResult struct {
	Text     string
	Metadata map[string]any
}

type Parser interface {
	Supports(mime string) bool
	// Parse 拿到 mime 是为了让 dispatcher 内部按 mime 路由；具体格式实现可忽略此参数。
	Parse(ctx context.Context, r io.Reader, mime string) (*ParseResult, error)
}
```

- [x] **Step 5.5：写 `ports/splitter.go`**

```go
package ports

import "context"

type SplitOptions struct {
	ChunkSize int // tokens
	Overlap   int // tokens
}

type SplitChunk struct {
	Seq        int
	Content    string
	TokenCount int
}

type Splitter interface {
	Split(ctx context.Context, text string, opts SplitOptions) ([]SplitChunk, error)
}
```

- [x] **Step 5.6：写 `ports/embedder.go`**

```go
package ports

import "context"

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dim() int
	Model() string
}
```

- [x] **Step 5.7：写 `ports/llm.go`**（stub，Phase 2 用）

```go
package ports

import "context"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatOptions struct {
	Model       string
	Temperature float32
	MaxTokens   int
}

// LLMClient Phase 1 不实例化，仅占位防止后续 import cycle。
type LLMClient interface {
	Chat(ctx context.Context, msgs []Message, opts ChatOptions) (*Message, error)
}
```

- [x] **Step 5.8：写 `ports/vectorstore.go`**

```go
package ports

import (
	"context"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

type VectorStore interface {
	// InsertChunks 在单事务内 INSERT chunks + 向量列。
	// Phase 1 不需要 Search / Delete（Phase 2 加）。
	InsertChunks(ctx context.Context, items []domain.ChunkWithEmbedding) error
}
```

- [x] **Step 5.9：写 `ports/objectstorage.go`**

```go
package ports

import (
	"context"
	"io"
)

type ObjectStorage interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}
```

- [x] **Step 5.10：编译验证**

```powershell
cd e:\GoProject\it-wiki\backend
go build ./...
```

期望：无报错（只有 interface，无实现）。

- [x] **Step 5.11：完成检查**

勾选 5.1~5.11。

---

## 任务 6：Parser 实现（dispatcher + 4 格式，TDD）

**Files:**
- Create: `e:\GoProject\it-wiki\backend\internal\infra\parser\parser.go`
- Create: `e:\GoProject\it-wiki\backend\internal\infra\parser\markdown.go`
- Create: `e:\GoProject\it-wiki\backend\internal\infra\parser\pdf.go`
- Create: `e:\GoProject\it-wiki\backend\internal\infra\parser\docx.go`
- Create: `e:\GoProject\it-wiki\backend\internal\infra\parser\xlsx.go`
- Create: `e:\GoProject\it-wiki\backend\internal\infra\parser\parser_test.go`
- Create: `e:\GoProject\it-wiki\backend\internal\infra\parser\testdata\hello.md`

- [x] **Step 6.1：写 `parser/markdown.go`**

```go
package parser

import (
	"bytes"
	"context"
	"io"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type Markdown struct{}

func (Markdown) Supports(mime string) bool {
	return mime == "text/markdown" || mime == "text/x-markdown"
}

func (Markdown) Parse(ctx context.Context, r io.Reader, mime string) (*ports.ParseResult, error) {
	_ = mime
	src, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	md := goldmark.New()
	root := md.Parser().Parse(text.NewReader(src))

	var buf bytes.Buffer
	walkText(root, src, &buf)

	return &ports.ParseResult{
		Text:     buf.String(),
		Metadata: map[string]any{"format": "markdown"},
	}, nil
}

func walkText(n ast.Node, src []byte, w *bytes.Buffer) {
	if n == nil {
		return
	}
	if t, ok := n.(*ast.Text); ok {
		w.Write(t.Segment.Value(src))
	}
	switch n.(type) {
	case *ast.Paragraph, *ast.Heading, *ast.ListItem, *ast.CodeBlock, *ast.FencedCodeBlock:
		defer w.WriteString("\n\n")
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		walkText(c, src, w)
	}
}
```

- [x] **Step 6.2：写 `parser/pdf.go`**

```go
package parser

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	pdfreader "github.com/ledongthuc/pdf"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type PDF struct{}

func (PDF) Supports(mime string) bool {
	return mime == "application/pdf"
}

func (PDF) Parse(ctx context.Context, r io.Reader, mime string) (*ports.ParseResult, error) {
	_ = mime
	tmp, err := os.CreateTemp("", "pdf-*.pdf")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	size, err := io.Copy(tmp, r)
	if err != nil {
		return nil, fmt.Errorf("copy pdf to tmp: %w", err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	pdfDoc, err := pdfreader.NewReader(tmp, size)
	if err != nil {
		return nil, fmt.Errorf("pdf reader: %w", err)
	}

	var buf bytes.Buffer
	pages := pdfDoc.NumPage()
	for i := 1; i <= pages; i++ {
		p := pdfDoc.Page(i)
		if p.V.IsNull() {
			continue
		}
		t, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		buf.WriteString(t)
		buf.WriteString("\n\n")
	}

	return &ports.ParseResult{
		Text:     buf.String(),
		Metadata: map[string]any{"format": "pdf", "pages": pages},
	}, nil
}
```

- [x] **Step 6.3：写 `parser/docx.go`**（先试 docconv — v2 API 返回 3 值，已适配）

```go
package parser

import (
	"bytes"
	"context"
	"fmt"
	"io"

	docconv "code.sajari.com/docconv/v2"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type DOCX struct{}

func (DOCX) Supports(mime string) bool {
	return mime == "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
}

func (DOCX) Parse(ctx context.Context, r io.Reader, mime string) (*ports.ParseResult, error) {
	_ = mime
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	res, err := docconv.ConvertDocx(bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("docconv: %w", err)
	}
	return &ports.ParseResult{
		Text:     res,
		Metadata: map[string]any{"format": "docx"},
	}, nil
}
```

> ⚠️ docconv 编译失败 / 运行需外部 CLI 的降级路径：
> 1. `go mod edit -droprequire code.sajari.com/docconv/v2`
> 2. 改用 `github.com/fumiama/go-docx` 或自写 unzip+xml.Decoder 抽 `<w:t>`
> 3. 保持 interface 不变

- [x] **Step 6.4：写 `parser/xlsx.go`**

```go
package parser

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type XLSX struct{}

func (XLSX) Supports(mime string) bool {
	return mime == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
}

func (XLSX) Parse(ctx context.Context, r io.Reader, mime string) (*ports.ParseResult, error) {
	_ = mime
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	f, err := excelize.OpenReader(bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("excelize open: %w", err)
	}
	defer f.Close()

	var out strings.Builder
	sheets := f.GetSheetList()
	for _, sheet := range sheets {
		out.WriteString("# Sheet: ")
		out.WriteString(sheet)
		out.WriteString("\n\n")
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		for _, row := range rows {
			out.WriteString(strings.Join(row, "\t"))
			out.WriteString("\n")
		}
		out.WriteString("\n")
	}

	return &ports.ParseResult{
		Text:     out.String(),
		Metadata: map[string]any{"format": "xlsx", "sheets": len(sheets)},
	}, nil
}
```

- [x] **Step 6.5：写 `parser/parser.go`（dispatcher）**

```go
package parser

import (
	"context"
	"fmt"
	"io"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type Dispatcher struct {
	parsers []ports.Parser
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		parsers: []ports.Parser{
			Markdown{},
			PDF{},
			DOCX{},
			XLSX{},
		},
	}
}

func (d *Dispatcher) Supports(mime string) bool {
	for _, p := range d.parsers {
		if p.Supports(mime) {
			return true
		}
	}
	return false
}

func (d *Dispatcher) Parse(ctx context.Context, r io.Reader, mime string) (*ports.ParseResult, error) {
	for _, p := range d.parsers {
		if p.Supports(mime) {
			return p.Parse(ctx, r, mime)
		}
	}
	// 兜底：当文本处理
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("fallback read: %w", err)
		}
	}
	return &ports.ParseResult{
		Text:     string(buf),
		Metadata: map[string]any{"format": "unknown", "mime": mime},
	}, nil
}
```

- [x] **Step 6.6：写测试数据 `testdata/hello.md`**

```powershell
New-Item -ItemType Directory -Force "e:\GoProject\it-wiki\backend\internal\infra\parser\testdata" | Out-Null
@'
# Hello

This is a paragraph with **bold** and *italic*.

- Item one
- Item two
'@ | Set-Content -Path "e:\GoProject\it-wiki\backend\internal\infra\parser\testdata\hello.md" -Encoding UTF8
```

- [x] **Step 6.7：写 `parser_test.go`**

```go
package parser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkdownParse(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "hello.md"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	res, err := (Markdown{}).Parse(context.Background(), f, "text/markdown")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.Contains(res.Text, "Hello") {
		t.Errorf("expected 'Hello' in text, got %q", res.Text)
	}
	if !strings.Contains(res.Text, "Item one") {
		t.Errorf("expected 'Item one' in text, got %q", res.Text)
	}
}

func TestDispatcherSupports(t *testing.T) {
	d := NewDispatcher()
	cases := map[string]bool{
		"text/markdown":            true,
		"application/pdf":          true,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":       true,
		"application/octet-stream": false,
	}
	for mime, want := range cases {
		if got := d.Supports(mime); got != want {
			t.Errorf("Supports(%q) = %v, want %v", mime, got, want)
		}
	}
}

func TestDispatcherFallback(t *testing.T) {
	d := NewDispatcher()
	res, err := d.Parse(context.Background(), strings.NewReader("plain text"), "application/octet-stream")
	if err != nil {
		t.Fatalf("fallback parse: %v", err)
	}
	if res.Text != "plain text" {
		t.Errorf("fallback text mismatch: %q", res.Text)
	}
}
```

- [x] **Step 6.8：跑测试**

```powershell
cd e:\GoProject\it-wiki\backend
go test ./internal/infra/parser/... -v
```

期望：3 个测试 PASS。docconv 编译失败时按 6.3 备注降级。

- [x] **Step 6.9：完成检查**

勾选 6.1~6.9。

---

## 任务 7：Splitter + Tokenizer

**Files:**
- Create: `e:\GoProject\it-wiki\backend\internal\infra\tokenizer\tiktoken.go`
- Create: `e:\GoProject\it-wiki\backend\internal\infra\splitter\recursive_char.go`
- Create: `e:\GoProject\it-wiki\backend\internal\infra\splitter\recursive_char_test.go`

- [x] **Step 7.1：写 `tokenizer/tiktoken.go`**

```go
package tokenizer

import (
	"fmt"
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

type Tiktoken struct {
	enc *tiktoken.Tiktoken
}

var (
	defaultInstance *Tiktoken
	initOnce        sync.Once
	initErr         error
)

func Init(encoding string) error {
	initOnce.Do(func() {
		enc, err := tiktoken.GetEncoding(encoding)
		if err != nil {
			initErr = fmt.Errorf("get tiktoken encoding %q: %w", encoding, err)
			return
		}
		defaultInstance = &Tiktoken{enc: enc}
	})
	return initErr
}

func Default() *Tiktoken {
	if defaultInstance == nil {
		panic("tokenizer not initialized; call tokenizer.Init first")
	}
	return defaultInstance
}

func (t *Tiktoken) Count(s string) int {
	return len(t.enc.Encode(s, nil, nil))
}

func (t *Tiktoken) Encode(s string) []int {
	return t.enc.Encode(s, nil, nil)
}

func (t *Tiktoken) Decode(ids []int) string {
	return t.enc.Decode(ids)
}
```

- [x] **Step 7.2：写 `splitter/recursive_char.go`**

```go
package splitter

import (
	"context"
	"strings"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/tokenizer"
)

var defaultSeparators = []string{"\n\n", "。", "！", "？", ". ", "! ", "? ", "\n", " ", ""}

type RecursiveChar struct {
	separators []string
}

func New() *RecursiveChar {
	return &RecursiveChar{separators: defaultSeparators}
}

func (s *RecursiveChar) Split(ctx context.Context, text string, opts ports.SplitOptions) ([]ports.SplitChunk, error) {
	tok := tokenizer.Default()
	pieces := s.splitRecursive(text, opts.ChunkSize, tok)
	chunks := mergeWithOverlap(pieces, opts.ChunkSize, opts.Overlap, tok)

	out := make([]ports.SplitChunk, 0, len(chunks))
	for i, c := range chunks {
		out = append(out, ports.SplitChunk{
			Seq:        i,
			Content:    c,
			TokenCount: tok.Count(c),
		})
	}
	return out, nil
}

func (s *RecursiveChar) splitRecursive(text string, chunkSize int, tok *tokenizer.Tiktoken) []string {
	if tok.Count(text) <= chunkSize {
		return []string{text}
	}
	for _, sep := range s.separators {
		if sep == "" {
			return hardSplitByToken(text, chunkSize, tok)
		}
		if !strings.Contains(text, sep) {
			continue
		}
		parts := strings.Split(text, sep)
		var result []string
		for _, p := range parts {
			if p == "" {
				continue
			}
			result = append(result, s.splitRecursive(p, chunkSize, tok)...)
		}
		return result
	}
	return []string{text}
}

func hardSplitByToken(text string, chunkSize int, tok *tokenizer.Tiktoken) []string {
	ids := tok.Encode(text)
	var out []string
	for i := 0; i < len(ids); i += chunkSize {
		end := i + chunkSize
		if end > len(ids) {
			end = len(ids)
		}
		out = append(out, tok.Decode(ids[i:end]))
	}
	return out
}

func mergeWithOverlap(pieces []string, chunkSize, overlap int, tok *tokenizer.Tiktoken) []string {
	if len(pieces) == 0 {
		return nil
	}
	var chunks []string
	var current strings.Builder
	curCount := 0

	for _, p := range pieces {
		pCount := tok.Count(p)
		if curCount+pCount > chunkSize && current.Len() > 0 {
			chunks = append(chunks, current.String())
			tail := tailTokens(current.String(), overlap, tok)
			current.Reset()
			current.WriteString(tail)
			curCount = tok.Count(tail)
		}
		current.WriteString(p)
		curCount += pCount
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

func tailTokens(s string, n int, tok *tokenizer.Tiktoken) string {
	if n <= 0 {
		return ""
	}
	ids := tok.Encode(s)
	if len(ids) <= n {
		return s
	}
	return tok.Decode(ids[len(ids)-n:])
}
```

- [x] **Step 7.3：写 `splitter/recursive_char_test.go`**

```go
package splitter

import (
	"context"
	"strings"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/tokenizer"
)

func TestMain(m *testing.M) {
	if err := tokenizer.Init("cl100k_base"); err != nil {
		panic(err)
	}
	m.Run()
}

func TestSplitShortTextReturnsSingleChunk(t *testing.T) {
	s := New()
	chunks, err := s.Split(context.Background(), "hello world", ports.SplitOptions{ChunkSize: 800, Overlap: 120})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Content != "hello world" {
		t.Errorf("content mismatch: %q", chunks[0].Content)
	}
}

func TestSplitLongTextProducesMultipleChunks(t *testing.T) {
	para := strings.Repeat("This is a test sentence used to verify the splitter behavior. ", 100)
	text := para + "\n\n" + para
	s := New()
	chunks, err := s.Split(context.Background(), text, ports.SplitOptions{ChunkSize: 400, Overlap: 50})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("want >= 2 chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if c.TokenCount > 400+50 {
			t.Errorf("chunk %d token count %d exceeds limit", i, c.TokenCount)
		}
		if c.Seq != i {
			t.Errorf("chunk %d seq = %d", i, c.Seq)
		}
	}
}

func TestSplitChineseText(t *testing.T) {
	text := strings.Repeat("这是一段中文测试。", 200)
	s := New()
	chunks, err := s.Split(context.Background(), text, ports.SplitOptions{ChunkSize: 400, Overlap: 50})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("want >= 2 chunks, got %d", len(chunks))
	}
}
```

- [x] **Step 7.4：跑测试**

```powershell
cd e:\GoProject\it-wiki\backend
go test ./internal/infra/splitter/... -v
```

期望：3 个测试 PASS。tiktoken 首次 `GetEncoding` 会下载词表（联网）；离线环境设 `TIKTOKEN_CACHE_DIR`。

- [x] **Step 7.5：完成检查**

勾选 7.1~7.5。

---

## 任务 8：Embedder（OpenAI 兼容协议）

**Files:**
- Create: `e:\GoProject\it-wiki\backend\internal\infra\embedder\openai_compat.go`

- [x] **Step 8.1：写 `embedder/openai_compat.go`**

```go
package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type OpenAICompat struct {
	baseURL string
	apiKey  string
	model   string
	dim     int
	client  *http.Client
}

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
	Dim     int
}

func New(cfg Config) *OpenAICompat {
	return &OpenAICompat{
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		dim:     cfg.Dim,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (e *OpenAICompat) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	body, err := json.Marshal(embedRequest{Model: e.model, Input: texts})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding api %d: %s", resp.StatusCode, string(respBody))
	}

	var out embedResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("embedding api error: %s", out.Error.Message)
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("embedding count mismatch: got %d, want %d", len(out.Data), len(texts))
	}

	result := make([][]float32, len(texts))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(texts) {
			return nil, fmt.Errorf("invalid index %d", d.Index)
		}
		if len(d.Embedding) != e.dim {
			return nil, fmt.Errorf("embedding dim mismatch: got %d, want %d", len(d.Embedding), e.dim)
		}
		result[d.Index] = d.Embedding
	}
	return result, nil
}

func (e *OpenAICompat) Dim() int      { return e.dim }
func (e *OpenAICompat) Model() string { return e.model }
```

- [x] **Step 8.2：编译验证**

```powershell
cd e:\GoProject\it-wiki\backend
go build ./...
```

期望：无报错。集成验证留到任务 17。

- [x] **Step 8.3：完成检查**

勾选 8.1~8.3。

---

## 任务 9：VectorStore (pgvector via pgx)

**Files:**
- Create: `e:\GoProject\it-wiki\backend\internal\infra\vectorstore\pgvector.go`

- [x] **Step 9.1：写 `vectorstore/pgvector.go`**

```go
package vectorstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

type Pgvector struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Pgvector {
	return &Pgvector{pool: pool}
}

func (v *Pgvector) InsertChunks(ctx context.Context, items []domain.ChunkWithEmbedding) error {
	if len(items) == 0 {
		return nil
	}

	tx, err := v.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	batch := &pgx.Batch{}
	for _, it := range items {
		metaJSON, err := json.Marshal(it.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
		batch.Queue(
			`INSERT INTO chunks (kb_id, document_id, seq, content, token_count, embedding, metadata)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			it.KBID, it.DocumentID, it.Seq, it.Content, it.TokenCount,
			pgvector.NewVector(it.Embedding), metaJSON,
		)
	}

	br := tx.SendBatch(ctx, batch)
	for i := 0; i < len(items); i++ {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return fmt.Errorf("insert chunk %d: %w", i, err)
		}
	}
	if err := br.Close(); err != nil {
		return fmt.Errorf("close batch: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// ListByDocument 不在 ports.VectorStore 接口中（Phase 1 chunk 查询直接调）。
// 返回的 Chunk 不带 embedding。
func (v *Pgvector) ListByDocument(ctx context.Context, docID string, limit, offset int) ([]domain.Chunk, int, error) {
	var total int
	if err := v.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM chunks WHERE document_id = $1`, docID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count chunks: %w", err)
	}

	rows, err := v.pool.Query(ctx,
		`SELECT id, kb_id, document_id, seq, content, token_count, metadata, created_at
		 FROM chunks WHERE document_id = $1
		 ORDER BY seq ASC LIMIT $2 OFFSET $3`,
		docID, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query chunks: %w", err)
	}
	defer rows.Close()

	var out []domain.Chunk
	for rows.Next() {
		var c domain.Chunk
		var metaJSON []byte
		if err := rows.Scan(&c.ID, &c.KBID, &c.DocumentID, &c.Seq, &c.Content, &c.TokenCount, &metaJSON, &c.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan: %w", err)
		}
		if len(metaJSON) > 0 {
			_ = json.Unmarshal(metaJSON, &c.Metadata)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}
```

- [x] **Step 9.2：编译验证**

```powershell
go build ./...
```

期望：无报错。集成验证留到任务 17。

- [x] **Step 9.3：完成检查**

勾选 9.1~9.3。

---

## 任务 10：Object storage 扩展（Put / Get / Delete）

**Files:**
- Modify: `e:\GoProject\it-wiki\backend\internal\infra\storage\minio.go`
- Create: `e:\GoProject\it-wiki\backend\internal\infra\checksum\sha256.go`

- [x] **Step 10.1：扩展 `storage/minio.go`**

在现有 `MinioClient` 类型上加方法（追加在文件末尾，**不动 EnsureBucket / NewMinioClient**）：

```go
import (
	// 已有 import 保持不变, 在底部加：
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Put 上传对象。size 必填（MinIO 流式上传需要 Content-Length）。
func (m *MinioClient) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	_, err := m.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(m.cfg.Bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	return nil
}

func (m *MinioClient) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := m.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(m.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get %s: %w", key, err)
	}
	return out.Body, nil
}

func (m *MinioClient) Delete(ctx context.Context, key string) error {
	_, err := m.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(m.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3 delete %s: %w", key, err)
	}
	return nil
}
```

> ⚠️ 上面的 import 块只是说明加哪些新 import；实际 Edit 时把 import 合并到现有 import 块里，避免重复。

- [x] **Step 10.2：写 `checksum/sha256.go`**

```go
package checksum

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
)

// SHA256 计算 reader 全量字节的 sha256, 返回 "sha256:" + hex。
func SHA256(r io.Reader) (string, int64, error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return "", 0, err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), n, nil
}
```

- [x] **Step 10.3：写 `checksum/sha256_test.go`**

```go
package checksum

import (
	"strings"
	"testing"
)

func TestSHA256(t *testing.T) {
	sum, size, err := SHA256(strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("sha256: %v", err)
	}
	want := "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if sum != want {
		t.Errorf("sum = %q, want %q", sum, want)
	}
	if size != 5 {
		t.Errorf("size = %d, want 5", size)
	}
}
```

- [x] **Step 10.4：跑测试**

```powershell
go test ./internal/infra/checksum/... -v
```

期望：PASS。

- [x] **Step 10.5：编译验证**

```powershell
go build ./...
```

期望：无报错。

- [x] **Step 10.6：完成检查**

勾选 10.1~10.6。

---

## 任务 11：Ingestion service（上传 + 入队）

**Files:**
- Create: `e:\GoProject\it-wiki\backend\internal\service\ingestion_service.go`

> 这一服务**只**负责"接收上传 → 落对象存储 → INSERT documents → 入队"，**不**做解析（解析在 worker 里）。

- [x] **Step 11.1：写 `ingestion_service.go`**

```go
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/checksum"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

// ErrDuplicateChecksum 表示同 KB 已有相同 sha256 的文档。
type ErrDuplicateChecksum struct {
	ExistingDocID string
}

func (e *ErrDuplicateChecksum) Error() string {
	return "document with same checksum already exists: " + e.ExistingDocID
}

// JobEnqueuer 抽象 river client（避免直接 import river 到 service 层）。
type JobEnqueuer interface {
	EnqueueIngestion(ctx context.Context, docID string) error
}

type Ingestion struct {
	queries  *generated.Queries
	storage  ports.ObjectStorage
	enqueuer JobEnqueuer
}

func NewIngestion(q *generated.Queries, storage ports.ObjectStorage, enqueuer JobEnqueuer) *Ingestion {
	return &Ingestion{queries: q, storage: storage, enqueuer: enqueuer}
}

type UploadInput struct {
	KBID     string
	Title    string
	MimeType string
	Body     io.Reader
	Size     int64
}

// Upload 完成: 读 body → sha256 → 查重 → 落 storage → INSERT documents → 入队。
func (s *Ingestion) Upload(ctx context.Context, in UploadInput) (*domain.Document, error) {
	// 读全量到内存（spec MVP 限 50MB, 内存可承受）
	buf, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if int64(len(buf)) != in.Size && in.Size > 0 {
		// multipart header 给的 size 可能不准, 以实际为准
		in.Size = int64(len(buf))
	}

	// 算 sha256
	sum, _, err := checksum.SHA256(bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}

	// 查重
	kbUUID, err := uuid.Parse(in.KBID)
	if err != nil {
		return nil, fmt.Errorf("invalid kb id: %w", err)
	}
	existing, err := s.queries.FindDocumentByChecksum(ctx, generated.FindDocumentByChecksumParams{
		KbID:     kbUUID,
		Checksum: sum,
	})
	if err == nil {
		return nil, &ErrDuplicateChecksum{ExistingDocID: existing.ID.String()}
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("find by checksum: %w", err)
	}

	// 生成 doc id（提前用, 决定 storage key）
	docID := uuid.New()
	storageKey := fmt.Sprintf("%s/%s/%s", in.KBID, docID.String(), in.Title)

	// 落 storage
	if err := s.storage.Put(ctx, storageKey, bytes.NewReader(buf), in.Size, in.MimeType); err != nil {
		return nil, fmt.Errorf("storage put: %w", err)
	}

	// INSERT documents (status='pending')
	row, err := s.queries.CreateDocument(ctx, generated.CreateDocumentParams{
		KbID:       kbUUID,
		SourceType: "local-upload",
		SourceRef:  storageKey,
		Title:      in.Title,
		MimeType:   in.MimeType,
		Bytes:      in.Size,
		Checksum:   sum,
		Status:     string(domain.StatusPending),
		Metadata:   []byte("{}"),
	})
	if err != nil {
		// 回滚 storage
		_ = s.storage.Delete(ctx, storageKey)
		return nil, fmt.Errorf("create document: %w", err)
	}

	// 入队
	if err := s.enqueuer.EnqueueIngestion(ctx, row.ID.String()); err != nil {
		// 入队失败 → 标 failed 但不删 doc（让用户能看到）
		msg := err.Error()
		_ = s.queries.UpdateDocumentStatus(ctx, generated.UpdateDocumentStatusParams{
			ID:           row.ID,
			Status:       string(domain.StatusFailed),
			ErrorMessage: &msg,
		})
		return nil, fmt.Errorf("enqueue: %w", err)
	}

	return rowToDocFull(row), nil
}

// rowToDocFull 把 sqlc 生成的 row 转 domain.Document。
// 任务 15 的 document_service.go 也复用此函数 (同 service 包)，不要重复定义。
func rowToDocFull(r generated.Document) *domain.Document {
	doc := &domain.Document{
		ID:         r.ID.String(),
		KBID:       r.KbID.String(),
		SourceType: r.SourceType,
		SourceRef:  r.SourceRef,
		Title:      r.Title,
		MimeType:   r.MimeType,
		Bytes:      r.Bytes,
		Checksum:   r.Checksum,
		Status:     domain.DocStatus(r.Status),
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
	}
	if r.ErrorMessage != nil {
		doc.ErrorMessage = r.ErrorMessage
	}
	doc.Metadata = map[string]any{}
	if len(r.Metadata) > 0 {
		_ = json.Unmarshal(r.Metadata, &doc.Metadata)
	}
	return doc
}
```

> ⚠️ `generated.Document` 字段名取决于 sqlc 输出（驼峰命名、`KbID` 还是 `KBID`）。如果 sqlc 实际生成的字段名与上不同，按生成结果调整。`sqlc generate` 默认对 `kb_id` 列生成 `KbID`，对 `created_at` 生成 `CreatedAt`。

- [x] **Step 11.2：编译验证**

```powershell
go build ./...
```

期望：报错应该都在引用 `JobEnqueuer` 但还没实现这一点上——没关系，service 编译需要 worker 包提供实现，任务 12 处理。如果只是因为这点报错，先注释掉 main.go 中相关引用，让 service 包能独立 build。

可以用 `go build ./internal/service/...` 验证 service 包本身编译通过。

- [x] **Step 11.3：完成检查**

勾选 11.1~11.3。

---

## 任务 12：river Worker + Client setup

**Files:**
- Create: `e:\GoProject\it-wiki\backend\internal\worker\ingestion_job.go`
- Create: `e:\GoProject\it-wiki\backend\internal\worker\ingestion_worker.go`
- Create: `e:\GoProject\it-wiki\backend\internal\worker\client.go`

- [x] **Step 12.1：写 `worker/ingestion_job.go`**

```go
package worker

import "github.com/riverqueue/river"

type IngestionJobArgs struct {
	DocumentID string `json:"document_id"`
}

func (IngestionJobArgs) Kind() string { return "ingestion" }

// InsertOpts: 失败不重试（spec §5.1）。
func (IngestionJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 1,
	}
}
```

- [x] **Step 12.2：写 `worker/ingestion_worker.go`**

```go
package worker

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type IngestionWorker struct {
	river.WorkerDefaults[IngestionJobArgs]

	pool      *pgxpool.Pool
	queries   *generated.Queries
	storage   ports.ObjectStorage
	parser    ports.Parser // dispatcher
	splitter  ports.Splitter
	embedder  ports.Embedder
	vstore    ports.VectorStore
	chunkSize int
	overlap   int
	batchSize int
}

type WorkerDeps struct {
	Pool      *pgxpool.Pool
	Queries   *generated.Queries
	Storage   ports.ObjectStorage
	Parser    ports.Parser
	Splitter  ports.Splitter
	Embedder  ports.Embedder
	VStore    ports.VectorStore
	ChunkSize int
	Overlap   int
	BatchSize int
}

func NewIngestionWorker(d WorkerDeps) *IngestionWorker {
	return &IngestionWorker{
		pool: d.Pool, queries: d.Queries, storage: d.Storage,
		parser: d.Parser, splitter: d.Splitter, embedder: d.Embedder, vstore: d.VStore,
		chunkSize: d.ChunkSize, overlap: d.Overlap, batchSize: d.BatchSize,
	}
}

func (w *IngestionWorker) Work(ctx context.Context, job *river.Job[IngestionJobArgs]) error {
	docID, err := uuid.Parse(job.Args.DocumentID)
	if err != nil {
		return fmt.Errorf("invalid doc id: %w", err)
	}

	// 取 doc 元数据
	doc, err := w.queries.GetDocument(ctx, docID)
	if err != nil {
		return fmt.Errorf("get document: %w", err)
	}

	// 失败兜底：任何步骤报错 → 标 failed + error_message
	failed := func(stepErr error) error {
		msg := stepErr.Error()
		_ = w.queries.UpdateDocumentStatus(ctx, generated.UpdateDocumentStatusParams{
			ID: docID, Status: string(domain.StatusFailed), ErrorMessage: &msg,
		})
		log.Printf("[worker] doc %s failed: %v", docID, stepErr)
		return stepErr
	}

	// 1. parsing
	if err := w.setStatus(ctx, docID, domain.StatusParsing); err != nil {
		return failed(err)
	}
	reader, err := w.storage.Get(ctx, doc.SourceRef)
	if err != nil {
		return failed(fmt.Errorf("storage get: %w", err))
	}
	defer reader.Close()

	parsed, err := w.parser.Parse(ctx, reader, doc.MimeType)
	if err != nil {
		return failed(fmt.Errorf("parse: %w", err))
	}
	if parsed.Text == "" {
		return failed(fmt.Errorf("parser produced empty text"))
	}

	// 2. chunking
	if err := w.setStatus(ctx, docID, domain.StatusChunking); err != nil {
		return failed(err)
	}
	splitChunks, err := w.splitter.Split(ctx, parsed.Text, ports.SplitOptions{
		ChunkSize: w.chunkSize, Overlap: w.overlap,
	})
	if err != nil {
		return failed(fmt.Errorf("split: %w", err))
	}
	if len(splitChunks) == 0 {
		return failed(fmt.Errorf("splitter produced 0 chunks"))
	}

	// 3. embedding (batch)
	if err := w.setStatus(ctx, docID, domain.StatusEmbedding); err != nil {
		return failed(err)
	}
	allEmbeddings := make([][]float32, 0, len(splitChunks))
	for i := 0; i < len(splitChunks); i += w.batchSize {
		end := i + w.batchSize
		if end > len(splitChunks) {
			end = len(splitChunks)
		}
		texts := make([]string, 0, end-i)
		for _, c := range splitChunks[i:end] {
			texts = append(texts, c.Content)
		}
		embs, err := w.embedder.Embed(ctx, texts)
		if err != nil {
			return failed(fmt.Errorf("embed batch %d-%d: %w", i, end, err))
		}
		allEmbeddings = append(allEmbeddings, embs...)
	}
	if len(allEmbeddings) != len(splitChunks) {
		return failed(fmt.Errorf("embedding count mismatch: %d vs %d", len(allEmbeddings), len(splitChunks)))
	}

	// 4. insert chunks (single transaction)
	items := make([]domain.ChunkWithEmbedding, 0, len(splitChunks))
	for i, c := range splitChunks {
		items = append(items, domain.ChunkWithEmbedding{
			Chunk: domain.Chunk{
				KBID:       doc.KbID.String(),
				DocumentID: docID.String(),
				Seq:        c.Seq,
				Content:    c.Content,
				TokenCount: c.TokenCount,
				Metadata:   map[string]any{},
			},
			Embedding: allEmbeddings[i],
		})
	}
	if err := w.vstore.InsertChunks(ctx, items); err != nil {
		return failed(fmt.Errorf("insert chunks: %w", err))
	}

	// 5. ready
	if err := w.setStatus(ctx, docID, domain.StatusReady); err != nil {
		return failed(err)
	}
	log.Printf("[worker] doc %s ready (%d chunks)", docID, len(items))
	return nil
}

func (w *IngestionWorker) setStatus(ctx context.Context, id uuid.UUID, status domain.DocStatus) error {
	return w.queries.UpdateDocumentStatus(ctx, generated.UpdateDocumentStatusParams{
		ID:     id,
		Status: string(status),
	})
}
```

- [x] **Step 12.3：写 `worker/client.go`**

```go
package worker

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

type Client struct {
	rc *river.Client[pgxpool.Pool]
}

func NewClient(ctx context.Context, pool *pgxpool.Pool, w *IngestionWorker, maxWorkers int) (*Client, error) {
	workers := river.NewWorkers()
	if err := river.AddWorkerSafely(workers, w); err != nil {
		return nil, fmt.Errorf("add worker: %w", err)
	}

	rc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: maxWorkers},
		},
		Workers: workers,
	})
	if err != nil {
		return nil, fmt.Errorf("new river client: %w", err)
	}

	return &Client{rc: rc}, nil
}

func (c *Client) Start(ctx context.Context) error {
	return c.rc.Start(ctx)
}

func (c *Client) Stop(ctx context.Context) error {
	return c.rc.Stop(ctx)
}

// EnqueueIngestion 实现 service.JobEnqueuer 接口。
func (c *Client) EnqueueIngestion(ctx context.Context, docID string) error {
	_, err := c.rc.Insert(ctx, IngestionJobArgs{DocumentID: docID}, nil)
	if err != nil {
		return fmt.Errorf("insert ingestion job: %w", err)
	}
	return nil
}
```

- [x] **Step 12.4：编译验证**

```powershell
go build ./...
```

期望：worker 包编译通过。如果 river 类型签名报错（不同版本 API 差异），查 `https://github.com/riverqueue/river` 当前 README 调整。

- [x] **Step 12.5：完成检查**

勾选 12.1~12.5。

---

## 任务 13：HTTP 基础设施（errors / pagination / CORS）+ main.go 启动编排

**Files:**
- Create: `e:\GoProject\it-wiki\backend\internal\http\errors.go`
- Create: `e:\GoProject\it-wiki\backend\internal\http\pagination.go`
- Create: `e:\GoProject\it-wiki\backend\internal\http\cors.go`
- Modify: `e:\GoProject\it-wiki\backend\internal\http\router.go`
- Modify: `e:\GoProject\it-wiki\backend\cmd\server\main.go`

- [x] **Step 13.1：写 `http/errors.go`**

```go
package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

const (
	CodeValidationFailed     = "validation_failed"
	CodeKBNotFound           = "kb_not_found"
	CodeDocNotFound          = "doc_not_found"
	CodeDuplicateChecksum    = "duplicate_checksum"
	CodePayloadTooLarge      = "payload_too_large"
	CodeUnsupportedMediaType = "unsupported_media_type"
	CodeInternalError        = "internal_error"
)

type APIError struct {
	HTTPStatus int            `json:"-"`
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	Details    map[string]any `json:"details,omitempty"`
}

func (e *APIError) Error() string { return e.Message }

func NewAPIError(status int, code, message string) *APIError {
	return &APIError{HTTPStatus: status, Code: code, Message: message}
}

func WithDetails(e *APIError, details map[string]any) *APIError {
	e.Details = details
	return e
}

type errorEnvelope struct {
	Error envelopeBody `json:"error"`
}

type envelopeBody struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	RequestID string         `json:"request_id"`
	Details   map[string]any `json:"details,omitempty"`
}

func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	apiErr, ok := err.(*APIError)
	if !ok {
		apiErr = NewAPIError(http.StatusInternalServerError, CodeInternalError, err.Error())
	}
	reqID := middleware.GetReqID(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-Id", reqID)
	w.WriteHeader(apiErr.HTTPStatus)
	_ = json.NewEncoder(w).Encode(errorEnvelope{
		Error: envelopeBody{
			Code:      apiErr.Code,
			Message:   apiErr.Message,
			RequestID: reqID,
			Details:   apiErr.Details,
		},
	})
}

// WriteJSON 写成功响应。
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
```

- [x] **Step 13.2：写 `http/pagination.go`**

```go
package http

import (
	"net/http"
	"strconv"
)

type Pagination struct {
	Limit  int
	Offset int
}

const (
	defaultLimit = 20
	maxLimit     = 100
)

// ParsePagination 从 querystring 读 limit/offset, 校验并返回。
// 错误时返回 APIError(400, validation_failed)。
func ParsePagination(r *http.Request) (Pagination, error) {
	p := Pagination{Limit: defaultLimit, Offset: 0}

	if s := r.URL.Query().Get("limit"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 || v > maxLimit {
			return p, NewAPIError(http.StatusBadRequest, CodeValidationFailed,
				"limit must be int in [1,"+strconv.Itoa(maxLimit)+"]")
		}
		p.Limit = v
	}
	if s := r.URL.Query().Get("offset"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 0 {
			return p, NewAPIError(http.StatusBadRequest, CodeValidationFailed,
				"offset must be non-negative int")
		}
		p.Offset = v
	}
	return p, nil
}

// WriteListResponse 列表响应：写 X-Total-Count 并 JSON 序列化 items。
func WriteListResponse(w http.ResponseWriter, total int, items any) {
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	WriteJSON(w, http.StatusOK, items)
}
```

- [x] **Step 13.3：写 `http/cors.go`**

```go
package http

import "net/http"

// CORS Phase 1 用最宽松策略, 后期收紧。
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Expose-Headers", "X-Total-Count, X-Request-Id")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

- [x] **Step 13.4：扩展 `http/router.go` 加 Router 构造签名（暂不挂业务路由，任务 14-16 加）**

替换整个文件：

```go
package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handlers struct {
	KB    *KBHandler
	Doc   *DocumentHandler
	Chunk *ChunkHandler
}

func NewRouter(h Handlers) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(CORS)

	r.Get("/healthz", healthz)
	r.Get("/api/healthz", healthz)

	r.Route("/api/v1", func(r chi.Router) {
		// KB
		r.Post("/kbs", h.KB.Create)
		r.Get("/kbs", h.KB.List)
		r.Get("/kbs/{id}", h.KB.Get)
		r.Delete("/kbs/{id}", h.KB.Delete)

		// Docs
		r.Post("/kbs/{id}/docs", h.Doc.Upload)
		r.Get("/kbs/{id}/docs", h.Doc.ListByKB)
		r.Get("/docs/{id}", h.Doc.Get)
		r.Delete("/docs/{id}", h.Doc.Delete)

		// Chunks
		r.Get("/docs/{id}/chunks", h.Chunk.ListByDoc)
	})

	return r
}

func healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}
```

> 注意：这一步引用了 `KBHandler` / `DocumentHandler` / `ChunkHandler` 但都还没定义，编译会报错。任务 14-16 完成后才能编译通过。**暂时跳过 main.go 改动到任务 16 之后**。

- [x] **Step 13.5：完成检查（先勾 13.1-13.4，main.go 改在任务 16 后）**

勾选 13.1~13.4。**Step 13.5 之后的 main.go 编排步骤放到任务 16 末尾**（任务 16 完成后所有 handler 类型就位，再统一改 main.go）。

---

## 任务 14：KB service + handler

**Files:**
- Create: `e:\GoProject\it-wiki\backend\internal\service\kb_service.go`
- Create: `e:\GoProject\it-wiki\backend\internal\http\kb_handler.go`

- [x] **Step 14.1：写 `service/kb_service.go`**

```go
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type ErrKBNotFound struct{ ID string }

func (e *ErrKBNotFound) Error() string { return "knowledge base not found: " + e.ID }

type KB struct {
	queries    *generated.Queries
	embedModel string
	embedDim   int
}

func NewKB(q *generated.Queries, embedModel string, embedDim int) *KB {
	return &KB{queries: q, embedModel: embedModel, embedDim: embedDim}
}

type CreateKBInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *KB) Create(ctx context.Context, in CreateKBInput) (*domain.KnowledgeBase, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	row, err := s.queries.CreateKnowledgeBase(ctx, generated.CreateKnowledgeBaseParams{
		Name:        in.Name,
		Description: in.Description,
		EmbedModel:  s.embedModel,
		EmbedDim:    int32(s.embedDim),
		Settings:    []byte("{}"),
	})
	if err != nil {
		return nil, fmt.Errorf("create kb: %w", err)
	}
	return rowToKB(row), nil
}

func (s *KB) Get(ctx context.Context, id string) (*domain.KnowledgeBase, error) {
	u, err := uuid.Parse(id)
	if err != nil {
		return nil, &ErrKBNotFound{ID: id}
	}
	row, err := s.queries.GetKnowledgeBase(ctx, u)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ErrKBNotFound{ID: id}
		}
		return nil, fmt.Errorf("get kb: %w", err)
	}
	return rowToKB(row), nil
}

func (s *KB) List(ctx context.Context, limit, offset int) ([]*domain.KnowledgeBase, int, error) {
	rows, err := s.queries.ListKnowledgeBases(ctx, generated.ListKnowledgeBasesParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list kbs: %w", err)
	}
	total, err := s.queries.CountKnowledgeBases(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count kbs: %w", err)
	}
	out := make([]*domain.KnowledgeBase, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToKB(r))
	}
	return out, int(total), nil
}

func (s *KB) Delete(ctx context.Context, id string) error {
	u, err := uuid.Parse(id)
	if err != nil {
		return &ErrKBNotFound{ID: id}
	}
	// 先确认存在
	if _, err := s.queries.GetKnowledgeBase(ctx, u); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &ErrKBNotFound{ID: id}
		}
		return err
	}
	return s.queries.DeleteKnowledgeBase(ctx, u)
}

func rowToKB(r generated.KnowledgeBase) *domain.KnowledgeBase {
	kb := &domain.KnowledgeBase{
		ID:          r.ID.String(),
		Name:        r.Name,
		Description: r.Description,
		OwnerID:     r.OwnerID,
		EmbedModel:  r.EmbedModel,
		EmbedDim:    int(r.EmbedDim),
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
	kb.Settings = map[string]any{}
	if len(r.Settings) > 0 {
		_ = json.Unmarshal(r.Settings, &kb.Settings)
	}
	return kb
}
```

- [x] **Step 14.2：写 `http/kb_handler.go`**

```go
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

type KBHandler struct {
	svc *service.KB
}

func NewKBHandler(svc *service.KB) *KBHandler {
	return &KBHandler{svc: svc}
}

func (h *KBHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in service.CreateKBInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		WriteError(w, r, NewAPIError(http.StatusBadRequest, CodeValidationFailed, "invalid json body"))
		return
	}
	if in.Name == "" {
		WriteError(w, r, NewAPIError(http.StatusBadRequest, CodeValidationFailed, "name is required"))
		return
	}
	kb, err := h.svc.Create(r.Context(), in)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusCreated, kb)
}

func (h *KBHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	kb, err := h.svc.Get(r.Context(), id)
	if err != nil {
		var notFound *service.ErrKBNotFound
		if errors.As(err, &notFound) {
			WriteError(w, r, NewAPIError(http.StatusNotFound, CodeKBNotFound, err.Error()))
			return
		}
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, kb)
}

func (h *KBHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := ParsePagination(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	kbs, total, err := h.svc.List(r.Context(), p.Limit, p.Offset)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteListResponse(w, total, kbs)
}

func (h *KBHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.svc.Delete(r.Context(), id); err != nil {
		var notFound *service.ErrKBNotFound
		if errors.As(err, &notFound) {
			WriteError(w, r, NewAPIError(http.StatusNotFound, CodeKBNotFound, err.Error()))
			return
		}
		WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [x] **Step 14.3：编译验证**

```powershell
go build ./...
```

期望：当前还会缺 `DocumentHandler`、`ChunkHandler`。先 `go build ./internal/service/... ./internal/http/...` 单独 build 至少 KB 部分通过。

- [x] **Step 14.4：完成检查**

勾选 14.1~14.4。

---

## 任务 15：Document service + handler（含 multipart upload）

**Files:**
- Create: `e:\GoProject\it-wiki\backend\internal\service\document_service.go`
- Create: `e:\GoProject\it-wiki\backend\internal\http\document_handler.go`

- [x] **Step 15.1：写 `service/document_service.go`**

```go
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type ErrDocNotFound struct{ ID string }

func (e *ErrDocNotFound) Error() string { return "document not found: " + e.ID }

type Document struct {
	queries *generated.Queries
}

func NewDocument(q *generated.Queries) *Document {
	return &Document{queries: q}
}

func (s *Document) Get(ctx context.Context, id string) (*domain.Document, error) {
	u, err := uuid.Parse(id)
	if err != nil {
		return nil, &ErrDocNotFound{ID: id}
	}
	row, err := s.queries.GetDocument(ctx, u)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ErrDocNotFound{ID: id}
		}
		return nil, fmt.Errorf("get document: %w", err)
	}
	return rowToDocFull(row), nil
}

func (s *Document) ListByKB(ctx context.Context, kbID string, statusFilter *string, limit, offset int) ([]*domain.Document, int, error) {
	u, err := uuid.Parse(kbID)
	if err != nil {
		return nil, 0, &ErrKBNotFound{ID: kbID}
	}

	var statusParam *string
	if statusFilter != nil {
		s := *statusFilter
		statusParam = &s
	}

	rows, err := s.queries.ListDocumentsByKB(ctx, generated.ListDocumentsByKBParams{
		KbID:   u,
		Status: statusParam,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list docs: %w", err)
	}
	total, err := s.queries.CountDocumentsByKB(ctx, generated.CountDocumentsByKBParams{
		KbID:   u,
		Status: statusParam,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count docs: %w", err)
	}

	out := make([]*domain.Document, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToDocFull(r))
	}
	return out, int(total), nil
}

func (s *Document) Delete(ctx context.Context, id string) error {
	u, err := uuid.Parse(id)
	if err != nil {
		return &ErrDocNotFound{ID: id}
	}
	if _, err := s.queries.GetDocument(ctx, u); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &ErrDocNotFound{ID: id}
		}
		return err
	}
	return s.queries.DeleteDocument(ctx, u)
}

// rowToDocFull 已在 ingestion_service.go (任务 11) 定义，本文件直接复用。
```

- [x] **Step 15.2：写 `http/document_handler.go`**

```go
package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

type DocumentHandler struct {
	docSvc       *service.Document
	ingestionSvc *service.Ingestion
	maxUpload    int64
}

func NewDocumentHandler(docSvc *service.Document, ingestionSvc *service.Ingestion, maxUploadBytes int64) *DocumentHandler {
	return &DocumentHandler{
		docSvc: docSvc, ingestionSvc: ingestionSvc, maxUpload: maxUploadBytes,
	}
}

func (h *DocumentHandler) Upload(w http.ResponseWriter, r *http.Request) {
	kbID := chi.URLParam(r, "id")

	// 限大小
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUpload)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		// 区分: 超 max bytes 给 413, 其它 400
		if strings.Contains(err.Error(), "http: request body too large") {
			WriteError(w, r, NewAPIError(http.StatusRequestEntityTooLarge, CodePayloadTooLarge,
				"upload exceeds size limit"))
			return
		}
		WriteError(w, r, NewAPIError(http.StatusBadRequest, CodeValidationFailed, err.Error()))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		WriteError(w, r, NewAPIError(http.StatusBadRequest, CodeValidationFailed, "missing form field 'file'"))
		return
	}
	defer file.Close()

	mime := header.Header.Get("Content-Type")
	if mime == "" {
		mime = "application/octet-stream"
	}

	doc, err := h.ingestionSvc.Upload(r.Context(), service.UploadInput{
		KBID:     kbID,
		Title:    header.Filename,
		MimeType: mime,
		Body:     file,
		Size:     header.Size,
	})
	if err != nil {
		var dup *service.ErrDuplicateChecksum
		if errors.As(err, &dup) {
			WriteError(w, r, WithDetails(
				NewAPIError(http.StatusConflict, CodeDuplicateChecksum,
					"document with same content already exists in this KB"),
				map[string]any{"existing_doc_id": dup.ExistingDocID},
			))
			return
		}
		var kbnotfound *service.ErrKBNotFound
		if errors.As(err, &kbnotfound) {
			WriteError(w, r, NewAPIError(http.StatusNotFound, CodeKBNotFound, err.Error()))
			return
		}
		WriteError(w, r, err)
		return
	}

	WriteJSON(w, http.StatusCreated, doc)
}

func (h *DocumentHandler) ListByKB(w http.ResponseWriter, r *http.Request) {
	kbID := chi.URLParam(r, "id")
	p, err := ParsePagination(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	var statusFilter *string
	if s := r.URL.Query().Get("status"); s != "" {
		statusFilter = &s
	}
	docs, total, err := h.docSvc.ListByKB(r.Context(), kbID, statusFilter, p.Limit, p.Offset)
	if err != nil {
		var kbNF *service.ErrKBNotFound
		if errors.As(err, &kbNF) {
			WriteError(w, r, NewAPIError(http.StatusNotFound, CodeKBNotFound, err.Error()))
			return
		}
		WriteError(w, r, err)
		return
	}
	WriteListResponse(w, total, docs)
}

func (h *DocumentHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	doc, err := h.docSvc.Get(r.Context(), id)
	if err != nil {
		var notFound *service.ErrDocNotFound
		if errors.As(err, &notFound) {
			WriteError(w, r, NewAPIError(http.StatusNotFound, CodeDocNotFound, err.Error()))
			return
		}
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, doc)
}

func (h *DocumentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.docSvc.Delete(r.Context(), id); err != nil {
		var notFound *service.ErrDocNotFound
		if errors.As(err, &notFound) {
			WriteError(w, r, NewAPIError(http.StatusNotFound, CodeDocNotFound, err.Error()))
			return
		}
		WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [x] **Step 15.3：完成检查**

勾选 15.1~15.3。

---

## 任务 16：Chunk handler + main.go 启动编排

**Files:**
- Create: `e:\GoProject\it-wiki\backend\internal\http\chunk_handler.go`
- Modify: `e:\GoProject\it-wiki\backend\cmd\server\main.go`

- [x] **Step 16.1：写 `http/chunk_handler.go`**

```go
package http

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/zenith-wang/it-wiki/backend/internal/infra/vectorstore"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

type ChunkHandler struct {
	vstore *vectorstore.Pgvector
	docSvc *service.Document
}

func NewChunkHandler(vstore *vectorstore.Pgvector, docSvc *service.Document) *ChunkHandler {
	return &ChunkHandler{vstore: vstore, docSvc: docSvc}
}

func (h *ChunkHandler) ListByDoc(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "id")

	// 先确认 doc 存在（避免暴露 chunks 任意查询）
	if _, err := h.docSvc.Get(r.Context(), docID); err != nil {
		var notFound *service.ErrDocNotFound
		if errors.As(err, &notFound) {
			WriteError(w, r, NewAPIError(http.StatusNotFound, CodeDocNotFound, err.Error()))
			return
		}
		WriteError(w, r, err)
		return
	}

	p, err := ParsePagination(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	chunks, total, err := h.vstore.ListByDocument(r.Context(), docID, p.Limit, p.Offset)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteListResponse(w, total, chunks)
}
```

- [x] **Step 16.2：改 `cmd/server/main.go` 整合所有启动步骤**

完整替换：

```go
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	stdhttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/zenith-wang/it-wiki/backend/internal/config"
	httpx "github.com/zenith-wang/it-wiki/backend/internal/http"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/embedder"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/parser"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/splitter"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/storage"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/tokenizer"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/vectorstore"
	"github.com/zenith-wang/it-wiki/backend/internal/repo"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/migrations"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
	"github.com/zenith-wang/it-wiki/backend/internal/worker"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "[fatal] %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	bootCtx, bootCancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer bootCancel()

	// 1) goose business migrations (lib API; supports Go migrations)
	if err := runGooseMigrations(bootCtx, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("goose migrations: %w", err)
	}

	// 2) pgxpool
	pool, err := repo.NewPool(bootCtx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("pgx pool: %w", err)
	}
	defer pool.Close()

	// 3) river migrations
	rmgr, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("rivermigrate new: %w", err)
	}
	if _, err := rmgr.Migrate(bootCtx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("rivermigrate up: %w", err)
	}

	// 4) chunks.embedding dim check
	if err := checkEmbeddingDim(bootCtx, pool, cfg.EmbeddingDim); err != nil {
		return fmt.Errorf("embedding dim check: %w", err)
	}

	// 5) MinIO bucket
	mc, err := storage.NewMinioClient(bootCtx, storage.MinioConfig{
		Endpoint:     cfg.S3Endpoint,
		AccessKey:    cfg.S3AccessKey,
		SecretKey:    cfg.S3SecretKey,
		Region:       cfg.S3Region,
		Bucket:       cfg.S3Bucket,
		UsePathStyle: cfg.S3UsePathStyle,
	})
	if err != nil {
		return fmt.Errorf("minio client: %w", err)
	}
	if err := mc.EnsureBucket(bootCtx); err != nil {
		return fmt.Errorf("ensure bucket: %w", err)
	}

	// 6) tokenizer
	if err := tokenizer.Init(cfg.TokenizerEncoding); err != nil {
		return fmt.Errorf("tokenizer init: %w", err)
	}

	// 7) wire dependencies
	queries := generated.New(pool)
	parserDispatcher := parser.NewDispatcher()
	split := splitter.New()
	embed := embedder.New(embedder.Config{
		BaseURL: cfg.EmbeddingBaseURL,
		APIKey:  cfg.EmbeddingAPIKey,
		Model:   cfg.EmbeddingModel,
		Dim:     cfg.EmbeddingDim,
	})
	vstore := vectorstore.New(pool)

	// 8) river worker
	ingestionWorker := worker.NewIngestionWorker(worker.WorkerDeps{
		Pool: pool, Queries: queries, Storage: mc,
		Parser:   parserDispatcher, // *parser.Dispatcher 满足 ports.Parser 接口
		Splitter: split, Embedder: embed, VStore: vstore,
		ChunkSize: cfg.ChunkSize, Overlap: cfg.ChunkOverlap, BatchSize: cfg.EmbedBatchSize,
	})
	rclient, err := worker.NewClient(bootCtx, pool, ingestionWorker, cfg.RiverMaxWorkers)
	if err != nil {
		return fmt.Errorf("river client: %w", err)
	}

	// 9) services
	kbSvc := service.NewKB(queries, cfg.EmbeddingModel, cfg.EmbeddingDim)
	docSvc := service.NewDocument(queries)
	ingestionSvc := service.NewIngestion(queries, mc, rclient)

	// 10) handlers + router
	router := httpx.NewRouter(httpx.Handlers{
		KB:    httpx.NewKBHandler(kbSvc),
		Doc:   httpx.NewDocumentHandler(docSvc, ingestionSvc, cfg.UploadMaxBytes),
		Chunk: httpx.NewChunkHandler(vstore, docSvc),
	})

	// 11) start river worker pool
	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()
	if err := rclient.Start(runCtx); err != nil {
		return fmt.Errorf("river start: %w", err)
	}

	// 12) http server
	srv := &stdhttp.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		fmt.Printf("[server] listening on :%s\n", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			serveErr <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-stop:
		fmt.Printf("[server] received %v, shutting down\n", sig)
	case err := <-serveErr:
		return fmt.Errorf("http listen: %w", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	_ = srv.Shutdown(shutdownCtx)
	_ = rclient.Stop(shutdownCtx)
	return nil
}

func runGooseMigrations(ctx context.Context, dbURL string) error {
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	goose.SetBaseFS(migrations.EmbedMigrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.UpContext(ctx, db, ".")
}

func checkEmbeddingDim(ctx context.Context, pool interface {
	QueryRow(ctx context.Context, sql string, args ...any) interface{ Scan(dest ...any) error }
}, want int) error {
	// 简化: 直接用 SQL 查 atttypmod, pgvector 的 atttypmod = dim
	var attmod int
	row := pool.QueryRow(ctx, `
		SELECT atttypmod FROM pg_attribute
		 WHERE attrelid = 'chunks'::regclass
		   AND attname  = 'embedding'`)
	if err := row.Scan(&attmod); err != nil {
		return fmt.Errorf("query atttypmod: %w", err)
	}
	if attmod != want {
		return fmt.Errorf("chunks.embedding dim = %d, expected EMBEDDING_DIM = %d (run a new migration to recreate)", attmod, want)
	}
	return nil
}

- [x] **Step 16.3：编译验证**

```powershell
cd e:\GoProject\it-wiki\backend
go build ./...
```

期望：无报错。如果有，按报错信息修正 sqlc 生成字段名差异（`KbID` vs `KBID`，`UpdateDocumentStatus` 的可空字段类型 `*string` vs `pgtype.Text` 等）——这取决于 sqlc 版本，看 `internal/repo/generated/*.go` 实际字段名为准。

- [x] **Step 16.4：完成检查**

勾选 16.1~16.4。

---

## 任务 17：后端端到端集成测试（curl 全流程）

**Files:**
- Create: `e:\GoProject\it-wiki\backend\testdata\sample.md`（测试上传用的小 Markdown）

> 这是 Phase 1 后端的**核心验收门**：在 docker compose 上跑全栈，curl 完成 KB 创建 → 文档上传 → 轮询 ready → 列 chunks。

- [~] **Step 17.1：起 docker compose 全栈**

```powershell
cd e:\GoProject\it-wiki
docker compose -f deploy/docker-compose.yml --env-file .env up -d --build
docker compose -f deploy/docker-compose.yml ps
```

期望所有服务 `running (healthy)`，`minio-init` 是 `exited (0)`。

如果 backend 启动失败，先看日志：

```powershell
docker compose -f deploy/docker-compose.yml logs backend --tail 100
```

常见问题：
- `embedding dim check failed`：`.env` 中 `EMBEDDING_DIM` 与 0004 迁移已存在的列维度不一致——`docker compose down -v` 清掉数据卷重来
- `tiktoken get encoding failed`：容器没有 outbound 网络，预下载词表或允许联网
- `embedding api error`：`.env` 中 `EMBEDDING_API_KEY` / `EMBEDDING_BASE_URL` 没填真实值

- [~] **Step 17.2：准备测试数据 `testdata/sample.md`**

```powershell
New-Item -ItemType Directory -Force "e:\GoProject\it-wiki\backend\testdata" | Out-Null
@'
# RoboSense 工程知识库测试文档

## 项目简介

it-wiki 是一个基于 LLM 的团队知识库系统。它的核心能力包括：

- 多格式文档摄入（Markdown / PDF / DOCX / XLSX）
- 自动切片 + 向量化
- RAG 对话（Phase 2 实现）
- ReAct Agent 模式（Phase 3 实现）

## 技术栈

后端用 Go 1.22 + chi + Eino + sqlc + river + PostgreSQL 16 + pgvector。
前端用 Next.js 15 + Tailwind v4 + shadcn/ui。
对象存储用 MinIO。

## 阶段 1 验收

上传一份 Markdown，30 秒内文档状态变 ready，并能在文档详情页看到切片内容。
本文档本身就是验收用的测试样本。
'@ | Set-Content -Path "e:\GoProject\it-wiki\backend\testdata\sample.md" -Encoding UTF8
```

- [~] **Step 17.3：curl 创建 KB**

```powershell
$kb = curl.exe -s -X POST http://localhost:8080/api/v1/kbs `
  -H "Content-Type: application/json" `
  -d '{"name":"e2e-test","description":"Phase 1 acceptance test"}' | ConvertFrom-Json
$kb
$kbId = $kb.id
Write-Host "KB ID: $kbId"
```

期望：返回 JSON 含 `id`、`name="e2e-test"`、`embed_model`、`embed_dim`、`created_at`。HTTP 201。

- [~] **Step 17.4：curl 上传文档**

```powershell
$doc = curl.exe -s -X POST "http://localhost:8080/api/v1/kbs/$kbId/docs" `
  -F "file=@e:\GoProject\it-wiki\backend\testdata\sample.md;type=text/markdown" | ConvertFrom-Json
$doc
$docId = $doc.id
Write-Host "Doc ID: $docId, Status: $($doc.status)"
```

期望：返回 JSON 含 `id`、`status="pending"`。HTTP 201。

- [~] **Step 17.5：轮询 doc 状态直到 ready 或 failed（最多 60s）**

```powershell
$timeout = 60
$start = Get-Date
while ($true) {
  $d = curl.exe -s "http://localhost:8080/api/v1/docs/$docId" | ConvertFrom-Json
  Write-Host "[$([int]((Get-Date) - $start).TotalSeconds)s] status=$($d.status)"
  if ($d.status -eq "ready" -or $d.status -eq "failed") { break }
  if (((Get-Date) - $start).TotalSeconds -gt $timeout) {
    Write-Host "TIMEOUT"
    break
  }
  Start-Sleep -Seconds 2
}
$d
```

期望：30 秒内输出最终 `status="ready"`。如果 `failed`，查 `$d.error_message` 和 backend 日志。

- [~] **Step 17.6：curl 列 chunks**

```powershell
$chunks = curl.exe -s "http://localhost:8080/api/v1/docs/$docId/chunks?limit=10" | ConvertFrom-Json
Write-Host "Chunks count: $($chunks.Count)"
$chunks | ForEach-Object { Write-Host "seq=$($_.seq), tokens=$($_.token_count), preview=$($_.content.Substring(0, [Math]::Min(50, $_.content.Length)))" }
```

期望：至少 1 个 chunk，每个含 `seq`、`token_count`、`content`、不含 `embedding`。

也检查响应 header `X-Total-Count`：

```powershell
curl.exe -s -I "http://localhost:8080/api/v1/docs/$docId/chunks?limit=10" | Select-String "X-Total-Count"
```

- [~] **Step 17.7：测试重复上传 → 409**

```powershell
$dup = curl.exe -s -w "%{http_code}" -X POST "http://localhost:8080/api/v1/kbs/$kbId/docs" `
  -F "file=@e:\GoProject\it-wiki\backend\testdata\sample.md;type=text/markdown"
Write-Host $dup
```

期望：HTTP 409，response body 含 `"code":"duplicate_checksum"` 和 `"existing_doc_id":"$docId"`。

- [~] **Step 17.8：测试超大文件 → 413（构造 >50MB 假文件）**

```powershell
$big = "e:\GoProject\it-wiki\backend\testdata\big.bin"
$fs = [System.IO.File]::Create($big)
$fs.SetLength(60 * 1024 * 1024)  # 60MB sparse
$fs.Close()
$res = curl.exe -s -w "%{http_code}" -X POST "http://localhost:8080/api/v1/kbs/$kbId/docs" `
  -F "file=@$big" --max-time 30
Write-Host $res
Remove-Item $big
```

期望：HTTP 413，body 含 `"code":"payload_too_large"`。

- [~] **Step 17.9：清理**

```powershell
curl.exe -s -X DELETE "http://localhost:8080/api/v1/docs/$docId" -w "%{http_code}`n"
curl.exe -s -X DELETE "http://localhost:8080/api/v1/kbs/$kbId" -w "%{http_code}`n"
```

期望：两次都 204。

- [~] **Step 17.10：完成检查**

勾选 17.1~17.10。**任务 17.5 必须成功（30s ready）才算后端闭环验收通过。**

---

## 任务 18：前端依赖 + API client + Provider

**Files:**
- Modify: `e:\GoProject\it-wiki\frontend\package.json`
- Create: `e:\GoProject\it-wiki\frontend\components\providers.tsx`
- Modify: `e:\GoProject\it-wiki\frontend\app\layout.tsx`
- Create: `e:\GoProject\it-wiki\frontend\lib\api\client.ts`
- Create: `e:\GoProject\it-wiki\frontend\lib\api\kb.ts`
- Create: `e:\GoProject\it-wiki\frontend\lib\api\docs.ts`
- Create: `e:\GoProject\it-wiki\frontend\lib\api\chunks.ts`
- Create: `e:\GoProject\it-wiki\frontend\lib\schemas\index.ts`

- [~] **Step 18.1：安装依赖**

```powershell
cd e:\GoProject\it-wiki\frontend
pnpm add @tanstack/react-query@^5 zustand@^5 zod@^3 sonner@^1
pnpm add -D @tanstack/react-query-devtools@^5
pnpm dlx shadcn@latest add input label table dialog badge progress sonner
```

> shadcn 现在版本（4.7+）已用 base-nova style，组件代码会用 `@base-ui/react` 而非 Radix——保持现状即可，功能等价。

- [x] **Step 18.2：写 `lib/schemas/index.ts`**（zod schemas 与后端 JSON 对齐）

```ts
import { z } from "zod";

export const kbSchema = z.object({
  id: z.string(),
  name: z.string(),
  description: z.string(),
  owner_id: z.string(),
  embed_model: z.string(),
  embed_dim: z.number(),
  settings: z.record(z.unknown()).default({}),
  created_at: z.string(),
  updated_at: z.string(),
});
export type KB = z.infer<typeof kbSchema>;

export const docStatusEnum = z.enum([
  "pending", "parsing", "chunking", "embedding", "ready", "failed",
]);
export type DocStatus = z.infer<typeof docStatusEnum>;

export const docSchema = z.object({
  id: z.string(),
  kb_id: z.string(),
  source_type: z.string(),
  source_ref: z.string(),
  title: z.string(),
  mime_type: z.string(),
  bytes: z.number(),
  checksum: z.string(),
  status: docStatusEnum,
  error_message: z.string().nullable().optional(),
  metadata: z.record(z.unknown()).default({}),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Doc = z.infer<typeof docSchema>;

export const chunkSchema = z.object({
  id: z.string(),
  kb_id: z.string(),
  document_id: z.string(),
  seq: z.number(),
  content: z.string(),
  token_count: z.number(),
  metadata: z.record(z.unknown()).default({}),
  created_at: z.string(),
});
export type Chunk = z.infer<typeof chunkSchema>;

export const apiErrorSchema = z.object({
  error: z.object({
    code: z.string(),
    message: z.string(),
    request_id: z.string().optional(),
    details: z.record(z.unknown()).optional(),
  }),
});
```

- [x] **Step 18.3：写 `lib/api/client.ts`**（fetch wrapper）

```ts
import { apiErrorSchema } from "@/lib/schemas";

const BASE = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export class APIError extends Error {
  code: string;
  status: number;
  details?: Record<string, unknown>;
  requestId?: string;
  constructor(status: number, code: string, message: string, details?: Record<string, unknown>, requestId?: string) {
    super(message);
    this.code = code;
    this.status = status;
    this.details = details;
    this.requestId = requestId;
  }
}

export interface FetchOptions {
  method?: string;
  body?: BodyInit | null;
  headers?: Record<string, string>;
  // 列表请求返回 { items, total }; 普通请求返回 data
  parseTotal?: boolean;
}

export async function apiFetch<T>(path: string, opts: FetchOptions = {}): Promise<T> {
  const res = await fetch(BASE + path, {
    method: opts.method ?? "GET",
    headers: {
      ...(opts.body && !(opts.body instanceof FormData) ? { "Content-Type": "application/json" } : {}),
      ...(opts.headers ?? {}),
    },
    body: opts.body,
  });

  if (res.status === 204) {
    return undefined as T;
  }

  if (!res.ok) {
    let parsed: { error?: { code: string; message: string; details?: Record<string, unknown>; request_id?: string } } = {};
    try {
      parsed = await res.json();
    } catch {
      // ignore parse failure
    }
    const e = parsed.error ?? { code: "internal_error", message: res.statusText };
    throw new APIError(res.status, e.code, e.message, e.details, e.request_id);
  }

  return (await res.json()) as T;
}

export async function apiFetchList<T>(
  path: string,
  opts: FetchOptions = {},
): Promise<{ items: T[]; total: number }> {
  const res = await fetch(BASE + path, {
    method: opts.method ?? "GET",
    headers: opts.headers,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    const e = (body as any).error ?? { code: "internal_error", message: res.statusText };
    throw new APIError(res.status, e.code, e.message, e.details, e.request_id);
  }
  const total = parseInt(res.headers.get("X-Total-Count") ?? "0", 10);
  const items = (await res.json()) as T[];
  return { items, total };
}
```

- [x] **Step 18.4：写 `lib/api/kb.ts`**

```ts
import { apiFetch, apiFetchList } from "./client";
import type { KB } from "@/lib/schemas";

export const kbApi = {
  async create(input: { name: string; description?: string }): Promise<KB> {
    return apiFetch<KB>("/api/v1/kbs", {
      method: "POST",
      body: JSON.stringify({ name: input.name, description: input.description ?? "" }),
    });
  },
  async list(limit = 20, offset = 0): Promise<{ items: KB[]; total: number }> {
    return apiFetchList<KB>(`/api/v1/kbs?limit=${limit}&offset=${offset}`);
  },
  async get(id: string): Promise<KB> {
    return apiFetch<KB>(`/api/v1/kbs/${id}`);
  },
  async delete(id: string): Promise<void> {
    await apiFetch<void>(`/api/v1/kbs/${id}`, { method: "DELETE" });
  },
};
```

- [x] **Step 18.5：写 `lib/api/docs.ts`**

```ts
import { apiFetch, apiFetchList } from "./client";
import type { Doc, DocStatus } from "@/lib/schemas";

export const docApi = {
  async upload(kbId: string, file: File): Promise<Doc> {
    const form = new FormData();
    form.append("file", file);
    return apiFetch<Doc>(`/api/v1/kbs/${kbId}/docs`, {
      method: "POST",
      body: form,
    });
  },
  async listByKB(
    kbId: string,
    opts: { status?: DocStatus; limit?: number; offset?: number } = {},
  ): Promise<{ items: Doc[]; total: number }> {
    const qs = new URLSearchParams();
    qs.set("limit", String(opts.limit ?? 20));
    qs.set("offset", String(opts.offset ?? 0));
    if (opts.status) qs.set("status", opts.status);
    return apiFetchList<Doc>(`/api/v1/kbs/${kbId}/docs?${qs}`);
  },
  async get(id: string): Promise<Doc> {
    return apiFetch<Doc>(`/api/v1/docs/${id}`);
  },
  async delete(id: string): Promise<void> {
    await apiFetch<void>(`/api/v1/docs/${id}`, { method: "DELETE" });
  },
};
```

- [x] **Step 18.6：写 `lib/api/chunks.ts`**

```ts
import { apiFetchList } from "./client";
import type { Chunk } from "@/lib/schemas";

export const chunkApi = {
  async listByDoc(docId: string, limit = 20, offset = 0): Promise<{ items: Chunk[]; total: number }> {
    return apiFetchList<Chunk>(`/api/v1/docs/${docId}/chunks?limit=${limit}&offset=${offset}`);
  },
};
```

- [x] **Step 18.7：写 `components/providers.tsx`**

```tsx
"use client";

import { useState, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ReactQueryDevtools } from "@tanstack/react-query-devtools";
import { Toaster } from "@/components/ui/sonner";

export function Providers({ children }: { children: ReactNode }) {
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            staleTime: 5_000,
            retry: 1,
            refetchOnWindowFocus: false,
          },
        },
      }),
  );

  return (
    <QueryClientProvider client={client}>
      {children}
      <Toaster richColors position="top-right" />
      <ReactQueryDevtools initialIsOpen={false} />
    </QueryClientProvider>
  );
}
```

- [x] **Step 18.8：改 `app/layout.tsx` 包 Providers**

```tsx
import type { Metadata } from "next";
import { Providers } from "@/components/providers";
import "./globals.css";

export const metadata: Metadata = {
  title: "it-wiki",
  description: "团队知识库 Agent",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="zh-CN">
      <body className="min-h-screen bg-background text-foreground antialiased">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
```

- [~] **Step 18.9：验证**

```powershell
cd e:\GoProject\it-wiki\frontend
pnpm typecheck
pnpm build
```

期望：typecheck 干净；build compile 阶段成功（Windows symlink EPERM 可忽略）。

- [~] **Step 18.10：完成检查**

勾选 18.1~18.10。

---

## 任务 19：KB 列表页 + 创建对话框

**Files:**
- Modify: `e:\GoProject\it-wiki\frontend\app\page.tsx`
- Create: `e:\GoProject\it-wiki\frontend\lib\hooks\use-kbs.ts`
- Create: `e:\GoProject\it-wiki\frontend\components\kb\kb-card.tsx`
- Create: `e:\GoProject\it-wiki\frontend\components\kb\kb-create-dialog.tsx`

- [x] **Step 19.1：写 `lib/hooks/use-kbs.ts`**

```ts
"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { kbApi } from "@/lib/api/kb";

export function useKbs(limit = 20, offset = 0) {
  return useQuery({
    queryKey: ["kbs", { limit, offset }],
    queryFn: () => kbApi.list(limit, offset),
  });
}

export function useCreateKb() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; description?: string }) => kbApi.create(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["kbs"] }),
  });
}

export function useDeleteKb() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => kbApi.delete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["kbs"] }),
  });
}
```

- [x] **Step 19.2：写 `components/kb/kb-card.tsx`**

```tsx
"use client";

import Link from "next/link";
import { Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useDeleteKb } from "@/lib/hooks/use-kbs";
import type { KB } from "@/lib/schemas";
import { toast } from "sonner";

export function KBCard({ kb }: { kb: KB }) {
  const del = useDeleteKb();

  function handleDelete() {
    if (!confirm(`删除知识库 "${kb.name}"？所有文档和切片会一并删除。`)) return;
    del.mutate(kb.id, {
      onSuccess: () => toast.success("已删除"),
      onError: (e) => toast.error("删除失败: " + (e as Error).message),
    });
  }

  return (
    <div className="rounded-lg border bg-card p-4 flex flex-col gap-2">
      <Link href={`/kbs/${kb.id}/docs`} className="text-lg font-semibold hover:underline">
        {kb.name}
      </Link>
      <p className="text-sm text-muted-foreground line-clamp-2">{kb.description || "（无描述）"}</p>
      <div className="text-xs text-muted-foreground">
        {kb.embed_model} · dim={kb.embed_dim} · {new Date(kb.created_at).toLocaleDateString()}
      </div>
      <div className="mt-auto pt-2 flex justify-end">
        <Button size="sm" variant="ghost" onClick={handleDelete} disabled={del.isPending}>
          <Trash2 className="size-4" />
        </Button>
      </div>
    </div>
  );
}
```

- [x] **Step 19.3：写 `components/kb/kb-create-dialog.tsx`**

```tsx
"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger, DialogFooter,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useCreateKb } from "@/lib/hooks/use-kbs";
import { toast } from "sonner";

export function KBCreateDialog() {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");
  const create = useCreateKb();

  function reset() {
    setName("");
    setDesc("");
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    create.mutate({ name: name.trim(), description: desc.trim() }, {
      onSuccess: () => {
        toast.success(`已创建 "${name.trim()}"`);
        reset();
        setOpen(false);
      },
      onError: (e) => toast.error("创建失败: " + (e as Error).message),
    });
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { setOpen(v); if (!v) reset(); }}>
      <DialogTrigger asChild>
        <Button>新建知识库</Button>
      </DialogTrigger>
      <DialogContent>
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>新建知识库</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="kb-name">名称</Label>
              <Input id="kb-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="例如：工程团队 KB" autoFocus />
            </div>
            <div className="space-y-2">
              <Label htmlFor="kb-desc">描述（可选）</Label>
              <Input id="kb-desc" value={desc} onChange={(e) => setDesc(e.target.value)} />
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => setOpen(false)} disabled={create.isPending}>取消</Button>
            <Button type="submit" disabled={!name.trim() || create.isPending}>
              {create.isPending ? "创建中..." : "创建"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
```

- [x] **Step 19.4：改 `app/page.tsx`**

```tsx
"use client";

import { KBCard } from "@/components/kb/kb-card";
import { KBCreateDialog } from "@/components/kb/kb-create-dialog";
import { useKbs } from "@/lib/hooks/use-kbs";

export default function Home() {
  const { data, isLoading, isError, error } = useKbs();

  return (
    <main className="container mx-auto max-w-6xl p-8">
      <header className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">it-wiki</h1>
          <p className="text-muted-foreground text-sm">团队知识库 Agent</p>
        </div>
        <KBCreateDialog />
      </header>

      {isLoading && <p className="text-muted-foreground">加载中...</p>}
      {isError && <p className="text-destructive">加载失败：{(error as Error).message}</p>}
      {data && data.items.length === 0 && (
        <div className="text-center py-16 text-muted-foreground">
          <p className="mb-2">还没有知识库</p>
          <p className="text-sm">点击右上角"新建知识库"开始</p>
        </div>
      )}
      {data && data.items.length > 0 && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {data.items.map((kb) => <KBCard key={kb.id} kb={kb} />)}
        </div>
      )}
      {data && (
        <p className="mt-6 text-xs text-muted-foreground">共 {data.total} 个</p>
      )}
    </main>
  );
}
```

- [~] **Step 19.5：浏览器手动验证**

```powershell
cd e:\GoProject\it-wiki\frontend
pnpm dev
```

打开 http://localhost:3000：
- 看到 "it-wiki" 标题 + 右上角"新建知识库"按钮
- 点击按钮 → 对话框打开 → 输入名称 → 提交
- 列表刷新出现新 KB 卡片
- 点击删除图标 → 确认 → 卡片消失

`Ctrl+C` 停。

- [~] **Step 19.6：完成检查**

勾选 19.1~19.6。

---

## 任务 20：文档列表页 + 上传组件 + 状态轮询

**Files:**
- Create: `e:\GoProject\it-wiki\frontend\app\kbs\[kbId]\docs\page.tsx`
- Create: `e:\GoProject\it-wiki\frontend\lib\hooks\use-docs.ts`
- Create: `e:\GoProject\it-wiki\frontend\components\docs\doc-table.tsx`
- Create: `e:\GoProject\it-wiki\frontend\components\docs\doc-uploader.tsx`
- Create: `e:\GoProject\it-wiki\frontend\components\docs\ingest-status-badge.tsx`

- [x] **Step 20.1：写 `lib/hooks/use-docs.ts`**

```ts
"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { docApi } from "@/lib/api/docs";
import type { Doc } from "@/lib/schemas";

const TERMINAL_STATUSES: Doc["status"][] = ["ready", "failed"];

export function useDocsByKB(kbId: string, limit = 20, offset = 0) {
  return useQuery({
    queryKey: ["docs", kbId, { limit, offset }],
    queryFn: () => docApi.listByKB(kbId, { limit, offset }),
    enabled: !!kbId,
    // 列表里如果有 non-terminal 状态的 doc, 2s 自动刷新一次
    refetchInterval: (q) => {
      const items = q.state.data?.items ?? [];
      const inProgress = items.some((d) => !TERMINAL_STATUSES.includes(d.status));
      return inProgress ? 2_000 : false;
    },
  });
}

export function useDoc(id: string) {
  return useQuery({
    queryKey: ["doc", id],
    queryFn: () => docApi.get(id),
    enabled: !!id,
    refetchInterval: (q) => {
      const d = q.state.data;
      if (!d) return 2_000;
      return TERMINAL_STATUSES.includes(d.status) ? false : 2_000;
    },
  });
}

export function useUploadDoc(kbId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (file: File) => docApi.upload(kbId, file),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["docs", kbId] });
    },
  });
}

export function useDeleteDoc(kbId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => docApi.delete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["docs", kbId] }),
  });
}
```

- [x] **Step 20.2：写 `components/docs/ingest-status-badge.tsx`**

```tsx
import { Badge } from "@/components/ui/badge";
import type { DocStatus } from "@/lib/schemas";

const variantMap: Record<DocStatus, { label: string; className: string }> = {
  pending:   { label: "排队中",   className: "bg-gray-200 text-gray-800" },
  parsing:   { label: "解析中",   className: "bg-blue-100 text-blue-800" },
  chunking:  { label: "切片中",   className: "bg-blue-100 text-blue-800" },
  embedding: { label: "向量化中", className: "bg-blue-100 text-blue-800" },
  ready:     { label: "就绪",     className: "bg-green-100 text-green-800" },
  failed:    { label: "失败",     className: "bg-red-100 text-red-800" },
};

export function IngestStatusBadge({ status }: { status: DocStatus }) {
  const v = variantMap[status];
  return (
    <Badge variant="secondary" className={v.className}>
      {v.label}
    </Badge>
  );
}
```

- [x] **Step 20.3：写 `components/docs/doc-uploader.tsx`**

```tsx
"use client";

import { useRef, useState } from "react";
import { UploadCloud } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useUploadDoc } from "@/lib/hooks/use-docs";
import { toast } from "sonner";

const ACCEPT = ".md,.markdown,.pdf,.docx,.xlsx,text/markdown,application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet";

export function DocUploader({ kbId }: { kbId: string }) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const upload = useUploadDoc(kbId);

  function pickFile() {
    inputRef.current?.click();
  }

  function handleFiles(files: FileList | null) {
    if (!files || files.length === 0) return;
    for (const f of Array.from(files)) {
      upload.mutate(f, {
        onSuccess: () => toast.success(`已上传 ${f.name}, 摄入中...`),
        onError: (e: any) => {
          if (e?.code === "duplicate_checksum") {
            toast.info(`${f.name} 已存在 (existing_doc_id=${e.details?.existing_doc_id})`);
          } else if (e?.code === "payload_too_large") {
            toast.error(`${f.name} 超过 50MB 限制`);
          } else {
            toast.error(`${f.name} 上传失败: ${e.message}`);
          }
        },
      });
    }
  }

  return (
    <div
      onDragOver={(e) => { e.preventDefault(); setDragging(true); }}
      onDragLeave={() => setDragging(false)}
      onDrop={(e) => {
        e.preventDefault();
        setDragging(false);
        handleFiles(e.dataTransfer.files);
      }}
      className={`border-2 border-dashed rounded-lg p-6 text-center transition-colors ${
        dragging ? "border-primary bg-primary/5" : "border-muted-foreground/30"
      }`}
    >
      <UploadCloud className="size-8 mx-auto mb-2 text-muted-foreground" />
      <p className="text-sm text-muted-foreground mb-3">
        拖入文件到此处，或
      </p>
      <Button onClick={pickFile} disabled={upload.isPending}>
        {upload.isPending ? "上传中..." : "选择文件"}
      </Button>
      <input
        ref={inputRef}
        type="file"
        className="hidden"
        accept={ACCEPT}
        multiple
        onChange={(e) => handleFiles(e.target.files)}
      />
      <p className="text-xs text-muted-foreground mt-3">
        支持 .md / .pdf / .docx / .xlsx, 单文件 ≤ 50MB
      </p>
    </div>
  );
}
```

- [x] **Step 20.4：写 `components/docs/doc-table.tsx`**

```tsx
"use client";

import Link from "next/link";
import { Trash2 } from "lucide-react";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { IngestStatusBadge } from "./ingest-status-badge";
import { useDeleteDoc } from "@/lib/hooks/use-docs";
import type { Doc } from "@/lib/schemas";
import { toast } from "sonner";

export function DocTable({ kbId, docs }: { kbId: string; docs: Doc[] }) {
  const del = useDeleteDoc(kbId);

  function fmt(bytes: number): string {
    if (bytes < 1024) return bytes + " B";
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
    return (bytes / 1024 / 1024).toFixed(1) + " MB";
  }

  function onDelete(d: Doc) {
    if (!confirm(`删除文档 "${d.title}"？切片会一并删除。`)) return;
    del.mutate(d.id, {
      onSuccess: () => toast.success("已删除"),
      onError: (e) => toast.error("删除失败: " + (e as Error).message),
    });
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>文件</TableHead>
          <TableHead>状态</TableHead>
          <TableHead>大小</TableHead>
          <TableHead>上传时间</TableHead>
          <TableHead className="w-12" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {docs.map((d) => (
          <TableRow key={d.id}>
            <TableCell>
              <Link href={`/kbs/${kbId}/docs/${d.id}`} className="font-medium hover:underline">
                {d.title}
              </Link>
              {d.status === "failed" && d.error_message && (
                <p className="text-xs text-destructive mt-1 line-clamp-1">{d.error_message}</p>
              )}
            </TableCell>
            <TableCell><IngestStatusBadge status={d.status} /></TableCell>
            <TableCell className="text-muted-foreground">{fmt(d.bytes)}</TableCell>
            <TableCell className="text-muted-foreground text-xs">
              {new Date(d.created_at).toLocaleString()}
            </TableCell>
            <TableCell>
              <Button size="sm" variant="ghost" onClick={() => onDelete(d)} disabled={del.isPending}>
                <Trash2 className="size-4" />
              </Button>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
```

- [x] **Step 20.5：写 `app/kbs/[kbId]/docs/page.tsx`**

```tsx
"use client";

import Link from "next/link";
import { use } from "react";
import { ArrowLeft } from "lucide-react";
import { DocUploader } from "@/components/docs/doc-uploader";
import { DocTable } from "@/components/docs/doc-table";
import { useDocsByKB } from "@/lib/hooks/use-docs";

export default function KBDocsPage({ params }: { params: Promise<{ kbId: string }> }) {
  const { kbId } = use(params);
  const { data, isLoading, isError, error } = useDocsByKB(kbId);

  return (
    <main className="container mx-auto max-w-5xl p-8">
      <Link href="/" className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground mb-4">
        <ArrowLeft className="size-4 mr-1" /> 返回知识库列表
      </Link>

      <h1 className="text-2xl font-bold mb-6">文档</h1>

      <div className="mb-6">
        <DocUploader kbId={kbId} />
      </div>

      {isLoading && <p className="text-muted-foreground">加载中...</p>}
      {isError && <p className="text-destructive">加载失败：{(error as Error).message}</p>}
      {data && data.items.length === 0 && (
        <p className="text-center py-12 text-muted-foreground">还没有文档</p>
      )}
      {data && data.items.length > 0 && (
        <>
          <DocTable kbId={kbId} docs={data.items} />
          <p className="mt-4 text-xs text-muted-foreground">共 {data.total} 个文档</p>
        </>
      )}
    </main>
  );
}
```

- [~] **Step 20.6：浏览器验证**

`pnpm dev`，从 KB 列表点进 KB → 看到文档页 → 拖一个 sample.md 上去 → 看到行出现，状态 `排队中` → 几秒后变 `解析中` → `切片中` → `向量化中` → `就绪`。
**轮询要正常工作（每 2s 刷新一次）。**

- [~] **Step 20.7：完成检查**

勾选 20.1~20.7。

---

## 任务 21：文档详情页 + chunks 列表（分页）

**Files:**
- Create: `e:\GoProject\it-wiki\frontend\app\kbs\[kbId]\docs\[docId]\page.tsx`
- Create: `e:\GoProject\it-wiki\frontend\lib\hooks\use-chunks.ts`
- Create: `e:\GoProject\it-wiki\frontend\components\chunks\chunk-list.tsx`

- [x] **Step 21.1：写 `lib/hooks/use-chunks.ts`**

```ts
"use client";

import { useQuery } from "@tanstack/react-query";
import { chunkApi } from "@/lib/api/chunks";

export function useChunks(docId: string, limit = 20, offset = 0) {
  return useQuery({
    queryKey: ["chunks", docId, { limit, offset }],
    queryFn: () => chunkApi.listByDoc(docId, limit, offset),
    enabled: !!docId,
  });
}
```

- [x] **Step 21.2：写 `components/chunks/chunk-list.tsx`**

```tsx
"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { useChunks } from "@/lib/hooks/use-chunks";

const PAGE_SIZE = 10;

export function ChunkList({ docId }: { docId: string }) {
  const [page, setPage] = useState(0);
  const { data, isLoading, isError, error } = useChunks(docId, PAGE_SIZE, page * PAGE_SIZE);

  if (isLoading) return <p className="text-muted-foreground">加载中...</p>;
  if (isError) return <p className="text-destructive">加载失败：{(error as Error).message}</p>;
  if (!data || data.items.length === 0) return <p className="text-muted-foreground">无切片</p>;

  const totalPages = Math.ceil(data.total / PAGE_SIZE);

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        共 {data.total} 个切片 · 第 {page + 1} / {totalPages} 页
      </p>

      <div className="space-y-3">
        {data.items.map((c) => (
          <div key={c.id} className="rounded-md border bg-card p-4">
            <div className="text-xs text-muted-foreground mb-2">
              #{c.seq} · {c.token_count} tokens
            </div>
            <pre className="whitespace-pre-wrap text-sm leading-relaxed font-sans">{c.content}</pre>
          </div>
        ))}
      </div>

      <div className="flex justify-between items-center pt-2">
        <Button size="sm" variant="outline" disabled={page === 0} onClick={() => setPage(p => p - 1)}>上一页</Button>
        <Button size="sm" variant="outline" disabled={page + 1 >= totalPages} onClick={() => setPage(p => p + 1)}>下一页</Button>
      </div>
    </div>
  );
}
```

- [x] **Step 21.3：写 `app/kbs/[kbId]/docs/[docId]/page.tsx`**

```tsx
"use client";

import Link from "next/link";
import { use } from "react";
import { ArrowLeft } from "lucide-react";
import { useDoc } from "@/lib/hooks/use-docs";
import { IngestStatusBadge } from "@/components/docs/ingest-status-badge";
import { ChunkList } from "@/components/chunks/chunk-list";

export default function DocDetailPage({ params }: { params: Promise<{ kbId: string; docId: string }> }) {
  const { kbId, docId } = use(params);
  const { data: doc, isLoading, isError, error } = useDoc(docId);

  return (
    <main className="container mx-auto max-w-5xl p-8">
      <Link href={`/kbs/${kbId}/docs`} className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground mb-4">
        <ArrowLeft className="size-4 mr-1" /> 返回文档列表
      </Link>

      {isLoading && <p className="text-muted-foreground">加载中...</p>}
      {isError && <p className="text-destructive">加载失败：{(error as Error).message}</p>}

      {doc && (
        <>
          <header className="mb-6">
            <div className="flex items-center gap-3 mb-1">
              <h1 className="text-2xl font-bold">{doc.title}</h1>
              <IngestStatusBadge status={doc.status} />
            </div>
            <div className="text-xs text-muted-foreground">
              {doc.mime_type} · {(doc.bytes / 1024).toFixed(1)} KB · {doc.checksum.slice(0, 24)}...
            </div>
            {doc.status === "failed" && doc.error_message && (
              <p className="mt-2 text-sm text-destructive bg-destructive/10 p-2 rounded">
                {doc.error_message}
              </p>
            )}
          </header>

          <section>
            <h2 className="text-lg font-semibold mb-3">切片预览</h2>
            {doc.status === "ready" ? (
              <ChunkList docId={docId} />
            ) : doc.status === "failed" ? (
              <p className="text-muted-foreground">摄入失败，无切片可显示</p>
            ) : (
              <p className="text-muted-foreground">摄入中，请稍候...</p>
            )}
          </section>
        </>
      )}
    </main>
  );
}
```

- [~] **Step 21.4：浏览器验证**

`pnpm dev` → 上传 sample.md → 等 ready → 点文件名进详情页：
- header 显示文件名 + 就绪 badge + mime/size/checksum
- "切片预览"区域出现至少 1 个切片卡片，显示 seq + token count + 内容
- 上一页/下一页按钮（如果切片多）

- [~] **Step 21.5：完成检查**

勾选 21.1~21.5。

---

## 任务 22：阶段 1 端到端验收

**Files:** 无新增

> 这是 Phase 1 的总验收。任何一项失败都视为 Phase 1 未完成。

- [~] **Step 22.1：全栈干净重启**

```powershell
cd e:\GoProject\it-wiki
docker compose -f deploy/docker-compose.yml --env-file .env down -v
docker compose -f deploy/docker-compose.yml --env-file .env up -d --build
docker compose -f deploy/docker-compose.yml ps
```

期望：5 个服务全部 healthy，`minio-init` exited(0)。

- [~] **Step 22.2：检查 backend 启动日志**

```powershell
docker compose -f deploy/docker-compose.yml logs backend --tail 60
```

期望包含：
- `goose: applied`（6 条迁移）
- `rivermigrate`（river 系统表）
- `[main] minio bucket "it-wiki-docs" ready`
- `[server] listening on :8080`

**不**应包含 `embedding dim check failed` / `embedding api error` 等。

- [~] **Step 22.3：前端浏览器全流程**

打开 http://localhost:3000：

1. KB 列表为空，点"新建知识库" → 输入 "phase1-final-test" → 创建
2. 列表出现新 KB 卡片，点卡片进文档页
3. 拖入 `e:\GoProject\it-wiki\backend\testdata\sample.md`
4. 表格出现新行，状态从 `排队中` → `解析中` → … → `就绪`（**30 秒内**）
5. 点文件名进详情页
6. "切片预览"区域显示至少 1 个 chunk，能看到 Markdown 文本内容

**这一步是 Phase 1 的核心验收。失败必查 backend 日志。**

- [~] **Step 22.4：边界用例验证**

| 用例 | 操作 | 期望 |
|---|---|---|
| 重复上传 | 再次拖同样 sample.md | toast 提示已存在，不创建新 doc |
| 大文件 | 拖一个 >50MB 文件 | toast 提示超限 |
| 不支持格式 | 拖一个 .exe | 上传 201 + worker 失败 → 状态 `failed` + error_message |
| 删除 KB | 删一个 KB | 它下面的所有 doc 和 chunk 一并消失（DB CASCADE） |
| 前端断网后恢复 | DevTools → Network → Offline → 重新 Online | 列表自动恢复 |

- [~] **Step 22.5：更新 CLAUDE.md §2**

把第 2 节"当前阶段"改为：

```markdown
## 2. 当前阶段

> **当前进度**：阶段 1 已完成（文档摄入闭环就绪）。下一步：阶段 2 RAG 对话最小闭环。需要触发 brainstorming 或 writing-plans 写阶段 2 的实施计划。
```

- [~] **Step 22.6：完成检查**

勾选 22.1~22.6。**阶段 1 实施计划全部完成。**

---

## 阶段 1 验收清单（最终）

执行人在最后一并核对：

- [ ] `docker compose up -d --build` 起来 5 个服务全 healthy
- [ ] `curl /healthz` 返回 200
- [ ] `curl POST /api/v1/kbs` 创建 KB 返回 201 + 完整资源
- [ ] `curl POST /kbs/:id/docs` 上传 Markdown 返回 201, status=pending
- [ ] 30 秒内轮询 `/docs/:id` 看到 status=ready
- [ ] `curl GET /docs/:id/chunks` 返回非空数组 + `X-Total-Count` header
- [ ] 重复上传同文件返回 409 + duplicate_checksum + existing_doc_id
- [ ] 超 50MB 上传返回 413 + payload_too_large
- [ ] 前端 KB 列表/详情/文档列表/文档详情/chunks 预览全部能跑通
- [ ] 上传后前端轮询能看到状态从 pending 推进到 ready
- [ ] `go test ./...` 全部 PASS
- [ ] `pnpm typecheck` 干净
- [ ] CLAUDE.md §2 已更新为"阶段 1 已完成"
- [ ] **未做 git commit**

---

## 阶段 1 已识别延伸事项（不在本阶段做）

记录在此供阶段 2 启动时参考：

1. **VectorStore.Search**：Phase 2 RAG flow 需要 `Search(kbID, query, topK, filter)` 方法 + IVFFlat 索引参数调优（`probes`）
2. **JWT / RBAC**：MVP 用 `owner_id = 'local-admin'` 写死，企业部署时换 JWT middleware
3. **Embedding 缓存**：相同 content 重复 embed 是浪费，Phase 2 可加 LRU 或 redis
4. **更细 mime 嗅探**：当前用 multipart header 的 Content-Type 不可信，可用 `mimetype` 库二次嗅
5. **river dashboard**：river 自带 web UI 监控 job 队列，Phase 2 接入便于演示
6. **Eino 集成**：D1 决策延后到 Phase 2，届时 Embedder/LLMClient/VectorStore 包成 Eino component
7. **chunk neighbors API**：spec §6.4 引用抽屉行为需要 `/api/v1/kbs/:id/chunks/:chunkId/neighbors?window=2`，Phase 2 实现
8. **Migration rollback 安全**：当前 0004 down 直接 drop chunks，应加 confirm 标志位防误删生产数据

---

_本计划完成后请回到 brainstorming/writing-plans 触发阶段 2 计划编写。_



