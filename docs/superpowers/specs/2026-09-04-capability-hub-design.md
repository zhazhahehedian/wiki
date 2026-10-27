# 能力中心(Capability Hub)设计 · Design Spec

- 日期:2026-09-04
- 状态:草案(待 review)
- 当前方向（2026-09-19 D12 更新）：能力市场＋知识服务平台。保留 MCP/Skill 注册治理，新增面向授权业务应用的知识库管理与检索 API。原 it-wiki 的聊天/Agent 产品层继续退出生产；文档摄入与检索基础按[知识服务设计](2026-09-19-knowledge-service-design.md)恢复和改造，不执行原知识数据退役计划。

---

## 1. 背景与方向转变

### 1.1 初次转向的历史背景（2026-09-04）

原 it-wiki 目标是团队内部知识库对话 Agent。实际判断:

- **知识库对内价值被飞书覆盖**:企业文档、知识库、多维表格用飞书原生能力更顺;个人知识图谱可用 Obsidian + AI。再做一套 KB 聊天产品边际价值低。
- **公司已有 LLM 分发底座**:内部平台 **Token Hub**(`tokenhub.robosense.cn`,new-api 改造,OpenAI 兼容,已接飞书 SSO)已经解决了模型 key 分发、部门/额度/计费、用量日志。聊天/游乐场也是它天然能力。
- **真正缺的是能力(MCP / Skill)的注册、治理与分发**:开发者写了内部 MCP / Skill,没有一个受治理的地方发布出来供其他系统发现和调用;而这正是 2026 年 agent 生态的竞争焦点(Skill/MCP 生态 + 安全治理)。

### 1.2 转向后的定位

**能力中心 = 内部能力市场与治理分发 + 知识服务。** 用户于 2026-09-19 确认新增外部应用检索用途（D12）。它复用两块既有基础设施:

- 上游接 **飞书**(身份 + 文档能力来源)
- 下游接 **Token Hub**(模型 key + 计费,供游乐场使用)

平台提供目录、治理、发现、凭证分发和薄游乐场，并新增知识库管理、文档索引与授权检索 API；外部应用负责聊天、会话和回答生成。平台仍不代理第三方 MCP 的运行时调用，不在线执行 Skill。

---

## 2. 目标与非目标

### 2.1 v1 目标

1. 开发者能把**内部 MCP server**(远程 Streamable HTTP)**注册/发布**到平台,带描述、工具清单、版本、可见范围。
2. 开发者能把 **Skill**(SKILL.md + 附件包)发布到平台,可导出为 Claude Skills zip。
3. 平台提供**治理**:草稿 → 提交审核 → 管理员审核 → 上线/下线;版本管理;审计日志。
4. 使用方(人)能**发现**有权限看到的能力,并**获取接入凭证**(受权限门禁 + 审计)后直连目标 MCP。
5. 其他系统能通过**只读 discovery API** 程序化发现有权访问的能力元信息。
6. 一个**游乐场**:网页端对话,用户配置 API 基础地址、API Key 和模型 ID,经后端调用 Token Hub 或其他 OpenAI 兼容接口,测模型。
7. **知识服务（D12，待实现）**：知识库/文档管理、异步索引、检索测试和面向授权应用的检索 API；应用凭证、显式库授权、引用、更新/删除一致性与审计。具体分期和验收见 [K1 设计](2026-09-19-knowledge-service-design.md)。

### 2.2 非目标(v1 明确不做,见 §13)

- ❌ 运行时网关 / 代理(平台**不进 MCP 调用链路**)
- ❌ broker / 委托身份鉴权(v1 用共享凭证保险箱,见 §8)
- ❌ Skill 在线执行 / sandbox 运行
- Skill 自然语言**生成引导**的轻量版本已于 2026-09-06 获用户授权提前实施（D10）；完整联网研究/自动执行仍不属于 v1。
- ❌ 对外部合作方/客户输出能力(商业化方向,更远)
- ❌ 按部门配置审批人(v1 仅平台管理员审批)
- ❌ 游乐场会话历史持久化(v1 对话临时,不落库)
- ❌ 修改 Token Hub 代码(它由他人维护,零 fork)
- ❌ 本平台完整知识聊天产品、会话管理及回答 Agent；K1 仅交付知识管理和检索，外部业务系统构建对话。

---

## 3. 整体架构

```
┌──────────────────────────────────────────────────────┐
│  Token Hub (new-api fork · 他人维护 · 我们不动)         │
│  飞书SSO · sk- key · 模型目录 · 部门/计费/用量           │
│  OpenAI 兼容: /v1/chat/completions                     │
└──────────────▲────────────────────▲───────────────────┘
    身份(飞书 open_id 同源关联)         │ 模型调用(带用户 sk- key)
               │                     │
┌──────────────┴─────────────────────┴──────────────────┐
│  能力中心 Capability Hub(本项目,改造自 it-wiki)          │
│  Go chi/sqlc/pg · river · MinIO · Next.js/shadcn        │
│  自有飞书 OAuth(复用 it-wiki)→ 同一个 open_id            │
│                                                        │
│  ① 游乐场   ──(反向代理,带用户 sk- key)──▶ Token Hub     │
│  ② 注册中心 + 治理 + 发现 + 凭证保险箱  ★核心             │
│     capabilities / versions / credentials / audit       │
└───────────────────────────┬────────────────────────────┘
        发现 + 获取接入凭证(权限+审计) │  (平台不在 MCP 调用链路上)
                              ▼
   其他系统 / agent ───直连───▶ 各业务自建的 MCP server
   (飞书 Aily / Claude / Coze / 内部 bot / CI …)
```

