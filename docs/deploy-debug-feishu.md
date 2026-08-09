# 飞书集成部署与排错

本文只描述当前代码已经实现的行为。飞书登录、导入和手动同步都要求同一租户、同一登录用户的资源所有权；服务不会自动轮询飞书内容，也没有事件订阅。

## 1. 飞书应用配置

在飞书开放平台创建企业自建应用，启用网页 OAuth，并配置以下用户授权只读 scope：

| Scope | 用途 |
|---|---|
| `offline_access` | 获取 refresh token，在用户不重新登录时刷新访问令牌 |
| `contact:user.base:readonly` | 读取登录用户的 open ID、tenant key 和显示名 |
| `contact:user.email:readonly` | 读取登录用户邮箱 |
| `docx:document:readonly` | 读取新版文档（docx） |
| `sheets:spreadsheet:readonly` | 读取电子表格 |
| `bitable:app:readonly` | 读取多维表格 |
| `wiki:wiki:readonly` | 解析知识库节点并读取其底层资源 |

当前 `requiredFeishuScopes()` 会逐项检查上述字符串，缺少任意一项都会把 callback 重定向为 `feishu_reauth_required`，且不会创建本地 session。不需要写入、管理、聊天消息或全量通讯录权限；不要为了排错扩大权限范围。

飞书控制台登记的 redirect URL 必须与 `FEISHU_REDIRECT_URL` 逐字符一致，包括 scheme、host、port、path 和尾部斜杠。默认本地值是：

```text
http://localhost:8080/api/v1/auth/feishu/callback
```

登录入口是 `GET /api/v1/auth/feishu/start`。服务端生成一次性 state，并写入只在 callback path 可用的 state cookie；callback 是 `GET /api/v1/auth/feishu/callback`。不要从浏览器或日志复制完整授权 URL，因为 query 含一次性 state，callback query 还含授权 code。

`FEISHU_TENANT_KEY` 是唯一允许的 tenant key。callback 从飞书 user info 取得的 tenant key 必须与 OAuth state 绑定的值一致，否则返回 `tenant_not_allowed`。这是一套单租户限制，不是租户 allowlist；多租户尚未实现。

## 2. 环境变量

启用飞书时，下列变量必须成组配置；只配置其中一部分会导致启动失败：

| 变量 | 说明 |
|---|---|
| `FEISHU_APP_ID` | 飞书应用 ID |
| `FEISHU_APP_SECRET` | 飞书应用 secret，只放在运行环境 |
| `FEISHU_REDIRECT_URL` | 与飞书控制台完全一致的 callback URL |
| `FEISHU_TENANT_KEY` | 允许登录的单一租户 key |
| `OAUTH_ENCRYPTION_KEY` | 加密落库 access/refresh token 的 AES key，必须为 16、24 或 32 bytes |
| `FRONTEND_ORIGIN` | 前端精确 origin，例如 `https://wiki.example.com`；不能带 path、query 或尾随斜杠 |
| `SESSION_COOKIE_SECURE` | 生产 HTTPS 必须为 `true`；本地纯 HTTP 才使用 `false` |
| `SESSION_TTL` | 本地 session 有效期，Go duration，例如 `24h` |
| `BOOTSTRAP_OWNER_FEISHU_OPEN_ID` | 仅用于给遗留 NULL owner 数据绑定初始 owner，见第 4 节 |

同步与存活性变量：

| 变量 | 默认值 | 说明 |
|---|---:|---|
| `FEISHU_SYNC_JOB_TIMEOUT` | `10m` | 单次飞书抓取 job timeout |
| `INGESTION_JOB_TIMEOUT` | `20m` | snapshot 解析、切块、embedding job timeout |
| `FEISHU_RECONCILE_JOB_TIMEOUT` | `2m` | 单次 reconciler job timeout |
| `FEISHU_RECONCILE_INTERVAL` | `5m` | 扫描卡住同步的间隔，必须小于 lease |
| `FEISHU_SYNC_LEASE` | `45m` | 判定 `syncing` 已失活的租约；必须大于 sync + ingestion timeout |
| `RIVER_RESCUE_STUCK_JOBS_AFTER` | `30m` | River 救援卡住 job 的阈值，必须大于每个 worker timeout |
| `FEISHU_RECONCILE_BATCH_SIZE` | `100` | 每批扫描数量，范围 1..500 |
| `FEISHU_RECONCILE_MAX_BATCHES` | `10` | 单次最多扫描批数，范围 1..20 |

飞书 API base、HTTP timeout、重试和资源大小目前没有环境变量。当前代码固定使用 `https://open.feishu.cn`、每请求 15s timeout、最多 3 次重试（429、5xx、网络或读取错误，遵循 `Retry-After`，退避上限 30s）、单响应 8 MiB、默认最多 10,000 行和 10 MiB canonical output。不要配置未实现的 `FEISHU_API_*` 或 row-size 变量；需要改变这些限制时先增加 `config.go` 支持和测试。

