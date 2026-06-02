# 阶段 0 · 工程脚手架 · 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 搭建 it-wiki 项目的工程骨架：`docker compose up` 后能看到前端首页、后端 `/healthz` 返回 200、MinIO Console 可登录、bucket 已存在；所有工具链（goose / sqlc / pnpm / docker）就绪等待阶段 1 使用。

**Architecture:** 三层物理布局（backend / frontend / deploy），单容器单职责，docker-compose 编排；postgres 与 minio 为基础设施，backend Go 服务通过 entrypoint 内的指数退避等待 minio 就绪后建 bucket；frontend 是 Next.js 15 standalone 输出。

**Tech Stack:** Go 1.22 + chi · aws-sdk-go-v2 (S3 client → MinIO) · cenkalti/backoff · pgvector/pgvector · pressly/goose · sqlc · Next.js 15 + React 19 + TypeScript 5 + Tailwind CSS v4 + shadcn/ui · MinIO RELEASE.2024-* · PostgreSQL 16

---

## 重要约定

1. **本阶段不做 git commit**：用户决定立项前不入库（私有公司托管库未就绪）。每个任务的最后一步是"完成本地验证 + 在本文件勾选 checkbox"，**不要** `git add` / `git commit`。立项后由用户一次性追溯入库。
2. **版本敏感片段**：Tailwind v4、Next.js 15、shadcn 都是 2025 年内有破坏性变化的栈。任务中标注 ⚠️ 的步骤要求在执行时**核对官方文档当前页**而不是凭记忆补。
3. **PowerShell 优先**：开发环境是 Windows + PowerShell 7。docker 命令在 Bash 和 PowerShell 都通用；shell 脚本（`init-bucket.sh`）只在 Linux 容器内跑。
4. **不要扩展范围**：阶段 0 只做"能跑"，业务表 / API / 前端页面统统留给阶段 1。看到 spec 里 `kb_handler.go` 之类的目录请**不要创建**，免得脚手架阶段长出半成品代码。

---

## 文件清单

阶段 0 完成后项目根的文件结构（**仅列阶段 0 涉及的**）：

```
it-wiki/
├── .env.example                              # 任务 1
├── .gitignore                                # 已存在
├── CLAUDE.md                                 # 已存在
├── Makefile                                  # 任务 11
├── README.md                                 # 任务 11
├── backend/
│   ├── .dockerignore                         # 任务 5
│   ├── Dockerfile                            # 任务 5
│   ├── Makefile                              # 任务 4
│   ├── go.mod                                # 任务 2
│   ├── go.sum                                # 任务 2 (tidy 生成)
│   ├── sqlc.yaml                             # 任务 4
│   ├── cmd/server/main.go                    # 任务 2 + 3
│   └── internal/
│       ├── config/config.go                  # 任务 2
│       ├── http/router.go                    # 任务 2
│       ├── infra/storage/minio.go            # 任务 3
│       └── repo/
│           ├── queries/.gitkeep              # 任务 4
│           └── migrations/.gitkeep           # 任务 4
├── frontend/
│   ├── .dockerignore                         # 任务 9
│   ├── .gitignore                            # 任务 6
│   ├── Dockerfile                            # 任务 9
│   ├── components.json                       # 任务 8
│   ├── next.config.ts                        # 任务 6
│   ├── package.json                          # 任务 6
│   ├── postcss.config.mjs                    # 任务 7
│   ├── tsconfig.json                         # 任务 6
│   ├── app/
│   │   ├── globals.css                       # 任务 7
│   │   ├── layout.tsx                        # 任务 6
│   │   └── page.tsx                          # 任务 6 / 任务 8 更新
│   ├── components/ui/button.tsx              # 任务 8
│   └── lib/utils.ts                          # 任务 8
└── deploy/
    ├── docker-compose.yml                    # 任务 10
    └── init/init-bucket.sh                   # 任务 10
```

---

## 任务 1：建立顶层目录结构 + `.env.example`

**Files:**
- Create: `e:\GoProject\it-wiki\.env.example`
- Create: `e:\GoProject\it-wiki\backend\` (目录)
- Create: `e:\GoProject\it-wiki\frontend\` (目录)
- Create: `e:\GoProject\it-wiki\deploy\` (目录)
- Create: `e:\GoProject\it-wiki\deploy\init\` (目录)

- [ ] **Step 1.1：创建目录**

PowerShell 命令：

```powershell
New-Item -ItemType Directory -Force `
  "e:\GoProject\it-wiki\backend", `
  "e:\GoProject\it-wiki\frontend", `
  "e:\GoProject\it-wiki\deploy\init"
```

- [ ] **Step 1.2：写 `.env.example`**

把以下内容**完整**写入 `e:\GoProject\it-wiki\.env.example`：

```env
# =============================================================
# it-wiki 环境变量模板
# 用法: 复制本文件为 .env, 替换标记 [REPLACE_ME] 的值
# .env 已在根 .gitignore 中, 不会入库
# =============================================================

# ----- PostgreSQL -----
POSTGRES_USER=itwiki
POSTGRES_PASSWORD=itwiki_dev_pass
POSTGRES_DB=itwiki
# 容器内连接串(给 backend 容器用)
DATABASE_URL=postgres://itwiki:itwiki_dev_pass@postgres:5432/itwiki?sslmode=disable

# ----- MinIO -----
MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=minioadmin_dev_pass
# 容器内 endpoint(给 backend 用)
S3_ENDPOINT=http://minio:9000
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin_dev_pass
S3_BUCKET=it-wiki-docs
S3_REGION=us-east-1
S3_USE_PATH_STYLE=true

# ----- LLM (OpenAI 兼容) -----
LLM_BASE_URL=https://api.deepseek.com/v1
LLM_API_KEY=[REPLACE_ME]
LLM_MODEL=deepseek-chat

# ----- Embedding (OpenAI 兼容) -----
EMBEDDING_BASE_URL=https://dashscope.aliyuncs.com/compatible-mode/v1
EMBEDDING_API_KEY=[REPLACE_ME]
EMBEDDING_MODEL=text-embedding-v3
EMBEDDING_DIM=1024

# ----- Backend -----
PORT=8080
LOG_LEVEL=info

# ----- Frontend -----
# 浏览器侧调用后端的地址(开发期 docker compose 直连 backend:8080,
# 但浏览器跑在宿主机, 所以用 localhost:8080)
NEXT_PUBLIC_API_BASE_URL=http://localhost:8080
```

