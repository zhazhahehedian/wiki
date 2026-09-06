# CLAUDE.md · 能力中心（Capability Hub）

项目事实与资料索引。开发、验证、提交和整理规则统一见 [AGENTS.md](AGENTS.md)；按任务读取相关章节，无需每个 session 通读 spec 或阶段计划。

## 1. 当前方向与实现边界

项目于 2026-09-04 从 it-wiki 知识库 Agent 转向公司内部 MCP / Skill 能力注册、治理、发现与凭证分发平台。当前产品设计见 [能力中心 spec](docs/superpowers/specs/2026-09-04-capability-hub-design.md)。该设计明确替代原 KB/RAG 产品方向。

- 平台提供能力目录、版本与审核、按权限发现、共享凭证保险箱，以及薄的 Token Hub 游乐场。
- 平台不进入 MCP 运行时调用链路；消费者取得 endpoint 和凭证后直连 MCP。v1 不在线执行 Skill，不持久化游乐场会话，不修改 Token Hub。
- 阶段 A 已复用飞书 OAuth、session/CSRF、加密、config/repo、River schema、MinIO 与 Go/Next.js 骨架，提供新导航与 UI。KB/RAG 已从生产启动装配及路由移除；旧实现与历史表暂留，后续通过独立迁移退役数据。
- registry 的阶段 B Owner 草稿闭环与阶段 C 治理已实现（版本、审核、上线快照、可信部门/可见范围、审计）；凭证与机器 discovery 仍待实施；playground 的加密配置与流式代理已实施，真实模型验收另行记录。文档整理不启动产品退役或删库。

## 2. 当前阶段

阶段 A 平台骨架与 UI 已实现；前端支持工作台、注册中心展示切换、游乐场前端与模型接口配置、我的发布、账号信息和移动导航。自动化检查与浏览器预览见 [阶段 A 计划](docs/superpowers/plans/2026-09-05-capability-hub-phase-a-plan.md)。云 PostgreSQL/MinIO 已完成实际连通、迁移与隔离测试；游乐场后端已提前实现，记录见[游乐场后端计划](docs/superpowers/plans/2026-09-05-playground-backend-plan.md)。真实飞书登录、回调与会话已通过，当前本地后端端口为 8088；真实模型调用已由用户于 2026-09-06 明确暂缓（当前不在公司内网），不阻塞后续开发；接续说明见[session 交接](docs/superpowers/plans/2026-09-06-capability-hub-handoff.md)。阶段 B Owner 草稿闭环已实现，云端隔离数据库/MinIO 和浏览器流程验证通过，见[阶段 B 计划](docs/superpowers/plans/2026-09-06-capability-hub-phase-b-plan.md)。用户于 2026-09-06 授权提前实施的 Skill 创建助手已实现：通用流程 / 女娲方法适配、需求追问、素材、编辑预览和保存 Owner 草稿，见 [Skill 引导创建计划](docs/superpowers/plans/2026-09-06-skill-builder-plan.md)。阶段 C 治理已实现并通过隔离云 PostgreSQL/MinIO 与多角色浏览器验收，见 [阶段 C 计划](docs/superpowers/plans/2026-09-06-capability-hub-phase-c-plan.md)及[验证记录](docs/verification/2026-09-06-phase-c/README.md)。草稿供 Owner/Admin 管理，消费者仅见授权上线快照；新版审核不影响旧版上线。D–E 仍待实施。现有开发库的 0014/0015 迁移与常驻服务更新未在本轮执行；运行态启用需使用项目迁移入口、新 server 装配及 BOOTSTRAP_ADMIN_FEISHU_OPEN_ID，真实账号新增角色验收未冒充为已完成。

设计状态存在历史记录差异：该分支原 CLAUDE.md 写“review 通过”，spec 页首仍标“草案（待 review）”。本轮不代替产品评审裁定其状态；涉及未决设计时核实相关决策，其余已明确的工作可继续。

原 it-wiki 阶段 4 的实现与待运行态验收仅作为迁移背景，不再作为能力中心的当前阶段。新阶段完成需有相应验收证据，不因整理文档自动推进进度。

## 3. 技术与命令来源

