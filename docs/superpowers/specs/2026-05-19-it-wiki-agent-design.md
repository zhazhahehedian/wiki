# IT-Wiki · 知识库 Agent 设计 (PRD/Spec)

- 起草日期：2026-05-19
- 作者：Zenith Wang · 与 Claude 共建
- 状态：MVP 阶段设计已锁定，待实施

---

## 0. 项目概述

一个面向**团队/企业内部**的知识库 Agent。用户上传文档建立知识库，与 AI 进行基于检索增强生成（RAG）的多轮对话。MVP 阶段以"端到端走通"为目标，每一层都做最小实现，但用 interface 抽象保留演进路径。

### 核心使用场景

- 上传若干技术文档/会议纪要/产品文档到一个知识库
- 在对话中向 AI 提问，AI 基于知识库检索回答，并显示引用来源
- 点击引用查看原文上下文（chunk + 前后片段）
- 高级模式（ReAct）让 LLM 自主决定是否调用工具

### 不在 MVP 范围内的能力

明确排除（V1.5/V2 再做）：

- 用户系统、登录、组织、权限（先单用户）
- 飞书云文档 / Notion / URL 抓取（先本地上传）
- 混合检索（BM25 + 向量）、reranker
- 引用全文定位 + 高亮
- 文档版本管理、多 KB 联合检索
- 评估/A-B-test 面板

---

## 1. 技术栈

### 后端

| 组件 | 选型 | 理由 |
|---|---|---|
| 语言/运行时 | Go 1.22+ | 性能、并发模型适配 LLM 流式场景 |
| HTTP 框架 | chi | 标准库 `net/http` 兼容，中间件干净 |
| LLM 编排 | Eino（字节开源） | 国产生态主流，ReAct/Workflow 模板可用（阶段 3 决策：MVP 手写 ReAct 循环未引入 Eino，推迟到 V1.5 再评估，见阶段 3 spec D1） |
| LLM 适配 | OpenAI 兼容协议 | 可对接 DeepSeek/Qwen/百炼/vLLM 等 |
| DB 访问 | sqlc | 类型安全、AI 编码友好（错就编译挂） |
| 迁移工具 | goose | 简单、SQL-first |
| 任务队列 | river | Postgres-backed，免引入 Redis |
| 对象存储 | MinIO（S3 兼容） | 一开始就走 S3 语义，后期可平迁阿里云 OSS / AWS S3 |
| 数据库 | PostgreSQL 16 + pgvector | 业务数据 + 向量 一体化 |

### 前端

| 组件 | 选型 | 理由 |
|---|---|---|
| 框架 | Next.js 15（App Router） | 主流、SSR/CSR 灵活 |
| 语言 | TypeScript 5.x | 必备 |
| 样式 | Tailwind CSS v4 | CSS-first 配置，新版本 |
| 行为组件 | Radix UI primitives | 无样式，跟 Tailwind 配合最干净 |
| 组件模板 | shadcn/ui | 基于 Radix 复制到仓库，可自由改 |
| UI 状态 | Zustand 5 | 轻量，足够 |
| 服务端状态 | TanStack Query 5 | 缓存/失效/重试一体 |
| 表单 | react-hook-form + zod | 标配 |
| Markdown 渲染 | react-markdown + rehype-highlight | LLM 输出渲染 |
| Toast | sonner | shadcn 推荐 |
| 图标 | lucide-react | 与 shadcn 默认一致 |
| SSE 消费 | 原生 EventSource + 自封 hook | TanStack Query 不管 SSE |

**有意不引入**：Server Actions、Redux/Jotai、tRPC、next-auth（MVP 单用户）。

### 部署

- 单机 Docker Compose（postgres + minio + backend + frontend 四容器）
- 一次性 `mc` init 容器或后端 entrypoint 内做 `EnsureBucket`（带 backoff 等 MinIO 就绪）
- 暂不考虑 K8s/多副本

