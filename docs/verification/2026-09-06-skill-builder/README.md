# Skill 创建助手验证 · 2026-09-06

## 范围与环境

页面使用当前 Next.js 生产构建，Headless Edge 通过真实 HTTP 访问独立测试后端。登录态使用测试用户与真实 session/CSRF 中间件；用户模型配置存入独立云 PostgreSQL 并加密；模型上游为本地确定性假服务。生成结果经实际注册中心服务保存到云 PostgreSQL/MinIO。数据库、用户、对象均为 UUID 隔离的测试资源，结束后清理。没有调用公司模型，没有迁移现有开发库或更新常驻服务。

女娲部分为固定版本的方法适配，验证的是协议与交互闭环；人物思维的质量、公司模型兼容与真实生成效果尚未验收。来源和 MIT 许可见 [ATTRIBUTION.md](../../../backend/internal/skillbuilder/prompts/ATTRIBUTION.md)。

## 自动化

- `go test ./...` 通过，`go build ./cmd/server` 通过。新增服务测试覆盖模板/历史、大小上限、无效与截断输出、取消、修复未完成编辑、frontmatter/相对链接/文本附件、保存校验；HTTP 测试覆盖身份、session/CSRF、禁止客户端 system、错误脱敏。目录查询取消时不再写出 500。
- 前端 35 个文件、144 项测试通过，类型检查与生产构建通过。新增 7 项测试覆盖配置门禁、追问/迭代、编辑预览/保存、生成和保存失败保留编辑、取消/晚到响应、配置切换/卸载取消、附件与 API session/CSRF。
- WSL 验证使用临时源代码副本及既有 Linux 依赖缓存，未替换仓库内的 Windows `node_modules`。本次未新增或升级依赖。
- 显式启用 `REGISTRY_TEST_DATABASE_URL` 和 `REGISTRY_TEST_S3=1` 的 `TestRegistryCloudIntegration` 通过；内部独立数据库迁移 up/down/up，OpenAI/Anthropic 本地上游、未配置账号、生成不自动保存、修改后保存、冲突、Owner 隔离和 zip 每文件逐字节一致性通过。默认不设环境变量时该云端测试会跳过，不能将普通 `go test` 当成云端验收。

## 浏览器

桌面 1440×1100、移动 390×844。完整流程覆盖通用模板追问、粘贴/导入素材、生成后编辑、无效输出保留、附件预览、保存并跳转详情、下载 zip、目录 Skill 筛选，以及女娲模板生成和移动端保存。无页面运行时错误、移动端无横向溢出。下载 zip 已另行检查文件名、自动文件头及人工补充的附件内容；临时下载包检查后删除。

- [初始创建页](desktop-start.png)
- [草案编辑与附件](desktop-editor.png)
- [生成失败保留修改](desktop-error-preserved.png)
- [保存后的注册中心详情](saved-detail.png)
- [女娲方法桌面页](desktop-nuwa.png)
- [女娲方法移动页](mobile-nuwa.png)
- [浏览器检查摘要](browser-result.json)

云端测试入口：`backend/internal/http/registry_integration_test.go` 调用 `verifySkillBuilderIntegration`（`skill_builder_integration_test.go`）。可选 `REGISTRY_BROWSER_CHECK=1` 开启 localhost:18089 测试 fixture，配合 API 地址为该端口的 localhost:13000 前端构建，访问 `/__test/session`；完成后 POST `/__test/finish` 触发清理。这些测试路由不会进入生产 server。
