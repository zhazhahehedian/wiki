# CLAUDE.md · 能力中心（Capability Hub）

项目事实与资料索引。工作规则见 [AGENTS.md](AGENTS.md)。

## 1. 定位

公司内部「能力市场＋知识服务平台」（2026-09-04 由 it-wiki 转向，2026-09-19 D12 扩展）。产品设计见 [能力中心 spec](docs/superpowers/specs/2026-09-04-capability-hub-design.md) 与 [知识服务设计](docs/superpowers/specs/2026-09-19-knowledge-service-design.md)。

- MCP / Skill 注册、审核治理、按权限发现、内部市场下载与复用；平台不进入 MCP 运行时链路，不在线执行 Skill。
- 知识库管理与授权检索 API；对话和回答生成由接入的业务系统负责。旧 KB 聊天 / ReAct 产品不恢复。
- 模型游乐场：用户自配 OpenAI 兼容或 Anthropic 接口，配置按用户加密保存，对话不持久化。

## 2. 进度

| 模块 | 状态 |
|---|---|
| A 骨架、B 注册草稿、C 治理、游乐场、Skill 创建 | 已实现并合入 main |
| 内部能力市场（D11） | 已实现，未提交 |
| K1 知识服务 | 已实现，未提交；缺平台 embedding 配置，真实检索质量与真实账号未验收 |
| D 凭证保险箱与机器发现、E 健康检查、K2 飞书云文档同步、K3 用户委托 | 待实施 |

当前状态与环境细节以 [K1 交接](docs/superpowers/plans/2026-09-19-knowledge-service-handoff.md) 为准。

## 3. 技术栈与命令

- 后端：Go（版本见 [go.mod](backend/go.mod)）、chi、pgx/sqlc、goose、River、PostgreSQL + pgvector、MinIO。
- 前端：Next.js 15、React 19、Tailwind v4、TanStack Query、Base UI/shadcn（base-nova）、Vitest。不引入 Eino、Zustand、Radix。
- 命令见 AGENTS.md 与 Makefile；迁移在 `backend/` 执行 `go run ./cmd/migrate up`。仓库无 Compose，使用现有远程 PostgreSQL/MinIO，不依赖本地 Docker。
- 环境变量以 [.env.example](.env.example) 为准，新增配置同步更新它。

## 4. 必须守住的约束

- **迁移**：server 启动会自动迁移。业务库停在版本 15；未经授权不要让 server 或测试连业务库，迁移只在隔离库验证。schema 变更只追加新迁移，不手改 `internal/repo/generated`。
- **权限**：保留 session/CSRF/Origin 校验；Owner/Admin、可见范围、审核状态与原子上线按 spec；检索在候选查询阶段限定授权库，应用凭证只存摘要。
- **秘密**：平台配置走 env；用户模型密钥与产品凭证加密入库；都不进 Git、日志或响应。
- **后台任务**：持久化业务任务用 River。
- **验证口径**：fixture / 模拟上游通过不等于真实模型或真实账号验收，汇报时分开写。

## 5. 资料索引

| 任务 | 资料 |
|---|---|
| 角色、数据模型、治理 | 能力中心 spec §5–7 |
| 凭证、发现、健康检查（待实施） | 能力中心 spec §8–10 |
| 知识服务 API 与配置 | [knowledge-api.md](docs/knowledge-api.md) |
| Skill 创建 / 市场 | [Skill 创建计划](docs/superpowers/plans/2026-09-06-skill-builder-plan.md)、[市场计划](docs/superpowers/plans/2026-09-19-skill-market-plan.md) |
| 飞书认证与排错、K2 同步参考 | [飞书部署排错](docs/deploy-debug-feishu.md)、[飞书设计](docs/superpowers/specs/2026-08-08-feishu-auth-and-source-ingestion-design.md)、[同步恢复设计](docs/superpowers/specs/2026-08-09-feishu-sync-liveness-design.md) |
| 验证记录与截图 | `docs/verification/` |
