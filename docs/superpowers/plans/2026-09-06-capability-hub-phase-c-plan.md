# Capability Hub · 阶段 C 治理实施记录

状态：已实现，隔离云端与浏览器验收通过；现有开发库部署和真实人员角色配置未执行。用户于 2026-09-06 授权启动阶段 C；实施契约见 spec §7.1。范围来自 [能力中心 spec](../specs/2026-09-04-capability-hub-design.md) §5–7、§11、§13。下列接力问题已按现有基础收敛到 spec §7.1；实现与验证入口见本文末尾。

## 目标与边界

完成 Owner 提交 → Admin 审核通过/驳回 → 上线/下线 → 审计查询的治理闭环，落实已发布能力的 org/allowlist/department 可见范围。阶段 D 的凭证存取、领取审计与系统 discovery API，阶段 E 的健康任务仍留在对应阶段。公司模型联调继续暂缓；C 无需模型连接即可验证。

已有基础：B 的 capabilities/versions/allowlist、revision、current_version_id/draft_version_id、Owner 隔离、事务仓储和 Skill 包；认证/session/CSRF、SQL/MinIO、Skill 创建助手均已实现。启动 C 前尚无 Admin/审核/审计；本轮已新增角色 bootstrap、可信部门、审核和审计模块，侧栏与 `/hub/[section]` 已接 reviews/audit/profiles。

## 开发前收敛的契约（已落实，见 spec §7.1）

1. **已上线版本与新草稿的关系。** spec 规定新版本走审核且原子切换上线指针；现有能力仅一个 status，名称/描述/可见范围/名单也是能力级可变字段，Save 只接受 draft。仅保留 current_version_id 不足以保证上线内容不受草稿编辑影响。明确新版编辑和待审时旧版是否继续对消费者可见，以及元数据/名单是否随审核一起生效；再决定版本快照或独立草稿状态等最小 schema 调整。不要直接把 published 改成 draft 而无意令旧版消失，也不能让待审附件或名单通过公开接口生效。
2. **部门与身份。** 现有 users/profile 没有已实现的部门授权字段；表单里的 capability.department 只是 Owner 填写的发布配置。spec 允许飞书资料或用户 profile，但实施前需明确来源和信任程度。不要把客户端自由提交的“当前用户部门”当权限证明；缺少可信部门时不能因空值相同而放行。本轮采用 Admin 维护的可信 profile，不新增 OAuth scope，org/allowlist/department 均已实现和测试。
3. **Admin 初始化。** 采用 spec 的 `BOOTSTRAP_ADMIN_FEISHU_OPEN_ID`，持久化角色并遵循“只补空、不覆盖”。明确未登录目标账号的首次绑定、已有管理员时的行为和身份撤销规则；复用稳定的 Feishu 身份映射，不复用旧 KB 的 owner bootstrap 作为管理员权限。当前该 env 尚未接入配置/启动代码。
4. **审核对象与强制操作。** 审核需绑定提交的版本和 revision，待审内容不可被静默修改。明确重复提交/审批、驳回理由、Owner 下线、Admin 强制上线/下线、offline 重新提交，以及 Admin 审核自身能力的规则。沿用 spec 已明确流程；未明确规则写入 spec 后实施，不能靠前端按钮决定权限。
5. **已发布 Skill 格式。** 手动上传在 B 只要求根 SKILL.md 和包路径等基础校验，manifest 来自表单；创建助手已经自动构造 frontmatter。C 的提交/审核应对两条入口一致校验必要 frontmatter、附件引用和导出格式，保留旧草稿可修复，不把历史草稿直接判定为已上线格式合格。

## 实施顺序与结果

### C1 · 角色与数据库基础

- [x] 收敛上述上线/草稿、部门身份和审核契约，更新 spec 的对应条款。
- [x] 检查迁移目录最新编号，新增治理/角色/审计及必要快照 schema 的 goose 迁移；启动 C 时最新为 0014；本轮新增 0015，不修改既有迁移来替代新迁移。
- [x] 按需要更新 sqlc queries 并生成代码，保持 repo 为 SQL 边界。
- [x] 实现 Admin bootstrap 与服务端角色读取；给前端提供必要的当前用户权限元数据，客户端不能自行授予角色。
- [x] 独立数据库 up/down/up，验证保留 B、游乐场及用户数据，回滚不误删已存在的表。

### C2 · 治理状态机与原子审计

- [x] 实现 Owner 提交、Admin 通过/驳回、Owner/Admin 下线、Admin 强制操作、offline 重提；每种操作校验身份、状态、版本与 revision。
- [x] 审核中锁定提交内容；上线版本不可被同名草稿写接口覆写；发新版有明确入口。
- [x] 状态变化、指针切换、必要的元数据/名单生效与 audit_log 在同一数据库事务完成；事务失败不得出现“已上线但没有审计”。
- [x] 审计记录 actor、action、target、必要前后状态/版本、理由、request_id 和时间；不写 API Key、凭证或完整模型素材。
- [x] 确定 B 的 create/edit 从 C 起如何记审计；历史操作不伪造 actor/时间补录。Skill 创建助手保存也复用同一注册服务审计入口。

