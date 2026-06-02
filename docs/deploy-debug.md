# IT-Wiki 生产部署与调试指南

> **环境前提**：8c8g Linux 服务器，1Panel 已管理 PostgreSQL 16、MinIO。Go 环境已就绪（`go version` ≥ 1.22），Node.js 20+ 已安装。

---

## 目录

1. [pgvector 安装（关键前置）](#1-pgvector-安装关键前置)
2. [数据库准备](#2-数据库准备)
3. [MinIO Bucket 准备](#3-minio-bucket-准备)
4. [上传项目 & 配置 .env](#4-上传项目--配置-env)
5. [构建后端](#5-构建后端)
6. [跑迁移](#6-跑迁移)
7. [构建前端](#7-构建前端)
8. [进程托管（systemd）](#8-进程托管systemd)
9. [验收测试（Task 17 curl）](#9-验收测试task-17-curl)
10. [常见问题排查](#10-常见问题排查)

---

## 1. pgvector 安装（关键前置）

`chunks.embedding` 列类型是 `vector(1024)`，**PostgreSQL 必须装 pgvector 扩展**，否则迁移失败。

### 检查是否已装

```bash
# 在 1Panel 的 PostgreSQL 终端执行，或 psql 直连
psql -U postgres -c "SELECT * FROM pg_available_extensions WHERE name = 'vector';"
```

- 有一行 `vector` 且 `installed_version` 非空 → 已装，跳过本节。
- 没有 → 需要安装。

### 方案 A：apt 安装（PG 在宿主机非容器内）

```bash
# 确认 PG 版本
psql -U postgres -c "SELECT version();"

# 1) 先配置 PostgreSQL APT Repository（Ubuntu/Debian 默认仓库版本旧、无 pgvector）
sudo apt install -y curl ca-certificates
sudo install -d /usr/share/postgresql-common/pgdg
sudo curl -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc \
  https://www.postgresql.org/media/keys/ACCC4CF8.asc
echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] \
  https://apt.postgresql.org/pub/repos/apt $(. /etc/os-release && echo $VERSION_CODENAME)-pgdg main" \
  | sudo tee /etc/apt/sources.list.d/pgdg.list
sudo apt update

# 2) 安装 pgvector（包名格式：postgresql-<主版本号>-pgvector）
sudo apt install -y postgresql-16-pgvector

# 3) 重启 PG（不需要重新初始化数据目录）
sudo systemctl restart postgresql
```

### 方案 B：1Panel 的 PG 是 Docker 容器（**推荐**）

1Panel 默认用 `postgres:16` 镜像，不含 pgvector。**最稳妥**的做法是换成官方 pgvector 镜像，数据卷不变：

1. 1Panel 控制台 → 应用 → PostgreSQL → 停止
2. 编辑应用配置，将镜像改为：`pgvector/pgvector:pg16`（注意主版本号要跟原来一致，避免数据格式不兼容）
3. 启动应用 — 因为数据卷复用，库和用户都还在
4. `CREATE EXTENSION vector` 即可启用

### 方案 C：在容器内从源码编译（保留原镜像）

如果不能改镜像（比如生产 PG 是托管或共享的），可以 exec 进容器编译：

```bash
docker exec -it <postgres容器名> bash
apt update && apt install -y build-essential git postgresql-server-dev-16
cd /tmp
git clone --branch v0.8.2 https://github.com/pgvector/pgvector.git
cd pgvector
make && make install
# 退出容器后重启
docker restart <postgres容器名>
```

> ⚠️ 这种方式容器重建后扩展会丢失，需重做。生产长期方案还是用方案 B。

---

## 2. 数据库准备

以 `psql` 连接到 PostgreSQL（用户名/密码查 1Panel 配置）。

```sql
-- 创建专用用户和数据库
CREATE USER itwiki WITH PASSWORD 'your_strong_password';
CREATE DATABASE itwiki OWNER itwiki;

-- 连接到新库后启用扩展（必须 superuser 执行一次）
\c itwiki
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- 验证
SELECT extname, extversion FROM pg_extension WHERE extname IN ('vector','pgcrypto');
```

> **注**：`goose` 迁移会自动执行 `CREATE EXTENSION IF NOT EXISTS vector`（0001_extensions.sql），但首次需确保当前用户有权限，或提前以 superuser 创建好。

---

## 3. MinIO Bucket 准备

在 1Panel 的 MinIO 控制台（或访问 `http://<服务器IP>:9001`）：

1. 登录控制台（用户名/密码在 1Panel 应用详情里）
2. 创建 Bucket，名称：`it-wiki-docs`
3. 记录 Access Key 和 Secret Key（在 1Panel > MinIO > 访问凭证，或 Identity > Service Accounts）

> **S3 端点**：MinIO 对外暴露的 API 端口通常是 **9000**。如果 1Panel 配置了反代，用对应域名/端口。

---

## 4. 上传项目 & 配置 .env

### 上传到服务器

```bash
# 本地执行（rsync 或 scp）
rsync -av --exclude='.git' --exclude='frontend/node_modules' --exclude='backend/bin' \
  e:/GoProject/it-wiki/ user@<服务器IP>:/opt/it-wiki/

# 或在服务器上 git clone（如有 git 仓库）
```

### 编写 .env

```bash
cd /opt/it-wiki
cp .env.example .env
vim .env   # 或 nano .env
```

按实际情况填写以下关键字段：

```env
# ----- PostgreSQL -----
# 1Panel PG 默认监听 localhost:5432（或自定义端口，查 1Panel 配置）
DATABASE_URL=postgres://itwiki:your_strong_password@localhost:5432/itwiki?sslmode=disable

# ----- MinIO -----
# 1Panel MinIO API 端口通常是 9000
S3_ENDPOINT=http://localhost:9000
S3_ACCESS_KEY=<你的AccessKey>
S3_SECRET_KEY=<你的SecretKey>
S3_BUCKET=it-wiki-docs
S3_REGION=us-east-1
S3_USE_PATH_STYLE=true

# ----- LLM（可先填 placeholder，Phase 1 验收不需要 LLM 对话）-----
LLM_BASE_URL=https://api.deepseek.com/v1
LLM_API_KEY=sk-placeholder
LLM_MODEL=deepseek-chat

# ----- Embedding（必须真实有效，文档摄入时调用）-----
EMBEDDING_BASE_URL=https://dashscope.aliyuncs.com/compatible-mode/v1
EMBEDDING_API_KEY=sk-xxxxxxxxx
EMBEDDING_MODEL=text-embedding-v3
EMBEDDING_DIM=1024

# ----- Backend -----
PORT=8080
LOG_LEVEL=info

# ----- Splitter / Embedder runtime -----
TOKENIZER_ENCODING=cl100k_base
CHUNK_SIZE=800
CHUNK_OVERLAP=120
EMBED_BATCH_SIZE=64
UPLOAD_MAX_MB=50
RIVER_MAX_WORKERS=4

# ----- Frontend -----
# 浏览器访问后端的地址，填服务器公网/局域网 IP
NEXT_PUBLIC_API_BASE_URL=http://<服务器IP>:8080
```

---

## 5. 构建后端

```bash
cd /opt/it-wiki/backend

# 加载环境变量（make run / migrate 都依赖 DATABASE_URL 等）
set -a && source /opt/it-wiki/.env && set +a

# 拉依赖
go mod download

# 编译（输出到 bin/server）
make build
# 等价于：go build -o bin/server ./cmd/server

# 验证二进制
./bin/server --help 2>&1 | head -3 || true
ls -lh bin/server
```

---

## 6. 跑迁移

迁移在 server 启动时也会自动跑（`goose.UpContext`），但建议先手动确认：

```bash
cd /opt/it-wiki/backend
set -a && source /opt/it-wiki/.env && set +a

# 查看迁移状态
make migrate-status

# 执行迁移
make migrate-up

# 再次确认
make migrate-status
```

期望输出（6 条 Applied）：

```
    Applied At                  Migration
    =======================================
    2026-05-26 ...  -- 0001_extensions.sql
    2026-05-26 ...  -- 0002_knowledge_bases.sql
    2026-05-26 ...  -- 0003_documents.sql
    2026-05-26 ...  -- 0004_chunks_table.go
    2026-05-26 ...  -- 0005_conversations.sql
    2026-05-26 ...  -- 0006_messages.sql
```

验证 chunks 列类型：

```bash
psql "$DATABASE_URL" -c "\d chunks" | grep embedding
# 期望：embedding | vector(1024)
```

---

## 7. 构建前端

```bash
cd /opt/it-wiki/frontend

# 确保 pnpm 可用
corepack enable && corepack prepare pnpm@9.12.0 --activate
# 或：npm install -g pnpm

pnpm install --frozen-lockfile

# 构建（standalone 模式，Linux 无 symlink 限制，完整成功）
pnpm build

# ⚠️ 关键步骤：standalone 模式不会自动复制 public/ 和 .next/static/
# 必须手动拷贝，否则浏览器访问时所有静态资源（CSS / 字体 / 图片）会 404
cp -r public .next/standalone/
cp -r .next/static .next/standalone/.next/

# 验证产物
ls .next/standalone/         # 应能看到 server.js / public / .next
ls .next/standalone/.next/   # 应能看到 static
```

> Next.js 官方文档（v15）明确说明：standalone 构建会生成 `server.js` 但 **不会**复制 `public/` 和 `.next/static/`，必须手动拷贝。Dockerfile 里已经做了这步，但 bare-metal 部署需自己加。

---

## 8. 进程托管（systemd）

### 8.1 后端 systemd 服务

```bash
sudo tee /etc/systemd/system/it-wiki-backend.service > /dev/null <<'EOF'
[Unit]
Description=it-wiki backend
After=network.target

[Service]
Type=simple
User=nobody
WorkingDirectory=/opt/it-wiki/backend
EnvironmentFile=/opt/it-wiki/.env
ExecStart=/opt/it-wiki/backend/bin/server
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now it-wiki-backend
sudo systemctl status it-wiki-backend
```

### 8.2 前端 systemd 服务

```bash
sudo tee /etc/systemd/system/it-wiki-frontend.service > /dev/null <<'EOF'
[Unit]
Description=it-wiki frontend
After=it-wiki-backend.service

[Service]
Type=simple
User=nobody
WorkingDirectory=/opt/it-wiki/frontend/.next/standalone
Environment=NODE_ENV=production
Environment=PORT=3000
Environment=HOSTNAME=0.0.0.0
EnvironmentFile=/opt/it-wiki/.env
ExecStart=/usr/bin/node server.js
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now it-wiki-frontend
sudo systemctl status it-wiki-frontend
```

### 8.3 快速重启命令

```bash
sudo systemctl restart it-wiki-backend
sudo systemctl restart it-wiki-frontend
sudo journalctl -u it-wiki-backend -f    # 实时日志
sudo journalctl -u it-wiki-frontend -f
```

---

## 9. 验收测试（Task 17 curl）

全部命令在服务器上执行。假设后端 `localhost:8080`，前端 `localhost:3000`。

### 9.1 健康检查

```bash
curl -s http://localhost:8080/healthz
# 期望：{"status":"ok"}
```

### 9.2 新建知识库

```bash
curl -s -X POST http://localhost:8080/api/v1/kbs \
  -H 'Content-Type: application/json' \
  -d '{"name":"测试KB","description":"端到端验收"}' | jq .
```

期望返回包含 `id`、`embed_model`、`embed_dim` 的 JSON，记录 `KB_ID`：

```bash
KB_ID=$(curl -s -X POST http://localhost:8080/api/v1/kbs \
  -H 'Content-Type: application/json' \
  -d '{"name":"测试KB","description":"端到端验收"}' | jq -r '.id')
echo "KB_ID=$KB_ID"
```

### 9.3 列出知识库

```bash
curl -s "http://localhost:8080/api/v1/kbs?limit=10&offset=0" | jq .
# 期望：{"items":[...],"total":1}
```

### 9.4 上传 Markdown 文档

```bash
# 准备一个测试文件
cat > /tmp/test.md << 'EOF'
# 测试文档

这是用于端到端验收的测试文档。

## 章节一

内容一内容一内容一。

## 章节二

内容二内容二内容二。
EOF

# 上传
DOC_RESP=$(curl -s -X POST "http://localhost:8080/api/v1/kbs/$KB_ID/docs" \
  -F "file=@/tmp/test.md;type=text/markdown")
echo $DOC_RESP | jq .
DOC_ID=$(echo $DOC_RESP | jq -r '.id')
echo "DOC_ID=$DOC_ID"
```

期望返回 `status: "pending"`。

### 9.5 轮询文档状态（等待 ready）

```bash
# 每 3 秒查一次，最多等 60 秒
for i in $(seq 1 20); do
  STATUS=$(curl -s "http://localhost:8080/api/v1/docs/$DOC_ID" | jq -r '.status')
  echo "[$(date +%T)] status=$STATUS"
  [ "$STATUS" = "ready" ] && break
  [ "$STATUS" = "failed" ] && { echo "摄入失败！"; curl -s "http://localhost:8080/api/v1/docs/$DOC_ID" | jq .error_message; break; }
  sleep 3
done
```

期望在 30 秒内变为 `ready`。

### 9.6 查看切片

```bash
curl -s "http://localhost:8080/api/v1/docs/$DOC_ID/chunks?limit=10&offset=0" | jq .
# 期望：items 不为空，每个 item 有 content / token_count / seq
```

### 9.7 重复上传同一文件（去重验证）

```bash
curl -s -X POST "http://localhost:8080/api/v1/kbs/$KB_ID/docs" \
  -F "file=@/tmp/test.md;type=text/markdown" | jq .
# 期望：409 {"error":{"code":"duplicate_checksum","details":{"existing_doc_id":"..."}}}
```

### 9.8 删除文档

```bash
curl -s -X DELETE "http://localhost:8080/api/v1/docs/$DOC_ID"
# 期望：204 No Content

# 确认 chunks 级联删除
curl -s "http://localhost:8080/api/v1/docs/$DOC_ID/chunks" | jq .
# 期望：404
```

### 9.9 前端页面

浏览器访问 `http://<服务器IP>:3000`，应看到知识库列表页，能新建 KB、上传文件，刷新后文档状态变为 `ready`，点进去能看到切片内容。

---

## 10. 常见问题排查

### 后端启动失败

**`panic: missing required env var: DATABASE_URL`**  
→ `.env` 未加载或路径错误。检查 systemd `EnvironmentFile=` 路径，或手动 `source .env` 再运行。

**`goose up: dial error... vector extension`**  
→ pgvector 未安装。见第 1 节。

**`embedding dim mismatch`**  
→ `EMBEDDING_DIM` 与 chunks 表列维度不一致。重建 chunks 表：`make migrate-reset && make migrate-up`（**会清空数据**）。

**`minio client: connection refused`**  
→ `S3_ENDPOINT` 填的地址后端容器/进程不可达。如果 MinIO 是 Docker 容器，用宿主机 IP 而不是 `localhost`（容器内 localhost 指容器自身）。

**`rivermigrate up: ...`**  
→ river 需要在 PostgreSQL 里建几张内部表。只要 DATABASE_URL 有 DDL 权限就没问题；如果用了受限账号，先用 superuser 跑一次 `make migrate-up`。

### 摄入卡在 `parsing` / `embedding`

```bash
# 查 backend 日志
sudo journalctl -u it-wiki-backend -n 100 --no-pager | grep -i "error\|fail\|panic"
```

常见原因：
- `embedding` 阶段：`EMBEDDING_API_KEY` 无效或额度不足 → 查日志里的 HTTP 状态码
- `parsing` 阶段：文件损坏或 MIME 类型识别错误

### 前端白屏 / API 请求 404

检查 `NEXT_PUBLIC_API_BASE_URL` 是否填了浏览器可访问的地址（**不能填 `localhost`**，浏览器跑在客户端机器上，`localhost` 指的是用户电脑）。

```bash
# 查前端日志
sudo journalctl -u it-wiki-frontend -n 50 --no-pager
```

### 端口被占用

```bash
ss -tlnp | grep -E '8080|3000'
# 找到 PID 后 kill，或改 .env 里的 PORT
```

### 查看 river 队列状态

River 维护一张 `river_job` 表，每条 Job 一行。`state` 取值（来自 River 状态机文档）：

| state | 含义 |
|-------|------|
| `available` | 等待 worker 拉取 |
| `running` | 正在执行 |
| `retryable` | 失败但还可重试 |
| `completed` | 执行成功 |
| `discarded` | 失败次数超限，已放弃（Phase 1 不重试，失败一次就 discarded） |
| `cancelled` | 主动取消 |
| `scheduled` / `pending` | 定时或被阻塞 |

```bash
# 看最近 10 条 Job 状态
psql "$DATABASE_URL" -c "
  SELECT id, kind, state, attempt, max_attempts,
         to_char(created_at, 'HH24:MI:SS') AS created,
         errors
    FROM river_job
   ORDER BY created_at DESC
   LIMIT 10;"

# 按 state 聚合看队列健康度
psql "$DATABASE_URL" -c "
  SELECT state, COUNT(*) FROM river_job GROUP BY state ORDER BY 2 DESC;"

# 看具体错误（errors 是 jsonb 数组，每次失败追加一项）
psql "$DATABASE_URL" -c "
  SELECT id, kind, state, errors
    FROM river_job
   WHERE state IN ('discarded','retryable')
   ORDER BY created_at DESC
   LIMIT 5;"
```

`errors` JSONB 元素结构：`{"at": "...", "attempt": 1, "error": "...", "trace": "..."}`。展开看：

```bash
psql "$DATABASE_URL" -c "
  SELECT id, kind, e->>'error' AS error_msg, e->>'at' AS occurred_at
    FROM river_job, jsonb_array_elements(errors) e
   WHERE state = 'discarded'
   ORDER BY id DESC LIMIT 5;"
```

---

## 快速参考

| 操作 | 命令 |
|------|------|
| 看后端日志 | `journalctl -u it-wiki-backend -f` |
| 看前端日志 | `journalctl -u it-wiki-frontend -f` |
| 重启后端 | `systemctl restart it-wiki-backend` |
| 重启前端 | `systemctl restart it-wiki-frontend` |
| 查迁移状态 | `cd /opt/it-wiki/backend && source /opt/it-wiki/.env && make migrate-status` |
| 健康检查 | `curl http://localhost:8080/healthz` |
| 查 river 队列 | `psql $DATABASE_URL -c "SELECT state, COUNT(*) FROM river_job GROUP BY state;"` |
