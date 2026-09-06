# 阶段 B 验证记录 · 2026-09-06

## 验证环境

- Go 1.25.7；Next.js 15.5.18、项目锁定 pnpm 9.12.0，未升级依赖。
- 原 Windows node_modules 缺少 Linux rolldown 原生绑定。使用独立 WSL 源码副本及既有 Linux 依赖缓存运行前端检查；没有替换原工作区依赖、修改 lockfile 或 `.env.local`。
- 云 PostgreSQL 每次创建 UUID 临时库，MinIO 使用随机对象 key；测试结束只删除本次创建的数据库与对象。
- 浏览器使用独立 Edge headless profile，前端 13000 / 测试后端 18089，临时用户通过真实 session/CSRF 中间件。此 fixture 不接真实飞书 OAuth，不会编入生产 server；模型接口未接入 fixture，页面 provider 的 playground connection 请求为 404，不代表真实模型验收。

## 已通过

- 后端全量 `go test ./...`；`go build ./cmd/server ./cmd/migrate`。默认测试套件中需要外部 env 的旧集成测试仍按原规则 skip，下面单独记录实际执行的云测试。
- `TestRegistryCloudIntegration`：0014 up/down/up；回滚保留 user_llm_keys；session/CSRF/Owner 隔离；重复 slug/旧 revision/历史版本不可覆盖；并发更新一胜一冲突；SQL 故障时元数据、名单和版本整体回滚；字面搜索、类型/部门/状态与分页；MCP 版本和跨能力指针外键；Skill MinIO zip 往返及下载权限。
- registry 单测：工具 schema、URL、名单、保留 slug 校验；目录穿越/重复/文件目录冲突、数量/体积和平台特殊文件名；上传失败不保存版本；不确定提交不误删附件。
- 浏览器实操：MCP 创建 → 详情/工具参数 → 编辑并创建新版本 → 查看历史；Skill 多文件上传 → 下载；卡片/表格与搜索；“我的发布”移动端（390 px，无横向溢出）。无未捕获页面异常。预填 textarea 标签关联的问题已修复并重测。
- 前端全量 Vitest：33 个文件、137 项通过；TypeScript 与生产构建通过。sqlc 生成与 diff 空白检查通过（使用 `core.autocrlf=true` 识别该混合 Windows/WSL 工作区的实际差异）。

## 截图

以下来自云端临时库的真实 UI 操作，含测试用户与测试条目；截图中的“审核中”由隔离测试 SQL 设置，用于验证只读编辑限制，不能视作阶段 C 审核功能已实现。

- [MCP 创建表单](mcp-editor.png)
- [MCP 详情与工具 schema](mcp-detail.png)
- [注册中心卡片](registry-cards.png)
- [注册中心表格](registry-table.png)
- [我的发布移动端](publications-mobile.png)

补充最终布局检查：使用固定只读 mock 数据验证移动端搜索框宽度、详情/编辑页导航选中态和工作台创建入口。[最终移动端布局](registry-mobile-layout.png) 为该检查的截图，不计作额外云运行态验收。

`TestPlaygroundCloudIntegration` 也已单独通过（迁移回滚固定到历史 0013，再升级至最新 schema）；它使用本地假模型验证既有代理逻辑，不调用公司模型。

## 边界

B 是 Owner 草稿闭环：配置的 org/department/allowlist 不开放跨用户读取。Admin、审核上线、凭证、Discovery 和健康检查在后续阶段实施。Skill 保存原始 SKILL.md 与附件，不执行脚本；Claude 专用 frontmatter 校验/完整兼容验收尚未实施。公司真实模型调用继续按用户要求暂缓。

既有开发库未在本次更新 schema，原常驻服务未重启。接入新代码时，在已有安全环境变量下从 backend 运行 `go run ./cmd/migrate up`（至 0014），再启动新 server；禁止对现有库运行 reset。