关键点:

- 能力中心与 Token Hub 是**两个独立服务**,只通过「同源飞书 open_id」和「用户自带 sk- key」松耦合,不依赖 Token Hub 私有接口。
- MCP 使用方拿到 endpoint + 凭证后直连 MCP server，平台不转发调用。知识检索则由本平台内置服务处理，外部应用通过独立应用凭证调用；上述历史架构图的知识服务扩展见 D12 设计。

---

## 4. 术语

| 术语 | 含义 |
|---|---|
| **能力 Capability** | 平台注册的一个可复用单元,类型为 `mcp` 或 `skill` |
| **MCP 条目** | 一个远程 Streamable HTTP 的内部 MCP server 的注册记录 |
| **Skill 条目** | 一个 SKILL.md + 附件包(Claude Skills 格式)的注册记录 |
| **版本 Version** | 能力的一个具体版本,含该版本的规格(endpoint/工具清单 或 SKILL.md/附件) |
| **Owner** | 能力的发布/维护者(飞书 open_id) |
| **Admin** | 平台管理员,负责审核、上下线、治理 |
| **消费方 Consumer** | 使用能力的人或系统(拿凭证直连 MCP,或使用 Skill) |
| **接入凭证 Credential** | MCP 条目的共享访问凭证(加密存储,受权限门禁 + 审计后发放) |

---

## 5. 角色与权限

| 角色 | 能力 |
|---|---|
| **登录用户**(任意飞书用户) | 浏览有权见到的能力;获取有权访问能力的接入凭证;使用游乐场 |
| **Owner** | 创建/编辑/发新版本自己的能力;提交审核;设置可见范围 |
| **Admin** | 审核通过/驳回;强制上线/下线;查看全部审计;可见范围兜底覆盖 |

- Bootstrap admin 通过 env `BOOTSTRAP_ADMIN_FEISHU_OPEN_ID` 指定(参考 it-wiki 的 bootstrap owner 模式;只补空,不覆盖)。
- 身份来自平台自有飞书 OAuth(复用 it-wiki 的 `internal/auth`),得到 `open_id`;与 Token Hub 用户天然同源(同一飞书租户)。

---

## 6. 数据模型

遵循 [AGENTS.md](../../../AGENTS.md) 的数据库规约：schema 变更新增 goose migration；查询按需修改 sqlc queries，sqlc 输入变化时重新生成并调整受影响消费者。纯查询变更不要求迁移，迁移不要求无关 service 改动。新平台查询沿用 sqlc，不向 service 内散落裸 SQL。D12 恢复知识索引用途，保留 chunks/embedding/pgvector 并核实维度及旧数据归属；停止原表退役计划，不以重置数据库实现权限改造。

### 6.1 `capabilities`

| 列 | 类型 | 说明 |
|---|---|---|
| id | uuid pk | |
| slug | text unique | URL 友好唯一标识 |
| type | text | `mcp` \| `skill` |
| name | text | 展示名 |
| description | text | 简介 |
| owner_open_id | text | 负责人飞书 open_id |
| department | text | 负责部门名称快照（展示/筛选；历史未关联 ID 记录仍按原名称规则授权） |
| department_id | text | 飞书 open_department_id；新部门关联按 ID 授权，见 2026-10-27 增补 |
| status | text | `draft` \| `in_review` \| `published` \| `offline` |
| is_live | boolean | 消费者上线标记；新版 draft/in_review 时可仍为 true |
| published_metadata | jsonb null | 已审核生效的名称/描述/部门/可见范围/名单快照 |
| review_reason | text | 最近驳回或下线说明，仅管理视图可见 |
| current_version_id | uuid null | 当前上线版本；与 capability_id 联合外键防止跨能力引用 |
| draft_version_id | uuid null | 当前编辑版本；与上线指针分离，Owner 编辑不会切换上线版本 |
| revision | bigint | 从 1 开始的乐观锁；条目、版本和名单在同一事务保存，旧 revision 返回冲突 |
| visibility | text | `department` \| `allowlist` \| `org`(全公司可见) |
| created_at / updated_at | timestamptz | |

### 6.2 `capability_versions`

公共列 + 按类型可空列(避免多态表,简化 sqlc)。

