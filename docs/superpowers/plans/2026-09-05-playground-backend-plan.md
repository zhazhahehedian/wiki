# Capability Hub 游乐场后端与云联调

依据：能力中心 spec §6.7、§11.3、D8/D9。用户在阶段 A 前端完成后授权提前实现游乐场后端与联调；不扩大到注册中心/治理业务。

## 实施契约

- `GET/PUT/DELETE /api/v1/playground/connection`：按平台 session 的 user UUID 隔离配置；GET 只返回协议、地址、模型、默认模型、版本和 hasKey。保存时 Key 加密；Key 留空仅在地址不变时保留旧值，版本冲突返回 409。
- `POST /api/v1/playground/models`：认证和 CSRF 保护；用表单草稿地址/Key 请求兼容 `/models`，不保存草稿。空 Key 只复用当前用户同地址同协议的已存密钥；20 秒总超时、每页 1 MiB 响应上限、最多 5000 个模型（Anthropic 最多 5 页）、仅返回去重后的模型 ID，沿用流代理的出站地址限制。UI 支持搜索、勾选、默认模型与手动回退，关闭表单或更改地址/Key 时取消未完成获取。
- `POST /api/v1/playground/chat/stream`：接受用户选择的已配置模型、临时消息和可选 temperature/max_tokens。使用服务端解密的 Key 按协议调用 OpenAI 兼容 `/chat/completions` 或 Anthropic 原生 `/v1/messages`，输出规范化 delta/done/error SSE；不存对话。
- 所有接口保留认证和变更请求的 Origin/CSRF 检查；CORS 允许 PUT。请求体、消息数、模型列表及流响应设置上限，45 秒调用超时，取消传递到上游。
- 公网模型请求仅允许 HTTPS，DNS 解析结果检查后按 IP 拨号，禁止重定向。只有运维显式配置的 `PLAYGROUND_TRUSTED_ORIGINS` 可以走私网、HTTP 或系统代理；该名单是完整 origin，不支持通配符。
- 配置秘密与业务 Key 继续复用 AES-GCM；加密载荷绑定用户 ID、API 地址与协议；旧密文缺省协议按 OpenAI 兼容读取，密钥不进入响应、日志、浏览器持久存储或 Query devtools。UI 保存成功后清空 Key 草稿。
- 新增 goose 0012/0013 和 sqlc 查询；user_llm_keys 使用 users.id 外键、TEXT[] models、包含 nonce 的 key_ciphertext 和乐观锁 version。
- `DATABASE_PROXY_URL` 为可选 SOCKS5 开发连接，迁移和运行池共用。S3/飞书使用标准 HTTP_PROXY/HTTPS_PROXY；自定义模型仅在显式可信 origin 下使用代理。

## 完成情况

- [x] 数据库迁移、sqlc 生成、配置服务及加密/隔离/版本冲突保护。
- [x] 模型流式代理、错误脱敏、取消和出站地址限制。
- [x] 模型自动发现，列表搜索与勾选、默认模型、失败后的手动回退；测试草稿不落库、同地址密钥复用、跨用户拒绝、空/畸形/超大响应与取消。
- [x] OpenAI/Anthropic 协议选择、Claude 根地址规范化、原生鉴权/系统提示词/流解析、模型分页，旧配置与密文兼容回归。
- [x] 前端持久化配置、加载失败/保存失败处理、发送/停止/重新生成。
- [x] 云 PostgreSQL capability_hub 与 MinIO capability-hub-dev 创建；capability_hub 已迁移到 0013。
- [x] 临时独立数据库执行全量 up、0013/0012 down/up；加密、跨用户读删隔离、CSRF、版本冲突和模型流测试通过，测试库在结束时清理。
- [x] MinIO 唯一测试对象写入与读回一致，删除成功，测试前缀剩余对象为 0。
- [x] 浏览器通过测试专用后端完成：配置保存、流式回复、刷新恢复配置、Key 不回显、留空保留 Key 后再次调用成功。测试后端仅监听本机，不编入生产 server；采用隔离测试账号和模拟模型。
- [x] 真实飞书登录：本地凭据已配置，后端与回调统一使用 8088；用户完成飞书登录，callback 302、auth/me 200、账号配置 GET 200，页面显示真实账号。
- [ ] 真实模型联调【2026-09-06 用户明确暂缓】：当前不在公司内网，无法调用公司模型。暂时跳过，不阻塞后续注册中心开发；回到可访问公司模型的环境后再恢复。现有模拟协议与端到端测试不计为真实模型验收。

自动化检查：前端 31 个测试文件、127 项测试及 typecheck 通过；后端全量测试与 server/migrate 构建在 WSL 通过。Windows 全量测试曾因遗留 tokenizer 下载 DNS 失败，WSL 缓存环境回归通过；新增云集成测试使用 Windows Go 1.25.7。WSL Next.js standalone 生产构建通过；之后只调整了协议提示文案。git diff --check 通过，检查了 391 余个受版本控制及待添加文件，未发现本地环境秘密写入源码。

协议依据：[Anthropic 模型列表](https://platform.claude.com/docs/en/api/models/list)、[Messages](https://platform.claude.com/docs/en/api/messages/create)、[流事件](https://platform.claude.com/docs/en/build-with-claude/streaming)。Claude 原生模型列表与流式对话使用模拟上游验证，真实服务仍需用户 Key。

清理：一次手动浏览器 fixture 达到八分钟时限后退出，测试库受残留连接影响未立即删除；随后仅对该 UUID 测试库执行强制断开清理，并修正 fixture 清理逻辑，新的云集成测试通过。未删除开发库或用户数据。

## 继续联调检查（2026-09-05）

- 恢复已停止的本地前后端进程，明确前端 3000、后端 8088；避免 Next.js 继承后端 PORT。登录页与 healthz 实测均为 200。
- 正式服务旧 `/api/v1/kbs` 返回 404；未认证的配置 GET、模型发现 POST 与对话 POST 均返回 401。
- 仅查询云数据库配置条目数，当前为 0；未读取或解密用户模型 Key。真实模型验收仍待用户保存有效配置。
- 未改业务逻辑，本次更新启动说明和运行态证据，无需重复全量测试。
