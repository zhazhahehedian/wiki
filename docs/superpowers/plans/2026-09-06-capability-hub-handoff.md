# Capability Hub · 新 session 接力（2026-09-06）

本文件已合并 A、B 和 Skill 创建助手的最新状态，替代本文件此前“从 B 开始”的交接。下一阶段为 **C 治理**；当前 session 按用户要求只整理接力材料，C 尚未开始。

## 新 session 可直接使用的提示词

> 继续 Capability Hub，先阅读 CLAUDE.md、2026-09-06-capability-hub-handoff.md 和 2026-09-06-capability-hub-phase-c-plan.md，保留当前全部未提交改动，开始阶段 C 治理开发。先结合 spec 收敛上线版本与新版草稿隔离、Admin bootstrap 和部门权限契约，再实现状态机、审核队列、访问控制与审计。保留 Skill 创建助手；公司模型联调继续暂缓。不要提交或推送。

## 工作区与授权边界

- Windows：`E:\workspace\GoProject\wiki`；WSL：`/mnt/e/workspace/GoProject/wiki`。执行前确认 cwd，避免应用把 Windows 路径拼到 WindowsApps 目录。
- 当前分支经本次读取确认为 `feat/capability-hub-pivot`。规则、旧 KB 路由退役、A、游乐场、B、Skill 创建助手等修改全部未提交/未推送，包含大量新增文件；不得 reset、覆盖、盲目清理或将它们误判为无关残留。
- WSL Git 与 Windows 换行符配置不同。检查内容使用 `git -c core.autocrlf=true -c core.safecrlf=false diff`；不要批量转换换行符。该 diff 不显示 untracked 文件内容，审阅时一并检查新增文件。
- 新 session 以用户发出的任务为授权依据；本交接不执行提交、推送、部署、删库或 C 实现。不要因旧计划的 commit/分支/Skill 清单重复询问已授权的例行操作。

## 当前已实现

| 模块 | 已实现范围 | 资料 |
|---|---|---|
| A 平台骨架 | 飞书 OAuth、session/CSRF、Go/Next.js shell、导航和设置；旧 KB/RAG 从生产装配及路由退役，遗留代码/表留待独立迁移 | [A 计划](2026-09-05-capability-hub-phase-a-plan.md) |
| 提前实施 F 游乐场 | 用户级 AES-GCM 模型配置、模型发现/选择/手动 ID、OpenAI 兼容与 Anthropic 原生协议、流式对话/停止/重新生成；对话不持久化 | [游乐场计划](2026-09-05-playground-backend-plan.md) |
| B 注册中心 | MCP/Skill Owner 草稿创建编辑、版本历史、卡片/表格/筛选分页、详情、文件包上传下载；revision 与事务保护；草稿仅 Owner 可见 | [B 计划](2026-09-06-capability-hub-phase-b-plan.md) |
| Skill 引导创建 | `/hub/registry/create-skill`；通用流程/女娲方法适配、追问、粘贴/导入素材、正文/附件编辑预览、校验后保存 Owner 草稿；复用用户模型配置 | [Skill 计划](2026-09-06-skill-builder-plan.md) |

C 治理、D 凭证与发现、E 健康检查尚未实施。当前“可见范围”是保存的发布配置，并未开放跨用户读取。草稿保存不等于审核上线；current_version_id 尚未由生产审核流程设置。女娲只作为固定版本的生成方法模板，不执行外部 Skill、不联网研究或运行脚本；来源/MIT 许可见 [归属说明](../../../backend/internal/skillbuilder/prompts/ATTRIBUTION.md)。

## 最新验证与运行状态