- Go 要求以 [backend/go.mod](backend/go.mod) 为准（当前 1.25.7）；现有后端为 chi、pgx/sqlc、goose、River、PostgreSQL。v1 运行时不需要 embedding/pgvector；当前历史迁移重放仍需 pgvector 和 EMBEDDING_DIM，独立表退役迁移尚未实施。
- 前端以 [frontend/package.json](frontend/package.json) 和 lockfile 为准：Next.js 15、React 19、Tailwind v4、TanStack Query、Base UI/shadcn；测试使用 Vitest + Testing Library。[components.json](frontend/components.json) 指定 base-nova，按现有组件实际 exports/props 使用。
- Eino 未引入；现有手写 ReAct 属于待退役产品。不要根据早期脚手架示例引入 Eino、Zustand 或 Radix。
- MinIO 用于对象存储；新平台游乐场由用户填写 OpenAI 兼容 API 地址和 API Key，获取模型列表后选择模型与默认模型，也支持手动模型 ID,不依赖 Token Hub 源码或私有接口。当前配置按 users.id 隔离并加密保存,后端按 OpenAI 兼容或 Anthropic 原生协议转发流式对话；对话不持久化。新 UI 按 §11 的白色侧栏、白灰内容与低饱和蓝主操作设计，已去掉渐变和装饰光晕。
- 开发/验证命令见 AGENTS.md 和 Makefile。迁移在 `backend/` 使用 `go run ./cmd/migrate up` 或 `make migrate-up`，支持 Go migrations。
- 当前 checkout 不提供 Compose 编排；已移除引用缺失文件的根 Makefile 目标，使用现有 PostgreSQL/MinIO 与本地 Go/Next.js 命令开发。

## 4. 按任务读取的资料

| 任务 | 资料 |
|---|---|
| 定位、架构、非目标 | [能力中心 spec](docs/superpowers/specs/2026-09-04-capability-hub-design.md) §1–3、§15 |
| 角色、数据模型、治理 | 同上 §5–7；[阶段 C 实施记录](docs/superpowers/plans/2026-09-06-capability-hub-phase-c-plan.md)（已实现，隔离验收通过） |
| 凭证、发现与健康检查 | 同上 §8–10 |
| Skill 对话创建、女娲方法适配 | [Skill 引导创建计划](docs/superpowers/plans/2026-09-06-skill-builder-plan.md)，spec D10；[来源与许可](backend/internal/skillbuilder/prompts/ATTRIBUTION.md) |
| 新 UI、遗留退役、阶段验收 | 同上 §11–13、§16 |
| 复用认证、配置、运行排错 | [飞书部署排错](docs/deploy-debug-feishu.md)；仅复用适用章节，KB 导入内容是历史流程 |
| 遗留摄入/维度维护 | [阶段 1 决策](docs/superpowers/specs/2026-05-20-phase-1-decisions.md) |
| 遗留 RAG/ReAct 维护 | [RAG 设计](docs/superpowers/specs/2026-06-02-phase-2-deterministic-rag-design.md)、[ReAct 设计](docs/superpowers/specs/2026-07-08-phase-3-react-agent-design.md) |
| 遗留飞书同步维护 | [飞书设计](docs/superpowers/specs/2026-08-08-feishu-auth-and-source-ingestion-design.md)、[同步恢复设计](docs/superpowers/specs/2026-08-09-feishu-sync-liveness-design.md) |

`docs/superpowers/plans/` 中旧 it-wiki 计划属于历史参考；能力中心阶段 A 有独立计划。仅在维护或迁移对应内容时参考；旧提交、分支、环境和全站验收指令不自动生效。新方向以能力中心 spec 明确替代的决策为准，其余实现事实仍需核对当前配置与相关代码。

## 5. 按功能触发的质量约束

- 认证复用：保留 session/CSRF/Origin、加密与 request_id 日志；不要把旧 KB owner bootstrap 当作新平台 admin bootstrap 已实现。新配置在对应实现时同步 `.env.example`。
- 能力治理与凭证：验证 Owner/Admin 权限、可见范围、审核状态转换、原子上线、凭证获取权限与审计、加密和轮换；Discovery 元信息不包含凭证。具体产品契约见新 spec。
- 配置秘密使用 env；平台托管的 MCP 凭证和用户 Token Hub key 按设计加密存储。两者都不得进入 Git 或日志，不能将“secret 走 env”泛化为禁止产品凭证入库。
- 后台持久化业务任务使用 River；健康检查范围是已发布 MCP 的存活。相关改动说明并验证重试策略，不继承旧飞书 reconciler 的产品边界。
- 遗留功能维护时继续保护 checksum 去重、owner 隔离、SSE 取消和旧快照失败保留；退役任务则按新 spec 删除其产品路径和迁移表，不为了旧护栏保留已明确退役的功能。
- 依赖、部署或外部协议相关实现按当前环境核实，不能把历史 spec 中的版本描述视为已验证的协议事实。

环境配置以 [.env.example](.env.example) 和实际配置入口为准；此处不复制整份配置。新平台未实现的变量、路由和模块应明确标为规划。
