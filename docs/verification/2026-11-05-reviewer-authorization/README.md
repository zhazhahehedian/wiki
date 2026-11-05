# 全平台审核员验证记录

日期：2026-11-05。对应[审核资格设计](../../superpowers/specs/2026-11-05-reviewer-authorization.md)。

## 交付内容

- 独立 Reviewer，第一版审核全平台 MCP / Skill，允许自审；管理员在“用户与审核授权”页为已登录用户授予/撤销，必填原因并保留修订校验和原子审计。
- 审核员可读待审版本及当前上线版本对照、预览/下载相应 Skill 文件、通过/驳回。审核完成后的非 Owner/Admin 仅收到操作回执并返回队列。
- 设置页显示角色和审核范围；提交详情显示可审核人员和无人可审核提示。审核员不会获得用户管理、完整审计、强制上线、管理性下线、可见范围覆盖或知识库权限。
- 发布 API 不继承 Reviewer 专属权限；现有 Owner/Admin、上线可见范围、Session/Origin/CSRF、版本和发布快照边界保留。

## 实际验证

使用 Windows 上已有 Docker Desktop 的 `pgvector/pgvector:pg17` 镜像建立本任务独立容器，数据库端口仅绑定回环地址。测试在该容器创建 UUID 命名的一次性数据库；未读取根 `.env`，未连接公司或公开站业务库。Go、Node 与现有 Linux 依赖均在 Ubuntu WSL 中执行，前端使用 Corepack 提供项目指定的 pnpm 9.12.0。

| 检查 | 结果 |
|---|---|
| `sqlc generate`、`gofmt` | 完成；生成差异只涉及治理查询、profile 模型与接口 |
| `go test ./...` | 通过；通过显式 `REGISTRY_TEST_DATABASE_URL` 启用 Registry/Governance 隔离数据库集成 |
| `go build ./cmd/server`（输出到临时路径） | 通过 |
| `TestGovernanceCloudIntegration` | 通过，新增审核员用例见下文；补充用例后再次单独通过 |
| `TestDirectoryPublishingIntegration` | 使用同一独立 PostgreSQL 容器、另一个一次性数据库通过 |
| 前端 `pnpm test` | 26 个测试文件、118 项通过 |
| 前端 `pnpm typecheck` | 通过 |
| 前端 `pnpm build` | 通过，21 个静态页面生成完成；未改依赖版本或锁文件 |
| 浏览器验收 | 本地 Next.js + 测试路由 + 一次性数据库，通过下述页面流程 |
| `git diff --check` | 通过 |

新增集成覆盖：普通用户/审核员不能自授角色、CSRF、伪造 `is_admin` 字段、缺失布尔值、未登录目标、过期修订号、授权审计失败回滚、迁移 20 的 down/up、审核人分页、撤权后名单移除、MCP 通过/驳回、审核员自审、Skill 待审/上线/历史版本隔离、撤权后文件拒绝、旧页面审批拒绝、市场和私有知识库隔离、发布 API 上下文不能读取待审材料、管理员专属动作拒绝。

并发撤权采用实际 PostgreSQL 锁验证两个顺序：

1. 撤权先持有角色写锁：批准操作等待，撤权提交后批准失败。
2. 批准先持有角色共享锁：撤权等待，批准完整发布并写审计后撤权生效。

原治理集成继续覆盖并发批准、旧版持续上线、快照切换、审计回滚、强制操作和 Admin 自审。

浏览器使用测试身份完成：管理员按姓名搜索并撤销/重新授予审核资格；审核员仅显示审核队列入口，打开待审详情后通过并返回空队列；设置页显示审核员和“全平台 MCP / Skill，允许自审”；Owner 详情显示管理员和审核员名单；普通用户直接访问审核队列显示无权限提示。检查桌面截图与 390 像素窄屏，窄屏页面未发生横向溢出，结束时恢复默认视口。

## 验收边界

- 浏览器身份、模型上游与 Skill 存储使用测试 fixture；不等于真实飞书账号、MinIO 或公司环境发布验收。
- 全套 Go 测试中未开启无关的远程模型/Playground/K1 全链路集成；本次另行验证了 Reviewer 不能列出、读取或管理他人的私有知识库。
- 产品 MCP 保险箱/机器发现仍未实施，本次未新增该类权限。
- 迁移 20 回退会移除新审核员标记；重新升级默认均为 false。原管理员、能力记录和审计保留，已在一次性数据库验证。尚未在业务库执行迁移。
- 验收时未提交、推送或部署，也未给真实用户授权。用户随后授权提交并推送，见[本批交付与接力说明](../../releases/2026-11-05-reviewers-mcp-packages.md)；部署和真实用户授权仍待公司环境执行。原有部署和登录排错工作保持原状。

## 本机 Docker 故障处理

测试期间 Docker Engine 可从 Windows 访问，但 Ubuntu 集成等待 `backend.sock` 失败；单独重启 Docker Desktop 未恢复完整连接。正常退出 Docker Desktop，执行 `wsl --shutdown`，再启动 Docker Desktop 后，Windows 与 Ubuntu 内的 `docker version` 均连接到 Engine 29.9.2，测试容器恢复可用。未重装 daemon、重置数据或改动 WSL 配置。类似症状见 [Docker 问题记录 #582](https://github.com/docker/desktop-feedback/issues/582)；本次以本机实测恢复为结论，不把问题记录的原因推断当作已证实根因。
