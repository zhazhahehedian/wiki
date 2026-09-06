# 阶段 C · 治理验证（2026-09-06）

## 实施范围

Owner 提交、Admin 审核/驳回、强制上线、Owner/Admin 下线、offline 重提、新版草稿、原子上线快照、可见范围覆盖、可信部门与治理审计。Web 上线目录与 Owner 管理列表分离；管理详情提供待审/上线规格对照和实际 Skill 文件。对应契约为 [spec §7.1](../../superpowers/specs/2026-09-04-capability-hub-design.md#71-阶段-c-实施契约2026-09-06)。凭证保险箱、机器 discovery、健康任务仍属 D/E。

`is_live` 与维护流程状态独立；待审新版不会改变消费者看到的旧版。通过审核时版本指针、元信息/名单快照与审计同事务切换。版本 ID 和 revision 绑定审核对象；已上线过的版本不可覆写。Admin 覆盖权限后，没有独立新版草稿的下线重提沿用覆盖范围。

## 自动化验证

- 后端 `go test ./...` 通过，`go build ./cmd/server` 通过（构建输出放在临时目录）。默认云集成测试因没有显式环境变量而跳过；不能以此代替以下云端验证。
- 前端 36 个测试文件、152 项测试通过；TypeScript 检查与 Next.js 生产构建通过。覆盖 Owner/目录缓存分离、审批对象、驳回理由与错误保留、Admin 门禁、请求 session/CSRF、现有 Skill 创建助手等。
- 显式启用云 PostgreSQL/MinIO 的 `TestRegistryCloudIntegration` 通过 B 和 C 主流程、Skill 创建助手及浏览器回归；fixture 已正常结束并清理。
- `TestGovernanceCloudIntegration` 已通过（303.977s），覆盖 C 的最终边界检查：首次登录 Admin 绑定、撤权后不恢复、权限矩阵、重复与过期动作、并发审批、提交/编辑竞争、审计故障回滚、上线快照隔离、部门撤销/覆盖、附件访问、审计筛选分页。
- 0014 与 0015 均在 UUID 隔离数据库完成 up/down/up；0015 回滚保留已有用户、能力、版本和 `user_llm_keys` 表。测试不修改现有开发库，不退役 KB/RAG 数据。

WSL 校验使用临时源码副本与已有 Linux 依赖缓存，没有更换原工作区的 Windows node_modules，也没有升级项目依赖。浏览器使用 Playwright/Headless Chromium；缺失的浏览器系统库仅下载解包到临时目录，没有安装到系统。首轮本机代理连接失败；重跑仅在测试进程清除 HTTP(S)/数据库代理并直连，未修改 `.env`。

## 浏览器验收

使用独立云数据库、MinIO 与四个模拟登录身份，保留实际 session/Origin/CSRF 中间件。页面运行真实 Next.js 生产构建，API 使用真实 registry/repo/storage；不调用公司模型。浏览器脚本与结果位于本目录：

- [验收脚本](browser-check.cjs)、[最终布局复查脚本](layout-check.cjs)、[检查摘要](checks.json)
- [执行结果](browser-result.json)
- [审核队列](review-queue.png)、[待审详情](review-detail.png)、[Owner 驳回反馈](owner-rejected.png)
- [消费者上线详情](consumer-published.png)、[版本对照](version-comparison.png)
- [可信部门授权](trusted-department.png)、[移动端详情](mobile-consumer.png)
- [审计筛选](audit-filtered.png)、[Skill 实际文件](skill-file-review.png)

浏览器完整流程通过：Owner 创建/提交、Admin 驳回、Owner 修订、批准上线、新版待审旧版不变、部门授权、下线重提、审计筛选与门禁、Skill 内容与实际下载。桌面 1440×1000、移动 390×844；无页面运行时错误、无横向溢出。最后修正了搜索按钮挤压换行，并在最终生产构建上重拍审核/部门页面，另附 [移动端部门授权](mobile-department.png)。

## 复现入口与运行态边界

从 backend 运行，先在当前进程提供数据库及 S3 环境变量：

```sh
REGISTRY_TEST_DATABASE_URL="$DATABASE_URL" REGISTRY_TEST_S3=1 \
  go test ./internal/http -run '^TestGovernanceCloudIntegration$' -count=1 -v -timeout 25m
```

增加 `REGISTRY_BROWSER_CHECK=1` 可启动仅绑定 `127.0.0.1:18089` 的测试服务。前端以 `NEXT_PUBLIC_API_BASE_URL=http://localhost:18089` 构建，在 localhost:13000 启动；顺序运行本目录 browser-check.cjs 与 layout-check.cjs，Playwright 从 NODE_PATH 提供。`/__test/session?as=owner|admin|consumer|outsider` 只存在于测试 fixture；完成后 POST `/__test/finish`，让测试完成并清理精确对象 key 与独立数据库。这些测试身份与路由不进入生产 server。

本轮未迁移现有开发库、未替换常驻服务、未指定真实人员的 Admin 角色。启用当前开发环境时，用项目迁移入口更新到 0015，并配置 `BOOTSTRAP_ADMIN_FEISHU_OPEN_ID` 后启动新 server。Bootstrap 持久化且仅补空，修改或删除 env 不转移/撤销已有角色；角色撤销需受控运维处理，已消费的 bootstrap 不会重新授权。

真实飞书 OAuth/session 沿用前阶段已经通过的结果；本轮治理的多角色验收使用模拟身份，未将其记为新增真实飞书角色运行态验收。公司模型联调继续按用户决定暂缓，不作为 C 前提。
