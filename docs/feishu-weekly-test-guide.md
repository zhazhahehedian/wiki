# 飞书身份、云文档摄入与 Agent 扩展一周测试指引

本文用于在真实飞书测试应用、PostgreSQL/pgvector、MinIO、后端和前端环境中，连续一周验证飞书身份、云文档摄入、River 异步任务、用户隔离和 Agent 扩展。本轮只验证测试分支，不直接在生产数据上做故障注入。

目标分支：`codex/feishu-auth-agent-seams`

相关运行说明：`docs/deploy-debug-feishu.md`

## 1. 验收原则

以下条件全部满足，才能建议进入合并或发布：

- 后端、前端自动化测试和生产构建通过。
- 两个同租户测试账号只能访问各自的 KB、Document、Chunk、Conversation 和 Chat。
- docx、sheet、bitable、wiki 均能导入并形成可检索的 Canonical Markdown chunks。
- 首次导入、正文变化、正文不变但 revision/metadata 变化、失败重试都符合 staged snapshot 语义。
- 同步失败不会替换旧 active snapshot 或旧 chunks。
- River 延迟认领、进程重启和 stale sync reconciliation 可恢复。
- OAuth Token、Session Token、CSRF Token、飞书正文、授权 code/state 和完整对象存储 key 不出现在前端响应、测试报告或日志中。
- 没有未关闭的 P0/P1 缺陷；P2 必须有复现步骤、影响说明和处理决定。

## 2. River 部署说明

River 不是独立服务器，不需要单独安装服务。它作为 Go 依赖嵌入后端，使用当前 `DATABASE_URL` 指向的 PostgreSQL：

1. 后端启动时先执行 Goose migrations。
2. 后端随后通过 `rivermigrate` 创建或升级 River schema。
3. 同一后端进程注册 `ingestion`、`feishu_sync` 和 `feishu_reconcile` workers。
4. `feishu_reconcile` 按 `FEISHU_RECONCILE_INTERVAL` 周期运行，并在后端启动时立即注册一次。

服务器只需要 PostgreSQL 可达、数据库账号有建表/迁移权限，并正常启动后端。不要额外部署所谓的 River Server。

启动后用以下只读查询确认 River 就绪：

```sql
SELECT to_regclass('public.river_job') AS river_job_table;

SELECT kind, state, count(*)
FROM river_job
WHERE kind IN ('ingestion', 'feishu_sync', 'feishu_reconcile')
GROUP BY kind, state
ORDER BY kind, state;
```

第一条应返回 `river_job`。第二条允许初始为空；执行导入或同步后应出现对应 kind。

## 3. 测试环境与数据

### 3.1 测试角色

准备同一 `FEISHU_TENANT_KEY` 下的两个独立账号：

| 角色 | 用途 |
|---|---|
| 账号 A | owner，创建 KB、导入文档、发起同步和聊天 |
| 账号 B | non-owner，验证跨用户访问全部失败；同时创建自己的 KB 验证正向路径 |

两个账号使用不同浏览器 profile 或一个普通窗口加一个无痕窗口，避免 Cookie 混用。

### 3.2 飞书测试资源

准备以下受控资源，并确保账号 A 有只读权限：

| 编号 | 类型 | 建议内容 |
|---|---|---|
| F-DOCX-01 | docx | 标题、段落、有序/无序列表、引用、代码块、表格、链接、中文和特殊字符 |
| F-SHEET-01 | sheet | 至少两个 sheet，包含空单元格、数字、日期、中文、公式结果和多页数据 |
| F-BITABLE-01 | bitable | 文本、数字、单选、多选、人员、日期、链接等字段和多页记录 |
| F-WIKI-01 | wiki | 指向一个受支持底层资源的 Wiki 节点 |

额外准备：

- 一个账号 A 无权读取的飞书资源，用于权限失败测试。
- 一个可修改的 docx，用于正文变化、仅标题变化和恢复测试。
- 一个不相关域名 URL、一个 malformed URL 和一个不支持的飞书路径。

不要把真实文档正文复制到缺陷系统。测试报告只记录资源编号、Document ID 和脱敏证据。

### 3.3 环境变量检查

上线前逐项确认，不在终端历史或报告中输出值：