---

## 2. 架构总览

```
┌─────────────────────────────────────────────────────────────┐
│  前端  Next.js 15 (App Router) + Tailwind v4 + Radix UI    │
│        + Zustand + TanStack Query + TypeScript              │
└────────────────────────┬─────────────────────────────────────┘
                         │ REST + SSE
┌────────────────────────▼─────────────────────────────────────┐
│  后端  Go 1.22+ · chi                                        │
│  ├─ Eino                Agent/RAG/工具编排                   │
│  ├─ Service 层          usecase                              │
│  ├─ Domain 层 (ports)   纯领域 + interface 定义              │
│  │   DataSource / Parser / Splitter / Embedder /            │
│  │   VectorStore / LLMClient / TaskQueue                    │
│  └─ Infra 层            ports 的具体实现                     │
└────────────────────────┬─────────────────────────────────────┘
                         │
                ┌────────▼─────────┐
                │  PostgreSQL 16   │  业务 + pgvector + river 队列
                └──────────────────┘
                ┌──────────────────┐
                │  MinIO (S3 API)  │  原始上传文件 + V1.5 飞书抓回的副本
                └──────────────────┘
```

### 架构核心原则

1. **可演进单体**：单进程部署，但内部按 ports & adapters 解耦
2. **interface 在前**：所有外部依赖（解析器、向量库、LLM、队列）先定义 interface 再写实现
3. **配置驱动**：embedding 维度、模型 base_url 等通过环境变量注入
4. **失败显式**：MVP 阶段失败不静默重试，前端展示错误状态等用户操作

---

## 3. 后端目录结构

```
backend/
├── cmd/server/main.go
├── internal/
│   ├── config/
│   ├── http/
│   │   ├── router.go
│   │   ├── kb_handler.go
│   │   ├── doc_handler.go
│   │   ├── chunk_handler.go
│   │   ├── chat_handler.go
│   │   └── middleware/
│   ├── service/
│   │   ├── kb_service.go
│   │   ├── ingestion_service.go
│   │   ├── retrieval_service.go
│   │   └── chat_service.go
│   ├── domain/
│   │   ├── kb.go
│   │   ├── document.go
│   │   ├── chunk.go
│   │   ├── chat.go
│   │   └── ports/
│   │       ├── datasource.go
│   │       ├── parser.go
│   │       ├── splitter.go
│   │       ├── embedder.go
│   │       ├── vector_store.go
│   │       ├── llm_client.go
│   │       └── task_queue.go
│   ├── infra/
│   │   ├── parser/{markdown,pdf,docx,xlsx}.go
│   │   ├── splitter/recursive_char.go
│   │   ├── embedder/openai_compatible.go
│   │   ├── llm/openai_compatible.go
│   │   ├── vectorstore/pgvector.go
│   │   ├── datasource/local_upload.go
│   │   ├── storage/minio.go           # aws-sdk-go-v2 + MinIO endpoint
│   │   └── queue/river_queue.go
│   ├── repo/
│   │   ├── queries/*.sql
│   │   ├── migrations/*.sql
│   │   └── generated/
│   ├── agent/
│   │   ├── rag_agent.go
│   │   ├── react_agent.go
│   │   └── tools/{kb_retrieval,calculator}.go
│   └── worker/ingestion_worker.go
├── sqlc.yaml
├── go.mod
└── Dockerfile
```

### 端口接口（domain/ports）签名

