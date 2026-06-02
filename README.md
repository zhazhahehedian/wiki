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