- [ ] **Step 1.3：验证**

```powershell
Test-Path "e:\GoProject\it-wiki\.env.example"
Get-ChildItem "e:\GoProject\it-wiki" -Directory | Select-Object Name
```

期望输出包含 `True` 和 `backend`、`deploy`、`docs`、`frontend` 四个目录名。

- [ ] **Step 1.4：复制 .env.example → .env 供后续任务使用**

```powershell
Copy-Item "e:\GoProject\it-wiki\.env.example" "e:\GoProject\it-wiki\.env"
```

注意：`.env` 在 `.gitignore` 中，不会被入库。

- [ ] **Step 1.5：完成检查**

任务 1 完成。在本文件勾选 Step 1.1~1.5。**不要执行 git commit。**

---

## 任务 2：后端 Go module + chi + /healthz

**Files:**
- Create: `e:\GoProject\it-wiki\backend\go.mod`
- Create: `e:\GoProject\it-wiki\backend\cmd\server\main.go`
- Create: `e:\GoProject\it-wiki\backend\internal\config\config.go`
- Create: `e:\GoProject\it-wiki\backend\internal\http\router.go`

- [ ] **Step 2.1：创建后端子目录**

```powershell
New-Item -ItemType Directory -Force `
  "e:\GoProject\it-wiki\backend\cmd\server", `
  "e:\GoProject\it-wiki\backend\internal\config", `
  "e:\GoProject\it-wiki\backend\internal\http", `
  "e:\GoProject\it-wiki\backend\internal\infra\storage", `
  "e:\GoProject\it-wiki\backend\internal\repo\queries", `
  "e:\GoProject\it-wiki\backend\internal\repo\migrations"
```

- [ ] **Step 2.2：初始化 Go module**

工作目录切到 backend：

```powershell
cd e:\GoProject\it-wiki\backend
go mod init github.com/zenith-wang/it-wiki/backend
```

> ⚠️ module path 后期入库后改企业域名时要全局替换。先用占位 `github.com/zenith-wang/it-wiki/backend`。

- [ ] **Step 2.3：写 `internal/config/config.go`**

```go
package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port     string
	LogLevel string

	DatabaseURL string

	S3Endpoint     string
	S3AccessKey    string
	S3SecretKey    string
	S3Bucket       string
	S3Region       string
	S3UsePathStyle bool

	LLMBaseURL string
	LLMAPIKey  string
	LLMModel   string

	EmbeddingBaseURL string
	EmbeddingAPIKey  string
	EmbeddingModel   string
	EmbeddingDim     int
}

func Load() (*Config, error) {
	dim, err := strconv.Atoi(getEnv("EMBEDDING_DIM", "1024"))
	if err != nil {
		return nil, fmt.Errorf("invalid EMBEDDING_DIM: %w", err)
	}
	usePathStyle, _ := strconv.ParseBool(getEnv("S3_USE_PATH_STYLE", "true"))

	cfg := &Config{
		Port:             getEnv("PORT", "8080"),
		LogLevel:         getEnv("LOG_LEVEL", "info"),
		DatabaseURL:      mustEnv("DATABASE_URL"),
		S3Endpoint:       mustEnv("S3_ENDPOINT"),
		S3AccessKey:      mustEnv("S3_ACCESS_KEY"),
		S3SecretKey:      mustEnv("S3_SECRET_KEY"),
		S3Bucket:         mustEnv("S3_BUCKET"),
		S3Region:         getEnv("S3_REGION", "us-east-1"),
		S3UsePathStyle:   usePathStyle,
		LLMBaseURL:       getEnv("LLM_BASE_URL", ""),
		LLMAPIKey:        getEnv("LLM_API_KEY", ""),
		LLMModel:         getEnv("LLM_MODEL", ""),
		EmbeddingBaseURL: getEnv("EMBEDDING_BASE_URL", ""),
		EmbeddingAPIKey:  getEnv("EMBEDDING_API_KEY", ""),
		EmbeddingModel:   getEnv("EMBEDDING_MODEL", ""),
		EmbeddingDim:     dim,
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("missing required env var: %s", key))
	}
	return v
}
```

- [ ] **Step 2.4：写 `internal/http/router.go`**

```go
package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/healthz", healthz)
	r.Get("/api/healthz", healthz)

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

- [ ] **Step 2.5：写 `cmd/server/main.go`**

```go
package main