`.env.example` 使用占位符。不要提交真实 app secret、OAuth 加密 key、用户 token 或飞书文档正文。

## 3. Cookie、CORS 与 CSRF

认证成功后后端设置两个 `SameSite=Lax` cookie：

- `it_wiki_session`：`HttpOnly`，浏览器脚本不可读。
- `it_wiki_csrf`：非 `HttpOnly`，前端读取后在 mutation 上发送 `X-CSRF-Token`。

两个 cookie 在 `SESSION_COOKIE_SECURE=true` 时都带 `Secure`。前端 API client 固定使用 `credentials: "include"`。所有 POST/PATCH/DELETE 请求必须同时满足：请求 `Origin` 精确等于 `FRONTEND_ORIGIN`，session 有效，`X-CSRF-Token` 与该 session 绑定的 CSRF hash 匹配。CORS 只回显这个单一 origin，并发送 `Access-Control-Allow-Credentials: true`。

生产环境应让反向代理终止 HTTPS，设置 `SESSION_COOKIE_SECURE=true`，并保证浏览器看到的前端 origin 与 `FRONTEND_ORIGIN` 一致。当前 cookie 是 `SameSite=Lax`，不支持跨站点第三方 cookie 部署；前后端应同站点，最好由同一 HTTPS 域名反代。不要在代理层重写 callback path 或把 HTTP callback 暴露给生产用户。

常见现象：

- `401 unauthenticated`：session cookie 未发送、过期或已撤销；检查 Secure/HTTPS、domain/path 和 `credentials: include`。
- `403 csrf_rejected`：Origin 不匹配、CSRF cookie/header 缺失，或 header 属于另一 session。
- 浏览器 CORS 报错但后端健康：检查 `FRONTEND_ORIGIN` 是否只有 origin，以及反代是否保留 `Origin`。

## 4. 遗留 ownership bootstrap

启动时会统计 `knowledge_bases.owner_user_id IS NULL` 和 `conversations.owner_user_id IS NULL`。只要还有一条遗留记录，`BOOTSTRAP_OWNER_FEISHU_OPEN_ID` 就必须设置为预期 owner 的飞书 open ID，否则服务 fail closed，报错 `BOOTSTRAP_OWNER_FEISHU_OPEN_ID is required while legacy rows lack ownership`。

bootstrap 优先复用已有 `(provider=feishu, provider_user_id=open_id)` 用户；找不到时创建稳定的 `Legacy owner` 用户。事务只更新 NULL owner，不会覆盖或抢占已有 owner。处理完成后再次启动是幂等的；确认无 NULL owner 后可以清空该变量。

所有 KB、conversation、Feishu OAuth account、导入和同步查询都按 owner 约束。跨 owner 的导入或同步对外返回 `404 resource_not_found`，避免泄露资源是否存在。

## 5. 导入与手动同步语义

`POST /api/v1/kbs/{kbID}/feishu-imports` 接受 `{"url":"..."}`，验证 URL 和 owner 后创建 pending 文档，并在同一数据库事务中入队，返回 `202 Accepted`。浏览器不能提交 user ID 或 OAuth account ID；后端从 session 解析它们。支持 docx、sheet、bitable 和 wiki URL。

`POST /api/v1/docs/{docID}/sync` 是手动同步，body 为空或 `{}`，验证文档 owner 和当前用户的飞书 OAuth account 后入队并返回 `202 Accepted`。它不是就地替换：worker 先从飞书读取 canonical Markdown，写入 MinIO 的 pending snapshot，再解析、切块和 embedding；全部成功后才原子 promotion 为 active snapshot。

失败的 resync 会把 `sync_status` 置为 `failed` 并保留上一次成功的 `content_ref`、checksum、revision 和 chunks，因此检索仍使用旧 snapshot。首次导入失败且没有 active snapshot 时，文档整体 status 才会是 `failed`。飞书权限被撤销、源文档被删除或 OAuth 失效都不会自动删除本地内容；需要管理员按数据保留策略手动删除 KB/文档。

系统没有远端自动轮询，也没有飞书事件订阅。前端在用户触发 import/sync 后每 2s 查询本地文档状态，并为 worker claim race 保留最多 60s 的 UI watch；这个 60s 只是浏览器观察窗口，不是 worker timeout。后台 `reconciler` 按 `FEISHU_RECONCILE_INTERVAL` 扫描超过 `FEISHU_SYNC_LEASE` 的本地 `syncing` 记录，恢复已有 pending snapshot 的 ingestion/metadata promotion，或重新入队 source sync；它不会检查飞书是否出现新版本。

状态判断：

- `sync_status=syncing`：worker 已 claim，或尚在 snapshot/ingestion 链路中。
- `sync_status=failed`：查看 `last_sync_error` 的脱敏原因；旧 snapshot 可能仍可用。
- `sync_status=idle` 且 `last_synced_at` 更新：最近一次 promotion 或 unchanged 检查完成。
- `409 sync_in_progress`：已有同步持有 lease，不要并发重复点击。

