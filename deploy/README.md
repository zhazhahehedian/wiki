# 服务器部署

本目录只编排平台后端、前端和 HTTPS 代理，复用外部 PostgreSQL/pgvector 与 MinIO，不创建或重建 1Panel 的基础服务。现有 80/443 已被其他网站代理占用时，先调整代理方案，不直接启动 `edge`。

## 目录和配置

推荐使用 `/opt/capability-hub/releases/<release>` 保存源码，`shared/` 保存配置，`backups/` 保存备份；后三者都不放入 Git。部署目录和密钥文件分别使用 700、600 权限。

在 `/opt/capability-hub/shared/deploy.env` 配置非秘密的部署参数：

```dotenv
SITE_DOMAIN=hub.example.com
RELEASE_TAG=<release>
# 可选：仅发布前端时覆盖其标签，后端仍使用 RELEASE_TAG
FRONTEND_RELEASE_TAG=<frontend-release>
BACKEND_ENV_FILE=/opt/capability-hub/shared/backend.env
INFRA_NETWORK=1panel-network
```

`backend.env` 以根目录 `.env.example` 为字段参考，使用原始 `NAME=value`，不要给值添加 shell 引号；Compose 使用 `format: raw` 保留密钥中的 `$` 等字符，需要支持该选项的 Compose 版本。不要输出完整 `docker compose config` 或容器环境，验证配置使用 `config -q`。

- 数据库和 S3 地址改用同一 Docker 网络中的实际服务名及容器端口，不使用宿主机的公网映射端口。
- 沿用现有 `OAUTH_ENCRYPTION_KEY`，否则已经加密保存的凭证无法解密。不要将本机开发代理配置带到服务器。
- `FRONTEND_ORIGIN=https://hub.example.com`、`FEISHU_REDIRECT_URL=https://hub.example.com/api/v1/auth/feishu/callback`、`SESSION_COOKIE_SECURE=true`。飞书控制台需要登记同一个回调地址。
- 前端构建时将 `NEXT_PUBLIC_API_BASE_URL` 设为空，通过同源 `/api` 访问后端。不要把本机 `.env.local` 打包进镜像。
- embedding 必须确定模型、接口地址、实际输出维度及密钥。0017 会固定向量维度；未配置 embedding 时知识路由不装配，但启动仍会运行数据库迁移。
- 重排配置使用 Chat Completions 兼容接口，三项 `KNOWLEDGE_OPTIMIZER_*` 同时提供。
- 本次已选百炼北京 `tongyi-embedding-vision-plus-2026-03-06`：`EMBEDDING_PROVIDER=dashscope`、`EMBEDDING_BASE_URL=https://dashscope.aliyuncs.com/api/v1`、`EMBEDDING_DIM=1024`；原生接口适配只用于文本。优化模型为 `deepseek-flash`，base 为 `https://api.deepseek.com`。输入预算及限制见 [知识服务 API](../docs/knowledge-api.md)。先在隔离环境验证两套密钥，再启动业务库服务。

## 构建与上线

从 release 目录运行，以下构建命令不会启动应用或迁移数据库：

```sh
docker compose --env-file /opt/capability-hub/shared/deploy.env -f deploy/compose.yaml config -q
docker compose --env-file /opt/capability-hub/shared/deploy.env -f deploy/compose.yaml build backend frontend
```

上线前完成数据库 `pg_dump -Fc`、对象存储备份及备份可读性检查。对象文件热备不等同于一致性快照，最终备份应协调写入停止或使用对象存储支持的备份流程。在隔离数据库恢复备份，使用镜像内 `/app/migrate up` 验证升级，并在该隔离库验证对应回退和再升级。不得把回退命令用在业务库做常规清理。

确认业务库升级授权、模型维度、DNS A/AAAA、服务器 80/443 入站规则和飞书回调后启动：

```sh
docker compose --env-file /opt/capability-hub/shared/deploy.env -f deploy/compose.yaml up -d
docker compose --env-file /opt/capability-hub/shared/deploy.env -f deploy/compose.yaml ps
```

**后端启动会自动执行 Goose 和 River 迁移。** 回滚镜像不会回滚数据库；如迁移不兼容，先停止应用写入，再按审核过的恢复方案处理，禁止直接 reset。保留上个 release、镜像和 HTTPS 数据卷，不执行 `down -v`。

验证 HTTPS、健康接口、未登录拒绝、真实飞书登录、管理员发布与普通用户访问，以及配置 embedding 后的真实索引和检索。fixture 或仅健康接口通过不等于完整上线验收。这些容器可在 1Panel 容器列表管理，但此配置不会自动登记为 1Panel 网站或应用商店应用。

## 参考

- [Caddy HTTPS 前提与证书续期](https://caddyserver.com/docs/automatic-https)
- [Caddy 反向代理和流式响应](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy)
- [Compose env_file 格式](https://docs.docker.com/reference/compose-file/services/#env_file)
