# 2026-09-06 Capability Hub 推送说明

本批次从 `feat/capability-hub-pivot` 推送并合并到 `main`，按以下三组提交保留功能说明。此前已推送的产品转向设计一并随分支合入。

## 1. 后端：能力注册、治理、模型接口与 Skill 创建

提交：`3dc9a72`。

- 生产启动切换为 Capability Hub，复用飞书 OAuth、session/CSRF、PostgreSQL、MinIO、加密和 River schema；旧 KB/RAG API 不再挂载。
- MCP/Skill Owner 草稿创建编辑、版本、文件校验、附件与 ZIP 导出。
- 提审、通过、驳回、下线、新建草稿、强制上线与可见范围调整；原子审计、不可变发布版本及独立上线快照，新版审核期间旧版仍可用。
- 首次 Admin bootstrap、可信部门资料、管理端权限及按授权过滤的消费者目录。
- 按用户加密保存模型连接，OpenAI 兼容 / Anthropic 原生协议、模型发现、SSE 流式代理、取消、参数范围校验及受控网络目标。
- Skill 创建助手支持通用流程与女娲方法适配、需求追问、文本素材、文件预览，用户确认后保存 Owner 草稿。
- 包含 0012–0015 migrations、SQL 输入及对应 sqlc 生成代码，以及功能和权限回归测试。

## 2. 前端：工作台、治理页面与游乐场

提交：`04bc3a0`。

- 新 `/hub` 入口与导航；旧 `/kbs/**` 入口重定向。注册中心卡片/表格、搜索筛选、发布编辑、版本详情、我的发布、审核队列、审计和部门授权。
- 模型接口配置支持获取/勾选模型、手动模型 ID 与默认模型；游乐场支持临时流式对话、停止与重新生成。
- 输入工具栏提供模型选择及独立参数浮层，未启用的参数不发送；处理 Claude 参数限制，保留系统提示词及恢复默认操作。
- Skill 助手可编辑生成结果并确认保存。
- 界面统一为白色侧栏、白灰内容与低饱和蓝主操作，移除渐变与光晕；保留灰/琥珀/绿状态文字与颜色，适配移动端。

## 3. 清理与文档

- 默认 `.env.example` 移除旧 LLM、Embedding 调用、KB splitter/sync/RAG、旧 Owner bootstrap 与无编排消费者的容器初始化变量；保留当前平台配置及历史迁移必需的 `EMBEDDING_DIM`。真实 `.env` 不修改、不提交。
- 根 Makefile 移除依赖缺失 Compose 文件的命令，提供明确的本地构建/检查目标；删除 backend Makefile 中未引用变量。
- 忽略规则集中到根 `.gitignore`，删除重复的前端忽略文件及已有结果目录中的多余 `.gitkeep`。保留 Docker 打包依赖的 `frontend/public/.gitkeep`。
- 将本次已修改文本文件的换行恢复为 Git 中的 LF，避免几万行无意义差异；未全仓格式化。
- 更新 README/CLAUDE/AGENTS、当前设计、阶段记录和截图证据；历史文档保留用途边界。文档契约测试区分当前平台配置与遗留配置。
- 历史后端代码、历史表及 migrations 保留，用于回归与后续专门退役；不执行删库、卷清理或数据重置。

## 验证与启用边界

合并前后端全量测试与 server 构建、sqlc 生成一致性检查、前端 155 项测试、lint、typecheck、生产构建、Makefile 帮助和 diff 检查均通过。阶段性的隔离 PostgreSQL/MinIO、模拟多角色和浏览器证据见 [阶段 B](../verification/2026-09-06-phase-b/README.md)、[阶段 C](../verification/2026-09-06-phase-c/README.md)、[Skill 创建](../verification/2026-09-06-skill-builder/README.md)与 [UI](../verification/2026-09-06-ui-refresh/README.md)。本次清理没有新增 schema，未重复运行需显式 opt-in 的云服务集成。

真实飞书登录已在前序联调中验证；公司模型调用按用户要求暂缓，当前推送不冒充模型验收。合并不等于部署：现有开发数据库/常驻服务未在本批次升级，首次启用需核对迁移、OAuth 加密配置与 Admin bootstrap。凭证分发、机器 discovery 和健康检查仍待实施。
