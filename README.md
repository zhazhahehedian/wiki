# 能力中心（Capability Hub）

公司内部「能力市场＋知识服务平台」，由 it-wiki 转向改造。平台不做 MCP 运行时网关，也不负责对话或回答生成。

- **注册与治理**：MCP / Skill 草稿、版本、审核 / 驳回 / 下线、不可变发布快照、可信部门与审计；消费者只看到授权上线内容。
- **内部市场**：浏览授权上线的 Skill，下载安装，或复制为自己的草稿。
- **对话创建 Skill**：通用流程或女娲方法，生成后编辑预览并保存为草稿。
- **模型游乐场**：用户自配 OpenAI 兼容 / Anthropic 接口，配置加密保存，流式对话不持久化。
- **知识服务（K1）**：知识库与文档管理、解析预览、异步索引、授权检索，以及供业务系统调用的检索 API，见 [knowledge-api.md](docs/knowledge-api.md)。

凭证保险箱、机器 discovery、健康检查与飞书云文档同步待实施。进度与资料索引见 [CLAUDE.md](CLAUDE.md)，设计见 [能力中心 spec](docs/superpowers/specs/2026-09-04-capability-hub-design.md) 与 [知识服务设计](docs/superpowers/specs/2026-09-19-knowledge-service-design.md)。

## 对话创建 Skill

在设置页配置自己的模型接口后，进入 **注册中心 → 对话创建 Skill**，选择「通用工作流程」或「专家思维 · 女娲方法」，描述希望完成的任务。可粘贴或导入文本素材，回答助手追问，再编辑名称、正文和文本附件。点击「文件预览」查看 SKILL.md/附件，确认后保存到注册中心；详情页可下载 Skill 包或继续配置草稿信息。

对话和未保存草案仅在当前页面保留。生成失败或停止会保留上一版草案；生成不会自动发布，保存后成为 Owner/Admin 可管理的草稿，经审核上线后才进入授权目录。女娲模板为需求诊断与方法提炼的轻量适配，不包含自动联网研究或 Skill 执行。实现边界和验证记录见 [Skill 引导创建计划](docs/superpowers/plans/2026-09-06-skill-builder-plan.md)。

## 本地开发

Go 版本以 `backend/go.mod` 为准（当前要求 1.25.7）；Node.js、pnpm 与依赖以 `frontend/package.json` 和 lockfile 为准（pnpm 9.12.0）。

1. 首次配置时复制 `.env.example` 为本地 `.env`，已有配置不要覆盖。填写 `DATABASE_URL`、`S3_ENDPOINT`、`S3_ACCESS_KEY`、`S3_SECRET_KEY`、`S3_BUCKET` 等基础设施变量。
2. 填写 `FEISHU_APP_ID`、`FEISHU_APP_SECRET`、`FEISHU_REDIRECT_URL` 和 `FRONTEND_ORIGIN`。可选 `FEISHU_TENANT_KEY` 用于限制租户。用 `openssl rand -hex 16` 生成私有随机字符串填入 `OAUTH_ENCRYPTION_KEY`，不可使用模板值。
3. 飞书应用需要 `offline_access`、`contact:user.base:readonly`、`contact:user.email:readonly` 三个 scope。回调地址必须与控制台一致，本地默认 `http://localhost:8080/api/v1/auth/feishu/callback`。HTTPS 部署设置 `SESSION_COOKIE_SECURE=true`。

后端从进程环境读取变量，不自动加载根目录 `.env`。将本地配置导入环境后启动；以下示例适用于 Bash/WSL，并要求 `.env` 使用 shell 兼容的赋值格式：

```bash
set -a
source .env
set +a
cd backend
go run ./cmd/server
```

启动会运行 goose 与 River schema 迁移，并确保 MinIO bucket 存在。请指向专用开发库；历史迁移仍需要 pgvector 和 `EMBEDDING_DIM`，表退役在后续独立迁移中处理。启动不依赖 LLM、Embedding、tokenizer 或旧 KB 同步 worker，也不依赖 Redis/MySQL；模型只在用户主动发送对话时调用。

数据库可选 `DATABASE_PROXY_URL=socks5://127.0.0.1:7897`；对象存储及飞书使用标准 `HTTP_PROXY`/`HTTPS_PROXY`，本地地址应包含在 `NO_PROXY`。仅在当前网络确实需要时配置代理。公网模型地址只允许 HTTPS；私网、HTTP 或需要代理的模型服务须由运维在 `PLAYGROUND_TRUSTED_ORIGINS` 中逐个登记完整 origin（如 `https://tokenhub.robosense.cn`，不包含 `/v1`）。

自定义本地后端端口时，需同时设置根目录 `.env` 中的 `PORT`、`FEISHU_REDIRECT_URL`，以及 `frontend/.env.local` 中的 `NEXT_PUBLIC_API_BASE_URL`；回调须与飞书控制台一致。当前联调使用 8088，示例默认仍为 8080。

另开终端启动前端，明确指定 3000，避免继承后端 `PORT=8088` 导致端口冲突：

```bash
cd frontend
pnpm install --frozen-lockfile
pnpm dev --port 3000
```

访问 `http://localhost:3000`；API 默认 `http://localhost:8080`，健康检查 `/healthz`。前端 `/` 与旧 `/kbs/**` 重定向到 `/hub`，未登录会进入登录页；生产后端旧 KB/文档/会话 API 返回 404。真实访问需要可用的认证后端，UI 预览时使用的临时模拟身份服务不属于产品认证实现。

本仓库不提供 Compose 编排；已清理引用缺失文件的 `up/down/logs/ps/restart/build/clean` 根目标。使用 `make help` 查看现有构建和检查命令，数据库与对象存储需单独准备。默认环境模板只保留当前平台配置及历史迁移所需的 `EMBEDDING_DIM`。

## 验证

按改动风险选择检查，规则见 [AGENTS.md](AGENTS.md)。合并前使用以下检查：

```bash
cd backend
go test ./...
cd ../frontend
pnpm test
pnpm typecheck
pnpm build
```

standalone 构建在 Linux/WSL 验证；Windows 可能在最终 symlink-copy 阶段因权限报 EPERM。测试中的模拟 OAuth 不替代真实飞书授权验收。认证部署与排错见 [飞书集成部署与排错](docs/deploy-debug-feishu.md)，其中 KB 导入/同步部分仅供遗留维护。

## 结构

- `backend/`：Go/chi、pgx/sqlc、goose、River、PostgreSQL + pgvector、MinIO；`internal/knowledge` 为知识服务。遗留 KB/RAG 代码暂留，未接入生产路由。
- `frontend/`：Next.js 15、React 19、Tailwind v4、Base UI/shadcn。
- `docs/`：产品设计、实施计划、验证记录与发布说明（`docs/releases/`）。
