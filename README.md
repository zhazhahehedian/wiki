# 能力中心（Capability Hub）

公司内部 MCP / Skill 能力注册、治理、发现与凭证分发平台，由 it-wiki 转向改造。平台不做知识库或 MCP 运行时网关。

阶段 A 的平台骨架与 UI 已实现：复用飞书认证、session/CSRF、数据库和对象存储基础设施，提供工作台、注册中心、游乐场、我的发布及设置入口。游乐场的「接口配置」和设置页支持用户填写 API 地址、API Key、模型 ID 列表与默认模型；可选择 OpenAI 兼容或 Claude（Anthropic）协议，自动获取并勾选模型，也可手动填写。配置按账号加密保存到后端，支持流式对话、停止与重新生成，对话仅保留在页面内存。阶段 B 注册中心已支持 MCP/Skill 草稿创建编辑、版本历史、卡片/表格、搜索筛选和附件上传下载；阶段 C 已支持审核/驳回/下线、不可变发布版本、独立上线快照、可信部门与审计。草稿由 Owner/Admin 管理，消费者只能浏览授权上线内容；凭证分发与机器 discovery 仍待实施。注册中心新增「对话创建 Skill」：支持通用流程与女娲方法适配、需求追问、文本素材、可编辑预览及保存草稿。云 PostgreSQL/MinIO 已连通并完成隔离集成测试；真实飞书登录、回调与会话已验证；真实模型调用另行验收。

设计见 [能力中心 spec](docs/superpowers/specs/2026-09-04-capability-hub-design.md)，实施与验证见 [阶段 A 计划](docs/superpowers/plans/2026-09-05-capability-hub-phase-a-plan.md)及[游乐场后端联调记录](docs/superpowers/plans/2026-09-05-playground-backend-plan.md)。

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

## 结构与阶段

- `backend/`：Go/chi、pgx/sqlc、goose、River、MinIO 与认证基础设施；遗留 KB/RAG 代码暂留，已从生产路由和启动装配中移除。
- `frontend/`：Next.js 15、React 19、Tailwind v4、Base UI/shadcn；白色侧栏、白灰内容与低饱和蓝主操作，参考用户提供的 micu/new-api 游乐场交互。
- `docs/`：产品设计和实施计划。项目事实与按任务读取的索引见 [CLAUDE.md](CLAUDE.md)。

A 平台骨架与提前实施的 F 游乐场代码已完成，真实 OAuth 已通过，模型验收另行记录；B 注册中心 Owner 草稿闭环已实现并通过隔离云服务和浏览器验证，记录见[阶段 B 计划](docs/superpowers/plans/2026-09-06-capability-hub-phase-b-plan.md)；C 治理已实现并通过隔离云服务和多角色浏览器验证，记录见[阶段 C 计划](docs/superpowers/plans/2026-09-06-capability-hub-phase-c-plan.md)。D 凭证与发现、E 健康检查仍待实施。原 [it-wiki 设计](docs/superpowers/specs/2026-05-19-it-wiki-agent-design.md) 仅供遗留维护和迁移参考。

## 本次推送功能

按功能划分的提交、清理项、验证结果及运行态前置条件见 [2026-09-06 推送说明](docs/releases/2026-09-06-capability-hub.md)。游乐场现支持输入工具栏选择模型、参数独立启用、系统提示词，以及 OpenAI/Claude 协议差异处理；界面截图见 [UI 验证记录](docs/verification/2026-09-06-ui-refresh/README.md)。