- 后端全量 `go test ./...`、server 构建通过；后续目录取消请求修复及保留 slug 回归的影响包测试通过。B 时 migrate 构建也已验证。
- 最新前端为 **35 个测试文件、144 项测试**通过，typecheck 与生产构建通过。127/137 是更早 A/B 的历史计数，不是最新状态。
- B 和 Skill 创建助手分别完成独立云 PostgreSQL/MinIO 与 Edge 浏览器验证；Skill 流程含追问、素材导入、编辑/预览、无效输出保留、保存、zip 下载、女娲模板和移动端。模拟 OpenAI/Anthropic 上游验证协议；不代表公司模型真实连通或生成质量通过。
- 最新证据：[B 验证](../../verification/2026-09-06-phase-b/README.md)、[Skill 验证及截图](../../verification/2026-09-06-skill-builder/README.md)。默认 `go test` 跳过未配置的云端测试，显式运行记录才是云端验收。
- 真实飞书登录、回调和会话已在 A 验证；真实退出登录后的失效检查仍待补，自动化 logout/session/CSRF 已通过。
- 本次交接检查时 3000、8088、13000、18089 没有监听进程。临时 Linux 构建副本、浏览器脚本、测试库和测试对象已清理；新 session 按需启动自己的服务。

## 数据库与环境注意点

- 仓库迁移最新 **0014**；既有云开发库上次确认在 **0013**，B/Skill 开发没有迁移该库。本次仅整理文档，未连接开发库复核版本；运行前读取当前版本，使用项目入口在 `backend/` 执行 `go run ./cmd/migrate up`。Skill 创建助手本身没有新增迁移。
- 后端 server 启动会执行迁移。做隔离测试时明确数据库目标，不要以“查看启动”为由无意更新共享库。C schema 变化需新增 migration 并在独立库验证 up/down/up，不编辑 0014 代替升级。
- 根 `.env` 为平台配置，`frontend/.env.local` 为前端地址；均被 Git 忽略。不要回显或复制密钥到交接/日志。后端不自动读取根 `.env`，启动时向进程传入配置。
- 常规本地约定：前端 3000、后端 8088；回调 `http://localhost:8088/api/v1/auth/feishu/callback`。前端显式 `pnpm dev --port 3000`，避免继承后端 PORT。启动前核实当前配置与端口，而非假设服务仍在运行。
- 云 PostgreSQL/MinIO 之前直连曾握手断开，现有本机 7897 代理路径已验证；数据库支持 SOCKS5，HTTP 使用标准代理。不要重复修改服务器安全组排查已定位的本地路径问题。
- `deploy/docker-compose.yml` 仍缺失，Compose 目标不可用；不阻塞直接 Go/Next.js 和云端隔离测试。历史迁移重放仍需 pgvector/EMBEDDING_DIM，生产能力中心不做 embedding；遗留表退役另做专门迁移。

## WSL 验证工具线索

- Go：`/usr/local/go/bin/go`；sqlc：`/home/dpbug/.local/bin/sqlc`。
- Linux Node：`/home/dpbug/.nvm/versions/node/v24.19.0/bin`。在 frontend 包目录运行 pnpm，按 packageManager 使用 **pnpm 9.12.0**；不在根目录误用另一版本。
- 仓库 node_modules 为 Windows 环境，WSL 曾缺少原生绑定。已验证做法：复制 frontend 源码到自己的临时 Linux 目录，排除 node_modules、.next、.env*、tsbuildinfo，链接已有 `/home/dpbug/.cache/wiki-phase-a-build-20260905/node_modules`。本次已确认该缓存与 Node/sqlc 路径仍存在；新 session 先核对依赖版本，勿覆盖 Windows 依赖或无关缓存。
- 云端 registry 测试使用 `REGISTRY_TEST_DATABASE_URL`、`REGISTRY_TEST_S3=1`，自行创建 UUID 数据库及对象，测试后清理。变量由本地配置传入且不能回显。可选浏览器 fixture 见 Skill 验证文档；其中测试身份不计为真实飞书登录验收。

## 接下来先做什么

按 [阶段 C 接力清单](2026-09-06-capability-hub-phase-c-plan.md) 推进，先读 spec §5–7 和实际 repo 边界。优先收敛新版草稿/上线内容隔离、Admin 初始化、部门授权数据来源和审核对象锁定，然后依次做 schema/角色、原子状态机/审计、权限查询、审核 UI 与集成验收。

公司模型真实联调继续由用户明确暂缓，不反复索要公司 Key，不作为 C 的阻塞项。保留当前 Skill 创建助手，不扩展成自动研究或执行系统。完成 C 后再整理 D 凭证与发现的接力内容。