## 6. OAuth callback 与 API 错误

callback 永远重定向到已配置的前端，不接受 query 提供的任意跳转地址。失败时仅附加稳定错误码：

| 前端 query | 含义 |
|---|---|
| `oauth_cancelled` | 用户在飞书取消授权 |
| `oauth_state_invalid` | state cookie 缺失/不匹配/过期/已消费，或 callback 缺少 code |
| `tenant_not_allowed` | 登录身份不属于 `FEISHU_TENANT_KEY` |
| `feishu_reauth_required` | scope 不足或 OAuth account 要求重新授权 |
| `auth_service_unavailable` | token/user info/持久化/session 创建等非公开内部错误 |

导入/同步常见 JSON 错误码：

| HTTP / code | 排查方向 |
|---|---|
| `400 invalid_request` | JSON 非严格格式、缺少 URL，或 sync body 非空对象 |
| `401 reauth_required` | refresh token 失效；重新走飞书登录 |
| `404 resource_not_found` | KB/文档/account 不存在，或 owner 不匹配 |
| `409 resource_already_imported` | 同一 KB 已导入同一 canonical 飞书资源 |
| `409 sync_in_progress` | 同步仍在 lease 内 |
| `422 unsupported_feishu_url` / `unsupported_resource` | URL host/path/type 不支持 |
| `403 resource_forbidden` | 飞书 API 明确拒绝访问（同步 worker 中通常记录为脱敏的 source load failure） |
| `413 request_too_large` | import/sync 请求 body 超过 8 KiB |

## 7. 分项排错

### Tenant 或 scope

先查看前端 login error code，不要记录 callback query。确认应用已发布到目标租户、当前用户可用，并核对 scope 的精确字符串。`tenant_not_allowed` 不是 CORS 问题；`feishu_reauth_required` 需要在控制台增加缺失只读 scope 后重新授权。

### Rate limit 或飞书 5xx

客户端对 HTTP 429/5xx 自动重试最多 3 次并遵循 `Retry-After`。持续限流最终表现为同步失败，安全错误通常是 `source load failed`；稍后点“重试同步”。不要把 provider response body 或 Authorization header 加到日志。

### Stuck sync 与 reconciler

先看文档 `sync_status`、`updated_at`、`last_sync_error`，再确认 River worker 正常运行。少于 `FEISHU_SYNC_LEASE` 的 `syncing` 不会被 reconciler 抢占。超过 lease 后，确认周期性 reconcile job 已启动，并检查 `feishu_recovery_enqueue_failed`（只含 document ID 和 recovery type）。不要把 `FEISHU_SYNC_LEASE` 设得短于完整 sync + ingestion timeout。

### MinIO

启动阶段 `EnsureBucket` 失败会阻止服务就绪。同步中的 `snapshot write failed` 通常表示 `S3_ENDPOINT`、`S3_ACCESS_KEY`、`S3_SECRET_KEY`、`S3_BUCKET`、`S3_REGION` 或 `S3_USE_PATH_STYLE` 配置错误。确认 backend 能访问 endpoint、bucket 可创建/写入；不要在日志中输出 S3 secret。resync 写入失败不会替换 active snapshot。

### Bootstrap

看到 `BOOTSTRAP_OWNER_FEISHU_OPEN_ID is required...` 时不要随意填其他人的 open ID。先从飞书身份或现有 `oauth_accounts` 核对目标 owner，再配置并重启。bootstrap 只处理 NULL owner；已有 owner 不会被改变。

### 日志脱敏

HTTP access log 只记录 method、path、status、bytes、duration 和 request ID，不记录 query。worker 只记录稳定 event/error code、document ID、claim timestamp 或 storage key hash。排错时可以关联 `request_id`、document ID 和状态字段；禁止记录 access token、refresh token、app secret、OAuth state/code、完整授权 URL query、飞书 API response body、文档正文或未脱敏 storage key。

## 8. 验证

不连接真实飞书即可运行 fake OAuth + fake Feishu API E2E：

```bash
cd backend
go test ./internal/http -run 'TestFeishu(OAuthImportSyncOwnershipE2E|OperatorDocumentationMatchesRuntimeContracts)' -count=1 -v
go test ./...

cd ../frontend
pnpm lint
pnpm typecheck
pnpm test
pnpm build
```

E2E 覆盖 start → callback state/cookie → token/user info → local session → 带 credentials/CSRF/Origin 的 import/sync，以及第二用户的 cross-owner 拒绝。测试使用 TLS `httptest.Server`，不会访问真实飞书，也不依赖 PostgreSQL 或 MinIO。完整 migration、PostgreSQL/pgvector、MinIO 和 River 运行态仍应在可用 Docker 环境中另行验证。