| 列 | 类型 | 说明 |
|---|---|---|
| id | uuid pk | |
| capability_id | uuid fk | ON DELETE CASCADE |
| version | text | 语义版本或递增号 |
| changelog | text | |
| created_by | text | open_id |
| created_at | timestamptz | |
| published_at | timestamptz null | 首次上线时间；非空版本不可覆写 |
| **— MCP —** | | |
| mcp_endpoint | text null | Streamable HTTP endpoint |
| mcp_transport | text null | 默认 `streamable-http` |
| mcp_auth_scheme | text null | 如 `bearer` |
| tools | jsonb null | 工具清单(名称/描述/入参 schema),作者声明或 introspect 回填 |
| **— Skill —** | | |
| skill_bundle_key | text null | MinIO 对象 key(SKILL.md + 附件打包) |
| skill_manifest | jsonb null | name/description/when-to-use 等元信息 |

阶段 B 实施约束：slug/type 创建后固定（`new`、`create-skill` 为页面保留标识）；`(capability_id, version)` 唯一。同名版本只能编辑当前草稿，新版本标签创建新记录，历史记录保留。B 暂仅开放 Owner 自己的列表/详情/附件与草稿写入，可见范围配置不提前开放跨用户读取。见[阶段 B 计划](../plans/2026-09-06-capability-hub-phase-b-plan.md)。

### 6.3 `capability_credentials`(仅 MCP)

| 列 | 类型 | 说明 |
|---|---|---|
| id | uuid pk | |
| capability_id | uuid fk unique | 一个能力一份活跃凭证 |
| ciphertext / nonce | bytea | 加密后的凭证(复用 it-wiki 的 `OAUTH_ENCRYPTION_KEY` 加密实现) |
| rotated_at | timestamptz null | 轮换时间 |
| created_at | timestamptz | |

### 6.4 权限:`capability_allowlist` / 部门可见

- `visibility=org`:全公司登录用户可见/可取凭证。
- `visibility=department`：新关联以 `department_id` 与当前用户真实飞书部门 ID 匹配；组织架构可见范围只用于选择候选，不能当成部门归属。本人归属使用已有 OAuth 加密/刷新机制，缓存最多五分钟，过期或查询失败时不使用旧快照。历史空 `department_id` 记录保留 Admin 维护的 `platform_profiles.department` 规则，不自动按同名映射。权限和重新授权细节见 [2026-10-27 增补](2026-10-27-directory-and-publishing-api.md)。
- `visibility=allowlist`:`capability_allowlist(capability_id, open_id)` 明确名单。
- 账号绑定发布 API 复用 Owner/Admin 与以上审核规则；凭证按 read/write/submit 限权并强制到期，可撤销，摘要存储。提交只进入审核，不提供 API 直接上线。详见 [发布 API](../../publishing-api.md)。
- Admin 始终可见/可管。

### 6.5 `credential_grants`(审计:谁取了凭证)

| 列 | 类型 | 说明 |
|---|---|---|
| id | uuid pk | |
| capability_id | uuid fk | |
| open_id | text | 取凭证的人 |
| granted_at | timestamptz | |
| request_id | text | 关联日志 |

### 6.6 `audit_log`(通用治理审计)

| 列 | 类型 | 说明 |
|---|---|---|
| id | uuid pk | |
| actor_open_id | text | |
| action | text | `create`/`submit`/`approve`/`reject`/`publish`/`offline`/`rotate_credential`/`grant_credential` … |
| target_type / target_id | text | |
| detail | jsonb | |
| request_id | text | |
| created_at | timestamptz | |

### 6.7 `user_llm_keys`(游乐场)

| 列 | 类型 | 说明 |
|---|---|---|
| user_id | uuid pk references users(id) | 平台用户身份隔离，随用户删除 |
| protocol | text | openai（默认）或 anthropic；明确选择协议，不根据域名推测 |
| base_url | text | OpenAI 兼容基础路径，默认 Token Hub `https://tokenhub.robosense.cn/v1`；Anthropic 原生接口保存根地址，不含末尾 `/v1` |
| key_ciphertext | bytea | AES-GCM 密文包含 nonce，载荷绑定 user_id、base_url 与 protocol；不强制 Key 前缀 |
| models | text[] | 1–100 个选定或手动输入的模型 ID；不依赖 Token Hub 私有模型目录 |
| default_model | text | 必须属于 models |
| version | bigint | 乐观锁，防止旧页面覆盖新配置 |
| created_at / updated_at | timestamptz | |

---

## 7. 治理流程

```
draft ──(owner 提交)──▶ in_review ──(admin 通过)──▶ published
  ▲                        │                          │
  └──────(admin 驳回)───────┘        (admin/owner 下线)─┴──▶ offline ──(重新提交)──▶ in_review
```

- Owner 创建能力/新版本 → `draft`;可反复编辑。
- 提交审核 → `in_review`,进入 admin 审核队列。
- Admin 通过 → `published`,设置 `current_version_id`;驳回 → 回 `draft`(带驳回理由,记 audit)。
- 下线 → `offline`(能力仍在,但不对普通用户可见/不可取凭证)。
- 每个状态跃迁写 `audit_log`。
- 新版本:发新版本走同样审核;上线新版本原子切换 `current_version_id`(参考 it-wiki 的 atomic promotion 思路)。