```text
FEISHU_APP_ID
FEISHU_APP_SECRET
FEISHU_REDIRECT_URL
FEISHU_TENANT_KEY
OAUTH_ENCRYPTION_KEY
SESSION_COOKIE_SECURE
SESSION_TTL
FRONTEND_ORIGIN
BOOTSTRAP_OWNER_FEISHU_OPEN_ID
RIVER_MAX_WORKERS
FEISHU_SYNC_JOB_TIMEOUT
INGESTION_JOB_TIMEOUT
FEISHU_RECONCILE_JOB_TIMEOUT
FEISHU_RECONCILE_INTERVAL
FEISHU_SYNC_LEASE
RIVER_RESCUE_STUCK_JOBS_AFTER
FEISHU_RECONCILE_BATCH_SIZE
FEISHU_RECONCILE_MAX_BATCHES
```

生产式 HTTPS 测试必须满足：

- `SESSION_COOKIE_SECURE=true`。
- `FRONTEND_ORIGIN` 只有 scheme、host 和可选 port，不带 path 或尾随 `/`。
- `FEISHU_REDIRECT_URL` 与飞书控制台逐字符一致。
- `OAUTH_ENCRYPTION_KEY` 是私有随机 16、24 或 32-byte key，不使用模板值。

## 4. 每日证据和安全规则

每天开始先记录：

```bash
git branch --show-current
git rev-parse HEAD
git status --short
```

每天结束保存以下信息：

| 字段 | 记录内容 |
|---|---|
| 日期/环境 | 测试日期、服务器和浏览器版本 |
| Commit | `git rev-parse HEAD` 输出 |
| 用例 | 本文用例编号 |
| 结果 | PASS / FAIL / BLOCKED |
| 证据 | HTTP status、脱敏日志时间段、Document ID、截图编号 |
| 缺陷 | 缺陷编号、严重度、复现概率 |
| 清理 | 测试 KB、对象、临时配置是否恢复 |

禁止记录：

- `FEISHU_APP_SECRET`、`OAUTH_ENCRYPTION_KEY` 或 LLM/Embedding/S3 secret。
- access token、refresh token、session cookie、CSRF cookie/header 的实际值。
- OAuth callback 的完整 query、授权 code/state 或完整授权 URL。
- 飞书 API response body、文档正文或 MinIO 完整 object key。

允许记录：request ID、Document ID、KB ID、HTTP status、稳定 error code、时间戳和 storage key hash。

## 5. Day 1：部署、迁移和自动化基线

### D1-01 分支和依赖

- [ ] 部署 `origin/codex/feishu-auth-agent-seams`。
- [ ] 确认服务器 HEAD 与测试记录一致。
- [ ] 确认 `.env` 未纳入 Git，且没有使用 `.env.example` 中的模板 secret。
- [ ] 确认 PostgreSQL、pgvector、MinIO、LLM 和 Embedding endpoint 可达。

### D1-02 Goose 和 River schema

在后端目录执行：

```bash
go run ./cmd/migrate status
go run ./cmd/migrate up
go run ./cmd/migrate status
```

预期：Goose migrations 全部 applied。启动后端后执行第 2 节 River 查询，`river_job` 存在。

后端启动本身也会执行 Goose 和 River migration；显式命令用于在启动前发现数据库权限或版本问题。

### D1-03 服务健康

```bash
export API_BASE_URL=https://wiki-test.example.com
curl -fsS "$API_BASE_URL/healthz"
curl -fsS "$API_BASE_URL/api/healthz"
```

将 `API_BASE_URL` 设为本次测试后端的真实 HTTPS origin。预期：两者均返回 HTTP 200。日志中应看到 MinIO bucket ready 和 server listening，不应出现 `river start`、`rivermigrate`、ownership bootstrap 或 embedding dimension 错误。

### D1-04 自动化回归

后端：

```bash
cd backend
go test ./internal/http -run 'TestFeishu(OAuthImportSyncOwnershipE2E|OperatorDocumentationMatchesRuntimeContracts)' -count=1 -v
go test ./internal/infra/feishu ./internal/worker ./internal/service -count=1
go test ./...
go vet ./...
sqlc compile
sqlc vet
```

前端：

```bash
cd frontend
pnpm lint
pnpm typecheck
pnpm test
pnpm build
```

Windows 本地运行时用 `pnpm.cmd`。Linux 服务器不应出现 Windows standalone symlink `EPERM`；任何编译、类型或页面生成错误都算失败。

Day 1 退出标准：所有自动化命令通过，后端可启动，Goose/River schema 正常，健康检查为 200。

## 6. Day 2：OAuth、Session 和双账号隔离

### D2-01 账号 A 正常登录和注销

