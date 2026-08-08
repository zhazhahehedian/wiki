# 飞书身份与云文档摄入设计

- 日期：2026-08-08
- 状态：设计已确认，待实施计划
- 范围：单租户内部使用

## 1. 目标与范围

本阶段为知识库增加公司内部飞书登录，以及用户将自己有权限访问的飞书云资源导入指定知识库的能力。导入支持单个 URL，首版支持飞书新版文档（docx）、表格、多维表格和 Wiki。所有资源在切分前转换为平台规范 Markdown，随后复用现有 Parser、Splitter、Embedding、VectorStore 和 River 摄入流程。

本阶段不实现完整 RBAC/ABAC、租户市场、自动轮询、飞书事件订阅、文件夹批量导入、飞书机器人、知识推送、Skill 注册平台、Python/REST 执行器、文档版本回滚和 OCR。重新同步采用用户主动触发的快照语义。

## 2. 现状与设计原则

当前项目已经具备通用的文档摄入链路：本地上传写入 MinIO，Document 记录来源，River 投递 ingestion job，worker 完成解析、切分、Embedding 和 chunks 原子替换。`documents.source_type/source_ref/metadata` 已预留外部来源信息，但现有上传服务将来源硬编码为 `local-upload`，HTTP 路由也没有用户身份隔离。

设计原则：

1. 飞书接入是 Source Adapter，不复制第二套 RAG 管线。
2. HTTP Handler 只做校验和编排，飞书 API 访问放在 infra adapter。
3. 飞书拉取与本地摄入分成两个异步阶段，重新同步失败不得破坏旧 chunks。
4. 用户隔离先按 `user_id` 实现，不提前引入完整组织权限系统。
5. LiveAgent 采用复用优先、依赖边界优先的移植策略；保留 MIT 来源声明，不引入其桌面端或 Gateway 运行时。

## 3. 总体架构

```text
Browser
  -> Feishu OAuth / local session
  -> authenticated REST API
  -> Feishu Import Service
  -> River FeishuSyncWorker
  -> canonical Markdown snapshot in MinIO
  -> River IngestionWorker
  -> Parser -> Splitter -> Embedder -> VectorStore
  -> chunks / retrieval / chat
```

后端模块边界：

- `internal/auth`：飞书 OAuth、Session、当前用户上下文、CSRF 相关逻辑。
- `internal/domain/ports`：`FeishuClient`、资源 Loader 等接口。
- `internal/infra/feishu`：OAuth Client、资源读取和 URL resolver。
- `internal/service`：Auth Service、Feishu Import Service 和用户归属校验。
- `internal/worker`：新增 `FeishuSyncWorker`；现有 `IngestionWorker` 继续负责规范文档摄入。
- `frontend/components/liveagent`：移植后的 LiveAgent 风格组件和适配层。
- `frontend/styles/liveagent.css`：选择性迁移的主题 token、对话和 shell 样式。

## 4. 身份模型与安全

### 4.1 OAuth 与 Session

配置项：`FEISHU_APP_ID`、`FEISHU_APP_SECRET`、`FEISHU_REDIRECT_URL`、`FEISHU_TENANT_KEY`、`OAUTH_ENCRYPTION_KEY`、`SESSION_COOKIE_SECURE`、`SESSION_TTL`、`FRONTEND_ORIGIN`。

公开接口：

```text
GET  /api/v1/auth/feishu/start
GET  /api/v1/auth/feishu/callback
GET  /api/v1/auth/me
POST /api/v1/auth/logout
```

回调流程：生成一次性、短时有效的 OAuth state；回调交换用户 Token，读取用户信息并校验 `tenant_key`；检查所需只读 scopes；upsert 本地用户和 OAuth Account；签发本地 opaque Session。浏览器只保存 `HttpOnly + Secure + SameSite=Lax` Cookie，数据库只保存 Session Token 哈希。

OAuth access/refresh token 使用环境变量提供的 AES-GCM 主密钥加密存储。Token 不返回浏览器、不写日志、不放进 Document metadata。刷新使用锁或 singleflight，避免并发刷新造成 refresh token 竞态。`invalid_grant` 等不可恢复错误标记为 `reauth_required`，不删除已经导入的本地文档。

### 4.2 单租户和用户隔离

首版只允许 `FEISHU_TENANT_KEY` 对应的公司租户。新增 `users`、`oauth_accounts`、`user_sessions` 表；`knowledge_bases` 和 `conversations` 增加 `owner_user_id`。Document 和 Chunk 通过 KB 归属完成隔离。所有 KB、Document、Chunk、Conversation、Chat 查询都必须带当前用户过滤。

除健康检查和 OAuth 入口外，所有 `/api/v1` 接口必须登录。CORS 只允许配置的 `FRONTEND_ORIGIN` 并启用 credentials；状态变更校验 Origin 并携带 Session 绑定的 CSRF Token。日志可以记录 user/document/resource type，但不得记录 Token、正文或完整授权 URL 参数。

## 5. 文档模型与摄入流程

### 5.1 Document 字段

`documents` 增加或调整以下字段：

```text
source_type       local-upload / feishu-docx / feishu-sheet /
                  feishu-bitable / feishu-wiki
source_ref        飞书资源的规范身份
content_ref       MinIO 中规范化 Markdown 快照的位置
remote_revision   飞书版本号或更新时间
source_url        用户粘贴的原始 URL
oauth_account_id  本次同步使用的授权账户
pending_content_ref
pending_checksum
pending_remote_revision
sync_status       idle / syncing / failed
last_sync_error
last_synced_at
```