---

### 7.1 阶段 C 实施契约（2026-09-06）

- `status` 表示当前维护流程，`is_live` 独立表示对消费者上线；`published_metadata` 保存已审核的名称、描述、部门、可见范围与名单快照。Owner 发新版显式进入 `new-draft`，随后编辑/提交/驳回均保留旧版上线。审批一次事务切换版本指针与快照；消费者永远只读上线快照与当前版本，不能读草稿或旧版本附件。Owner/Admin 管理详情可查看历史和待审内容。
- `capability_versions.published_at` 标识已上线过的不可变版本；同名已上线版本永不覆写。初次发布和每次编辑保留 revision 乐观锁；所有治理动作带 `revision` 与 `version_id`，事务内锁定能力行后重新核对。提交后不可编辑；重复或过期动作返回 409。
- Owner 提交、创建新草稿；Admin 通过/驳回（驳回必须填理由）。v1 允许 Admin 审核自己的能力，使用相同校验且留下 actor 审计，不声称实现双人审批。Admin 可带理由强制发布 draft/in_review/offline 的当前候选，仍校验完整包格式，不跳过原子发布。Owner/Admin 下线须说明理由，同时停止旧版可见并结束当前审核；offline 可重新提交当前候选，无草稿时重提原上线版本。重新编辑已上线版本必须使用新版本号。
- Admin 可带理由覆盖当前上线快照的可见范围；审核中拒绝覆盖，避免审核对象漂移。覆盖与审计原子提交并递增 revision，Owner 可在管理详情看到生效快照；没有独立新版草稿时同步维护配置，防止下线重提撤销覆盖；存在独立草稿时，其后续审核仍以该次提交配置为准。
- `platform_profiles(open_id,is_admin,department,revision)` 保存服务端角色与可信部门。Admin 页面只维护已登录用户的部门，不提供用户自授角色入口。Bootstrap 由 `BOOTSTRAP_ADMIN_FEISHU_OPEN_ID` 指定，用持久单例记录消费并串行执行：仅首次且无 Admin 时补角色，可在目标首次登录前预配；已有 Admin 不覆盖，后续改 env 不转移角色。角色不绑定浏览器输入；取消 env 不撤销已授角色。角色撤销通过受控运维修改持久 profile，已消费 bootstrap 不会恢复撤销的角色；未来角色管理 UI 独立实施。
- 手动上传与创建助手共用发布格式检查：包目录名与 `SKILL.md` frontmatter 的 name 相同（最长 64），description 为非空字符串（最长 1024 字），必须有正文；所有 Markdown 包内相对文件引用必须存在。允许携带脚本但平台仅存储/展示/下载，不执行或加载远程内容。旧草稿仍可保存修复，提交和批准必须通过校验；历史 Skill 标识超过 64 字符时，因标识不可变，需要用合规标识重新创建。
- 从 C 起 create/edit（含 Skill 创建助手）与所有治理写入都在同一事务写 `audit_log`；记录 actor、动作、目标、版本/revision、状态变化、理由、request_id 和时间，不复制能力正文、工具 schema、完整名单、模型素材或凭证。历史 B 操作不补造审计。审核/审计/部门资料 API 仅 Admin 可读写；普通消费者详情不包含名单和治理审计。
- Web 目录使用 `/api/v1/catalog` 与上线快照详情；`/api/v1/capabilities` 保留 Owner 管理列表，详情按 Owner/Admin 管理权限或已发布访问权限投影。两种列表使用不同前端缓存键。机器 discovery 与凭证 API 仍属 D。

---

## 8. 接入凭证模型(共享凭证保险箱)

**决策(D5):v1 采用共享凭证保险箱。**

- 平台为每个 MCP 能力存**一份加密的共享访问凭证**(`capability_credentials`,复用 it-wiki 的 `OAUTH_ENCRYPTION_KEY` 加解密)。
- 使用方在详情页点「获取接入凭证」→ 平台校验其对该能力的权限(部门/白名单/org)→ 返回 `endpoint + 凭证`,并写一条 `credential_grants` + `audit_log`。
- 使用方拿凭证**直连** MCP server;**鉴权由 server 端认这个 Bearer token**,平台不进调用链路、不做每次调用鉴权。
- 支持凭证**轮换**(`rotate`,记 audit)。

**已知弱点与缓解**:共享凭证泄露即广域暴露、难按人吊销/追踪。缓解:加密存储、获取门禁 + 审计、支持轮换;并在 §13 标注 V1.5 评估升级到「委托身份」或「短时令牌 broker」。

**MCP 规范上下文**:MCP `2026-07-28` 正式规范已转无状态、鉴权硬化(RFC 9207 / CIMD)。因平台只存元数据 + endpoint、不进调用链路,受规范演进影响面小;健康检查用规范无关的轻量探测(§10)。

---

