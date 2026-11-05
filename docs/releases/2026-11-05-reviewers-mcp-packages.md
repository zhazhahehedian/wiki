# 2026-11-05 交付与 Claude Code 接力：审核员、MCP 程序包

目标分支：`main`。本批包含两项功能，均已在本地实现并验证；用户随后授权提交、推送 GitHub，便于公司电脑接力。公司服务尚未因本批交付而升级，不能将代码同步视为部署完成。

## 本批功能

1. **独立审核员**：管理员在「用户与审核授权」授予/撤销；第一版全平台 MCP / Skill 都可审核，允许自审。审核员可读必要待审材料并通过/驳回，不获得平台管理、无关私有能力或知识库权限。审核与撤权的并发顺序由数据库锁保证；发布 API 凭证不继承审核员专属权限。
2. **MCP 程序包上传和下载**：注册/编辑上传单个 ZIP（最大 100 MiB、2000 条目、解压后 500 MiB）；支持无 Endpoint 的 stdio/HTTP 程序包，也兼容已有远程 Endpoint。审核后，授权使用者在市场详情下载并自行运行；管理/审核页可下载其允许查看的版本。平台复用现有 MinIO 适配器存储，不公开 bucket 地址、不托管运行。

版本替换、旧上线版本持续可用、发布快照切换、权限收紧、下线、CSRF 和 API scope 边界保留。ZIP 不落盘解压或执行，校验路径、重复、特殊文件、加密、体积与 CRC。

## 公司电脑接力入口

先读根目录 `AGENTS.md`、`CLAUDE.md` 和本文；按主题查看：

- [审核资格设计](../superpowers/specs/2026-11-05-reviewer-authorization.md)及[审核员验证记录](../verification/2026-11-05-reviewer-authorization/README.md)。
- [MCP 分发设计](../superpowers/specs/2026-11-05-mcp-package-distribution.md)、[上传下载验证及截图](../verification/2026-11-05-mcp-packages/README.md)、[发布 API](../publishing-api.md)。

在公司 checkout 检查本地改动后，用 `git pull --ff-only origin main` 获取代码；有本地改动或分叉时先保留、核对，不强制重置。不要把 Windows 的 node_modules 或构建产物带到 Linux；沿用公司现有 Docker/构建部署方式。不要直接套用历史部署记录推断当前服务器状态。

## 数据库与发布注意事项

- 新增 `0020_platform_reviewers.sql`：profile 增加 `is_reviewer`，默认 false；原管理员保持原权限。
- 新增 `0021_mcp_packages.sql`：允许 MCP 绑定 ZIP 和 stdio；复用历史列 `skill_bundle_key`，无需重新上传旧 Skill。sqlc 已再生成，相关输入和合法生成输出一并交付。
- 升级前确认业务库当前迁移版本并做好备份，保留现有数据。使用项目 `backend/cmd/migrate`（`go run ./cmd/migrate up`）；server 启动也会自动迁移。不要用裸 goose 命令替代项目入口，仓库含 Go 迁移。
- **回退限制**：已有 MCP 程序包/stdio 版本时，0021 down 会明确拒绝，避免丢包或改写已发布版本。0020 down 会移除审核员标记，重新 up 默认 false。不要把数据库回退当作常规清理。
- 既有首次管理员 bootstrap 已是单例机制；不能通过改环境变量或重置 bootstrap 给领导提权。真实审核员由现有管理员在页面授权，用户需先登录一次。
- 更新前后端服务后，再验证公司反向代理的请求体大小/超时与现有 MinIO 配置。100 MiB 上传沿用当前 API 超时及有界内存缓冲，尚无分片上传/断点续传。

## 已完成与待完成的验证

已完成：Go 全套测试与服务构建；隔离真实 PostgreSQL 的迁移 up/down、原数据保留、角色/并发/版本/可见性集成；发布 API 上传下载及 scope；前端 26 文件、123 测试、typecheck、生产构建。最后的小范围整理后，受影响后端测试、前端 12 项相关测试和 typecheck 再次通过。

浏览器使用本地虚构账号实际执行了文件选择器上传 → 保存无 Endpoint 草稿 → 提交 → 独立审核员通过 → 普通使用者下载；上传来源与最终下载 SHA-256 一致。截图与细节见上面的验证记录。临时服务、容器、一次性数据库与下载样本已清理。

**公司环境仍需执行**：确认实际服务版本/配置并升级；用真实登录身份验证管理员授权、审核员自审和全平台审核；用公司 MinIO 完成 MCP ZIP 上传、审核、普通使用者下载、字节校验，并验证撤权/下线后拒绝新下载。检查原有远程 MCP 和 Skill 下载仍正常。平台不负责运行包内 MCP，包内 README/依赖/启动命令由发布者提供。

本轮没有对公司业务库、MinIO、真实账号权限作改动。模型出网、K1 真实知识写入等既有环境问题不因本批功能而宣称解决。

## 公司电脑 review 后补充（未提交前请看）

按用户确认继续在公司电脑开发：上传/下载 3 分钟超时、并发上传上限、迁移 0022（`bundle_key` 更名、包 SHA-256/大小、上传疑似凭证扫描）、下载审计与校验头、MCP 包 README/文件预览、下载次数与下载记录、飞书消息通知（默认关闭，配置见 [飞书消息通知](../feishu-notifications.md)）、MinIO 孤儿包定期清理、Skill/MCP ZIP 规则合并；删除未接入的 K2 飞书文档加载与旧 source 领域代码（约 2000 行，需要时从 git 历史 `4ecaf52` 恢复）；前端 `shadcn` 移至 devDependencies。设计见 [MCP 分发补充](../superpowers/specs/2026-11-05-mcp-package-distribution.md)。

- 0022 与全部注册/治理/MCP 包/通知集成已在公司电脑 Docker 一次性 PostgreSQL 18（pgvector 0.8.2）与本地 MinIO 上通过，见[隔离验证](../verification/2026-11-05-company-hardening/README.md)；业务库与公司 MinIO 仍未改动。
- 0022 down 只删除可推导的校验与扫描数据；之后 0021 的原有守卫照常生效。
- 飞书通知只做过模拟上游单元测试；开启前需启用机器人能力与 `im:message:send_as_bot`，并用真实账号验收。
- 公司反向代理需允许约 100 MiB 请求体与不少于 3 分钟的读写超时。

## 未包含的本机原有工作

`deploy/offline/`、`deploy/README.md`、`docs/deploy-debug-feishu.md` 和 `backend/internal/http/auth_handler_test.go` 的原有离线部署/内网 HTTP 排错改动不属于这次功能批次，保留在原电脑工作区，未随本次提交上传。公司已有部署目录与配置应继续保留；不要因为 GitHub 中没有这些本地文件就删除公司文件。私有配置、凭证、缓存及离线镜像包不进入本次交付。
