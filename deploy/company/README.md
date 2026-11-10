# 公司内网发版流程

适用于公司服务器 `SZXVM-AI-Invocation01`（10.20.6.231，CentOS 7，经堡垒机登录，离线运行）。服务器上是独立的离线部署包（安装目录 `/opt/capability-hub-company-20261101`），不使用本仓库 `deploy/compose.yaml`。2026-11-05 首次按本流程从 `20261101-c0c150b` 升级到 `20261105-8a78e4c`（迁移 19 → 22）。

## 1. 开发与验证（公司 Windows 电脑）

1. 在功能分支开发，按 AGENTS.md 的风险要求测试。涉及数据库时，用 Docker 起一次性 PostgreSQL / MinIO 跑集成测试（只绑定 `127.0.0.1`）：

   ```bash
   docker run -d --name caphub-it -e POSTGRES_PASSWORD=<随机> -p 127.0.0.1:55432:5432 pgvector/pgvector:0.8.2-pg18
   cd backend
   REGISTRY_TEST_DATABASE_URL='postgres://postgres:<随机>@127.0.0.1:55432/postgres?sslmode=disable' \
   DIRECTORY_TEST_DATABASE_URL=同上 PLAYGROUND_TEST_DATABASE_URL=同上 \
   KNOWLEDGE_TEST_DISPOSABLE=1 KNOWLEDGE_TEST_DATABASE_URL=同上 go test -count=1 ./...
   ```

   测试只创建、删除自己的随机库；用完 `docker rm -f -v caphub-it`。
2. 提交并推送。GitHub 需经本机代理：`git -c http.proxy=http://127.0.0.1:7897 push`（只对这条命令生效）。

## 2. 打离线包（公司 Windows 电脑）

前提：Docker Desktop 运行中；构建需要访问 Docker Hub，Docker Desktop 跟随 Windows 系统代理，请先打开代理软件的「系统代理」。

```bash
bash deploy/company/build-release.sh            # 输出到仓库旁的 ../release/
```

脚本要求工作区已提交（标签为 `日期-提交号`），构建 `linux/amd64` 前后端镜像，用一次性 PostgreSQL 18 + MinIO 做冒烟（迁移空库、`/healthz`、前端首页），然后导出：

```
release/capability-hub-<TAG>/
  capability-hub-images-<TAG>.tar.gz   # docker save 的两个镜像，含全部基础层
  TAG  MIGRATION_VERSION  upgrade.sh  SHA256SUMS
```

导出失败时可用 `SKIP_BUILD=1` 复用已构建的镜像重新导出。

## 3. 升级服务器

1. 经堡垒机把整个 `capability-hub-<TAG>/` 目录上传到 `rsdev` 家目录。
2. 执行（会显示当前与目标版本并要求确认）：

   ```bash
   cd ~/capability-hub-<TAG>
   sudo bash upgrade.sh                     # 默认安装目录 /opt/capability-hub-company-20261101
   ```

   脚本依次：校验 SHA-256 → `docker load` → `pg_dump` 业务库到 `/opt/capability-hub-backups/`（校验可读）→ 备份 `config/compose.env`、`config/migration-verified` → 切换 `BACKEND_IMAGE`/`FRONTEND_IMAGE` → `scripts/verify-migrations.sh`（同实例临时库升级/回退/升级，失败则恢复原配置、业务库不动）→ `scripts/start-backend.sh`（迁移业务库）→ `scripts/start-base.sh`（换前端）→ 输出容器状态、首页与 `/healthz` 状态码，并核对迁移版本。

3. 浏览器验收：登录、市场、已有 MCP/Skill，以及本次功能的关键路径。

服务器注意：`docker` 位于 `/usr/local/bin`（root 的 PATH 默认没有，脚本已处理）；rsdev 需 `sudo` 才能访问 Docker。

## 4. 回退

- `verify-migrations.sh` 失败：脚本已恢复原配置，旧版本继续运行，业务库未变。
- 后端已迁移后需要回退：新迁移可能改了表结构，**不能直接把旧镜像跑在新库上**。用本次 dump 恢复数据库（`pg_restore --clean`），再恢复 `config/compose.env.before-<TAG>-<时间>` 与 `migration-verified.before-...`，执行 `start-backend.sh` 与 `start-base.sh`。操作前先确认备份时间点之后的新数据是否可以丢弃。
- 迁移 0021 起，存在 MCP 程序包后不可经迁移回退，只能用 dump 恢复。

## 5. 配置变更

只改配置（不换镜像）时：先备份 `config/backend.env`，修改后执行 `sudo bash scripts/start-backend.sh`，compose 会因配置变化重建后端。常见项：

- `PLAYGROUND_TRUSTED_ORIGINS=https://tokenhub.robosense.cn`：内网模型网关解析为私有地址，需要加入白名单才能在游乐场使用。
- 前置网关（`capability-hub-gateway`，Caddy）：MCP 程序包网页分片上传每个请求约 8 MiB，请求体上限不能低于 9 MiB；程序包下载最长 15 分钟，读写超时不应更短。
- `FEISHU_NOTIFY_ENABLED=true`：开放平台启用机器人与 `im:message:send_as_bot` 后开启，见 [飞书消息通知](../../docs/feishu-notifications.md)。
