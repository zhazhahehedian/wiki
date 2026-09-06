# UI 收敛与游乐场操作验证

2026-09-06。依据用户提供的米醋 API 游乐场截图，收敛 Capability Hub 的视觉与模型操作。

## 改动

- 白色侧栏、白灰内容、低饱和蓝主操作；去掉首页渐变、光晕和彩色装饰。首页改为能力目录、我的发布、游乐场入口与发布步骤。
- 注册中心、详情、治理、创建表单、Skill 助手与接口配置统一灰阶与圆角。状态保留文字与语义色：草稿/下线灰，审核中琥珀，上线绿。
- 模型选择移到输入工具栏；参数入口使用可关闭、支持 Escape 的浮层，包含温度、Top P、频率惩罚、存在惩罚、最大 Tokens、系统提示词。
- 数值参数默认关闭；只有启用值随请求发送，包括显式零值。温度与 Top P 择一，恢复默认会清除参数和提示词。参数只在当前页面保留。
- Claude 原生协议不发送惩罚参数，温度上限 1，不同时传温度和 Top P；未指定最大 Tokens 时仍提供必需的 4096。模型本身是否支持采样参数由上游决定。
- 沿用个人接口配置的加密存储、获取并勾选模型、手动 ID 回退和默认模型设置。

## 验证结果

- `go test ./internal/playground ./internal/skillbuilder ./internal/http`：通过。新增测试使用本地 HTTP 上游验证参数转发、关闭字段省略、零值、Claude 默认值、非法范围和不支持组合不发往上游。云集成仍为显式 opt-in，本次未运行。
- 前端 `pnpm test`：第一轮 36 文件、154 测试通过；后续数字框编辑修正与跨协议回归增加后，最终游乐场测试 12 项全部通过。
- `pnpm typecheck`：通过；最终 `pnpm build`：通过（含类型校验）。使用仓库版本对应的临时 Linux 构建目录，未替换 Windows node_modules 或升级依赖。
- `browser-check.cjs` 在最终生产构建上通过。使用 Playwright 拦截 API 的固定示例数据，检查获取/选择/保存模型、参数实际请求体、SSE 回复、Escape 关闭、卡片/表格切换、移动端弹窗与导航；1440×1000 与 390×844，无页面异常或横向溢出，参数浮层在视口内。
- 截图及机器结果见本目录。所有人物、能力与模型均为测试数据，不代表现有数据库内容或实际用户权限。

未迁移或修改真实数据库，未重启现有开发服务，未进行真实公司模型联调；本次验证不替代此前暂缓的公司网络验收。未提交或推送。

## 参考

- 用户提供的米醋 API 游乐场截图：白灰布局、输入工具栏、按需启用参数。
- [new-api 游乐场参数定义](https://github.com/QuantumNous/new-api/blob/main/web/src/features/playground/types.ts)：核对独立参数开关与请求字段；本次组件独立实现。
- [Claude Messages API](https://platform.claude.com/docs/en/api/http/messages/create)：核对采样参数、输出长度与模型支持限制。

## 复现

前端构建时设置 `NEXT_PUBLIC_API_BASE_URL=http://localhost:13001`，以 13001 端口启动前端。安装或使用独立环境中的 Playwright（验证版本 1.58.2），从仓库根运行：

```sh
NODE_PATH=/path/to/playwright/node_modules node docs/verification/2026-09-06-ui-refresh/browser-check.cjs
```

脚本仅在浏览器中模拟 `/api/v1/**`，不需要后端、数据库或真实模型密钥。截图写入本目录。
