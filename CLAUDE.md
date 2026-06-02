# CLAUDE.md · it-wiki

本文件为后续每次 Claude Code session 提供项目锚点。**第一件事就是读完它**，再读设计文档：

- 设计 spec（权威需求来源）：[docs/superpowers/specs/2026-05-19-it-wiki-agent-design.md](docs/superpowers/specs/2026-05-19-it-wiki-agent-design.md)

如果 spec 与本文件冲突，**spec 为准**；同时请把本文件同步更新。

---

## 1. 这是什么项目

面向团队/企业内部的知识库 Agent，采用 **vibe coding** 方式从零搭建。MVP 阶段单用户、单机 Docker Compose 部署，分 5 个阶段推进（阶段 0~4，详见 spec §7）。

- 后端：Go 1.22+ · chi · Eino · sqlc · goose · river · PostgreSQL 16 + pgvector
- 前端：Next.js 15 · Tailwind v4 · Radix + shadcn/ui · Zustand · TanStack Query
- 对象存储：MinIO（S3 兼容）
- LLM：OpenAI 兼容协议（DeepSeek / Qwen / 百炼 / vLLM 任选）

---

## 2. 当前阶段

> **当前进度**：阶段 1 代码全部完成（后端 + 前端），本地 `go build/vet/test` + `pnpm typecheck/build` 全部通过。剩余：任务 17（docker 端到端 curl）+ 任务 22（验收），需在有 Docker 的服务器环境补做。下一步：部署到服务器跑端到端验收，通过后推进阶段 2（RAG 对话）。

每完成一个阶段，更新这一节，把当前阶段往后推一格。

---

## 3. 锁定的版本（别擅自升级）

升级任何主版本前必须 grep 全仓 + 跑通端到端，并在 PR 描述里写理由。AI 写代码默认按这些版本的 API：

| 组件 | 版本 |
|---|---|
| Go | 1.22+ |
| Next.js | 15.x |
| React | 19.x |
| TypeScript | 5.x |
| Tailwind CSS | v4.x（CSS-first 配置） |
| PostgreSQL | 16.x |
| pgvector | 0.7+ |
| Eino | 最新 release（启动阶段写入 go.mod 后锁定） |

---

## 4. 目录约定

```
it-wiki/
├── backend/                 # Go 服务（详见 spec §3）
├── frontend/                # Next.js 应用（详见 spec §6.1）
├── deploy/
│   └── docker-compose.yml
├── docs/
│   └── superpowers/
│       ├── specs/           # 设计文档（来自 brainstorming）
│       └── plans/           # 实施计划（来自 writing-plans，按阶段拆分）
├── .env.example
├── CLAUDE.md                # 本文件
└── README.md
```

---

## 5. 工作流约定（AI 必须遵守）

### 5.1 数据库改动流程

绝不直接改生产 DB，**永远经过迁移**：

```
1. 在 backend/internal/repo/migrations/ 新建 NNNN_xxx.sql（goose 格式）
2. goose up                                # 本地验证
3. 在 backend/internal/repo/queries/ 添加/修改 .sql 查询
4. sqlc generate                            # 重新生成 Go 代码
5. 修改/新增 service 层代码使用新生成的 query
6. 编译 + 跑相关测试
```

**任何一步都不能跳过**。AI 经常想"直接在 service 里写裸 SQL" — 不允许，除非是 pgvector 特有语法 sqlc 暂不支持的情况，并在 PR 备注理由。

### 5.2 异步任务流程

后台任务一律走 river，**不要起裸 goroutine 跑业务逻辑**：

```
1. 在 backend/internal/worker/ 定义 Job + Worker
2. 在 main.go river client 启动时 AddWorker
3. service 层通过 task_queue port 投递
```

MVP 阶段所有 Job 失败**不自动重试**（spec §5.1 决策）。如果某个新 Job 需要重试，必须在代码注释中写明业务理由。

### 5.3 添加 shadcn 组件

```
cd frontend
pnpm dlx shadcn@latest add <component>
```

shadcn 组件复制到 `components/ui/` 后可以改样式，**不要把 ui/ 组件作为依赖去 import 到 components/ui 之外却又改 ui/ 源码** — 会引起跨组件意外破坏。

### 5.4 添加 Eino tool

```
1. 在 backend/internal/agent/tools/ 新建 xxx.go
2. 实现 eino tool 接口
3. 在 agent/react_agent.go 注册
4. 前端 tool-call-trace.tsx 增加该 tool 的展示分支（可选）
```

---

## 6. AI 协作护栏（重要）

这是 vibe coding 项目，AI 写代码占比高。以下规则**优先级高于 spec 中的具体技术选择**：

### 6.1 绝对不要做

