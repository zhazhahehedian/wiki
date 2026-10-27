# 2026-10-26 / 27 服务器部署验证

状态：**2026-10-27 00:17（北京时间）经用户授权完成业务库迁移并启动正式站点，公网健康及未登录拒绝检查通过；公司飞书登录验收待完成**。基于 main `2690a0f` 加本次尚未提交的部署与模型适配变更；没有提交或推送。

## 实际环境

- Ubuntu 22.04，1Panel v2.1.13，Docker 29.5.2、Compose v5.1.4。
- 复用 `1panel-network` 上的 PostgreSQL 18.2（pgvector 0.8.2）和 MinIO；不修改现有 MySQL、Redis。
- 正式域名为 `hub.sn0w.icu`，已解析到服务器，Let's Encrypt 证书已签发，HTTP 308 跳转 HTTPS。维护页已替换为正式应用，根路径 307、登录页 200。用户另将根域名与 www 指向同机，但平台入口仍为 hub。
- 业务数据库为 `capability_hub`，已从 Goose 15 升至 17，`knowledge_chunks.embedding` 为 `vector(1024)`；旧 `itwiki` 库未迁移，保持 Goose 12。
- PostgreSQL 的 pgvector 二进制位于现有容器的可写层，不在原始 `postgres:18.2-alpine` 镜像中。不要通过 1Panel 重建或升级该容器，直到明确持久化扩展的镜像方案；仅备份数据库不能恢复扩展二进制。

## 产物与备份

- 最终服务器源码：`/opt/capability-hub/releases/20261026-verified`；初始镜像及模型适配中间版本均保留。
- 最终镜像：`capability-hub-backend:20261026-verified`、`capability-hub-frontend:20261026-verified`。后端镜像 ID：`sha256:1069b5fe57c330ff2820a0b22daad3e8abcaa080ab4dc9d69a089f090d2b8d69`。
- 私密配置：`/opt/capability-hub/shared/backend.env`（600），保留原 OAuth 加密密钥，改用 Docker 内网连接基础服务；模型配置已就绪，密钥不进入 Git 或验证输出。
- 备份：`/opt/capability-hub/backups/20261026T143804Z/`，含两个库的 custom-format dump、MinIO 文件归档和 SHA-256 清单。
- 本机副本：仓库忽略目录 `.cache/deploy-20261026/20261026T143804Z/`，三个归档均与服务器清单校验一致。数据库 dump 可列目录；`capability_hub` 已完成实际恢复验证。MinIO 为在线文件备份，已验证归档可读，未进行停写一致性快照或对象恢复演练。
- 上线前另备份业务库至服务器 `backups/20261026T151453Z/`，已下载到本机相同时间戳目录并校验 SHA-256。备份后只读核对业务库仍为 15，users=1、capabilities=1、knowledge_bases=0。
- 授权后、迁移前最终备份：`backups/20261026T161702Z/`，含业务库 dump、MinIO 文件归档及 SHA-256 清单，本机副本位于 `.cache/deploy-20261026/20261026T161702Z/`，两个归档均已校验 SHA-256 一致。这是 UTC 时间戳，对应北京时间 2026-10-27 00:17。
- 当前 release 链接：`/opt/capability-hub/current` → `releases/20261026-verified`。前后端和 Caddy 均已运行，后端健康检查为 healthy，重启策略为 `unless-stopped`，前后端没有单独公开宿主机端口。

## 验证结果

| 检查 | 结果 |
|---|---|
| Compose `config -q`、Caddy validate | 通过 |
| 前后端生产镜像构建 | 通过；前端 Next.js 15.5.18，pnpm 9.12.0 |
| 隔离库备份恢复 | 通过，库名 `capability_hub_deploy_check_20261026` |
| 隔离库迁移 | 15 → 17 → 16 → 15 → 17 通过，最终选定维度也是 1024 |
| 既有数据条数 | users、capabilities、capability_versions、documents、knowledge_bases、chunks、user_llm_keys、oauth_accounts 与恢复时一致 |
| 隔离后端 /healthz | 200；生产容器 healthcheck 命令退出码 0 |
| 代理到前端 /login | 200，7360 bytes HTML |
| 代理到后端 /healthz | 200 |
| 未登录 /api/v1/auth/me、/api/v1/catalog | 401 |
| 早期无 embedding 的隔离知识路由 | 404，符合未装配行为；正式环境已配置模型，见下方生产检查 |
| 生产前端本机地址残留 | `.next/static` 不含 `http://localhost:8080` |
| Go 完整测试与编译 | WSL 通过；服务器 Docker 构建工具链完整测试通过，最后的限定词修正又验证了三个受影响包 |
| PostgreSQL/MinIO/River 隔离集成 | HTTP 与 repo 知识测试通过，覆盖版本发布、授权、删除、撤销与失败边界；此组 embedding/优化模型为 fixture |
| 实际模型接口 | 百炼北京 `tongyi-embedding-vision-plus-2026-03-06` 返回 1024 维；DeepSeek `deepseek-flash` 关闭 thinking 后返回有效 JSON |
| 实际模型索引 | 隔离库中三份测试文档通过 River 完成索引，使用真实 MinIO、百炼与 pgvector |
| 实际模型 HTTP 检索评测 | baseline 8/8、optimization 6/6 通过，无降级警告；权限拒绝和应用凭证撤销通过 |

[真实模型评测报告](real-model-evaluation.json)中的语料为仓库 fixture，不是用户业务文档。`unchanged` 包括对显式否定/限制词的确定性原文保护，不代表每题都调用了改写模型；14 题均执行了真实模型重排。小型评测全部通过不能外推为任意业务语料质量保证。验收使用独立 fixture 用户及应用 HTTP handler，不代替真实公司飞书登录。

临时验证容器与网络已清理，真实验收对象已删除、测试应用凭证已撤销，隔离库保留用于复核。

## 2026-10-27 正式启动

用户明确授权迁移并启动后，先补做最终备份，再使用最终后端镜像执行 `/app/migrate up`，0016/0017 均成功。迁移前后八张业务表条数一致；既有用户、能力、版本、个人模型密钥与 OAuth 账号整行聚合校验值一致，见[迁移结果](production-migration-result.json)。启动后再次读取业务库确认版本 17、1024 维及上述已有记录保留。

Compose 使用正式 Caddy 配置启动三个应用容器，`--wait` 成功。公网 HTTPS 证书验证成功，`/login`、`/healthz`、`/api/healthz` 返回 200；未登录访问认证、市场、知识库管理及机器检索接口均返回 401，见[正式环境健康记录](production-health.json)。这些检查不代替真实账号登录或生产知识写入验收。

## 待完成

后续更新：2026-10-27 已部署[公开动态落地页](../2026-10-27-landing/README.md)，根路径现在返回 200。前端独立升级为 `20261027-landing`，后端及数据库保持本次部署版本；`current` 链接指向新 release。上文镜像与根路径 307 为首次启动时记录。

1. 用户安排 2026-10-27 可能切换公司主体飞书应用；登记回调、核对租户和身份/管理员/资产映射，见[飞书切换交接](../../deploy-debug-feishu.md)。真实公司登录和市场发布/下载验收留到切换后。
2. 自动登录跳转检查曾被工具审批拦截，未返回具体原因，不计为通过。模型 HTTP 测试不受此影响，已经完成；真实 OAuth 仍待用户账号验收。