## 9. 发现(Discovery)

- **Web 目录**:登录后按权限过滤展示(§11 UI)。
- **只读 Discovery API**(供其他系统程序化发现):
  - `GET /api/v1/registry/capabilities?type=&department=&status=published` → 仅返回调用方**有权见到**的条目**元信息**(不含凭证)。
  - `GET /api/v1/registry/capabilities/{slug}` → 单条元信息 + 当前版本规格(工具清单等)。
  - 凭证获取是**独立**的、需鉴权 + 审计的端点:`POST /api/v1/registry/capabilities/{slug}/credential`。
- Discovery API 的调用方身份:v1 复用平台 session(浏览器)或后续为「系统对系统」发平台专用 token(V1.5,见 §13)。

---

## 10. 健康检查

- 新增 river job:周期性对 `published` 的 MCP endpoint 做**轻量探测**(HTTP 可达 / 初始化握手),回填 `capability_versions` 或独立健康表的健康状态与最近检测时间。
- 与 it-wiki 的「飞书 reconciler」不同:这里探测的是**已注册 MCP 的存活**,不轮询任何远端版本。
- 遵循项目异步规约:走 river,不起裸 goroutine;失败默认不自动重试(如需重试在代码注释写明理由)。

---

## 11. 前端(Next.js 15 + Tailwind v4 + shadcn/ui)

### 11.1 视觉风格(D6)

- 方向 **B · 明亮清爽 SaaS**,对齐 **micuapi / 新版 new-api**(现代 React/Next.js 风格 admin dashboard)。
- 基调（2026-09-06 更新）:**白色侧栏 + 白灰内容 + 低饱和蓝主操作（`#486484`）**，细边框、紧凑圆角。依据用户提供的米醋 API 游乐场截图，替代阶段 A 的深色侧栏、浅蓝渐变与装饰光晕；保留既有导航和卡片/表格切换。
- 状态色:上线=绿、待审核=琥珀、草稿/下线=灰。

### 11.2 导航(左侧栏)

`仪表盘 · 能力市场 · 游乐场 · 我的发布 · 审核队列(admin) · 审计日志(admin) · 设置`

K1 计划新增「知识库」入口及 Admin「应用接入」管理；知识库内提供文档处理状态、版本和检索测试，应用接入提供凭证与知识库授权。此为待实现导航，不代表当前页面已上线。

### 11.3 关键页面

- **能力市场（2026-09-19）**：沿用注册中心 `/hub/registry`，默认卡片网格，可切换表格。按类型、部门、名称/用途搜索，场景快捷入口使用全目录关键词搜索并重置分页。只展示授权上线快照，卡片展示真实版本及发布者，不虚构排行、热度或精选标签；状态筛选仅在「我的发布」管理页提供。
- **市场详情与 Skill 复用（2026-09-19）**：独立 `/hub/registry/{slug}/market` 通过 `/api/v1/catalog/{slug}` 读取当前上线版本，Owner/Admin 同样不混入管理草稿。提供正文和附件预览、包下载及通用安装指引，Markdown 不执行 HTML、不加载远程图片。`GET /api/v1/catalog/{slug}/template?version_id=...` 重新校验发布可见性并固定版本，变更返回 409，不可见或下线返回 404；读取来源不落盘新草稿、不调用模型。完整文本包进入创建助手，由用户编辑或对话修改后确认保存为自己的独立草稿；生成新标识，不复制来源所有权/权限、不修改原版。若存在脚本、二进制、额外 frontmatter 或超出生成器限制的内容，返回 422 并提示下载原包，禁止静默删减。外部市场、评分推荐和在线执行留待后续。
- **能力详情(两栏)**:
  - 左主栏:描述、**工具清单**(每个 tool 名称 + 说明 + 入参)、版本历史。
  - 右栏:**接入信息**(endpoint / 传输 / 鉴权 / 「获取接入凭证」)、可见范围(部门 + 白名单)、健康 / 元信息(负责人 / 更新时间 / 仓库)。
