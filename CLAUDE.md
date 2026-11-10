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
| 独立审核员与授权管理 | 2026-11-05 已实现并验证：全平台 MCP / Skill 审核、允许自审、管理员授予/撤销、角色与审核人可见；见[验证记录](docs/verification/2026-11-05-reviewer-authorization/README.md)；2026-11-05 已部署至公司内网服务器（镜像 20261105-8a78e4c，迁移 19→22），见[发版流程](deploy/company/README.md) |
| MCP 程序包分发 | 2026-11-05 已实现并验证：ZIP 上传、按版本授权下载、无需预部署 Endpoint；浏览器上传→审核→使用者下载通过。含迁移 0021；公司电脑 review 后追加 0022（包 SHA-256/大小、疑似凭证扫描）、包内容预览、下载审计与统计、飞书消息通知（默认关闭）、3 分钟传输超时、孤儿包清理；已在公司电脑 Docker 隔离 PostgreSQL 18 + MinIO 验证（含 0022 回退与通知 River 全链路），通知待真实账号验收，见[验证记录](docs/verification/2026-11-05-company-hardening/README.md)；见[验证记录](docs/verification/2026-11-05-mcp-packages/README.md)；2026-11-05 已部署至公司内网服务器（镜像 20261105-8a78e4c，迁移 19→22），见[发版流程](deploy/company/README.md)。2026-11-06 网页端分片与断点续传（上限 500 MiB，迁移 0023）已实现并在隔离环境验证；2026-11-06 已部署至公司内网服务器（镜像 20261106-8a78e4c-dirty，迁移 22→23，代码尚未提交），真实上传验收待完成，见[验证记录](docs/verification/2026-11-06-mcp-resumable-upload/README.md) |
| 内部能力市场（D11） | 已实现，已推送 main |
| K1 知识服务 | 主体、百炼/DeepSeek 适配与部署配置已实现；隔离真实模型评测通过，正式站点已启动，生产知识写入验收待完成；见 [2026-10-27 发布说明](docs/releases/2026-10-27-directory-publishing-deployment.md)。2026-11-10 试用反馈：DOCX 图片经百炼 `qwen3.7-plus` 识别为文字后入库（失败只警告不阻断，PDF 图片仍阻断），超限表格按行切分并重复表头，合并单元格展开；embedding 改用百炼 `text-embedding-v4`（1024 维，旧知识库需重建）；单张图片与 text-embedding-v4 真实模型调用已通过，真实文档上传验收待完成，见 [知识服务 API](docs/knowledge-api.md) |
| D 凭证保险箱与机器发现、E 健康检查、K2 飞书云文档同步、K3 用户委托 | 待实施 |
| 飞书目录选择与 MCP/Skill 发布 API | 2026-10-27 经用户授权部署至 hub.sn0w.icu，迁移 18/19 已完成隔离恢复与回退验证后上线；飞书新增权限、重新授权、真实部门与直属人员读取通过，已兼容组织根节点不可见的情况；完整 API 写入链路已完成隔离验证，见[发布记录](docs/verification/2026-10-27-directory-api-release/README.md)。2026-11-06 部门可见范围改为包含所有下级部门，模拟飞书隔离验证通过；同批部署，真实账号验收待完成，见[验证记录](docs/verification/2026-11-06-department-subtree/README.md) |

当前状态与环境细节以 [K1 交接](docs/superpowers/plans/2026-09-19-knowledge-service-handoff.md) 为准。

## 3. 技术栈与命令

- 后端：Go（版本见 [go.mod](backend/go.mod)）、chi、pgx/sqlc、goose、River、PostgreSQL + pgvector、MinIO。
- 前端：Next.js 15、React 19、Tailwind v4、TanStack Query、Base UI/shadcn（base-nova）、Vitest。不引入 Eino、Zustand、Radix。
- 公司内网发版（打离线包、服务器升级、回退）见 [deploy/company/README.md](deploy/company/README.md)。
- 命令见 AGENTS.md 与 Makefile；迁移在 `backend/` 执行 `go run ./cmd/migrate up`。本地开发使用现有远程 PostgreSQL/MinIO，不依赖本地 Docker；[服务器部署](deploy/README.md)提供仅编排应用与 HTTPS 代理的 Compose 配置。
- 环境变量以 [.env.example](.env.example) 为准，新增配置同步更新它。

## 4. 必须守住的约束

- **迁移**：server 启动会自动迁移。2026-10-27 经用户授权，业务库已从 15 经 17 升至 19，向量维度仍为 1024，原有数据校验通过；后续迁移仍先在隔离库验证，测试不得连接业务库。schema 变更只追加新迁移，不手改 `internal/repo/generated`。
- **权限**：保留 session/CSRF/Origin 校验；Owner/Admin、可见范围、审核状态与原子上线按 spec；检索在候选查询阶段限定授权库，应用凭证只存摘要。
- **秘密**：平台配置走 env；用户模型密钥与产品凭证加密入库；都不进 Git、日志或响应。
- **后台任务**：持久化业务任务用 River。
- **验证口径**：fixture / 模拟上游通过不等于真实模型或真实账号验收，汇报时分开写。

## 5. 资料索引

| 任务 | 资料 |
|---|---|
| 公司电脑 Claude Code 接力 / 本批发布 | [2026-11-05 审核员与 MCP 程序包交付](docs/releases/2026-11-05-reviewers-mcp-packages.md) |
| 角色、数据模型、治理 | 能力中心 spec §5–7 |
| 独立审核员、授权与撤权 | [审核资格设计](docs/superpowers/specs/2026-11-05-reviewer-authorization.md)、[隔离验证](docs/verification/2026-11-05-reviewer-authorization/README.md) |
| MCP 上传、下载与自行运行 | [程序包分发设计](docs/superpowers/specs/2026-11-05-mcp-package-distribution.md)、[隔离验证](docs/verification/2026-11-05-mcp-packages/README.md) |
| 凭证、发现、健康检查（待实施） | 能力中心 spec §8–10 |
| 知识服务 API 与配置 | [knowledge-api.md](docs/knowledge-api.md) |
| 飞书消息通知（审核、上线、新版本、下线） | [应用配置与规则](docs/feishu-notifications.md) |
| 飞书组织选择、发布 API 与账号凭证 | [设计](docs/superpowers/specs/2026-10-27-directory-and-publishing-api.md)、[飞书权限与重新授权](docs/feishu-directory-setup.md)、[发布 API](docs/publishing-api.md) |
| Skill 创建 / 市场 | [Skill 创建计划](docs/superpowers/plans/2026-09-06-skill-builder-plan.md)、[市场计划](docs/superpowers/plans/2026-09-19-skill-market-plan.md) |
| 飞书认证与排错、K2 同步参考 | [飞书部署排错](docs/deploy-debug-feishu.md)、[飞书设计](docs/superpowers/specs/2026-08-08-feishu-auth-and-source-ingestion-design.md)、[同步恢复设计](docs/superpowers/specs/2026-08-09-feishu-sync-liveness-design.md) |
| 验证记录与截图 | `docs/verification/` |
