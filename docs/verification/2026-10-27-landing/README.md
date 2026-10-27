# 2026-10-27 动态落地页

已部署到 https://hub.sn0w.icu。根路径从跳转工作台改为公开产品介绍；工作区权限边界保持不变。

## 页面与交互

- 能力连接图带轻量流动和悬浮动画，可手动暂停，并遵循 `prefers-reduced-motion`。
- 三个切换入口分别介绍能力市场、知识服务与模型游乐场。流程明确标为示意，不读取内部能力或知识内容。
- 未登录入口转 `/login`，有效登录状态提供工作台及相应功能入口。登录页确认会话后进入 `/hub`，避免再次停留在落地页。
- 公司飞书应用尚待配置，公开页保留“登录接入中”提示；完成公司应用配置时同步调整这段文案。

## 验证

- WSL 既有 Node 24 工具链：落地页 5 项、登录页 11 项测试通过；TypeScript 检查通过。
- 服务器生产镜像构建通过，包括 Next.js 编译、类型校验、18 个静态页面生成及打包。Windows pnpm 包装器未能使用已有依赖，因此未重新安装或清除 Windows 依赖；验证使用现有 WSL 依赖目录。
- 浏览器检查桌面和 390px 手机布局，并修正手机连接节点间距；手机功能切换通过。正式站点再次验证模型面板切换与动画暂停。
- [公网 HTTP 检查](health.json)：根路径、登录页、健康接口 200，未登录认证和市场接口 401。真实飞书 OAuth 未在本次验收范围内。
- [线上桌面截图](desktop-live.png)、[本地手机预览截图](mobile-preview.png)。手机截图来自同一源码的本地开发预览，底角开发工具不会出现在生产构建。

## 发布与回退

前端镜像 `capability-hub-frontend:20261027-landing`；后端继续使用 `20261026-verified`，容器 ID 未变化，无数据库迁移。配置新增可选 `FRONTEND_RELEASE_TAG`，未设置时沿用 `RELEASE_TAG`。

当前源码目录 `/opt/capability-hub/releases/20261027-landing`，`current` 链接已更新。Caddy 仍挂载上一版相同内容的 Caddyfile，本次只通过 `up -d --no-deps frontend` 替换前端。[发布记录](release.json)保存镜像及配置备份位置。

如需回退本次页面，将 `shared/deploy.env` 中 `FRONTEND_RELEASE_TAG` 改为 `20261026-verified`，使用当前 release 的 Compose 文件只重建 frontend；不要回退数据库。上版镜像与 `shared/deploy.env.before-landing` 已保留。当前修改均未提交或推送 Git。