- **发布 / 编辑**(标准表单/向导):基本信息(名称/类型/部门/描述)→ 类型专属(MCP:endpoint/传输/鉴权/工具清单;Skill:上传 SKILL.md + 附件)→ 可见范围/权限 → 提交审核。
- **游乐场**:new-api 游乐场式的网页对话页,套 B 风格;输入框工具栏选择模型与参数,输入框对话;「接口配置」与「设置 → 模型接口」共用配置,包含 API 基础地址、API Key、自动获取并勾选的模型列表及默认模型，保留手动 ID 输入。无需 Token Hub 源码、管理接口或用户分组接口。
- **游乐场布局（2026-09-07）**：对话画布占满导航栏下方的可用视口，移除页面内重复菜单标题和说明；消息列与底部输入框居中限宽，输入框采用浅灰圆角、轻阴影的悬浮样式，画布连续延伸到底部。长对话在画布内滚动，末条消息预留输入框高度；生成时跟随最新内容，用户上翻后暂停跟随并可一键回到底部。
- **模型生成等待与失败反馈（2026-09-08）**：游乐场与 Skill 创建共用的模型流式调用总时限为 5 分钟，等待上游响应头最多 60 秒；两条模型生成 POST 路由的外层请求时限为 6 分钟，预留鉴权和返回结构化错误的时间，其余路由仍为 60 秒，模型列表发现仍为 20 秒。Skill 创建在完整结果校验后展示草案，等待期间显示动画和实际经过时间，长等待提供说明但不虚构阶段或百分比；超时、响应中断、上游 HTTP 拒绝与无效草案分别提示，保留需求、素材与已有草案，取消释放上游请求。
- **草案校验诊断（2026-09-08）**：生成失败区分模型明确报告的输出长度上限、空响应、未闭合 JSON、格式/结构错误、附件及正文校验失败、响应体积超限；只有模型明确报告长度上限时才断言因上限截断。页面显示具体原因，日志仅记录安全原因码、接收字节数及请求编号，不保留模型原文；不完整或无效输出仍不替换原草案。
- **上游拒绝诊断（2026-09-08）**：上游 HTTP 拒绝保留状态码，仅按已知机器错误码区分上下文超限、模型线路不可用、负载过高、限流、额度不足及请求不受支持；无明确错误码的 503 不推断为输出截断或输入过长。错误正文最多读取 16 KiB、等待 2 秒，不保存或回传原文。生成成功和模型失败记录请求编号、模型、对话条数、对话/素材/草案字节数、输出预算与耗时，用于比较预设与自定义输入；不记录用户内容或密钥，不自动重试。
- **输出预算与当前模型兼容（2026-09-08）**：以用户常用的 Opus 5、Kimi K3、GPT-5.6 等模型为适配基线。Skill 创建默认请求 65536 个输出 token，用户可在“生成设置”调整为 1–1048576 的整数；生成接口未传 `max_tokens` 时使用同一默认值。共享模型代理与游乐场显式输出参数接受到 1048576，但不将该范围冒充所选模型的能力；模型不接受参数时明确提示检查或调低，长度截断时展示本次请求预算。对外 OpenAI 兼容请求将预算转为 `max_completion_tokens`，Anthropic 使用 `max_tokens`，未显式设置时默认 65536；浏览器字段保持兼容。Skill 生成使用模型默认采样行为，不强制温度。思考 token 可能计入预算，正文、附件和总大小仍独立校验。
- **游乐场参数（2026-09-06，2026-09-08 更新预算兼容）**：参考用户提供的米醋 API 截图与 new-api 游乐场操作，输入工具栏打开参数浮层。温度、Top P、频率惩罚、存在惩罚、最大 Tokens 独立启用，默认均关闭，仅发送启用值；温度与 Top P 择一调整。系统提示词留空不发送，设置仅在当前页面保留。OpenAI 兼容接口发送上述参数，其中输出预算转换为 `max_completion_tokens`；Anthropic 不支持惩罚字段，温度上限为 1，不同时发送温度与 Top P，未设置最大 Tokens 时后端按协议提供 65536。模型自身的参数支持范围由上游决定，不依据模型名猜测。
- **视觉规范（2026-09-06）**：白灰背景与细边框，收紧圆角，去掉渐变、光晕和装饰性彩色卡片；蓝色用于主操作、能力中心品牌图标和当前菜单（浅蓝底、蓝色文字与图标），草稿/下线用灰、审核中用琥珀、上线用绿，保留文字状态。首页以常用操作与发布步骤为主，不展示虚构统计或动态。工作区内容随窗口宽度展开，能力卡片根据可用宽度自动增减列数；设置表单和长文本保持适合阅读的宽度。选择控件统一使用白底、细边框和浅蓝选中态的自定义下拉，支持键盘操作；长选项在收起时省略、展开时换行，弹层适配窄屏。
- **公开落地页（2026-10-27）**：根路径 `/` 展示通用产品介绍，不再自动跳转工作台；原有首页视觉规范继续适用于 `/hub` 工作区。公开页可使用轻量连接动画与可切换的能力/知识/模型流程示意，不展示内部资产、真实活动或虚构统计。动画可暂停并遵循减少动态效果设置。未登录入口转 `/login`，已登录可进入工作台或相应功能；登录页确认有效会话后转 `/hub`。公司应用配置完成前明确展示登录接入中的提示。
- **操作说明（2026-10-27）**：工作区新增 `/hub/guide`，通过侧栏与仪表盘入口访问；同一份通用指南在 `/guide` 提供无须登录的分享页面，落地页导航与页脚提供入口。内容覆盖首次登录、能力复用、知识上传检索、个人模型配置、能力发布、应用知识接入及常见问题；公开页面不读取内部数据，业务功能仍按原有认证与权限访问。公司登录已由用户确认，落地页改为“使用公司飞书账号”提示。
- **审核队列 / 审计日志**(admin):表格向。

---

## 12. 后端结构与 it-wiki 迁移

### 12.1 保留(复用 it-wiki 骨架)

