# IT-Wiki · 阶段 1 实施决策（spec 补充）

> 本文档是 [2026-05-19-it-wiki-agent-design.md](2026-05-19-it-wiki-agent-design.md) 的补充，锁定阶段 1 实施层的 6 个开放决策。
> 原 spec §4 / §5.1 / §7 优先级高于本文档；冲突时**以原 spec 为准**，并同步修订本文档。
>
> 日期：2026-05-20
> 范围：仅阶段 1（文档摄入闭环），不涉及 Phase 2/3/4

---

## D1 · Eino 引入时机

**决策**：Phase 1 用裸 Go interface 定义 ports，**不引入 Eino 依赖**。

- `backend/internal/agent/ports/embedder.go`：
  ```go
  type Embedder interface {
      Embed(ctx context.Context, texts []string) ([][]float32, error)
  }
  ```
- `backend/internal/agent/ports/llm.go`：定义 `LLMClient` 接口但 Phase 1 不实例化（stub 实现可选）
- Phase 2 RAG flow 时再引入 Eino，写适配器把现有 `Embedder` / `LLMClient` 包成 Eino component

**理由**：Phase 1 验收点（上传 MD → ready → 看 chunks）用不到 LLM/RAG；提前引入 Eino 是技术债。

**未来重构面**：仅 `internal/agent/ports/` 几个文件，影响半径小。

---

## D2 · chunks.embedding 列维度模板化

**决策**：用 goose **Go-migration** 在迁移阶段从 `EMBEDDING_DIM` 渲染 `vector(N)` 列。

实现要点：

- 迁移文件后缀 `.go`（例如 `0003_chunks_table.go`），与 `.sql` 迁移共存
- 函数内读 `os.Getenv("EMBEDDING_DIM")` → `fmt.Sprintf("... embedding vector(%d), ...", dim)` → `tx.ExecContext`
- 启动时强校验：
  ```sql
  SELECT atttypmod FROM pg_attribute
   WHERE attrelid = 'chunks'::regclass
     AND attname  = 'embedding'
  ```
  得到的实际列维度必须 == `EMBEDDING_DIM`，否则 panic
- 改维度的唯一合法路径：新写一条迁移 drop chunks → recreate（spec §9 风险栏已确认接受此代价）

**理由**：goose 原生支持 Go-migration，迁移自包含、可回滚；与 SQL 迁移并存无冲突。backend Docker 镜像本就有 Go 编译产物，无额外负担。

---

## D3 · river 系统表迁移

**决策**：river 用自带的 `rivermigrate.Migrate()` 库 API 在 server 启动时跑，goose 只管业务表。

启动顺序（`cmd/server/main.go`）：

```
1. goose up               // 业务表: kbs / docs / chunks / conversations / messages
2. rivermigrate.Migrate() // river 系统表: river_job / river_leader / ...
3. dim 校验               // chunks.embedding 列维度 vs EMBEDDING_DIM
4. EnsureBucket           // MinIO bucket
5. 启动 river worker pool + chi http server
```

- goose 目录 `internal/repo/migrations/` 不包含任何 river 文件
- `rivermigrate.Migrate(ctx, DirectionUp, nil)` 是幂等的，重复启动安全

**理由**：river 官方推荐用法；两套迁移历史互不污染；新人 `make up` 一条命令即可。

---

## D4 · 解析库选型

Phase 1 spec §7 要求"Markdown 完整，PDF/DOCX/XLSX 粗糙能用"。