- [ ] 从 `/login` 发起飞书登录，确认只能跳转到配置的飞书授权地址。
- [ ] callback 后回到 `FRONTEND_ORIGIN`，业务页可加载。
- [ ] 浏览器确认 `it_wiki_session` 为 HttpOnly；HTTPS 环境两个 Cookie 均为 Secure、SameSite=Lax。
- [ ] 调用 `/api/v1/auth/me` 返回账号 A 的公开身份，不含 OAuth Token。
- [ ] 注销后业务 API 返回 `401 unauthenticated`，旧 session 不再可用。

### D2-02 账号 B 独立登录

- [ ] 用第二个浏览器 profile 登录账号 B。
- [ ] 确认账号 B 有独立 session，账号 A 的 Cookie 不被覆盖。
- [ ] 两个账号各创建一个 KB，记录 KB ID。

### D2-03 CORS 和 CSRF

- [ ] 浏览器正常 mutation 成功，request 携带 credentials、正确 Origin 和 `X-CSRF-Token`。
- [ ] 缺失 CSRF header 的 POST 返回 `403 csrf_rejected`。
- [ ] 使用非 `FRONTEND_ORIGIN` 的 Origin 发起 mutation 返回 403。
- [ ] 读取类请求不应要求 CSRF，但仍要求有效 session。

不要在截图中显示 Cookie 或 CSRF 值。

### D2-04 跨 owner 访问

账号 A 创建 KB、导入或上传一个文档并创建 Conversation。然后用账号 B 尝试：

- [ ] 读取、修改和删除账号 A 的 KB。
- [ ] 列出账号 A KB 的 Documents。
- [ ] 读取、同步、重新摄入和删除账号 A 的 Document。
- [ ] 通过猜测 Chunk ID 读取 chunk 或 neighbors。
- [ ] 读取或继续账号 A 的 Conversation/Chat。

预期：全部返回 `404 resource_not_found` 或等价不泄露存在性的响应，不得返回账号 A 的标题、状态、内容、chunks 或消息。

Day 2 退出标准：双账号正向功能正常，所有跨 owner 路径被拒绝，Cookie/CORS/CSRF 符合预期。

## 7. Day 3：四类飞书资源摄入

对 F-DOCX-01、F-SHEET-01、F-BITABLE-01、F-WIKI-01 分别执行以下步骤。

### D3-01 首次导入

- [ ] 账号 A 在自己的 KB 选择“从飞书导入”并提交一个 URL。
- [ ] HTTP 返回 202，文档先显示 pending/ingesting 或 syncing，而不是同步阻塞页面。
- [ ] `river_job` 出现 `feishu_sync`，随后出现 `ingestion`。
- [ ] 最终 Document `status=ready`、`sync_status=idle`、`last_synced_at` 非空。
- [ ] `pending_content_ref` 等 pending 字段全部清空。
- [ ] UI 显示 source type、原始 remote URL、remote revision 和最后同步时间。
- [ ] Chunks 可浏览，聊天检索能命中该资源的可识别测试语句。

使用以下只读 SQL 观察状态，不输出完整对象 key：

```sql
SELECT id,
       kb_id,
       source_type,
       status,
       sync_status,
       remote_revision,
       content_ref IS NOT NULL AS has_active_snapshot,
       left(md5(coalesce(content_ref, '')), 12) AS active_ref_hash,
       pending_content_ref IS NOT NULL AS has_pending_snapshot,
       last_synced_at,
       last_sync_error,
       updated_at
FROM documents
WHERE id = 'DOCUMENT_UUID';
```

在实际执行时将 `DOCUMENT_UUID` 替换为本次返回的 UUID。不要把查询改为输出 encrypted token、正文或完整 `content_ref`。

### D3-02 Canonical Markdown 检查

- [ ] docx：标题、列表、引用、代码和表格顺序合理，Unicode 未损坏。
- [ ] sheet：不同 sheet 有独立章节，行范围和 sheet metadata 正确。
- [ ] bitable：复杂字段稳定展开，多值字段顺序可解释，分页没有重复/缺失记录。
- [ ] wiki：source identity 保持 Wiki URL，同时正确读取底层受支持资源。
- [ ] 不支持的图片或块不会让整个导入崩溃，也不会生成损坏 Markdown。

### D3-03 URL、权限和重复导入