import (
	"context"
	"errors"
	stdhttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/config"
	httpx "github.com/zenith-wang/it-wiki/backend/internal/http"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	srv := &stdhttp.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpx.NewRouter(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		os.Stdout.WriteString("[server] listening on :" + cfg.Port + "\n")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			panic(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	os.Stdout.WriteString("[server] shutting down\n")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
```

- [ ] **Step 2.6：拉依赖**

```powershell
cd e:\GoProject\it-wiki\backend
go get github.com/go-chi/chi/v5@latest
go mod tidy
```

期望：`go.mod` 中出现 `github.com/go-chi/chi/v5`，`go.sum` 已生成。

- [ ] **Step 2.7：本地起后端验证**

需要 `.env` 中的 `S3_*` / `DATABASE_URL` 已填（占位也行，本步不连真服务）。在 PowerShell 中加载 .env 并运行：

```powershell
cd e:\GoProject\it-wiki\backend
# 简易加载 .env (PowerShell)
Get-Content "e:\GoProject\it-wiki\.env" | ForEach-Object {
  if ($_ -match '^([A-Z_]+)=(.*)$') {
    [System.Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process')
  }
}
go run ./cmd/server
```

另开 PowerShell：

```powershell
curl http://localhost:8080/healthz
```

期望响应：`{"status":"ok","time":"2026-..."}`。

按 `Ctrl+C` 停 server。

- [ ] **Step 2.8：完成检查**

任务 2 完成，勾选 Step 2.1~2.8。

---

## 任务 3：后端 MinIO `EnsureBucket`（aws-sdk-go-v2 + backoff）

**Files:**
- Create: `e:\GoProject\it-wiki\backend\internal\infra\storage\minio.go`
- Modify: `e:\GoProject\it-wiki\backend\cmd\server\main.go`

⚠️ aws-sdk-go-v2 的 endpoint resolver API 在 v2 内部经历过两次大变化（`EndpointResolver` → `EndpointResolverV2` → `BaseEndpoint` 字段）。**写完后必须用 `go build ./...` 验证编译通过**，否则按官方迁移指南调整。

- [ ] **Step 3.1：拉依赖**

```powershell
cd e:\GoProject\it-wiki\backend
go get github.com/aws/aws-sdk-go-v2/config@latest
go get github.com/aws/aws-sdk-go-v2/credentials@latest
go get github.com/aws/aws-sdk-go-v2/service/s3@latest
go get github.com/cenkalti/backoff/v4@latest
go mod tidy
```

- [ ] **Step 3.2：写 `internal/infra/storage/minio.go`**

```go
package storage

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/cenkalti/backoff/v4"
)

type MinioConfig struct {
	Endpoint     string
	AccessKey    string
	SecretKey    string
	Region       string
	Bucket       string
	UsePathStyle bool
}

type MinioClient struct {
	cfg    MinioConfig
	client *s3.Client
}

func NewMinioClient(ctx context.Context, mc MinioConfig) (*MinioClient, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(mc.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(mc.AccessKey, mc.SecretKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("aws config load: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(mc.Endpoint)
		o.UsePathStyle = mc.UsePathStyle
	})

	return &MinioClient{cfg: mc, client: client}, nil
}

// EnsureBucket waits for MinIO readiness with exponential backoff,
// then creates the bucket if missing. Idempotent.
func (m *MinioClient) EnsureBucket(ctx context.Context) error {
	op := func() error {
		_, err := m.client.HeadBucket(ctx, &s3.HeadBucketInput{
			Bucket: aws.String(m.cfg.Bucket),
		})
		if err == nil {
			return nil
		}

		var notFound *s3types.NotFound
		if errors.As(err, &notFound) {
			log.Printf("[minio] bucket %q not found, creating...", m.cfg.Bucket)
			_, cerr := m.client.CreateBucket(ctx, &s3.CreateBucketInput{
				Bucket: aws.String(m.cfg.Bucket),
			})
			if cerr != nil {
				return fmt.Errorf("create bucket: %w", cerr)
			}
			log.Printf("[minio] bucket %q created", m.cfg.Bucket)
			return nil
		}

		// 通用网络错误 → 让 backoff 重试
		return fmt.Errorf("head bucket: %w", err)
	}

	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 500 * time.Millisecond
	bo.MaxInterval = 5 * time.Second
	bo.MaxElapsedTime = 60 * time.Second

	return backoff.Retry(op, backoff.WithContext(bo, ctx))
}

func (m *MinioClient) Client() *s3.Client { return m.client }
```

- [ ] **Step 3.3：修改 `cmd/server/main.go` 在启动时调用 EnsureBucket**

把整个文件替换为：

```go
package main

import (
	"context"
	"errors"
	"log"
	stdhttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/config"
	httpx "github.com/zenith-wang/it-wiki/backend/internal/http"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[main] load config: %v", err)
	}

	bootCtx, bootCancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer bootCancel()

	mc, err := storage.NewMinioClient(bootCtx, storage.MinioConfig{
		Endpoint:     cfg.S3Endpoint,
		AccessKey:    cfg.S3AccessKey,
		SecretKey:    cfg.S3SecretKey,
		Region:       cfg.S3Region,
		Bucket:       cfg.S3Bucket,
		UsePathStyle: cfg.S3UsePathStyle,
	})
	if err != nil {
		log.Fatalf("[main] new minio client: %v", err)
	}
	if err := mc.EnsureBucket(bootCtx); err != nil {
		log.Fatalf("[main] ensure bucket: %v", err)
	}
	log.Printf("[main] minio bucket %q ready", cfg.S3Bucket)

	srv := &stdhttp.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpx.NewRouter(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("[server] listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			log.Fatalf("[server] listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Printf("[server] shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
```

- [ ] **Step 3.4：编译验证（不连 MinIO，仅确认编译通过）**

```powershell
cd e:\GoProject\it-wiki\backend
go build ./...
```

期望：无报错输出。如有报错且关键词是 `BaseEndpoint` 或 `EndpointResolver`，说明 aws-sdk-go-v2 版本 API 有差异，按 https://aws.github.io/aws-sdk-go-v2/docs/migrating/v1-to-v2/ 调整。

- [ ] **Step 3.5：完成检查**

任务 3 完成，勾选 Step 3.1~3.5。

> 注：本任务暂不做"真的连 MinIO"验证（要等任务 10 docker-compose 起来）。任务 10 端到端跑通时会顺带验证 EnsureBucket。

---

## 任务 4：后端 sqlc.yaml + 占位目录 + backend Makefile

**Files:**
- Create: `e:\GoProject\it-wiki\backend\sqlc.yaml`
- Create: `e:\GoProject\it-wiki\backend\internal\repo\queries\.gitkeep`
- Create: `e:\GoProject\it-wiki\backend\internal\repo\migrations\.gitkeep`
- Create: `e:\GoProject\it-wiki\backend\Makefile`

阶段 0 不写任何 migration 和 query，但 sqlc.yaml 和目录占位先就位，阶段 1 可立即 `make sqlc-gen`。

- [ ] **Step 4.1：写 `backend/sqlc.yaml`**

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
          # pgvector 适配在阶段 1 用到时再启用 pgvector/pgvector-go 的 sqlc plugin
```

> ⚠️ pgvector 的 sqlc 适配（[pgvector-go](https://github.com/pgvector/pgvector-go)）在阶段 1 添加 chunks 表时再启用，避免现在配错。

- [ ] **Step 4.2：建占位文件**

```powershell
New-Item -ItemType File -Path "e:\GoProject\it-wiki\backend\internal\repo\queries\.gitkeep" -Force | Out-Null
New-Item -ItemType File -Path "e:\GoProject\it-wiki\backend\internal\repo\migrations\.gitkeep" -Force | Out-Null
```

- [ ] **Step 4.3：写 `backend/Makefile`**

```makefile
# backend/Makefile
# 在 backend/ 目录下运行 make <target>
.PHONY: help build run test tidy sqlc-gen migrate-up migrate-down migrate-status migrate-new tools

DB_URL ?= $(DATABASE_URL)
MIGRATIONS_DIR := internal/repo/migrations

help: ## 显示可用命令
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?##/ { printf "  %-20s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

tools: ## 安装本地开发工具(goose / sqlc)
	go install github.com/pressly/goose/v3/cmd/goose@latest
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

build: ## 编译 server 二进制
	go build -o bin/server ./cmd/server

run: ## 本地运行 server (需先加载 .env)
	go run ./cmd/server

test: ## 跑所有测试
	go test ./...

tidy: ## go mod tidy
	go mod tidy

sqlc-gen: ## 根据 queries/*.sql + migrations/*.sql 生成 Go 代码
	sqlc generate

migrate-up: ## 应用所有迁移
	goose -dir $(MIGRATIONS_DIR) postgres "$(DB_URL)" up

migrate-down: ## 回滚一步迁移
	goose -dir $(MIGRATIONS_DIR) postgres "$(DB_URL)" down

migrate-status: ## 查看迁移状态
	goose -dir $(MIGRATIONS_DIR) postgres "$(DB_URL)" status

migrate-new: ## 新建一个迁移文件 (用法: make migrate-new NAME=create_kbs)
	@test -n "$(NAME)" || (echo "usage: make migrate-new NAME=xxx"; exit 1)
	goose -dir $(MIGRATIONS_DIR) create $(NAME) sql
```

- [ ] **Step 4.4：安装工具链（Windows 上）**

```powershell
cd e:\GoProject\it-wiki\backend
go install github.com/pressly/goose/v3/cmd/goose@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

把 `%USERPROFILE%\go\bin` 加入 PATH（若尚未加入）。

验证：

```powershell
goose --version
sqlc version
```

期望各自输出版本号。

- [ ] **Step 4.5：执行一次空 sqlc generate 验证配置**

```powershell
cd e:\GoProject\it-wiki\backend
sqlc generate
```

期望输出：`(no queries)` 或类似无错误信息（因为 queries/ 是空的）。**有报错说明 sqlc.yaml 写错了，按报错信息修。**

- [ ] **Step 4.6：完成检查**

任务 4 完成，勾选 Step 4.1~4.6。

---

## 任务 5：后端 Dockerfile（multi-stage）

**Files:**
- Create: `e:\GoProject\it-wiki\backend\Dockerfile`
- Create: `e:\GoProject\it-wiki\backend\.dockerignore`

- [ ] **Step 5.1：写 `backend/.dockerignore`**

```
bin/
dist/
*.test
*.out
.env*
!.env.example
vendor/
.git/
.idea/
.vscode/
```

- [ ] **Step 5.2：写 `backend/Dockerfile`**

```dockerfile
# syntax=docker/dockerfile:1.7

# ---------- builder ----------
FROM golang:1.22-alpine AS builder

WORKDIR /app

# 利用 layer 缓存先拉依赖
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 编译静态二进制
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" \
    -o /out/server ./cmd/server

# ---------- runtime ----------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && adduser -S app -G app

WORKDIR /app
COPY --from=builder /out/server /app/server

USER app
EXPOSE 8080

ENTRYPOINT ["/app/server"]
```

- [ ] **Step 5.3：本地 build 验证**

```powershell
cd e:\GoProject\it-wiki\backend
docker build -t it-wiki-backend:dev .
```

期望最后输出 `=> => writing image sha256:...` + `=> => naming to it-wiki-backend:dev`。

- [ ] **Step 5.4：完成检查**

任务 5 完成，勾选 Step 5.1~5.4。

---

## 任务 6：前端 Next.js 15 + TypeScript + 基础页面

**Files:**
- Create: `e:\GoProject\it-wiki\frontend\package.json`
- Create: `e:\GoProject\it-wiki\frontend\tsconfig.json`
- Create: `e:\GoProject\it-wiki\frontend\next.config.ts`
- Create: `e:\GoProject\it-wiki\frontend\.gitignore`
- Create: `e:\GoProject\it-wiki\frontend\app\layout.tsx`
- Create: `e:\GoProject\it-wiki\frontend\app\page.tsx`

⚠️ Next.js 15 + React 19 + TS 配置在 2025 年内有变化。下面给出的版本号在写本 plan 时是当前 latest 的近似值，**`pnpm install` 后用 `pnpm list` 核对实际拉到的版本，如果实际版本与下面不一致请更新 package.json**。

- [ ] **Step 6.1：建子目录**

```powershell
New-Item -ItemType Directory -Force `
  "e:\GoProject\it-wiki\frontend\app", `
  "e:\GoProject\it-wiki\frontend\components\ui", `
  "e:\GoProject\it-wiki\frontend\lib"
```

- [ ] **Step 6.2：写 `frontend/package.json`**

```json
{
  "name": "it-wiki-frontend",
  "version": "0.0.1",
  "private": true,
  "scripts": {
    "dev": "next dev",
    "build": "next build",
    "start": "next start -p 3000",
    "lint": "next lint",
    "typecheck": "tsc --noEmit"
  },
  "dependencies": {
    "next": "^15.0.0",
    "react": "^19.0.0",
    "react-dom": "^19.0.0"
  },
  "devDependencies": {
    "@types/node": "^22.0.0",
    "@types/react": "^19.0.0",
    "@types/react-dom": "^19.0.0",
    "typescript": "^5.6.0",
    "eslint": "^9.0.0",
    "eslint-config-next": "^15.0.0"
  },
  "packageManager": "pnpm@9.12.0"
}
```

- [ ] **Step 6.3：写 `frontend/tsconfig.json`**

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["dom", "dom.iterable", "esnext"],
    "allowJs": true,
    "skipLibCheck": true,
    "strict": true,
    "noEmit": true,
    "esModuleInterop": true,
    "module": "esnext",
    "moduleResolution": "bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "jsx": "preserve",
    "incremental": true,
    "plugins": [{ "name": "next" }],
    "paths": { "@/*": ["./*"] }
  },
  "include": ["next-env.d.ts", "**/*.ts", "**/*.tsx", ".next/types/**/*.ts"],
  "exclude": ["node_modules"]
}
```

- [ ] **Step 6.4：写 `frontend/next.config.ts`**

```ts
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone", // 让 Docker 镜像极小
  reactStrictMode: true,
};

export default nextConfig;
```

- [ ] **Step 6.5：写 `frontend/.gitignore`**

```
node_modules
.next
out
build
.env*.local
*.tsbuildinfo
next-env.d.ts
.turbo
```

- [ ] **Step 6.6：写 `frontend/app/layout.tsx`**

```tsx
import type { Metadata } from "next";
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
        {children}
      </body>
    </html>
  );
}
```

- [ ] **Step 6.7：写 `frontend/app/page.tsx`（占位首页，任务 8 后用 shadcn 组件升级）**

```tsx
export default function Home() {
  return (
    <main className="flex min-h-screen items-center justify-center p-8">
      <div className="text-center space-y-3">
        <h1 className="text-4xl font-bold tracking-tight">it-wiki</h1>
        <p className="text-muted-foreground">
          团队知识库 Agent · 阶段 0 脚手架就绪
        </p>
      </div>
    </main>
  );
}
```

- [ ] **Step 6.8：安装依赖**

```powershell
cd e:\GoProject\it-wiki\frontend
pnpm install
```

如果未安装 pnpm：`npm install -g pnpm`，或用 `corepack enable && corepack prepare pnpm@9 --activate`。

- [ ] **Step 6.9：完成检查**

`pnpm dev` 暂不运行（globals.css 还没写，会报错）。任务 6 完成，勾选 Step 6.1~6.9。

---

## 任务 7：Tailwind CSS v4 配置

**Files:**
- Create: `e:\GoProject\it-wiki\frontend\postcss.config.mjs`
- Create: `e:\GoProject\it-wiki\frontend\app\globals.css`
- Modify: `e:\GoProject\it-wiki\frontend\package.json`

⚠️ **关键版本注意**：Tailwind CSS v4 在 2025 年 1 月发布，与 v3 配置方式**完全不同**——CSS-first，不需要 `tailwind.config.js`，PostCSS 插件名是 `@tailwindcss/postcss`。如果执行时发现下面的写法不对（例如官方又改了 PostCSS 插件名），先查 https://tailwindcss.com 当前的 "Get Started with Next.js" 指南。

- [ ] **Step 7.1：安装 Tailwind v4 + PostCSS 插件**

```powershell
cd e:\GoProject\it-wiki\frontend
pnpm add -D tailwindcss @tailwindcss/postcss postcss
```

- [ ] **Step 7.2：写 `frontend/postcss.config.mjs`**

```js
export default {
  plugins: {
    "@tailwindcss/postcss": {},
  },
};
```

- [ ] **Step 7.3：写 `frontend/app/globals.css`**

```css
@import "tailwindcss";

/* shadcn/ui 颜色变量 (任务 8 init 后会被覆盖；先放占位) */
@layer base {
  :root {
    --background: 0 0% 100%;
    --foreground: 222.2 84% 4.9%;
    --muted: 210 40% 96.1%;
    --muted-foreground: 215.4 16.3% 46.9%;
    --border: 214.3 31.8% 91.4%;
    --ring: 222.2 84% 4.9%;
    --radius: 0.5rem;
  }

  @media (prefers-color-scheme: dark) {
    :root {
      --background: 222.2 84% 4.9%;
      --foreground: 210 40% 98%;
      --muted: 217.2 32.6% 17.5%;
      --muted-foreground: 215 20.2% 65.1%;
      --border: 217.2 32.6% 17.5%;
      --ring: 212.7 26.8% 83.9%;
    }
  }
}

@theme {
  --color-background: hsl(var(--background));
  --color-foreground: hsl(var(--foreground));
  --color-muted: hsl(var(--muted));
  --color-muted-foreground: hsl(var(--muted-foreground));
  --color-border: hsl(var(--border));
  --color-ring: hsl(var(--ring));
  --radius-md: var(--radius);
}
```

- [ ] **Step 7.4：起 dev server 验证**

```powershell
cd e:\GoProject\it-wiki\frontend
pnpm dev
```

浏览器访问 `http://localhost:3000`，期望看到：

- 居中显示 "it-wiki" 大标题
- 副标题 "团队知识库 Agent · 阶段 0 脚手架就绪"
- 跟随系统主题（暗色模式有效）

`Ctrl+C` 停 dev server。

- [ ] **Step 7.5：完成检查**

任务 7 完成，勾选 Step 7.1~7.5。

---

## 任务 8：shadcn/ui 初始化 + Button 组件

**Files:**
- Create: `e:\GoProject\it-wiki\frontend\components.json`
- Create: `e:\GoProject\it-wiki\frontend\lib\utils.ts`
- Create: `e:\GoProject\it-wiki\frontend\components\ui\button.tsx`
- Modify: `e:\GoProject\it-wiki\frontend\app\page.tsx`
- Modify: `e:\GoProject\it-wiki\frontend\app\globals.css`

⚠️ shadcn 适配 Tailwind v4 的方式在 2025 年内多次调整。**推荐路径**：跑 `pnpm dlx shadcn@latest init`，让它自己探测 Tailwind 版本并写好 `components.json` + `globals.css`。如果该命令交互问的选项与下面不同，按下面选项的"意图"回答（如 style=new-york、css variables=yes）。

- [ ] **Step 8.1：跑 shadcn init**

```powershell
cd e:\GoProject\it-wiki\frontend
pnpm dlx shadcn@latest init
```

交互选项（按 ↑/↓ 选择 + Enter）：

| 问题 | 选 |
|---|---|
| Which style would you like to use? | New York |
| Which color would you like to use as base color? | Slate |
| Would you like to use CSS variables for colors? | Yes |

执行完后期望生成 `components.json`、覆盖 `app/globals.css` 中的颜色变量、`lib/utils.ts` 创建好。

- [ ] **Step 8.2：核对 `components.json` 内容**

期望大致是：

```json
{
  "$schema": "https://ui.shadcn.com/schema.json",
  "style": "new-york",
  "rsc": true,
  "tsx": true,
  "tailwind": {
    "config": "",
    "css": "app/globals.css",
    "baseColor": "slate",
    "cssVariables": true,
    "prefix": ""
  },
  "aliases": {
    "components": "@/components",
    "utils": "@/lib/utils",
    "ui": "@/components/ui",
    "lib": "@/lib",
    "hooks": "@/hooks"
  },
  "iconLibrary": "lucide"
}
```

如果不一致，**保留 shadcn init 生成的版本**（它知道当前 Tailwind 版本兼容方式）。

- [ ] **Step 8.3：添加 Button 组件**

```powershell
cd e:\GoProject\it-wiki\frontend
pnpm dlx shadcn@latest add button
```

期望生成 `components/ui/button.tsx` + 依赖（`class-variance-authority`, `clsx`, `tailwind-merge`, `lucide-react` 等自动 install）。

- [ ] **Step 8.4：修改 `app/page.tsx` 用上 Button 验证**

```tsx
import { Button } from "@/components/ui/button";

export default function Home() {
  return (
    <main className="flex min-h-screen items-center justify-center p-8">
      <div className="text-center space-y-6">
        <h1 className="text-4xl font-bold tracking-tight">it-wiki</h1>
        <p className="text-muted-foreground">
          团队知识库 Agent · 阶段 0 脚手架就绪
        </p>
        <div className="flex justify-center gap-3">
          <Button>主按钮</Button>
          <Button variant="outline">次按钮</Button>
          <Button variant="ghost">幽灵按钮</Button>
        </div>
      </div>
    </main>
  );
}
```

- [ ] **Step 8.5：起 dev server 验证**

```powershell
cd e:\GoProject\it-wiki\frontend
pnpm dev
```

浏览器访问 `http://localhost:3000`，期望看到三个不同样式的按钮，hover 有视觉反馈。`Ctrl+C` 停。

- [ ] **Step 8.6：跑 typecheck**

```powershell
cd e:\GoProject\it-wiki\frontend
pnpm typecheck
```

期望无报错输出。

- [ ] **Step 8.7：完成检查**

任务 8 完成，勾选 Step 8.1~8.7。

---

## 任务 9：前端 Dockerfile

**Files:**
- Create: `e:\GoProject\it-wiki\frontend\Dockerfile`
- Create: `e:\GoProject\it-wiki\frontend\.dockerignore`

- [ ] **Step 9.1：写 `frontend/.dockerignore`**

```
node_modules
.next
out
build
.env*.local
.git
.idea
.vscode
*.tsbuildinfo
```

- [ ] **Step 9.2：写 `frontend/Dockerfile`**

```dockerfile
# syntax=docker/dockerfile:1.7

# ---------- deps ----------
FROM node:20-alpine AS deps
WORKDIR /app
RUN corepack enable && corepack prepare pnpm@9 --activate
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

# ---------- builder ----------
FROM node:20-alpine AS builder
WORKDIR /app
RUN corepack enable && corepack prepare pnpm@9 --activate
COPY --from=deps /app/node_modules ./node_modules
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
RUN pnpm build

# ---------- runner ----------
FROM node:20-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
RUN addgroup -S app && adduser -S app -G app

COPY --from=builder --chown=app:app /app/public ./public
COPY --from=builder --chown=app:app /app/.next/standalone ./
COPY --from=builder --chown=app:app /app/.next/static ./.next/static

USER app
EXPOSE 3000
ENV PORT=3000
ENV HOSTNAME=0.0.0.0

CMD ["node", "server.js"]
```

> 注：依赖 `next.config.ts` 中的 `output: "standalone"`（任务 6 已配置）。

- [ ] **Step 9.3：build 验证**

```powershell
cd e:\GoProject\it-wiki\frontend
docker build -t it-wiki-frontend:dev .
```

期望最后输出 `naming to it-wiki-frontend:dev`。

- [ ] **Step 9.4：完成检查**

任务 9 完成，勾选 Step 9.1~9.4。

---

## 任务 10：docker-compose.yml + MinIO bucket 初始化

**Files:**
- Create: `e:\GoProject\it-wiki\deploy\docker-compose.yml`
- Create: `e:\GoProject\it-wiki\deploy\init\init-bucket.sh`

- [ ] **Step 10.1：写 `deploy/init/init-bucket.sh`**

这是给 mc（MinIO Client）init 容器跑的一次性脚本，确保 bucket 存在（即使 backend 的 EnsureBucket 也能做，但 mc 跑一遍能在 backend 启动前就让 console 显示 bucket，对演示友好）。

```sh
#!/bin/sh
set -e

# 等待 minio 就绪
echo "[init-bucket] waiting for minio..."
until mc alias set local "${MINIO_ENDPOINT}" "${MINIO_ROOT_USER}" "${MINIO_ROOT_PASSWORD}" >/dev/null 2>&1; do
  sleep 1
done

echo "[init-bucket] ensuring bucket ${BUCKET} exists..."
mc mb --ignore-existing "local/${BUCKET}"

echo "[init-bucket] done."
```

> 注意：用 LF 换行（不是 CRLF），否则容器内 sh 解析失败。VSCode 右下角换行符可切换。

- [ ] **Step 10.2：写 `deploy/docker-compose.yml`**

```yaml
name: it-wiki

services:
  postgres:
    image: pgvector/pgvector:pg16
    restart: unless-stopped
    environment:
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: ${POSTGRES_DB}
    ports:
      - "5432:5432"
    volumes:
      - postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U $${POSTGRES_USER} -d $${POSTGRES_DB}"]
      interval: 5s
      timeout: 5s
      retries: 10

  minio:
    # ⚠️ 镜像 tag 落地时请到 https://hub.docker.com/r/minio/minio/tags 选当前最新稳定 RELEASE.* tag
    # (不要用 latest, 会漂移). 下面是一个示例 tag, 实际请替换.
    image: minio/minio:RELEASE.2025-01-20T14-49-07Z
    restart: unless-stopped
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: ${MINIO_ROOT_USER}
      MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD}
    ports:
      - "9000:9000"   # S3 API
      - "9001:9001"   # Web Console
    volumes:
      - minio-data:/data
    healthcheck:
      # MinIO 自带 health endpoint, 不依赖 mc alias
      test: ["CMD", "curl", "-fsS", "http://localhost:9000/minio/health/live"]
      interval: 5s
      timeout: 5s
      retries: 10

  minio-init:
    # ⚠️ 同样到 https://hub.docker.com/r/minio/mc/tags 选当前最新 tag
    image: minio/mc:RELEASE.2025-01-17T23-25-50Z
    depends_on:
      minio:
        condition: service_healthy
    environment:
      MINIO_ENDPOINT: http://minio:9000
      MINIO_ROOT_USER: ${MINIO_ROOT_USER}
      MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD}
      BUCKET: ${S3_BUCKET}
    volumes:
      - ./init/init-bucket.sh:/init-bucket.sh:ro
    entrypoint: ["/bin/sh", "/init-bucket.sh"]
    restart: "no"

  backend:
    build:
      context: ../backend
      dockerfile: Dockerfile
    image: it-wiki-backend:dev
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
      minio:
        condition: service_healthy
    environment:
      PORT: "8080"
      LOG_LEVEL: ${LOG_LEVEL}
      DATABASE_URL: ${DATABASE_URL}
      S3_ENDPOINT: ${S3_ENDPOINT}
      S3_ACCESS_KEY: ${S3_ACCESS_KEY}
      S3_SECRET_KEY: ${S3_SECRET_KEY}
      S3_BUCKET: ${S3_BUCKET}
      S3_REGION: ${S3_REGION}
      S3_USE_PATH_STYLE: ${S3_USE_PATH_STYLE}
      LLM_BASE_URL: ${LLM_BASE_URL}
      LLM_API_KEY: ${LLM_API_KEY}
      LLM_MODEL: ${LLM_MODEL}
      EMBEDDING_BASE_URL: ${EMBEDDING_BASE_URL}
      EMBEDDING_API_KEY: ${EMBEDDING_API_KEY}
      EMBEDDING_MODEL: ${EMBEDDING_MODEL}
      EMBEDDING_DIM: ${EMBEDDING_DIM}
    ports:
      - "8080:8080"
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/healthz"]
      interval: 5s
      timeout: 5s
      retries: 10

  frontend:
    build:
      context: ../frontend
      dockerfile: Dockerfile
    image: it-wiki-frontend:dev
    restart: unless-stopped
    depends_on:
      backend:
        condition: service_healthy
    environment:
      NEXT_PUBLIC_API_BASE_URL: ${NEXT_PUBLIC_API_BASE_URL}
    ports:
      - "3000:3000"

volumes:
  postgres-data:
  minio-data:
```

> 注意：
> - postgres 镜像用 `pgvector/pgvector:pg16` 已自带 pgvector 扩展，省去后续 `CREATE EXTENSION` 烦恼
> - backend `Dockerfile` 里没装 wget，alpine 默认有 `wget`（busybox 版），healthcheck 可用
> - 如果 backend 容器 healthcheck wget 报 not found，改用 `["CMD-SHELL", "nc -z localhost 8080"]`

- [ ] **Step 10.3：启动整套**

```powershell
cd e:\GoProject\it-wiki
docker compose -f deploy/docker-compose.yml --env-file .env up -d --build
```

期望最终所有服务 `started`。查看状态：

```powershell
docker compose -f deploy/docker-compose.yml ps
```

期望所有服务 `running` 或 `healthy`（`minio-init` 应该是 `exited (0)` —— 正常，它是一次性 init 容器）。

- [ ] **Step 10.4：查看日志确认无致命错误**

```powershell
docker compose -f deploy/docker-compose.yml logs backend --tail 50
docker compose -f deploy/docker-compose.yml logs minio-init --tail 30
docker compose -f deploy/docker-compose.yml logs frontend --tail 30
```

backend 日志应有：

```
[minio] bucket "it-wiki-docs" not found, creating...   # 或: 已存在(因为 minio-init 先建了)
[main] minio bucket "it-wiki-docs" ready
[server] listening on :8080
```

minio-init 日志应有：

```
[init-bucket] done.
```

- [ ] **Step 10.5：三个验收点（端到端）**

```powershell
# A. 前端
Start-Process "http://localhost:3000"
# 浏览器应看到 it-wiki 首页 + 三个按钮

# B. 后端
curl http://localhost:8080/healthz
# 期望: {"status":"ok","time":"..."}

# C. MinIO Console
Start-Process "http://localhost:9001"
# 用 .env 里 MINIO_ROOT_USER / MINIO_ROOT_PASSWORD 登录
# 应在 Buckets 列表看到 it-wiki-docs
```

三项任一失败，先看对应容器的 `docker compose logs` 排查。

- [ ] **Step 10.6：完成检查**

任务 10 完成，勾选 Step 10.1~10.6。**这是阶段 0 的核心验收点**，三个验收点全过才能进任务 11。

---

## 任务 11：顶层 Makefile + README

**Files:**
- Create: `e:\GoProject\it-wiki\Makefile`
- Create: `e:\GoProject\it-wiki\README.md`

- [ ] **Step 11.1：写顶层 `Makefile`**

```makefile
# it-wiki 顶层 Makefile
# 跨平台说明: Windows 上需要装 GNU Make (scoop install make) 或在 git bash 中运行
.PHONY: help up down logs ps restart build clean tools backend-test frontend-typecheck

COMPOSE := docker compose -f deploy/docker-compose.yml --env-file .env

help: ## 显示可用命令
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?##/ { printf "  %-22s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

up: ## 启动整套服务(detached + build)
	$(COMPOSE) up -d --build

down: ## 停止全部服务
	$(COMPOSE) down

logs: ## 跟随 backend 日志(其他服务用: docker compose logs <svc>)
	$(COMPOSE) logs -f backend

ps: ## 查看服务状态
	$(COMPOSE) ps

restart: ## 重启 backend + frontend(不重启数据库/对象存储)
	$(COMPOSE) restart backend frontend

build: ## 仅 build 镜像不启动
	$(COMPOSE) build

clean: ## 停止并删除 volume(慎用,会清空 DB 和上传文件)
	$(COMPOSE) down -v

tools: ## 安装本地开发工具链
	$(MAKE) -C backend tools

backend-test: ## 后端测试
	$(MAKE) -C backend test

frontend-typecheck: ## 前端类型检查
	cd frontend && pnpm typecheck
```

- [ ] **Step 11.2：写 `README.md`**

```markdown
# it-wiki

团队/企业知识库 Agent · 单机 Docker Compose Demo。

> **当前阶段**：阶段 0（工程脚手架）已就绪。详见 [CLAUDE.md](CLAUDE.md) 第 2 节。

---

## 快速启动

### 前置

- Docker Desktop 4.x（含 Docker Compose v2）
- GNU Make（Windows 推荐 `scoop install make`，或直接用 `docker compose` 命令）
- 可选：Go 1.22 + pnpm@9（仅本地开发用，不跑 Docker 不需要）

### 步骤

```bash
# 1. 复制环境变量模板并填写
cp .env.example .env
# 编辑 .env：至少填好 LLM_API_KEY 和 EMBEDDING_API_KEY（阶段 0 还用不上, 占位即可）

# 2. 启动整套服务
make up                 # 等价: docker compose -f deploy/docker-compose.yml --env-file .env up -d --build

# 3. 访问
# 前端首页:        http://localhost:3000
# 后端 healthz:    http://localhost:8080/healthz
# MinIO Console:   http://localhost:9001  (用户名/密码见 .env MINIO_ROOT_*)
```

### 停止 / 清理

```bash
make down               # 停止服务，保留数据
make clean              # 停止 + 清空 volumes(DB & 上传文件全删)
```

---

## 项目结构

```
it-wiki/
├── backend/      # Go 后端（chi + Eino + sqlc + river）
├── frontend/     # Next.js 15 前端（Tailwind v4 + shadcn/ui）
├── deploy/       # docker-compose + 初始化脚本
└── docs/         # 设计 spec / 实施计划
```

详细设计：[docs/superpowers/specs/2026-05-19-it-wiki-agent-design.md](docs/superpowers/specs/2026-05-19-it-wiki-agent-design.md)。

---

## 开发约定

参见 [CLAUDE.md](CLAUDE.md)。重点：

- DB 改动走 goose migration + sqlc generate，**不要直连改库**
- 异步任务走 river，**不要起裸 goroutine**
- 添加 shadcn 组件：`cd frontend && pnpm dlx shadcn@latest add <name>`

---

## 阶段进度

- [x] 阶段 0：工程脚手架
- [ ] 阶段 1：文档摄入闭环
- [ ] 阶段 2：RAG 对话最小闭环
- [ ] 阶段 3：ReAct Agent 模式
- [ ] 阶段 4：打磨 + Demo 友好
```

- [ ] **Step 11.3：验证 Makefile（如本地有 make）**

```powershell
cd e:\GoProject\it-wiki
make help
```

期望输出列出所有命令。如无 make 命令则跳过此步——Makefile 仅作约定参考。

- [ ] **Step 11.4：完成检查**

任务 11 完成，勾选 Step 11.1~11.4。

---

## 任务 12：阶段 0 端到端验收

**Files:** 无新增

- [ ] **Step 12.1：完整重启验证（模拟新人首次启动）**

```powershell
cd e:\GoProject\it-wiki

# 清空当前状态(如有)
docker compose -f deploy/docker-compose.yml --env-file .env down -v

# 全新启动
docker compose -f deploy/docker-compose.yml --env-file .env up -d --build
```

- [ ] **Step 12.2：等待全部就绪（最多 2 分钟）**

```powershell
# 反复运行直到看到 backend 是 healthy
docker compose -f deploy/docker-compose.yml ps
```

期望最终状态：

| Service | State |
|---|---|
| postgres | running (healthy) |
| minio | running (healthy) |
| minio-init | exited (0) |
| backend | running (healthy) |
| frontend | running |

- [ ] **Step 12.3：四项验收**

```powershell
# 1. 后端 healthz
curl http://localhost:8080/healthz
# 期望: HTTP 200 + JSON {"status":"ok","time":"..."}

# 2. 前端首页
Start-Process "http://localhost:3000"
# 期望: 浏览器看到 "it-wiki" 标题 + 三个按钮

# 3. MinIO Console
Start-Process "http://localhost:9001"
# 期望: 登录后在 Buckets 列表看到 it-wiki-docs

# 4. backend 启动日志包含 minio ready
docker compose -f deploy/docker-compose.yml logs backend --tail 20 | Select-String "minio bucket"
# 期望: 包含 [main] minio bucket "it-wiki-docs" ready
```

- [ ] **Step 12.4：失败重连测试（验证 EnsureBucket 重试）**

```powershell
# 停 minio 5 秒后重启, 同时 restart backend 看是否重连成功
docker compose -f deploy/docker-compose.yml stop minio
Start-Sleep 5
docker compose -f deploy/docker-compose.yml start minio
docker compose -f deploy/docker-compose.yml restart backend
Start-Sleep 30
docker compose -f deploy/docker-compose.yml logs backend --tail 20
```

期望：backend 启动初期 EnsureBucket 退避重试，最终 `minio bucket ready`。**这一步验证 spec §9 的"MinIO 启动慢导致 EnsureBucket 失败"风险已被缓解。**

- [ ] **Step 12.5：更新 CLAUDE.md 第 2 节**

把 [CLAUDE.md](../../../CLAUDE.md) 第 2 节"当前阶段"改为：

```markdown
## 2. 当前阶段

> **当前进度**：阶段 0 已完成（工程脚手架就绪）。下一步：阶段 1 文档摄入闭环。需要触发 brainstorming 或 writing-plans 写阶段 1 的实施计划。
```

- [ ] **Step 12.6：完成检查**

任务 12 完成，勾选 Step 12.1~12.6。**阶段 0 实施计划全部完成。**

---

## 阶段 0 验收清单（最终）

执行人在最后一并核对：

- [ ] `docker compose -f deploy/docker-compose.yml up -d --build` 一键启动后所有服务进入 healthy/exited(0)
- [ ] `curl http://localhost:8080/healthz` 返回 `{"status":"ok",...}`
- [ ] 浏览器访问 `http://localhost:3000` 看到 "it-wiki" 首页 + shadcn Button
- [ ] 浏览器访问 `http://localhost:9001` 登录后 Buckets 列表看到 `it-wiki-docs`
- [ ] backend 容器日志包含 `[main] minio bucket "it-wiki-docs" ready`
- [ ] 重启 MinIO 后 backend 能通过 backoff 重试成功
- [ ] `frontend/pnpm typecheck` 无错
- [ ] `backend/go build ./...` 无错
- [ ] `make help` / `cd backend && make help` 输出可用命令列表
- [ ] CLAUDE.md 第 2 节"当前阶段"已更新为"阶段 0 已完成"
- [ ] **未做 git commit**（按用户约定）

---

## 阶段 0 已识别的延伸事项（不在本阶段做）

记录在此供阶段 1 启动时参考：

1. **sqlc pgvector 适配**：阶段 1 加 chunks 表时启用 [pgvector-go](https://github.com/pgvector/pgvector-go) 的 sqlc plugin
2. **river 工具链**：阶段 1 加文档摄入 Job 时安装 river 库 + 在 main.go 启动 river client
3. **CORS 中间件**：阶段 1 前端开始调后端 API 时加 CORS（chi/cors）
4. **JSON 错误响应格式**：阶段 1 出现第一个业务 handler 时统一 error envelope
5. **Migrations 命名约定**：第一个迁移文件命名建议 `0001_init_extensions.sql`（创建 `vector` / `uuid-ossp` 扩展）
6. **存储路径约定**：spec 提到 `<kb_id>/<doc_id>/<filename>`，阶段 1 实现 ingestion_service 时落实

---

_本计划完成后请回到 brainstorming/writing-plans 触发阶段 1 计划编写。_