```go
type Parser interface {
    Supports(mime string) bool
    Parse(ctx context.Context, r io.Reader, hint Hint) (Document, error)
}

type Splitter interface {
    Split(ctx context.Context, doc Document, opts SplitOptions) ([]Chunk, error)
}

type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dim() int
}

type VectorStore interface {
    Upsert(ctx context.Context, kbID string, items []VectorItem) error
    Search(ctx context.Context, kbID string, query []float32, topK int, filter Filter) ([]SearchHit, error)
    Delete(ctx context.Context, kbID string, docIDs []string) error
}

type LLMClient interface {
    Chat(ctx context.Context, msgs []Message, opts ChatOptions) (Message, error)
    ChatStream(ctx context.Context, msgs []Message, opts ChatOptions) (<-chan StreamChunk, error)
}

type DataSource interface {
    ID() string
    List(ctx context.Context, params ListParams) ([]SourceItem, error)
    Fetch(ctx context.Context, item SourceItem) (io.ReadCloser, Metadata, error)
}
```

### Eino 的定位

> 阶段 3 实施注记（2026-07）：未引入 Eino。react_agent 为手写循环（internal/agent/），tools 实现 ports.Tool 接口。本节保留原设想供 V1.5 评估。

- `internal/agent/` 是 Eino 的薄封装
- `rag_agent` 直接调 `retrieval_service` + `LLMClient.ChatStream`
- `react_agent` 用 Eino 的 ReAct 模板 + 注册的 tools（kb_retrieval、calculator）
- MVP 默认 RAG 模式（更可控、便宜），ReAct 作为高级模式可在前端切换

---

## 4. 数据模型

5 张业务表（river 队列另有系统表，sqlc 不碰）。

### 4.1 knowledge_bases

```sql
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
CREATE INDEX ON knowledge_bases (owner_id);
```

- `owner_id` 预留认证插点，MVP 固定写 `local-admin`
- `embed_model` + `embed_dim` 记录该 KB 使用的 embedding 配置，**MVP 阶段全局只允许一个维度**（与环境变量 `EMBEDDING_DIM` 一致，pgvector 列维度是 schema 级别固定的），创建 KB 时校验 `embed_dim == EMBEDDING_DIM`，不一致直接拒绝。多维度共存属于 V2 范畴（需要按维度分表或独立 vector store 实例）

### 4.2 documents

```sql
CREATE TABLE documents (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id         UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    source_type   TEXT NOT NULL,        -- 'local-upload' / 'feishu' (V1.5)
    source_ref    TEXT NOT NULL,
    title         TEXT NOT NULL,
    mime_type     TEXT NOT NULL,
    bytes         BIGINT NOT NULL,
    checksum      TEXT NOT NULL,
    status        TEXT NOT NULL,        -- pending|parsing|chunking|embedding|ready|failed
    error_message TEXT,
    metadata      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON documents (kb_id, status);
CREATE UNIQUE INDEX ON documents (kb_id, checksum);
```

### 4.3 chunks（含 vector 列）

```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE chunks (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id         UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    document_id   UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    seq           INT  NOT NULL,
    content       TEXT NOT NULL,
    token_count   INT  NOT NULL,
    embedding     vector(/* dim 由 EMBEDDING_DIM 注入 */),
    metadata      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON chunks (kb_id, document_id, seq);
CREATE INDEX ON chunks USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
```

- chunks 与 embeddings 合表（简化 join）
- 向量索引用 IVFFlat（数据量小够用，HNSW 留到向量量 > 10 万再换）
- `embedding` 列维度在迁移阶段从 `EMBEDDING_DIM` 模板渲染（用 goose 的 Go migration 或在 entrypoint 跑模板替换），写死后不可变
- 启动时校验 DB 实际列维度 == `EMBEDDING_DIM`，不一致直接 panic（防止换模型后维度漂移）

### 4.4 conversations

```sql
CREATE TABLE conversations (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id         UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    title         TEXT NOT NULL DEFAULT '新对话',
    mode          TEXT NOT NULL,        -- 'rag' | 'react'
    user_id       TEXT NOT NULL DEFAULT 'local-admin',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON conversations (kb_id, user_id, updated_at DESC);
```

### 4.5 messages