### C3 · 查询和访问控制

- [x] 保留独立的 Owner 管理查询，Admin 审核查询服务端授权；普通目录仅返回允许访问的已发布内容。
- [x] Owner/Admin 可访问待审版本供管理，消费者只能访问授权的上线版本；列表、详情、版本与 zip 下载共用权限规则。
- [x] org/allowlist/department、offline、缺失身份、直接猜 slug/versionID 的行为一致；消费者详情不泄露草稿、完整名单或管理审计。
- [x] 目录与“我的发布”目前复用同一 Owner API/query key，开放团队浏览时分离查询含义和缓存键，不能只去掉 SQL 中的 owner 条件。
- [x] 为 D 留下统一权限判定和上线内容读取接口，不提前实现凭证或机器 token。

### C4 · HTTP 与界面

- [x] 设计提交/通过/驳回/上下线的明确动作端点，使用既有 session/Origin/CSRF，返回可辨识的未授权/状态冲突/版本冲突错误；不能直接信任客户端 PATCH status。
- [x] Owner 详情页展示状态、提交和下线入口、驳回理由、重新编辑/提交流程，以及上线版/待审版区分。
- [x] 接通 Admin 审核队列与审核详情，提供工具或 Skill 文件内容及版本差异供判断；必须能查看实际待审内容，而不只有名称/版本号。
- [x] 接通有权限门禁的审计列表/筛选/分页，更新侧栏占位和账号权限展示。前端隐藏按钮只辅助交互，服务端继续独立鉴权。
- [x] 同步保留 Skill 创建助手产出的草稿、保存失败内容、手动上传和下载体验。

### C5 · 验收与交接

- [x] 回归状态矩阵、Owner/Admin/普通用户越权、CSRF、重复操作、过期 revision、审批与编辑并发、事务中途失败、审核对象不漂移。
- [x] 验证旧版与新版按最终契约隔离；审批切换后列表/详情/下载一致；下线后消费者访问受阻。
- [x] 验证三种可见范围及缺失部门、未登录、非名单成员、猜测草稿版本附件；审计仅授权角色可读且内容不含秘密。
- [x] 后端影响包测试、集成测试与阶段完成时全量测试；前端相关 Vitest、typecheck、路由/服务端边界 build。
- [x] 用隔离云 PostgreSQL/MinIO、Owner/Admin/第三方测试账号跑浏览器闭环；模拟身份与真实飞书运行态验收分别记录，模型不作为 C 前提。
- [x] 更新本清单、spec、验证记录与 CLAUDE.md 的实际阶段状态；清理本阶段测试资源。本轮未提交/推送，按用户后续授权执行。

## 代码入口

| 内容 | 当前入口 |
|---|---|
| 产品事实与工作规则 | 根目录 CLAUDE.md、AGENTS.md |
| 认证与启动配置 | backend/internal/auth、backend/internal/http/auth_handler.go、backend/internal/config/config.go、backend/cmd/server/auth.go、backend/cmd/server/main.go |
| Registry 契约/校验 | backend/internal/registry/model.go、backend/internal/registry/service.go |
| SQL 与事务 | backend/internal/repo/queries/registry.sql、backend/internal/repo/registry_repository.go、backend/internal/repo/migrations/0014_capability_registry.sql |
| HTTP 路由 | backend/internal/http/registry_handler.go、backend/internal/http/router.go |
| 云测试与身份 fixture | backend/internal/http/registry_integration_test.go、backend/internal/http/skill_builder_integration_test.go |
| 管理与目录界面 | frontend/components/hub/registry-overview.tsx、capability-detail.tsx、capability-editor.tsx、hub-shell.tsx；frontend/app/hub/[section]/page.tsx |
| API/缓存 | frontend/lib/api/registry.ts、frontend/lib/api/auth.ts |
| Skill 创建助手 | backend/internal/skillbuilder、frontend/components/skill-builder |

运行前提、验证证据和新 session 启动提示见 [接力交接](2026-09-06-capability-hub-handoff.md)。

## 本轮交付与验证入口

- 新增 0015 迁移、`queries/governance.sql` 及 sqlc 输出；`registry/governance.go`、`skill_review.go` 与 `repo/governance_repository.go` 实现原子治理及权限。
- HTTP 动作沿用 session/Origin/CSRF；新增 `/catalog`、`/governance/me|reviews|audit|profiles`、版本文件预览与明确的治理动作路由。创建助手保存继续走 registry Save，事务内记录 create。
- 前端新增审核/审计/部门授权、待审与上线内容对照和文件预览，修正编辑器/创建助手的列表缓存刷新。仪表盘与帮助文案同步 C 状态。
- 证据与可复现入口见 [阶段 C 验证记录](../../verification/2026-09-06-phase-c/README.md)。真实账号角色配置与常驻服务切换属于运行态前提，未冒充本轮隔离测试结果。
