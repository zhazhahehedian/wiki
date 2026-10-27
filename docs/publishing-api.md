# MCP / Skill 发布 API

站内入口：`/hub/api-docs`（接口文档、完整调用示例、本人凭证创建/列出/撤销）。飞书后台配置见 [目录授权步骤](feishu-directory-setup.md)。本接口不替代知识检索应用凭证，也不实现机器 discovery 或 MCP 运行凭证保险箱。

## 凭证

网页登录后创建凭证，服务端绑定当前用户的飞书 OAuth 账号，不能指定其他账号。名称必填，有效期必填、最长 365 天；UI 默认 30 天。完整随机密钥 `chp_…` 只在创建响应返回一次，数据库只存 SHA-256 摘要、非敏感前缀和元信息。将其保存在 CI secret / 环境变量，不能写入仓库。

权限可分别选择 `capabilities:read`、`capabilities:write`、`capabilities:submit`；凭证不会扩大账号现有 Owner/Admin 或可见范围权限。管理凭证的接口只接受登录 Session，并保留 Origin/CSRF：

| 方法 | 路径 | 请求 |
|---|---|---|
| GET | `/api/v1/publishing/credentials` | 返回本人凭证列表，无完整密钥或摘要 |
| POST | `/api/v1/publishing/credentials` | `{"name":"CI","scopes":["capabilities:write","capabilities:submit"],"expires_at":"<未来 RFC3339 时间>"}` |
| DELETE | `/api/v1/publishing/credentials/{id}` | 仅能撤销本人凭证；成功 204 |

每次 API 请求查询数据库验证摘要、过期与撤销状态；撤销对之后的新请求立即生效，已通过认证的进行中请求可能完成。能力变更审计会附 `api_credential_id`，凭证创建/撤销也有审计。

## 能力接口

以下统一使用 `Authorization: Bearer $TOKEN`，浏览器 Cookie 不能代替。前缀为 `/api/v1/open/capabilities`。

| 方法 / 相对路径 | 权限 | 行为 |
|---|---|---|
| GET `/` | read | 列出本人能力；q、type、status、department、limit（1–100，默认24）、offset |
| GET `/{slug}` | read | 按账号和上线可见范围读取详情；Owner/Admin 可读管理视图 |
| GET `/{slug}/versions/{id}/bundle` | read | 下载授权可见的 Skill ZIP |
| POST `/` | write | 创建草稿，201 |
| PUT `/{slug}` | write | Owner 完整更新草稿，200；提供最新 revision |
| POST `/{slug}/new-draft` | write | Owner 对已发布/下线能力开新草稿，200 |
| POST `/{slug}/submit` | submit | Owner 提交指定候选进入 in_review，200 |

没有 API 审批、强制上线、角色修改或凭证管理通道。管理员仍在站内审核，沿用现有原子上线和审计。审查中的草稿不能编辑；发布新版本不会提前替换当前线上版本。

## 注册 MCP

```bash
export BASE="https://hub.example.com"
# TOKEN 由安全环境变量注入
curl --fail-with-body "$BASE/api/v1/open/capabilities" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  --data '{"slug":"team-search","type":"mcp","name":"团队搜索",
    "description":"搜索团队内容","visibility":"org","version":"1.0.0",
    "mcp_endpoint":"https://mcp.example.com/mcp","mcp_transport":"streamable-http",
    "mcp_auth_scheme":"bearer","tools":[]}'
```

支持 `mcp_auth_scheme=none/bearer`。Endpoint 不得包含凭证、查询参数或片段。工具项包含 `name`、`description`、`inputSchema`（对象 schema）；注册只记录规范，不在线调用 MCP。

保存返回详情，包括 `slug`、`status`、`revision`、`draft_version_id` 和 `versions`。创建 revision 传 0 或省略；更新不可省略。字段 `department_id` 是飞书 `open_department_id`，`department` 由服务端查证为显示名。`visibility=department` 必填部门 ID；`allowlist` 必须包含非空的 `open_id` 数组且仅用于 `visibility=allowlist`，最多200人。所选 ID 必须在绑定账号可见范围内，并已完成通讯录授权。

## 上传 Skill

保存 `metadata.json`：

```json
{"slug":"report-skill","type":"skill","name":"报告助手","description":"整理报告","visibility":"org","version":"1.0.0"}
```

`SKILL.md` 需有 YAML 文件头 `name: report-skill`（与 slug 一致，最长64）、`description: 整理报告`（最长1024）和非空正文。附件引用必须包含在完整包中。

```bash
curl --fail-with-body "$BASE/api/v1/open/capabilities" \
  -H "Authorization: Bearer $TOKEN" \
  -F 'metadata=<metadata.json;type=application/json' \
  -F 'files=@SKILL.md;filename=SKILL.md' \
  -F 'files=@template.md;filename=references/template.md'
```

上传的是多个源文件（不是 ZIP），最多50文件、总计10 MiB。`filename` 保留相对路径；拒绝目录穿越、重复和不安全路径。每次有文件的上传替换完整草稿包。仅更新同一未发布版本的元数据时可以省略 files，新版本必须上传完整包。

## 更新和提交

更新草稿：PUT `/{slug}`，使用完整元数据、最新 `revision`，MCP 支持 JSON / multipart，Skill 上传使用相同 multipart 格式。

已发布/下线：先 GET 最新详情，再 POST `/{slug}/new-draft`，传 `{"revision":<实际值>,"version_id":"<current_version_id>"}`；随后 PUT 完整内容与从响应得到的新 revision、未发布的 version。

提交请求：

```bash
curl --fail-with-body "$BASE/api/v1/open/capabilities/team-search/submit" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  --data '{"revision":1,"version_id":"<保存响应的 draft_version_id>","reason":"提交首版"}'
```

示例 revision 仅为首版示意，不能硬编码到更新流程。成功只进入待审，不能宣称已上线。

## 错误与重试

错误格式 `{"error":{"code":"...","message":"...","request_id":"..."}}`。400 无效字段/文件/Skill，401 凭证缺失、无效、过期或撤销，403 scope/账号角色/飞书目录权限不足，404 不存在或不可见，409 revision/状态/版本冲突，502 飞书目录暂不可用。

401 更新/重新创建凭证；403 检查选定 scope 和账号权限，目录错误走重新授权；409 读回最新详情，确认差异再操作。网络中断先 GET 检查请求是否已成功，不能盲目重复创建或重复提交。不支持覆盖已发布版本或通过客户端字段伪造 Owner。