```sql
CREATE TABLE messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL,      -- user|assistant|tool
    content         TEXT NOT NULL,
    citations       JSONB NOT NULL DEFAULT '[]'::jsonb,
    tool_calls      JSONB NOT NULL DEFAULT '[]'::jsonb,
    token_usage     JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON messages (conversation_id, created_at);
```

---

## 5. 关键数据流

### 5.1 文档摄入（异步，river 驱动）

```
POST /api/v1/kbs/:id/docs (multipart)
  └─ ingestion_service.UploadAndEnqueue
       ├─ 算 sha256, (kb_id, checksum) 已存在 → 409 复用
       ├─ 落盘到 storage
       ├─ INSERT documents (status='pending')
       └─ river.Enqueue(IngestionJob{doc_id})

worker.IngestionWorker (在独立 goroutine pool)
  每步失败 → status='failed', error_message, 不自动重试
  ├─ status='parsing'    → Parser.Parse() by mime
  ├─ status='chunking'   → Splitter.Split()
  ├─ status='embedding'  → 批量 Embedder.Embed() (默认 batch=64)
  └─ status='ready'      → VectorStore.Upsert() + INSERT chunks (同事务)
```

**决策摘要**：

- 失败不自动重试：embedding 重试 = 重复扣费
- 批量 embedding，单批失败整文档失败
- chunks 与向量同事务写入

### 5.2 RAG 对话（SSE 流式）

```
POST /api/v1/conversations/:id/messages (建立 SSE)
  └─ chat_service.Ask
       ├─ 取最近 5 轮历史
       ├─ Eino RAG flow:
       │    embed(query) → VectorStore.Search(topK=8)
       │    拼 prompt (system + retrieved + history + user)
       │    LLMClient.ChatStream
       ├─ SSE 边推边收 buffer
       └─ 流结束: INSERT user msg + assistant msg (含 citations)
```

**SSE 事件**：

```
event: retrieval   data: {hits: [{doc_id, chunk_id, score, snippet}, ...]}
event: token       data: {text: "..."}
event: error       data: {message: "..."}
event: done        data: {message_id, usage: {...}}
```

### 5.3 ReAct 对话

```
chat_service.Ask (mode='react')
  └─ Eino ReAct 模板:
       loop:
         LLM → ToolChoice
         if tool: 执行 tool, 推送 SSE tool_call / tool_result
         else: 流式输出 final answer
```

**额外 SSE 事件**：`tool_call`, `tool_result`, （可选 `thinking`）。

### 5.4 边界与错误处理

| 场景 | 处理 |
|---|---|
| LLM 限流/超时 | 单次失败返回错误，前端 toast，不自动重试 |
| Embedding 维度与 KB 配置不一致 | 启动时校验，运行时 panic |
| 检索 0 结果 | 仍走 LLM，prompt 明确"知识库无相关内容" |
| 上传超大文件 | MVP 限 50 MB，超过 413 |
| 同 KB 重复 checksum | 409 + 返回已存在 document_id |
| 用户中途断开 SSE | server 端 `ctx.Done()` 取消 LLM 流，避免烧 token |

---

## 6. 前端结构

### 6.1 目录

```
frontend/
├── app/
│   ├── layout.tsx
│   ├── page.tsx
│   ├── kbs/
│   │   ├── page.tsx
│   │   ├── new/page.tsx
│   │   └── [kbId]/
│   │       ├── layout.tsx
│   │       ├── docs/{page.tsx, [docId]/page.tsx}
│   │       ├── chats/{page.tsx, [convId]/page.tsx}
│   │       └── settings/page.tsx
│   └── api/
├── components/
│   ├── ui/                  # shadcn/ui
│   ├── layout/              # app-shell, rail, kb-panel, chat-panel,
│   │                          theme-toggle（rail + 二级面板布局）
│   ├── common/              # empty-state 等通用组件
│   ├── chat/                # chat-input, message-list, message-bubble,
│   │                          citation-chip, citation-drawer,
│   │                          agent-timeline, chat-header
│   ├── docs/                # doc-uploader, doc-table, ingest-status-badge
│   └── kb/                  # kb-create-dialog
├── lib/
│   ├── api/{client, kb, docs, chat, chunks}.ts
│   ├── hooks/{use-chat-stream, use-kbs, use-docs}.ts
│   ├── store/{ui-store, chat-store, citation-store}.ts
│   └── schemas/             # zod schemas
├── styles/globals.css
└── package.json
```