- `internal/auth`:飞书 OAuth / session / CSRF / Origin(直接复用)
- `internal/config`、`internal/repo`(migrations/queries/sqlc)、`internal/worker`(river)、MinIO 存储、加密实现(`OAUTH_ENCRYPTION_KEY`)
- chi 路由、request_id 日志、显式错误规约

### 12.2 新增

- `internal/registry`:capabilities / versions / 治理状态机
- `internal/credential`:凭证保险箱(加密存取、发放、轮换)
- `internal/audit`:审计写入/查询
- `internal/playground`:反代 Token Hub `/v1/chat/completions`
- health-check river job

### 12.3 旧知识产品处理（D12 替代原全面退役计划）

- 原聊天 UI、会话产品与 ReAct Agent 不恢复到生产路径。
- 文档管理、解析、切分、embedding、向量检索、引用和快照机制保留并评估复用；按照用户/应用授权与团队知识库边界适配后重新装配，禁止仅开启 legacy 路由。
- 飞书来源同步归入 K2，需验证来源权限、撤销、删除和增量更新；暂不以一层 MCP 包装代替知识服务本身。

> 迁移策略：保留现有代码与知识数据，新增治理与知识服务所需增量迁移。旧数据保持私有，不自动开放给应用；删除旧表或数据需独立明确授权。新增应用凭证不复用旧个人 Owner 身份，也不取用个人游乐场密钥做平台 embedding。

---

## 13. 阶段划分(实施顺序)

- **阶段 A · 平台骨架（历史实施）**:改造 it-wiki，移除旧 KB 生产路由；保留并跑通飞书登录 + shell + 侧栏导航。D12 保留知识数据，按新边界恢复知识服务。
- **阶段 B · 注册中心核心**:capabilities/versions 数据模型;发布/编辑;列表(卡片 + 表格开关);详情页。Owner 视角闭环。
- **阶段 C · 治理**:审核状态机(draft→in_review→published→offline);admin 审核队列;可见范围/权限;审计日志。
- **阶段 D · 凭证保险箱 + 发现**:加密凭证存取;「获取接入凭证」(权限 + 审计);只读 discovery API。
- **阶段 E · 健康检查**:river job 探测 MCP endpoint,回填健康状态。
- **阶段 F · 游乐场**:用户配置 API 基础地址、API Key(加密)、模型列表与默认模型;对话页经平台后端调用所选的 OpenAI 兼容或 Anthropic 原生接口。用户确认优先从 OpenAI 兼容 `/models` 获取可用 ID，并在页面搜索、勾选；手动输入作为无模型列表接口时的回退。

**2026-09-19 当前实施顺序**：A/B/C、提前实施的 F 与 Skill 创建/内部市场已有实现，验证状态以对应记录为准。下个 session 优先实施 **K1 知识服务基础闭环**（管理、索引、授权检索、应用接入）；K2 来源同步/检索优化、K3 可信用户委托随后实施。原 D/E 继续保留，未因知识服务规划而完成。详见 [实施计划](../plans/2026-09-19-knowledge-service-plan.md)。阶段 A 的历史“退役 KB”指当时移除生产路由，不再作为删除知识数据的指令。

### V1.5+ 推迟项(别在 v1 做)

- 接入鉴权从「共享凭证」升级到「委托身份(server 自校验飞书/OAuth)」或「短时令牌 broker」。
- 系统对系统的 discovery/MCP 凭证访问扩展仍待实施；K1 知识检索专用应用 token、显式库授权和撤销能力按 D12 提前实施。
- Skill 生成的完整联网调研、多 Agent 自动优化与运行时执行；轻量引导创建按 D10 提前实施。
- 按部门审批人;游乐场会话历史持久化;对外能力输出(商业化)。

---

## 14. 风险与缓解

| 风险 | 缓解 |
|---|---|
| 共享凭证弱点(泄露广域暴露、难按人吊销) | 加密存储 + 获取门禁 + 审计 + 轮换;V1.5 升级鉴权模型 |
| 与 Token Hub 耦合 | 只依赖「同源飞书 open_id」+「用户自带 sk- key」,不依赖其私有接口;影响面小 |
| MCP 规范演进(2026-07-28 无状态/鉴权硬化) | 平台只存元数据 + endpoint、不进调用链路;健康检查用规范无关探测 |
| Token Hub / new-api 由他人维护 | 我们不改它,零 fork 维护成本 |
| 部门数据来源不稳 | 使用 Admin 维护的可信 platform_profiles；不接受用户自报部门参与授权，部门资料缺失时拒绝部门范围访问 |

---

## 15. 决策记录