| 格式 | 库 | 备注 |
|---|---|---|
| Markdown | [yuin/goldmark](https://github.com/yuin/goldmark) | AST walk 抽取 plain text；忽略 frontmatter/HTML inline |
| PDF | [ledongthuc/pdf](https://github.com/ledongthuc/pdf) | pure-Go，MIT；多列 PDF 文字乱序属于"粗糙能用"范畴可接受 |
| DOCX | [sajari/docconv](https://github.com/sajari/docconv) | **先试**；若它需要外部 CLI（`wv` / `tidy` / `libreoffice`），到 Dockerfile 阶段加 `apk add` |
| XLSX | [xuri/excelize](https://github.com/xuri/excelize) | 事实标准；按 sheet 顺序 + 单元格行扫描拼文本 |

**抽象边界**：`backend/internal/agent/parser/` 下每种格式一个文件：

```go
type Parser interface {
    Parse(ctx context.Context, r io.Reader, mime string) (string, error)
}
```

实现按 MIME 路由：`md_parser.go` / `pdf_parser.go` / `docx_parser.go` / `xlsx_parser.go`，一个分发器 `parser.go` 根据文档 mime_type 选择。

**docconv 风险**：解析器实现任务开始时先单测一份 DOCX 看是否需要外部 CLI；如不依赖外部就保留，依赖且镜像增重 > 50 MB 则换 [fumiama/go-docx](https://github.com/fumiama/go-docx) 或自写 80 行 unzip+xml.Decoder。

---

## D5 · Tokenizer

**决策**：[pkoukk/tiktoken-go](https://github.com/pkoukk/tiktoken-go) + `cl100k_base` 编码。

- 启动时初始化一次 `encoding`，全局复用（goroutine-safe）
- `internal/agent/splitter/` 用它做 token 计数（chunk_size=800 token）
- `chunks.token_count` 列由它生成
- Config 预留 `TOKENIZER_ENCODING` env var（默认 `cl100k_base`），未来切到 `o200k_base` 不动代码

**理由**：splitter 的 token 计数需求是"防止超 chunk_size"，精度到个位数无意义。OpenAI 兼容 LLM（DeepSeek / Qwen / 百炼）的 tokenizer 与 cl100k_base 在中文场景下误差 < 15%，对 chunk 切分影响可忽略。

---

## D6 · HTTP API 契约

### 6.1 响应信封 = 裸 JSON + 错误信封

- 成功 2xx：直接返回 resource 或数组
  ```json
  {"id":"...", "name":"工程团队 KB", ...}
  [{...}, {...}]
  ```
- 错误 non-2xx：统一信封
  ```json
  {
    "error": {
      "code": "duplicate_checksum",
      "message": "doc with same checksum already exists in this KB",
      "request_id": "req_abc123",
      "details": {"existing_doc_id": "660e8400-..."}
    }
  }
  ```
- 列表分页元数据走 HTTP header：`X-Total-Count: 42`

**逃生通道**：若将来想要 Java 风格 `{code,msg,data}` 统一信封，加一层 chi middleware 拦截 ResponseWriter 即可，handler 代码无需改动。

### 6.2 分页

- querystring：`?limit=20&offset=0`
- 默认 `limit=20`，最大 `limit=100`（超过 400 `validation_failed`）
- 适用端点：`GET /kbs`、`GET /kbs/:id/docs`、`GET /docs/:id/chunks`

### 6.3 错误码常量

| code | HTTP | 含义 |
|---|---|---|
| `validation_failed` | 400 | 请求体字段不合法 / querystring 超限 |
| `kb_not_found` | 404 | KB id 不存在 |
| `doc_not_found` | 404 | doc id 不存在 |
| `duplicate_checksum` | 409 | 同 KB 已存在相同 sha256 |
| `payload_too_large` | 413 | 上传 > 50 MB |
| `unsupported_media_type` | 415 | mime_type 不在白名单 |
| `internal_error` | 500 | 后端未捕获异常 |

`request_id` 来源：chi middleware.RequestID 注入到 `X-Request-Id` header 并写入 error envelope。

### 6.4 数据格式

- 时间戳：RFC3339（`2026-05-20T15:30:00Z`），Go `time.Time` 默认 JSON 序列化即此格式
- UUID：JSON 字符串小写（`550e8400-e29b-41d4-a716-446655440000`），路径参数大小写不敏感
- 二进制：仅 `POST /kbs/:id/docs` 走 `multipart/form-data`，其余端点 JSON
- 编码：UTF-8

### 6.5 关键端点 request/response（细节由 writing-plans 完善）

**POST /api/v1/kbs** — 创建 KB
- Request: `{"name": "...", "description": "..."}` (description 可选)
- Response 201: 完整 KB 资源（含 server 注入的 `embed_model` / `embed_dim` / `created_at`）

**POST /api/v1/kbs/:id/docs** — 上传文档（multipart）
- Form: `file` (required)
- Response 201: 新 doc 资源（`status="pending"`）
- Response 409 duplicate_checksum: error envelope + `details.existing_doc_id`

**GET /api/v1/docs/:id/chunks?limit=20&offset=0**
- Response 200 + `X-Total-Count` header
- Body: chunks 数组（**不返回 embedding 向量**，太大；要看向量另起调试端点）

其余端点 schema 在 writing-plans 阶段细化。

---

## 决策总览

| ID | 决策摘要 |
|---|---|
| D1 | Phase 1 用裸 Go interface，Eino 推到 Phase 2 |
| D2 | goose Go-migration 渲染 chunks `vector(N)` 维度，启动强校验 |
| D3 | river 用 `rivermigrate.Migrate()` 在 server 启动时跑，goose 只管业务表 |
| D4 | Parser: goldmark / ledongthuc-pdf / **sajari-docconv（先试）** / xuri-excelize |
| D5 | Tokenizer: tiktoken-go + cl100k_base，预留 `TOKENIZER_ENCODING` env |
| D6 | HTTP: 裸 JSON + 错误信封 + limit/offset 分页 + RFC3339 + 小写 UUID |

---

## 下一步

用 `writing-plans` skill 生成阶段 1 实施计划，落到 `docs/superpowers/plans/2026-05-20-phase-1-ingestion-plan.md`。

---

_本文档于 2026-05-20 在 brainstorming session 中锁定。如阶段 1 实施过程中决策需要调整，请回到 brainstorming 更新本文件。_