### 6.2 对话页布局

```
┌──────────┬─────────────────────────────┬─────────────────┐
│ KB       │ ChatHeader (mode 切换)       │                 │
│ Sidebar  ├─────────────────────────────┤  CitationDrawer │
│ (固定)   │ MessageList                  │  (Sheet, 可关闭)│
│          │  ├ user                      │  chunk content  │
│          │  ├ citation cards (点击→开抽屉)               │
│          │  └ assistant (markdown 流式)                   │
│          ├─────────────────────────────┤  + 前后片段(灰) │
│          │ ChatInput                    │                 │
└──────────┴─────────────────────────────┴─────────────────┘
```

### 6.3 SSE 消费 Hook（约定）

```ts
function useChatStream(convId: string) {
  // EventSource → 监听 retrieval/token/tool_call/tool_result/done/error
  // 流结束 invalidateQueries(['messages', convId])
  // 返回 { stream, send, isStreaming, error }
}
```

### 6.4 引用抽屉行为

- 点击消息中的 citation card → Zustand `citation-store.setActive(chunkId)`
- Drawer 打开后调 `GET /api/v1/kbs/:id/chunks/:chunkId/neighbors?window=2`
- 主 chunk 高亮，前后各 2 个 chunk 灰色显示
- 关闭：Sheet 内置 onOpenChange / ESC / 点遮罩

---

## 7. 分阶段路线图

### 阶段 0 · 工程脚手架（~ 0.5 天）

**目标**：两个 hello world + Docker Compose 起来。

- backend Go 骨架，`/healthz` 200
- frontend Next.js + Tailwind v4 + shadcn 初始化
- docker-compose.yml（postgres + minio + backend + frontend）
- backend entrypoint 含 `EnsureBucket`（带 backoff 等 MinIO 就绪）
- Makefile / pnpm 任务脚本
- .env.example、README.md
- goose + sqlc 工具链就绪
- CLAUDE.md 初稿

**验收**：`docker compose up` 后：

- 访问 :3000 看到前端首页
- `curl /api/healthz` 返回 200
- 访问 :9001（MinIO Console）能用 .env 里的 credentials 登录，看到目标 bucket 已存在

### 阶段 1 · 文档摄入闭环（~ 2~3 天）

**目标**：能上传、能看到文档列表、能看到切片结果。**没有对话。**

后端：

- 5 张表 migrations + sqlc queries
- domain/ports 全部 interface 定义
- Parser（Markdown 完整，PDF/DOCX/XLSX 粗糙能用）
- Splitter（recursive char, chunk_size=800, overlap=120）
- Embedder + LLMClient（OpenAI 兼容）
- VectorStore（pgvector）
- river worker（IngestionWorker）
- HTTP: KB / Docs / Chunks CRUD

前端：

- KB 列表页 + 创建 KB
- 文档列表页 + 上传组件
- 文档详情页：chunks 预览
- TanStack Query hooks
- 上传后 polling 拉文档状态

**验收**：上传 Markdown，30 秒内状态变 `ready`，能看到切片内容。

### 阶段 2 · RAG 对话最小闭环（~ 2~3 天）

**目标**：核心功能 — 基于知识库回答 + 引用。

后端：

- chat_service + Eino RAG flow
- HTTP: conversations / messages（SSE）
- chunks/:id/neighbors

前端：