同一 KB 对同一资源只允许一条记录：

```sql
UNIQUE (kb_id, source_type, source_ref)
```

`checksum` 是规范化 Markdown 的 SHA-256。`metadata` 只放非敏感来源信息，例如 Wiki 路径、Sheet 名称、Bitable 表名/view、字段列表和转换器版本。

### 5.2 两阶段异步流程

接口：

```text
POST /api/v1/kbs/{kbID}/feishu-imports
Body: { "url": "https://..." }
POST /api/v1/docs/{docID}/sync
```

接口只解析 URL、校验 KB 归属、验证资源身份并投递任务，返回 `202 Accepted` 和 Document。`FeishuSyncWorker` 执行：

1. 读取并刷新用户级 access token。
2. 解析 URL 和资源类型，Wiki 先解析到底层资源。
3. 分页读取资源内容。
4. 转换为规范 Markdown。
5. 写入 MinIO 暂存快照，计算 revision 和 checksum。
6. revision 未变化时只更新同步时间；checksum 未变化时不重新 Embedding。
7. 有变化时写入 pending 字段并投递现有 ingestion job。

`IngestionWorker` 读取 pending snapshot，复用 Markdown Parser、Splitter、Embedder 和 VectorStore。chunks 原子替换成功后，才把 pending 字段提升为当前 `content_ref/checksum/remote_revision`，清空 pending，标记 ready。重新同步失败时旧 snapshot、旧 revision 和旧 chunks 保持不变；首次导入失败时显示 failed。

### 5.3 规范化规则

- docx：递归读取 Block，保留标题、段落、列表、引用、代码块和表格。
- sheet：每个工作表一个章节，使用单元格展示值生成 Markdown 表格，必要时按行分段。
- bitable：每张数据表一个章节，字段为表头、记录为行；人员、链接、附件等复杂字段转成稳定文本。
- wiki：解析节点路径和底层对象类型后委托对应 Loader。
- 图片保留说明文字和来源链接，首版不做 OCR。
- 所有 API 分页读取；单资源设置可配置大小/行数上限，超过上限明确失败，不静默截断。

Chunk metadata 保留 `source_type`、`source_url`、`section_path`、`sheet_name`、`row_start/row_end`、`remote_revision` 等定位信息，用于引用回原文。

## 6. API 错误与可观测性

标准错误码：

```text
unauthenticated
feishu_reauth_required
tenant_not_allowed
feishu_resource_forbidden
resource_not_found
resource_already_imported
sync_in_progress
unsupported_feishu_url
unsupported_feishu_resource
feishu_rate_limited
feishu_api_error
```

Feishu Client 对 429/5xx 做有限指数退避并尊重 `Retry-After`；River 任务不配置无限自动重试。状态变更、失败原因和 request ID 写入日志，禁止记录敏感 Token 和正文。

快照语义意味着：飞书权限撤销不会自动删除已经导入的本地知识；只有用户手动重新同步时才会发现权限变化。若未来要求实时权限撤销，需要单独增加轮询或事件订阅阶段。

## 7. 前端与 LiveAgent 复用

复用优先，但按依赖边界拆解移植：

- 直接或近似直接移植 `ComposerAttachmentCard`、`ThinkingActivity`、`TaskProgressIndicator`。
- 移植 `ChatTranscript`、`ChatHeader` 的布局、滚动和视觉结构，接入当前 SSE/React Query 数据。
- 从 `ChatComposerBar`、`ChatHistorySidebar` 提取输入栏和侧栏交互，不整文件复制。
- 不直接复制绑定 Gateway 协议、Tauri、Git、终端和桌面设置的 `GatewayTranscript`、`MentionComposer`、`GatewayApp`。
- 用 `TranscriptRow` 适配层把当前 `LocalChatMessage` 转换为 LiveAgent 风格的行模型。
- 迁移颜色 token、字体层级、滚动条、对话排版、工具状态和响应式 shell 到 `frontend/styles/liveagent.css`。
- 保留 LiveAgent MIT License、仓库 URL、来源 commit 和移植文件清单，记录在 `THIRD_PARTY_NOTICES.md`。

飞书 UI：登录页只保留飞书登录按钮；文档页提供“上传文件”和“从飞书导入”两个入口；文档行显示资源类型、同步状态、原文链接和重新同步操作；移动端侧栏使用 Sheet。

## 8. 测试与迁移

后端测试覆盖 OAuth state、Session、加密 Token、并发刷新、租户限制、URL 解析、四类 Loader、Markdown fixtures、分页、幂等、失败保留旧 chunks、用户隔离和 CORS/CSRF。使用 `httptest.Server` 模拟飞书 API，fixture 放在 `testdata/`。

前端测试覆盖路由保护、登录错误、导入 Dialog、状态展示、重复同步保护、注销、键盘操作和移动端布局。

旧数据通过 `BOOTSTRAP_OWNER_FEISHU_OPEN_ID` 指定归属用户。存在旧 KB 但未配置该变量时，服务不进入业务就绪状态，避免第一个登录用户意外取得旧数据。

验收路径：飞书登录；创建/打开 KB；分别导入 docx、sheet、bitable、wiki；等待 ready 并查看 chunks；聊天检索和引用；修改原文后手动同步；无权限 URL 返回 403；其他用户无法访问数据；桌面和移动布局通过人工检查。

## 9. 明确延期项

完整 RBAC/ABAC、自动同步/事件订阅、文件夹导入、飞书机器人、Skill 注册与执行器、文档历史回滚、知识推送、OCR、跨租户和 gRPC/SDK 不属于本 spec 的实施范围。
