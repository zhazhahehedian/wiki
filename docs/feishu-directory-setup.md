# 飞书部门与人员选择：权限与重新授权

对应实现：[目录与发布 API 设计](superpowers/specs/2026-10-27-directory-and-publishing-api.md)。已完成模拟上游和隔离数据库验证；2026-10-27 经用户后续授权部署至正式站点，管理员新增权限已由用户确认发布，见[部署记录](verification/2026-10-27-directory-api-release/README.md)。每个旧授权账号仍需完成以下重新授权步骤。

## 1. 新增用户身份权限

在飞书开放平台进入当前 Capability Hub 自建应用 → 开发配置 → 权限管理，选择 **用户身份权限**，开通：

| scope | 控制台名称 / 用途 |
|---|---|
| `contact:contact.base:readonly` | 获取通讯录基本信息；允许查询可见部门、直属人员及单个用户 |
| `contact:department.base:readonly` | 获取部门基础信息；返回部门 ID、名称 |
| `contact:user.department:readonly` | 获取用户组织架构信息；读取本人真实 `department_ids` |

继续保留现有 `offline_access`、`contact:user.base:readonly`、`contact:user.email:readonly`。本功能不需要手机号、工号、雇佣信息、全员应用身份通讯录权限，也不需要部门路径权限。平台 OAuth 发起时会请求新增 scope，但登录回调的基础登录要求保持原有三项；目录未授权单独返回 `directory_authorization_required`。

可批量导入以下用户权限（应与应用原有权限合并，不能覆盖删除其他业务已需权限）：

```json
{
  "scopes": {
    "user": [
      "offline_access",
      "contact:user.base:readonly",
      "contact:user.email:readonly",
      "contact:contact.base:readonly",
      "contact:department.base:readonly",
      "contact:user.department:readonly"
    ]
  }
}
```

在「版本管理与发布」创建应用版本，说明“增加用户可见组织和人员选择、本人部门归属校验”，提交企业管理员审批并发布。确认需要使用平台的人员包含在应用可用范围内。若申请流程提示数据权限，按照企业治理要求批准；本实现仍只使用用户身份令牌查询，不会以应用全员范围扩大个人可见范围。

## 2. 核对组织架构可见性

企业管理员在飞书管理后台 → 安全 → 成员权限 → 组织架构可见范围，检查试用账号实际可见的部门和人员。只按业务需要调整。两个同名部门以不同 `open_department_id` 区分。

「应用可用范围」「应用通讯录数据范围」「个人组织架构可见范围」「员工实际归属部门」不是同一个概念。本实现的选择候选由个人可见范围过滤，部门访问授权来自本人的 `department_ids`；不会因能看见研发部就认定该用户属于研发部。

部分企业禁止用户令牌读取组织根节点 `0`（例如返回 `40004`），但允许读取本人部门。选择器此时会明确提示，从本人真实部门中逐个经用户令牌确认可访问的部门进入，再逐级查询下级和直属人员。这个入口不代表完整组织清单，也不把浏览结果写入部门归属。无需为了使用本人部门而扩大应用身份权限；若需要更多组织可见范围，由企业管理员按业务要求调整个人可见性。

## 3. 用户重新授权

1. 确认后台新增权限版本与支持目录功能的代码已发布。当前站点于 2026-10-27 完成此步骤。OAuth 回调地址继续使用现有配置，无须因本功能改域名。
2. 用户打开站内能力编辑或知识库编辑中的“选择飞书部门 / 选择飞书用户”。如提示授权不足，点“重新授权飞书”；也可退出平台后重新点飞书登录。入口为后端 `/api/v1/auth/feishu/start`，不要手工拼接 token 或绕过 state。
3. 在飞书授权页确认新增通讯录权限，完成回调。旧 access/refresh token 的刷新不会自动扩大已授予的 scope，必须走一次用户授权。
4. 重新打开选择器，检查部门分页、下级导航、直属人员与个人飞书中可见范围一致；在设置页检查本人飞书部门 ID。
5. 若仍报权限不足，核对“用户身份”下已开通且已发布的权限、应用可用范围及个人可见范围。若只是某个部门不可见，重新授权不会覆盖企业可见性策略，应联系管理员核实该策略。

## 4. 权限验收清单

- 账号 A 可看见 B 部门但不属于 B：能选择 B 为能力可见部门；A 除了 Owner/Admin 特权，不能因此读到别人的 B 部门能力。
- 账号 B 确实属于所选部门：能发现并查看已审核上线的该部门能力；同名不同 ID 的部门不互通。
- 人员选择保存 `open_id`，部门保存 `open_department_id`；显示名由服务端查证。伪造不可见 ID 的请求应被拒绝。
- 未授权、scope 减少、刷新失败、本人归属字段缺失均不能获得新的部门 ID 授权。已有有效归属快照最多缓存五分钟，过期查询失败后不使用旧快照；角色和旧名称部门规则保持原有服务端校验。
- 已有字符串部门不自动转换或猜测 ID，原记录保留。编辑保存时重新选择飞书部门，或在不需要部门限制时清除部门；管理员的原“部门授权”页仅维护历史名称规则，不赋予新的飞书 ID 权限。

## 官方依据

- [获取子部门列表](https://open.feishu.cn/document/server-docs/contact-v3/department/children)：用户令牌遵守个人组织架构可见范围。实现逐级分页，`fetch_child=false`。
- [获取部门直属用户列表](https://open.feishu.cn/document/server-docs/contact-v3/user/find_by_department)：人员列表受用户可见范围过滤。
- [获取单个用户信息](https://open.feishu.cn/document/server-docs/contact-v3/user/get)：通过 `user_id_type=open_id`、`department_id_type=open_department_id` 查询本人真实部门。