- 对话列表 + 新建对话
- 对话详情页 + useChatStream
- MessageList、ChatInput、Markdown、citation cards
- CitationDrawer + citation-store

**验收**：已有文档 KB 中提问，流式输出 + 引用卡片 + 点击抽屉看上下文。

### 阶段 3 · ReAct Agent 模式（~ 1.5~2 天）

**目标**：mode 切换 + LLM 自主调用工具。

后端：

- Eino ReAct 模板
- tools 注册：kb_retrieval、list_documents（阶段 3 spec D5：calculator 无业务价值，换为可回答"知识库里有哪些文档"的 list_documents）
- SSE 增加 tool_call / tool_result
- conversations.mode 字段使能

前端：

- ChatHeader mode switch
- tool-call-trace 组件（可折叠）

**验收**：ReAct 模式下能看到 LLM 调用 kb_retrieval + list_documents 的完整轨迹。

### 阶段 4 · 打磨 + Demo 友好（~ 1~2 天）

**目标**：从"能跑"到"能演示"。

- 全局错误走 sonner toast
- 摄入失败的"重新处理"按钮
- 设置页：top_k / 温度 / system prompt
- 空状态引导
- string 集中（不做切换，留 i18n 准备）
- README + 截图 + 示例知识库
- 输入 token 计数显示

**验收**：把链接发同事，对方不看文档跑通 KB → 文档 → 问答 全流程。

### 总工期估算

- MVP 4 阶段加起来 7~10 天纯开发时间
- vibe coding 建议预留 50% buffer：**约 2 周**

---

## 8. V1.5 / V2 推迟项

### V1.5（MVP 之后下一波）

1. 飞书云文档 DataSource（OpenAPI、tenant_access_token、block 递归）
2. 混合检索（BM25 + 向量 + RRF）
3. Reranker 模型集成
4. 引用抽屉升级：全文定位 + 高亮（需 chunks.start_offset/end_offset）
5. 用户系统（本地账号 / OIDC）
6. 文档摄入 SSE 进度推送

### V2（看 V1.5 效果再说）

- 多 KB 同时检索
- HyDE / 查询改写
- 文档版本管理
- 评估面板（retrieval P/R 测试集）

---

## 9. 风险与开放问题

| 风险 | 缓解 |
|---|---|
| Eino 中文文档/示例还在演进 | 关键节点核对官方仓库 examples，必要时降级用裸 OpenAI SDK |
| Tailwind v4 + Next.js 15 + Radix 都是新版本 | 阶段 0 锁定版本号；遇到 API 不一致先查官方 changelog 而不是问 AI |
| AI 生成 sqlc query 可能不匹配 schema | 每次 `sqlc generate` 必须跑，编译报错立即修 |
| pgvector IVFFlat 数据量大时召回下降 | 监控 chunks 数，超 10 万切 HNSW |
| 异步 worker 卡死 | river 提供 job 监控；阶段 1 验收必跑一次故意失败用例 |
| 单机 demo 偶发 OOM | embedding 批量 64 控制内存；大文件分批入库 |
| MinIO 启动慢导致 backend EnsureBucket 失败 | entrypoint 用指数退避重试 30s；compose `depends_on` + healthcheck |
| MinIO credentials 泄露 | `.env` 严禁入库；`.env.example` 给占位值；README 提醒 dev 阶段也要改默认 root key |

---

## 10. 验收清单（MVP 结束时）

- [ ] `docker compose up` 一键启动
- [ ] 能创建至少 1 个 KB
- [ ] 能上传 Markdown / PDF / DOCX / XLSX 各 1 份并状态变 ready
- [ ] 能在 KB 内创建对话，RAG 模式收到引用 + 流式答复
- [ ] 点击引用打开抽屉看到前后上下文
- [ ] 切换 ReAct 模式能看到工具调用轨迹
- [ ] 错误状态有 toast，失败文档可重新处理
- [ ] README 能让新人 30 分钟内本地跑起来