- [ ] 同一 KB 重复导入同一 canonical resource，返回 `409 resource_already_imported`。
- [ ] 另一个 KB 可独立导入同一资源。
- [ ] 不相关域名、空 token、malformed URL 和不支持类型返回 422 typed error。
- [ ] 导入账号 A 无权读取的资源，最终为脱敏的 source load failure，不暴露飞书 response body。
- [ ] 请求 body 超过 8 KiB 返回 `413 request_too_large`。

Day 3 退出标准：四种类型全部至少一次成功；分页、重复、非法 URL 和无权限路径均有证据。

## 8. Day 4：同步、promotion 和旧数据保护

使用一个已有 ready snapshot 和稳定 chunks 的可修改 docx。开始前记录：

- Document ID、`remote_revision`、`checksum`、`active_ref_hash`。
- Chunk 数量和两条代表性 chunk 内容的本地证据编号。
- MinIO 中该 Document 前缀下的对象数量；报告中只记录数量和对象 key hash。

### D4-01 远端 revision 未变化

- [ ] 不修改飞书内容，点击“重新同步”。
- [ ] 请求返回 202，最终回到 `sync_status=idle`。
- [ ] active snapshot、checksum 和 chunks 不应被无意义替换。
- [ ] 不产生永久 pending snapshot。

### D4-02 正文不变但 metadata/revision 变化

- [ ] 只修改不会进入 Markdown 正文的 metadata，保持正文 checksum 不变；如果当前资源的标题会进入 Markdown，则不要用标题制造这个场景。
- [ ] 同步成功后 revision/title 更新，chunks 内容保持不变，citation metadata 更新。
- [ ] 新 snapshot 成为 active；旧 active 和 superseded pending 对象被清理。
- [ ] MinIO 中不出现每次同正文同步都多一个无引用对象的增长。

### D4-03 正文变化

- [ ] 增加一个唯一测试句并同步。
- [ ] promotion 前，旧 active snapshot 和旧 chunks 仍可用。
- [ ] 成功后 active hash、revision 和 checksum 更新，pending 清空。
- [ ] 旧 chunks 被原子替换；检索能命中新句且不再命中已删除句。
- [ ] 旧 active snapshot 被清理，当前 active snapshot 不被删除。

### D4-04 ingestion 失败保留旧版本

只在隔离测试环境执行。记录旧状态后，临时让 Embedding endpoint 不可用，再修改正文并同步：

- [ ] 同步最终为 `sync_status=failed`，`last_sync_error` 为脱敏稳定信息。
- [ ] 旧 `content_ref` hash、checksum、revision 和 chunks 保持不变。
- [ ] pending payload 保留到可恢复状态，不被错误 promotion。
- [ ] 恢复 Embedding endpoint 后重试，最终成功 promotion。

故障测试结束后立即恢复环境变量并重启后端。不要在共享环境修改真实业务文档。

### D4-05 generic reingest 和删除

- [ ] 对飞书远端文档调用 generic reingest，返回 `422 unsupported_operation`。
- [ ] 该操作不改变状态，也不新增 ingestion job；应改用 `/sync`。
- [ ] bulk reingest 只处理 `local-upload`，跳过所有飞书远端文档。
- [ ] 删除测试 Document 后数据库行消失，active/pending 对象和 chunks 被清理。

已知残余：PostgreSQL 删除成功后若 MinIO Delete 瞬时失败，目前只有结构化 hash 日志，没有 durable cleanup outbox。出现该情况应登记 P2、保留日志时间和 Document ID，并通过 bucket lifecycle 或人工清理处理。

Day 4 退出标准：三类同步路径、失败保留、remote reingest 拒绝和对象清理均验证完成。

## 9. Day 5：River 延迟、重启和 reconciler

故障注入必须使用专用测试 KB。开始前备份配置并确认可以恢复后端进程。

### D5-01 Job 可观察性

```sql
SELECT kind, state, count(*)
FROM river_job
WHERE kind IN ('ingestion', 'feishu_sync', 'feishu_reconcile')
GROUP BY kind, state
ORDER BY kind, state;

SELECT id,
       state,
       coalesce(args->>'metadata_only', 'false') AS metadata_only
FROM river_job
WHERE kind = 'ingestion'
ORDER BY id DESC
LIMIT 20;
```

- [ ] 导入时看到 `feishu_sync -> ingestion` 的状态推进。
- [ ] 正文不变但 metadata 变化时可看到 metadata-only ingestion。
- [ ] 周期性 `feishu_reconcile` 存在且不会无限重复堆积 active jobs。

### D5-02 浏览器观察超过 60 秒的延迟认领