- **D1（历史，范围由 D12 扩展）** 方向从 KB 聊天产品转「能力中心(MCP/Skill 注册分发)」。
- **D2** 建在哪 = 独立服务(改造 it-wiki),与 Token Hub 打通;**不动 Token Hub**。
- **D3** 身份 = 自有飞书 OAuth(复用 it-wiki),`open_id` 关联 Token Hub 用户。
- **D4** MCP 注册中心为目录，不做第三方 MCP 运行时网关；D12 新增的内置知识检索服务不受此限制。
- **D5** 接入凭证 = **共享凭证保险箱**(加密 + 权限门禁 + 审计),v1;V1.5 评估 broker/委托身份。
- **D6** UI = **B 风格**(对齐 micuapi/新版 new-api),白色侧栏 + 白灰内容 + 低饱和蓝主操作（2026-09-06 按用户反馈收敛配色，替代原深色侧栏方案）;列表卡片默认 + 表格开关;详情两栏。
- **D7（历史范围）** v1 范围 = 注册中心 + 治理(核心) + 薄游乐场；Skill 生成引导原押后，轻量版本由 D10 替代；当前范围进一步按 D12 扩展为能力市场＋知识服务。
- **D9（2026-09-05 用户确认）** 模型接口显式区分 OpenAI 兼容与 Claude（Anthropic）。OpenAI 在配置基础路径后追加 `/models`、`/chat/completions`，使用 Bearer Key；Anthropic 填根地址，由后端拼 `/v1/models`、`/v1/messages`，使用 x-api-key 与版本头，并转换系统提示词和 SSE。协议随账号配置保存，切换协议需重新提供 Key；旧配置默认 OpenAI，旧密文仍可读。
- **D8（2026-09-05 用户确认）** 游乐场采用用户配置 API 地址、API Key、模型列表与默认模型。公司 Token Hub 无源码不阻塞接入;仅依赖 OpenAI 兼容接口。用户随后授权提前实现阶段 F 后端：按平台 users.id 隔离、AES-GCM 加密保存配置并代理流式请求；浏览器仅持有配置元数据和临时对话，Key 不回显。保存配置不代表模型连通通过；真实 OAuth 与模型调用分别验收。自定义地址的后端转发须验证允许的目标、DNS/重定向及内网访问边界,不把未校验 URL 直接交给持有密钥的 HTTP 客户端。

- **D10（2026-09-06 用户确认）** 提前实施 Skill 轻量对话式生成：复用用户模型连接，提供通用工作流程模板及女娲方法适配的专家思维模板，经过需求澄清、可编辑预览、文件校验后保存 Owner 草稿。平台使用受控内置生成规则，不执行上传 Skill 或上游工具，不自动联网调研。已有 C–E 顺序与公司模型联调暂缓决定保持。见 [Skill 引导创建计划](../plans/2026-09-06-skill-builder-plan.md)。
- **D11（2026-09-19 用户确认）** 将已有授权目录完善为内部能力市场，先走通发现、详情、下载安装与「基于此创建」，继续复用治理与 Owner 草稿保存；不新增重复目录、不接外部市场、不在线执行 Skill。见 [Skill 市场计划](../plans/2026-09-19-skill-market-plan.md)。
- **D12（2026-09-19 用户确认，下一 session 开发）** 定位扩展为能力市场＋知识服务平台。恢复知识库管理、文档索引与授权检索 API，供其他业务系统开发对话；对话、历史和回答生成由接入方负责。替代“不做知识库”、摄入/向量表退役及知识检索应用凭证延后等相关决定；保留市场/治理/Skill/游乐场和第三方 MCP 直连边界。以 K1 为下一开发优先级，K2/K3 分期；来源限定与质量要求按知识服务设计 §2.1：K1 为文本型文档，K2 为飞书云文档及对应 Wiki，Excel/CSV/Sheets/多维表格另行评估；当前 session 仅改规划，不启动实现或数据库迁移。见 [知识服务设计](2026-09-19-knowledge-service-design.md)与[新 session 交接](../plans/2026-09-19-knowledge-service-handoff.md)。

---

## 16. 验收标准(v1)

1. Owner 用飞书登录后,能发布一个 MCP 能力(endpoint + 工具清单 + 可见范围),经 admin 审核上线。
2. 另一位有权限用户能在列表(卡片/表格可切)发现它、进详情、点「获取接入凭证」拿到 endpoint + 凭证,且该动作写入审计。
3. 无权限用户看不到该能力,discovery API 也不返回。
4. Skill 能上传 SKILL.md + 附件、审核上线、导出为 Claude Skills zip。
5. 健康检查能标出已上线 MCP 的存活状态。
6. 游乐场能用用户自行配置的 API 地址、API Key 和模型 ID 与 OpenAI 兼容服务对话,无需 Token Hub 源码或私有接口。
7. 全流程满足项目规约：按变更范围使用 goose/sqlc、River 异步、显式错误；平台配置秘密走 env，MCP 凭证与用户 Token Hub key 按 §6/§8 加密存储，不写入 Git 或日志。验证范围遵循 AGENTS.md；以上 v1 产品验收不作为每次维护修改的全量门禁。
8. D12 的知识服务按独立设计 §6 验收，包含真实文档索引、应用授权检索、稳定引用和撤权/删除一致性；不得因知识能力上线而宣称上述尚未完成的 D/E 已完成。