- ❌ **不要 `git commit --amend` 已 push 的提交**：分阶段开发会有多人/多 session 协作，amend 会破坏历史
- ❌ **不要 `git push --force`** 任何分支，尤其是 `main`
- ❌ **不要 `--no-verify`** 跳过钩子
- ❌ **不要混用 GORM**：DB 层定了 sqlc，混 GORM 会让 AI 后续生成的代码风格漂移
- ❌ **不要 `db.AutoMigrate` 或运行时建表**：所有 schema 走 goose migration
- ❌ **不要在 docker-compose 里加未在 spec 中讨论过的服务**（如 Redis、Elasticsearch）— 想加先回到 spec 讨论
- ❌ **不要把 LLM/Embedding/API Key 硬编码**：所有 secret 走 env
- ❌ **不要在 chunks 表里搞软删除**：硬删 + ON DELETE CASCADE，简单可靠

### 6.2 必须做

- ✅ **每次进入 session 先读 CLAUDE.md + 当前阶段的 plan**
- ✅ **改 schema 必须同步改 sqlc queries 并重新 generate**
- ✅ **embedding 维度变更必须重建 chunks 表**（spec §4.1 已经强制校验）
- ✅ **SSE handler 必须监听 `r.Context().Done()`** 在客户端断连时取消 LLM 流（spec §5.4）
- ✅ **所有上传文件先算 sha256 + 查 (kb_id, checksum) 唯一索引去重**（spec §5.1）
- ✅ **错误信息显式**：前端 toast、后端日志带 request_id

### 6.3 不确定时

- 不确定 Eino 某个 API 的写法 → **看官方仓库 examples**，不要凭记忆编
- 不确定 Tailwind v4 配置 → **看 tailwindcss.com v4 文档**，不要套 v3 写法
- 不确定 sqlc 怎么处理 pgvector → 用 `pgvector-go` 提供的 sqlc 适配
- 不确定 shadcn 组件 → `pnpm dlx shadcn@latest add` 再改，别从零写

---

## 7. 常用命令

> ⚠️ 阶段 0 尚未完成，以下命令仅为约定，实际可用以阶段 0 plan 完成后为准。

```bash
# 启动全栈（含 postgres + minio）
docker compose -f deploy/docker-compose.yml up -d

# 后端
cd backend
go run ./cmd/server               # 本地起 backend
goose -dir internal/repo/migrations postgres "$DATABASE_URL" up
sqlc generate                     # 重新生成 DB 代码
go test ./...

# 前端
cd frontend
pnpm install
pnpm dev                          # http://localhost:3000
pnpm build && pnpm start
pnpm lint && pnpm typecheck
```

---

## 8. 环境变量约定

`.env.example` 是真相来源，开发时复制为 `.env`。关键变量：

```env
# Database
DATABASE_URL=postgres://itwiki:itwiki@postgres:5432/itwiki?sslmode=disable

# MinIO (S3 兼容)
S3_ENDPOINT=http://minio:9000
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin     # 生产请改
S3_BUCKET=it-wiki-docs
S3_USE_PATH_STYLE=true       # MinIO 必需

# LLM（OpenAI 兼容）
LLM_BASE_URL=https://api.deepseek.com/v1
LLM_API_KEY=sk-xxxxxxxx
LLM_MODEL=deepseek-chat

# Embedding
EMBEDDING_BASE_URL=https://dashscope.aliyuncs.com/compatible-mode/v1
EMBEDDING_API_KEY=sk-xxxxxxxx
EMBEDDING_MODEL=text-embedding-v3
EMBEDDING_DIM=1024           # 必须与 chunks.embedding 列维度一致

# Server
PORT=8080
LOG_LEVEL=info
```

---

## 9. 决策来源索引

遇到设计层面的疑问，先查这里：

| 问题 | spec 章节 |
|---|---|
| 整体架构 / 技术栈 | §1, §2 |
| 后端目录 + 端口 interface | §3 |
| 数据库 schema | §4 |
| 文档摄入流程 | §5.1 |
| RAG 对话 + SSE 协议 | §5.2, §5.3 |
| 错误边界处理 | §5.4 |
| 前端目录 + 对话页布局 | §6 |
| 阶段划分 + 验收标准 | §7 |
| V1.5 / V2 推迟项（别在 MVP 做） | §8 |
| 已识别风险 + 缓解 | §9 |

---

## 10. 与 Claude 协作的小约定

- 想增改设计：**回到 brainstorming**（让 Claude 进入 brainstorming 模式或更新 spec），不要直接改代码
- 想增改实施步骤：**回到 writing-plans**，更新对应阶段的 plan
- 进入实施阶段：执行当前阶段 plan，**完成后更新本文件第 2 节**

---

_本文件由项目设计阶段建立，每个阶段完成后请同步更新第 2 节当前进度。_
