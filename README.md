# it-wiki

团队/企业知识库 Agent · 单机 Docker Compose Demo。

> **当前阶段**：飞书知识源集成（OAuth、owner 隔离、导入、snapshot 同步与恢复）已实现；阶段 4 的其余打磨仍在推进。详见 [CLAUDE.md](CLAUDE.md) 第 2 节。

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

### 飞书快速配置

1. 在飞书开放平台创建企业自建应用，添加本项目要求的 7 个用户只读 scope。
2. 将控制台 redirect URL 与 `.env` 的 `FEISHU_REDIRECT_URL` 配成完全相同的值；本地默认是 `http://localhost:8080/api/v1/auth/feishu/callback`。
3. 在 `.env` 填写 `FEISHU_APP_ID`、`FEISHU_APP_SECRET`、`FEISHU_TENANT_KEY`、`OAUTH_ENCRYPTION_KEY` 和 `FRONTEND_ORIGIN`。生产 HTTPS 同时设置 `SESSION_COOKIE_SECURE=true`。
4. 如果升级前已有 KB 或 conversation，先设置 `BOOTSTRAP_OWNER_FEISHU_OPEN_ID`；没有 NULL owner 时留空。
5. 启动后从 `http://localhost:8080/api/v1/auth/feishu/start` 登录，在知识库文档页导入飞书 URL，或对已有飞书文档执行“立即同步”。

完整 scope/env 清单、cookie/CSRF/反代要求、同步语义和排错方法见 [飞书集成部署与排错](docs/deploy-debug-feishu.md)。

不连接真实飞书的验证：

```bash
cd backend && go test ./...
cd ../frontend && pnpm lint && pnpm typecheck && pnpm test && pnpm build
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
- [x] 阶段 1：文档摄入闭环
- [x] 阶段 2：RAG 对话最小闭环
- [x] 阶段 2.5：检索质量修复与评测基线
- [x] 阶段 3：ReAct Agent 模式与 UI 3.5
- [ ] 阶段 4：飞书知识源集成已实现，其余打磨 + Demo 友好继续推进