使用 `RIVER_MAX_WORKERS=1` 和一个受控慢任务制造超过 75 秒的队列等待，再立即对第二个 ready 文档发起手动同步：

- [ ] 前 60 秒浏览器约每 2 秒查询文档状态。
- [ ] 60 秒后降低为约每 15 秒查询，而不是停止。
- [ ] River 最终 claim 后，页面无需手动刷新即可看到 syncing 和完成态。
- [ ] 完成后 detail/list 轮询停止，chunks 被刷新，不再多发一次 GET。
- [ ] 离开文档/列表页后实际 GET 停止；重新进入后仍能观察未完成任务。

用浏览器 Network 面板记录请求时间线，但不要保存 Cookie/header 值。

### D5-03 Crash/restart 恢复

为缩短隔离测试时间，可临时使用以下满足约束的配置：

```text
FEISHU_SYNC_JOB_TIMEOUT=20s
INGESTION_JOB_TIMEOUT=30s
FEISHU_RECONCILE_JOB_TIMEOUT=5s
FEISHU_RECONCILE_INTERVAL=10s
FEISHU_SYNC_LEASE=70s
RIVER_RESCUE_STUCK_JOBS_AFTER=45s
```

这些值只用于测试：reconcile interval 小于 lease，lease 大于 sync + ingestion timeout，River rescue threshold 大于每个 worker timeout。

- [ ] 在 Feishu sync 运行时终止后端进程，确认进程确实退出。
- [ ] 重启后端，确认 River schema 不重复失败，worker 正常启动。
- [ ] 等待 River rescue/reconciler，文档最终恢复或进入可解释 failed 状态。
- [ ] 如果 crash 时已有完整 pending snapshot，应恢复 ingestion 或 metadata promotion。
- [ ] 如果 pending 不完整，应重新执行 source sync，不能 promotion 半成品。
- [ ] 恢复后 pending 清空，旧 active/chunks 在成功前始终保留。

### D5-04 Reconciler 边界

- [ ] 未超过 `FEISHU_SYNC_LEASE` 的 syncing 文档不被抢占。
- [ ] 超过 lease 后只处理 owner/source 状态一致的 stale 文档。
- [ ] 单次扫描遵守 batch size/max batches，不因一个失败阻止后续记录。
- [ ] 日志只含 Document ID、recovery type、时间戳和稳定 error code。

测试结束后恢复正式 timeout/lease 配置并重启后端。

Day 5 退出标准：延迟认领、浏览器降频观察、进程重启和 stale reconciliation 全部有证据。

## 10. Day 6：Agent、Chat 和安全检查

### D6-01 默认 Agent

- [ ] 账号 A 在自己的 KB 创建 Conversation。
- [ ] 数据库中 `agent_id='knowledge-rag'`。
- [ ] 询问“知识库有哪些文档”，可触发 `list_documents`。
- [ ] 询问文档内容，可触发 `kb_retrieval` 并返回当前 owner KB 的引用。
- [ ] 未注册 Agent ID 返回 typed error，不 fallback 到任意 runner。

只读检查：

```sql
SELECT id, owner_user_id, agent_id, updated_at
FROM conversations
ORDER BY updated_at DESC
LIMIT 20;
```

### D6-02 Chat owner 隔离

- [ ] 账号 B 不能读取或继续账号 A Conversation。
- [ ] 账号 B 的 Agent 工具不能列出或检索账号 A KB 文档。
- [ ] 猜测 KB、Document、Chunk、Conversation ID 都不能泄露存在性。
- [ ] SSE 中途断开不会保存半截 assistant 消息。

### D6-03 Token 和日志脱敏

数据库只确认密文和 hash 存在，不读取或导出值：

```sql
SELECT provider,
       tenant_key <> '' AS has_tenant,
       octet_length(access_token_encrypted) > 0 AS access_token_encrypted,
       refresh_token_encrypted IS NULL
           OR octet_length(refresh_token_encrypted) > 0 AS refresh_token_encrypted,
       cardinality(scopes) AS scope_count,
       reauth_required
FROM oauth_accounts;

SELECT count(*) AS session_count,
       count(*) FILTER (WHERE expires_at <= now()) AS expired_sessions
FROM user_sessions;
```

- [ ] 前端 API 响应和 React Query cache 中没有 OAuth Token。
- [ ] HTTP access log 不记录 query string。
- [ ] OAuth callback 日志不记录 state/code。
- [ ] Worker 错误不记录 Authorization header、飞书 response body、正文或完整 storage key。
- [ ] UI 错误只显示稳定公开信息，不显示内部 provider/SQL/MinIO error。

