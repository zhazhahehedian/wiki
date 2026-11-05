# 公司电脑 review 后补充功能：隔离验证 · 2026-11-05

范围：传输超时与并发上限、迁移 0022（`bundle_key`、包 SHA-256/大小、疑似凭证扫描）、MCP 包内容预览、下载审计/统计/下载记录、MinIO 孤儿包清理、飞书消息通知、K2 冗余代码删除。设计见 [MCP 分发补充](../../superpowers/specs/2026-11-05-mcp-package-distribution.md) 与 [飞书消息通知](../../feishu-notifications.md)。

## 环境

公司 Windows 电脑，Docker Desktop 29.9.2（Linux 引擎）。一次性容器仅绑定 `127.0.0.1`，随机凭证，验证后已删除容器与卷：

- PostgreSQL：`pgvector/pgvector:0.8.2-pg18`（PostgreSQL 18.4，pgvector 0.8.2，与生产扩展版本一致）。测试只创建并删除自己的 UUID 库，结束后无残留。
- MinIO：`bitnamilegacy/minio:2025.7.23`（官方 `minio/minio` 已不在 Docker Hub 提供），随机桶 `caphub-it`，结束后桶内无对象。

未连接业务库、公司 MinIO 或真实飞书应用。

## 结果

| 检查 | 结果 |
|---|---|
| `go vet ./...`、`go test ./...`（单元） | 通过 |
| `TestRegistryCloudIntegration` / `TestGovernanceCloudIntegration`（含 `REGISTRY_TEST_S3=1` 真实 MinIO） | 通过：迁移 up/down/up、0022 回退后 0021 守卫仍拒绝有损回退、>10 MiB 上传与逐字节下载、下载审计与 `X-Bundle-SHA256`、回退后旧包不再报告校验值、下载次数与下载记录权限（使用者 404）、包文件清单（超 256 KiB 不内联、草稿对他人 404）、通知事件同事务入队且入队失败回滚审核状态 |
| `TestDirectoryPublishingIntegration` | 通过（17→19→17→19、发布 API 上传/提交） |
| `TestKnowledgeCloudIntegration`、`TestPlaygroundCloudIntegration` | 通过（未改动模块回归；embedding 与模型为 fixture） |
| `TestMinioListOlderIntegration`（新增） | 通过：真实 MinIO put/get/delete 与按时间过滤列举（孤儿清理依赖） |
| `TestNotifierEndToEndIntegration`（新增） | 通过：真实 PostgreSQL + River，提交 → 审核员、驳回（含原因）→ 发布者、通过 → 发布者；操作者不收到自己的通知；飞书端为记录器 |
| 前端 `pnpm test` / typecheck / lint | 124 测试通过；类型检查与 lint 通过 |
| 前端 `pnpm build` | 编译与 21 个页面生成通过；Windows 未开启开发者模式，standalone 复制符号链接失败（EPERM），Linux 镜像构建不受影响 |

未运行：`TestKnowledgeActiveDocumentsCloud`（读取根 `.env` 的远程库地址，测试不得连接远程库）。

## 仍待完成

- 飞书通知真实发送：需在开放平台启用机器人与 `im:message:send_as_bot`，部署后按 [飞书消息通知](../../feishu-notifications.md) 第 5 节用真实账号验收。
- 生产部署：业务库当前应为 19，升级到 22 前先备份；公司反向代理请求体约 100 MiB、读写超时不少于 3 分钟。
- 公司 MinIO 上的真实上传、审核、下载与字节校验。
