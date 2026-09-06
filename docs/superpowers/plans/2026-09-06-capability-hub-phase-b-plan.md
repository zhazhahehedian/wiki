# Capability Hub · 阶段 B 注册中心核心

依据 spec §6、§11、§13；用户于 2026-09-06 授权开始。保留现有未提交改动，不提交/推送；公司模型联调继续暂缓。

## 实施边界

- 阶段 B 完成 Owner 草稿闭环。普通用户暂时只访问自己的条目；可见范围作为发布配置保存，跨用户发现与 Admin 覆盖在 C/D 实施后开放，草稿始终不公开。
- 新增 capabilities、capability_versions、capability_allowlist。Owner 从 session users.id 关联 feishu OAuth open_id，客户端不能指定。slug/type 创建后固定。
- revision 乐观锁串行保护条目、版本与名单保存。草稿当前编辑版本由 draft_version_id 指向，current_version_id 留给 C 原子审核上线；同名版本只允许编辑当前草稿，历史版本不覆盖。新增版本标签必须唯一。
- MCP 只保存 endpoint、streamable-http、鉴权方式与声明工具，不探测、不代理、不接收凭证。
- Skill 表单上传 SKILL.md 和附件，服务端校验相对路径、数量/大小和 Markdown，生成 zip 放 MinIO；下载受 Owner 检查，附件不在线执行。本阶段不解析 Claude 专用 frontmatter，manifest 来自表单名称/描述；格式完整验收随 C 的审核/导出完善。
- REST `/api/v1/capabilities` 提供 Owner 列表/详情、创建、编辑和版本附件下载；不是 D 的跨用户 discovery API。写操作复用 session/Origin/CSRF。

## 工作项

- [x] 0014 迁移、sqlc 查询与生成、事务仓储。
- [x] registry 校验、Skill 打包/存储、HTTP 与生产装配。
- [x] 注册中心卡片/表格、筛选分页、创建编辑、详情版本、“我的发布”。
- [x] 数据库隔离、并发冲突/回滚、附件与 HTTP 权限回归；前端测试/typecheck/build、页面检查。
- [x] 最终 diff 与进度记录；区分实现、自动化检查和真实运行验收。

## 验证与接续

已通过后端全量测试、server/migrate 构建、云端独立数据库 0014 up/down/up 和真实 MinIO 上传/下载/清理。自动化覆盖 Owner/CSRF、并发 revision、事务回滚、版本历史、筛选分页、附件安全与不确定提交清理保护。Edge 实操完成 MCP 创建编辑/版本切换、Skill 上传下载、列表切换搜索及移动端我的发布。

前端全量 33 个测试文件、137 项测试、类型检查与生产构建通过；原工作区 Windows 依赖缺少 WSL 原生绑定，采用独立 Linux 验证副本及已有缓存，未改包版本或 lockfile。预填文本框标签关联问题经浏览器发现、修复并加回归。截图与详细边界见 [验证记录](../../verification/2026-09-06-phase-b/README.md)。

Skill 文件夹上传会去除所选根目录，保留内部路径；新版本必须提交完整文件包。保存前上传与数据库事务之间无法跨系统原子提交：确定未引用的对象会清理；提交结果不确定或数据库无法确认时保留对象并记录清理待办日志，后续恢复需先核实版本引用，不能盲删。未增加后台业务作业或改动 River 重试策略。

未改现有开发库、未重启既有 8088 服务、未提交或推送。下次开发运行通过项目迁移入口升级至 0014 后使用新 server。阶段 C–E 保持待实施，公司模型联调继续暂缓。

追加功能：用户随后授权的 Skill 对话创建已按独立 [Skill 引导创建计划](2026-09-06-skill-builder-plan.md) 实施，复用本阶段 Owner 草稿和附件包保存，不提前开放治理。