Day 6 退出标准：默认 Agent 工具链正常，所有工具受 owner 限制，Token 和错误边界无泄漏。

## 11. Day 7：全量回归和验收报告

### D7-01 Fresh 自动化回归

重新执行 Day 1 的全部后端和前端命令，并执行：

```bash
git diff --check
git status --short
```

预期：所有命令退出 0，worktree 没有测试产生的源码或 fixture 改动。

### D7-02 核心人工回归

- [ ] 账号 A/B 登录、注销和 session 失效。
- [ ] 四类资源各抽查一个成功导入。
- [ ] 抽查一个正文变化同步和一个失败后重试。
- [ ] 抽查跨 owner KB/Document/Chunk/Conversation 拒绝。
- [ ] 抽查 River job 和 reconciler 状态。
- [ ] 抽查 Agent 的 `list_documents` 和 `kb_retrieval`。
- [ ] 检查测试期间 MinIO 对象数量没有持续无引用增长。
- [ ] 检查日志没有安全禁记字段。

### D7-03 缺陷分级

| 级别 | 定义 | 发布处理 |
|---|---|---|
| P0 | 数据跨用户泄漏、Token/secret 泄漏、数据不可恢复损坏 | 立即停止测试和发布 |
| P1 | 登录主路径不可用、四类导入主路径失败、旧 snapshot/chunks 被错误替换、River 无法恢复 | 修复并完整回归后才能发布 |
| P2 | 有明确绕行方式的状态、运维或清理问题 | 记录 owner、期限和风险接受决定 |
| P3 | 文案、低影响显示或测试便利性问题 | 可进入后续迭代 |

### D7-04 最终报告模板

```markdown
# 飞书集成测试报告

- 测试分支：codex/feishu-auth-agent-seams
- Commit：记录 git rev-parse HEAD 的输出
- 环境：记录服务器、数据库、MinIO 和浏览器版本，不记录 secret
- 测试周期：记录开始和结束日期

## 结果

| 范围 | 通过 | 失败 | 阻塞 | 结论 |
|---|---:|---:|---:|---|
| 自动化 |  |  |  |  |
| OAuth/Session |  |  |  |  |
| Owner 隔离 |  |  |  |  |
| 四类资源导入 |  |  |  |  |
| 同步与旧数据保护 |  |  |  |  |
| River/reconciler |  |  |  |  |
| Agent/Chat |  |  |  |  |
| 安全与日志 |  |  |  |  |

## 未关闭缺陷

| ID | 级别 | 现象 | 复现率 | 影响 | 决定 |
|---|---|---|---:|---|---|

## 发布结论

- [ ] 建议合并/发布
- [ ] 修复 P0/P1 后重新评估
- [ ] 因环境阻塞暂不下结论
```

## 12. 失败时的最短排查路径

### 登录失败

1. 核对 redirect URL 是否逐字符一致。
2. 核对 scope、tenant 和应用发布范围。
3. 检查 Cookie Secure/Domain/Path、Origin 和 CORS。
4. 只记录公开 error code，不记录 callback query。

### 导入一直 pending

1. 查 `river_job` 的 kind/state。
2. 查 Document `sync_status`、`updated_at`、pending 是否完整。
3. 确认后端进程内 River workers 已启动。
4. 检查 Feishu API、MinIO 和 Embedding 可达性。
5. 超过 lease 后观察 reconciler，不手工 promotion 数据库字段。

### 同步失败

1. 先确认旧 active hash 和 chunks 是否仍在。
2. 根据脱敏 `last_sync_error` 区分 source、snapshot、parsing、embedding 或 promotion。
3. 恢复依赖后通过 UI 重试 `/sync`，不要对远端文档使用 generic reingest。

### 页面不再更新

1. 浏览器 Network 确认前 60 秒约 2 秒一次，之后约 15 秒一次。
2. 查 River job 是否尚未 claim。
3. 查 `updated_at`、`sync_status` 和 `last_synced_at` 是否推进。
4. 离页后 GET 停止是正常行为；重新进入页面继续观察。

### MinIO 对象异常增长

1. 按 Document 前缀统计数量，只记录 key hash。
2. 区分 current active、current pending 和无数据库引用对象。
3. 检查 snapshot cleanup 的结构化 warning。
4. 对无引用对象执行人工或 lifecycle 清理前，先备份并确认数据库不再引用。
