# 内部能力市场与 Skill 复用验证

日期：2026-09-19。范围见 [实施计划](../../superpowers/plans/2026-09-19-skill-market-plan.md)。

## 已实现

- 原注册中心入口展示为「能力市场」，保留类型、搜索、部门、分页与卡片/表格；场景快捷按钮向授权目录发起关键词搜索，切换条件重置分页。状态筛选只出现在我的发布。
- 目录查询一次关联当前上线版本号和发布者显示名，保留原有授权与发布快照过滤；只变更 SQL 查询并重新生成 sqlc，无 schema 迁移。
- `/hub/registry/{slug}/market` 为独立发布详情，与管理详情分开查询和缓存。展示 Markdown 正文、文件清单、附件预览、下载和通用安装说明；HTML 不执行、远程图片不加载。
- `/api/v1/catalog/{slug}/template?version_id=...` 重新读取授权上线快照；固定版本，过期为 409，下线/隐藏为 404。读取无写入副作用，不调用模型。
- 兼容文本包完整导入生成器，创建新的随机后缀标识；没有模型配置也可编辑、确认保存。保存沿用当前用户的 Owner 草稿流程，不自动上线，不修改来源。
- 脚本、二进制、额外 frontmatter、超限及不能完整保留的包明确拒绝复制（422），提示下载原包，不静默删除内容。
- 模板查询不参与 registry 保存后的批量失效，避免保存过程中重新加载来源、打断编辑。未保存内容仍只在页面内存中。

## 已通过

- `sqlc generate`；`go test -timeout 60s ./internal/registry ./internal/skillbuilder ./internal/http ./internal/repo`；`go build ./cmd/server`（输出至 Linux 缓存目录）。
- 新增后端用例覆盖文本/附件保持、新标识、脚本/二进制/额外文件头/体积等不支持转换；完整鉴权路由覆盖读者/Owner/Admin、无登录、不可见、下线、版本变化、未指定版本和不支持模板。HTTP 测试使用真实 ZIP 打包与解包、模拟仓储，不代表数据库授权集成已验收。
- 前端 6 个相关 Vitest 文件共 35 项通过，覆盖市场导航/筛选/分页、发布详情与管理缓存隔离、无权限、模板加载错误/重试、没有模型配置时编辑与保存、原模板不被修改；缓存与 Markdown 容器调整后相关 3 项测试重跑通过。
- `pnpm typecheck`、最终 `pnpm build` 通过，包含新动态路由。
- Playwright + 本机 Edge 无头浏览器，使用生产构建，桌面 1440×1000、手机 390×844 均通过：筛选/翻页重置、发布详情、附件预览、下载动作和文件名、版本固定的模板读取、编辑并保存为新草稿、保存不重新读取来源、不调用模型、不支持模板/无权来源错误状态。无横向溢出，无页面脚本错误。结果见 `result.json`。
- 截图检查：`desktop-market.png`、`desktop-detail.png`、`desktop-template.png` 及对应 `mobile-*`。截图中的条目均为测试数据，没有写入用户数据库。

## 远程数据库只读补验

用户指出应沿用现有远程服务器后，已通过项目 `.env` 和现有 WSL 直连方式连接配置中的数据库。在服务端只读事务内确认迁移版本为 15，并实际执行重新生成的 `ListCatalogCapabilities` 查询（含版本和发布者关联），成功返回。探测身份未授予 Admin/Owner/部门权限，仅查询全公司可见 Skill，本次返回 0 条；这证明连接、表结构和 SQL 可执行，不代表已有样本的字段值或写入流程已验收。检查未打印连接串、凭证或用户内容，未修改业务数据。

此前将 Docker 引擎不可用表述为数据库联调的阻塞条件不准确：项目已有可用远程数据库。未配置的是隔离集成测试入口 `REGISTRY_TEST_DATABASE_URL`，不能据此推断远程数据库不可连接。

## 待验收与运行条件

- `TestGovernanceCloudIntegration` 已补充真实仓储场景：来源复用、当前用户草稿所有权、目录版本/发布者、旧版与待审版隔离、版本变化和下线。本轮未运行完整写入集成测试：该入口会在远程服务器创建独立 UUID 测试库，在其中执行迁移/回滚、写入测试用户与草稿，最后删除测试库；不能与上述只读连通性/SQL 验证混为一谈。
- 浏览器拦截业务 API，下载返回测试内容；不冒充真实账号、数据库、MinIO 或公司模型验收。真实 ZIP 的打包与附件复制在 Go 测试中验证。
- 未重启现有业务服务，未提交或推送，未迁移数据库。实际使用需运行更新后的前后端并已有对当前用户可见的上线 Skill；空市场不会预置虚构内容。

## 复现浏览器验证

生产前端监听 `http://localhost:13019` 后执行：

```powershell
$env:NODE_PATH = 'C:/Users/10739/.cache/wiki-market-tools/node_modules'
$env:BROWSER_PATH = 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'
node docs/verification/2026-09-19-skill-market/browser-check.cjs
```

可用 `APP_URL` 覆盖地址；安装 Playwright 的路径通过 `NODE_PATH` 指定，`BROWSER_PATH` 可省略并使用 Playwright 自带 Chromium。业务请求全部拦截，不需要真实模型或账号密钥。
