# 2026-10-27 授权成功跳转修复

用户已确认公司飞书账号能够正常登录，但授权后落在公开落地页。原因是此前只修改前端登录页的有效会话跳转，后端 OAuth 回调仍将成功地址配置为 `/`。

生产认证装配现在将成功目标设为 `/hub`，登录失败仍进入 `/login?error=...`。公开根路径继续展示落地页。本次不修改身份映射、管理员角色、租户配置或数据归属。

验证：WSL 下 `go test ./cmd/server ./internal/http ./internal/service ./internal/auth` 通过。新增回归测试使用生产处理器配置与模拟成功认证流程，确认设置 Secure/HttpOnly 会话 Cookie 后返回 302 到 `/hub`，并忽略调用者传入的外站 redirect。该测试不冒充真实飞书授权重试。

生产镜像 `capability-hub-backend:20261027-auth-redirect` 构建成功，已仅重建后端容器并通过健康检查。前端容器未变化，Goose 保持 17；已有持久化登录会话未主动失效。当前 release 为 `/opt/capability-hub/releases/20261027-auth-redirect`。详见 [发布记录](release.json) 与 [公网检查](health.json)。真实飞书登录在修复前已由用户确认成功，修复后的最终跳转仍待用户重新登录验证。

上版后端镜像 `capability-hub-backend:20261026-verified` 与 `shared/deploy.env.before-auth-redirect` 保留，必要时只恢复后端镜像标签并重建 backend；不要回退数据库。无 Git 提交或推送。
