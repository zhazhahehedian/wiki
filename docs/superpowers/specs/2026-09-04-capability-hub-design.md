# 能力中心(Capability Hub)设计 · Design Spec

- 日期:2026-09-04
- 状态:草案(待 review)
- 取代方向:本 spec 标志项目从「知识库 Agent(it-wiki)」转向「内部 MCP / Skill 能力注册与分发平台」。原 [2026-05-19 it-wiki 设计](2026-05-19-it-wiki-agent-design.md) 中的 KB/RAG 对话产品层退役,详见 §12。

---

## 1. 背景与方向转变

### 1.1 为什么转

原 it-wiki 目标是团队内部知识库对话 Agent。实际判断:

- **知识库对内价值被飞书覆盖**:企业文档、知识库、多维表格用飞书原生能力更顺;个人知识图谱可用 Obsidian + AI。再做一套 KB 聊天产品边际价值低。
- **公司已有 LLM 分发底座**:内部平台 **Token Hub**(`tokenhub.robosense.cn`,new-api 改造,OpenAI 兼容,已接飞书 SSO)已经解决了模型 key 分发、部门/额度/计费、用量日志。聊天/游乐场也是它天然能力。
- **真正缺的是能力(MCP / Skill)的注册、治理与分发**:开发者写了内部 MCP / Skill,没有一个受治理的地方发布出来供其他系统发现和调用;而这正是 2026 年 agent 生态的竞争焦点(Skill/MCP 生态 + 安全治理)。

### 1.2 转向后的定位

**能力中心 = 公司内部 MCP / Skill 的注册中心 + 治理 + 分发层。** 它长在两块既有基础设施旁边:

- 上游接 **飞书**(身份 + 文档能力来源)
- 下游接 **Token Hub**(模型 key + 计费,供游乐场使用)

平台自己**不做知识库**、**不做运行时网关**,只做「目录 + 治理 + 发现 + 凭证分发」,外加一个薄的「游乐场」补齐门户体验。

---

## 2. 目标与非目标

### 2.1 v1 目标

1. 开发者能把**内部 MCP server**(远程 Streamable HTTP)**注册/发布**到平台,带描述、工具清单、版本、可见范围。
2. 开发者能把 **Skill**(SKILL.md + 附件包)发布到平台,可导出为 Claude Skills zip。
3. 平台提供**治理**:草稿 → 提交审核 → 管理员审核 → 上线/下线;版本管理;审计日志。
4. 使用方(人)能**发现**有权限看到的能力,并**获取接入凭证**(受权限门禁 + 审计)后直连目标 MCP。
5. 其他系统能通过**只读 discovery API** 程序化发现有权访问的能力元信息。
6. 一个**游乐场**:网页端对话,用户配置 API 基础地址、API Key 和模型 ID,经后端调用 Token Hub 或其他 OpenAI 兼容接口,测模型。

### 2.2 非目标(v1 明确不做,见 §13)

- ❌ 运行时网关 / 代理(平台**不进 MCP 调用链路**)
- ❌ broker / 委托身份鉴权(v1 用共享凭证保险箱,见 §8)
- ❌ Skill 在线执行 / sandbox 运行
- Skill 自然语言**生成引导**的轻量版本已于 2026-09-06 获用户授权提前实施（D10）；完整联网研究/自动执行仍不属于 v1。
- ❌ 对外部合作方/客户输出能力(商业化方向,更远)
- ❌ 按部门配置审批人(v1 仅平台管理员审批)
- ❌ 游乐场会话历史持久化(v1 对话临时,不落库)
- ❌ 修改 Token Hub 代码(它由他人维护,零 fork)

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
        发现 + 获取接入凭证(权限+审计) │  (平台不在调用链路上)
                              ▼
   其他系统 / agent ───直连───▶ 各业务自建的 MCP server
   (飞书 Aily / Claude / Coze / 内部 bot / CI …)
```

关键点:

- 能力中心与 Token Hub 是**两个独立服务**,只通过「同源飞书 open_id」和「用户自带 sk- key」松耦合,不依赖 Token Hub 私有接口。
- 能力中心是**目录**,不是网关。使用方拿到 endpoint + 凭证后**直连** MCP server,平台不转发调用。

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

遵循 [AGENTS.md](../../../AGENTS.md) 的数据库规约：schema 变更新增 goose migration；查询按需修改 sqlc queries，sqlc 输入变化时重新生成并调整受影响消费者。纯查询变更不要求迁移，迁移不要求无关 service 改动。新平台查询沿用 sqlc，不向 service 内散落裸 SQL。v1 不再需要 embedding/pgvector(chunks 表退役,见 §12),`vector` 扩展可保留备用。

### 6.1 `capabilities`

| 列 | 类型 | 说明 |
|---|---|---|
| id | uuid pk | |
| slug | text unique | URL 友好唯一标识 |
| type | text | `mcp` \| `skill` |
| name | text | 展示名 |
| description | text | 简介 |
| owner_open_id | text | 负责人飞书 open_id |
| department | text | 负责部门(展示/筛选/权限用) |
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
- `visibility=department`:同 `department` 用户可见。部门来源：服务端 `platform_profiles` 中由 Admin 维护的可信部门资料；不接受用户自报字段作为授权依据，缺失部门默认拒绝。飞书组织自动同步留待后续，不新增 OAuth scope。
- `visibility=allowlist`:`capability_allowlist(capability_id, open_id)` 明确名单。
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

`仪表盘 · 注册中心 · 游乐场 · 我的发布 · 审核队列(admin) · 审计日志(admin) · 设置`

### 11.3 关键页面

- **注册中心列表**:默认**卡片网格(发现向)**,右上角「卡片 / 表格」**视图开关**切换到**表格(治理向)**;顶部搜索 + 筛选(类型 / 部门 / 状态) + 「发布」按钮。
- **能力详情(两栏)**:
  - 左主栏:描述、**工具清单**(每个 tool 名称 + 说明 + 入参)、版本历史。
  - 右栏:**接入信息**(endpoint / 传输 / 鉴权 / 「获取接入凭证」)、可见范围(部门 + 白名单)、健康 / 元信息(负责人 / 更新时间 / 仓库)。
- **发布 / 编辑**(标准表单/向导):基本信息(名称/类型/部门/描述)→ 类型专属(MCP:endpoint/传输/鉴权/工具清单;Skill:上传 SKILL.md + 附件)→ 可见范围/权限 → 提交审核。
- **游乐场**:new-api 游乐场式的网页对话页,套 B 风格;输入框工具栏选择模型与参数,输入框对话;「接口配置」与「设置 → 模型接口」共用配置,包含 API 基础地址、API Key、自动获取并勾选的模型列表及默认模型，保留手动 ID 输入。无需 Token Hub 源码、管理接口或用户分组接口。
- **游乐场参数（2026-09-06）**：参考用户提供的米醋 API 截图与 new-api 游乐场操作，输入工具栏打开参数浮层。温度、Top P、频率惩罚、存在惩罚、最大 Tokens 独立启用，默认均关闭，仅发送启用值；温度与 Top P 择一调整。系统提示词留空不发送，设置仅在当前页面保留。OpenAI 兼容接口发送上述字段；Anthropic 不支持惩罚字段，温度上限为 1，不同时发送温度与 Top P，未设置最大 Tokens 时后端仍按协议提供 4096。模型自身的参数支持范围由上游决定，不依据模型名猜测。
- **视觉规范（2026-09-06）**：白灰背景与细边框，收紧圆角，去掉渐变、光晕和装饰性彩色卡片；蓝色用于主操作、能力中心品牌图标和当前菜单（浅蓝底、蓝色文字与图标），草稿/下线用灰、审核中用琥珀、上线用绿，保留文字状态。首页以常用操作与发布步骤为主，不展示虚构统计或动态。工作区内容随窗口宽度展开，能力卡片根据可用宽度自动增减列数；设置表单和长文本保持适合阅读的宽度。
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

### 12.3 退役(从平台移除;代码可留在历史/分支备参)

- KB/RAG 对话产品层:agent ReAct 循环、kb 检索工具、chunks/embedding 摄入、文档导入作为**产品**的部分。
- 说明:原「飞书文档检索」等能力**不再是平台内置产品**;它们的定位变为「一个团队可以自行发布到平台的示例 MCP」,其实现不属于 v1 平台代码。

> 迁移策略:在改造分支上,先保留 auth/config/repo/worker/minio,逐步删除 kb/rag/agent 产品代码并新增 registry/credential/audit/playground。DB 通过 goose 迁移做表的新增与退役(chunks 等)。

---

## 13. 阶段划分(实施顺序)

- **阶段 A · 平台骨架**:改造 it-wiki,退役 KB;保留并跑通飞书登录 + shell + 侧栏导航。
- **阶段 B · 注册中心核心**:capabilities/versions 数据模型;发布/编辑;列表(卡片 + 表格开关);详情页。Owner 视角闭环。
- **阶段 C · 治理**:审核状态机(draft→in_review→published→offline);admin 审核队列;可见范围/权限;审计日志。
- **阶段 D · 凭证保险箱 + 发现**:加密凭证存取;「获取接入凭证」(权限 + 审计);只读 discovery API。
- **阶段 E · 健康检查**:river job 探测 MCP endpoint,回填健康状态。
- **阶段 F · 游乐场**:用户配置 API 基础地址、API Key(加密)、模型列表与默认模型;对话页经平台后端调用所选的 OpenAI 兼容或 Anthropic 原生接口。用户确认优先从 OpenAI 兼容 `/models` 获取可用 ID，并在页面搜索、勾选；手动输入作为无模型列表接口时的回退。

### V1.5+ 推迟项(别在 v1 做)

- 接入鉴权从「共享凭证」升级到「委托身份(server 自校验飞书/OAuth)」或「短时令牌 broker」。
- 系统对系统的 discovery/凭证访问的平台专用 token。
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
| 部门数据来源不稳 | v1 可先用飞书部门字段/自报;权限以 org/allowlist 为主,department 为辅 |

---

## 15. 决策记录

- **D1** 方向从 KB 转「能力中心(MCP/Skill 注册分发)」。
- **D2** 建在哪 = 独立服务(改造 it-wiki),与 Token Hub 打通;**不动 Token Hub**。
- **D3** 身份 = 自有飞书 OAuth(复用 it-wiki),`open_id` 关联 Token Hub 用户。
- **D4** 注册中心 = **纯目录**,不做运行时网关。
- **D5** 接入凭证 = **共享凭证保险箱**(加密 + 权限门禁 + 审计),v1;V1.5 评估 broker/委托身份。
- **D6** UI = **B 风格**(对齐 micuapi/新版 new-api),白色侧栏 + 白灰内容 + 低饱和蓝主操作（2026-09-06 按用户反馈收敛配色，替代原深色侧栏方案）;列表卡片默认 + 表格开关;详情两栏。
- **D7** v1 范围 = 注册中心 + 治理(核心) + 薄游乐场;Skill 生成引导原押后，轻量版本由 D10 替代。
- **D9（2026-09-05 用户确认）** 模型接口显式区分 OpenAI 兼容与 Claude（Anthropic）。OpenAI 在配置基础路径后追加 `/models`、`/chat/completions`，使用 Bearer Key；Anthropic 填根地址，由后端拼 `/v1/models`、`/v1/messages`，使用 x-api-key 与版本头，并转换系统提示词和 SSE。协议随账号配置保存，切换协议需重新提供 Key；旧配置默认 OpenAI，旧密文仍可读。
- **D8（2026-09-05 用户确认）** 游乐场采用用户配置 API 地址、API Key、模型列表与默认模型。公司 Token Hub 无源码不阻塞接入;仅依赖 OpenAI 兼容接口。用户随后授权提前实现阶段 F 后端：按平台 users.id 隔离、AES-GCM 加密保存配置并代理流式请求；浏览器仅持有配置元数据和临时对话，Key 不回显。保存配置不代表模型连通通过；真实 OAuth 与模型调用分别验收。自定义地址的后端转发须验证允许的目标、DNS/重定向及内网访问边界,不把未校验 URL 直接交给持有密钥的 HTTP 客户端。

- **D10（2026-09-06 用户确认）** 提前实施 Skill 轻量对话式生成：复用用户模型连接，提供通用工作流程模板及女娲方法适配的专家思维模板，经过需求澄清、可编辑预览、文件校验后保存 Owner 草稿。平台使用受控内置生成规则，不执行上传 Skill 或上游工具，不自动联网调研。已有 C–E 顺序与公司模型联调暂缓决定保持。见 [Skill 引导创建计划](../plans/2026-09-06-skill-builder-plan.md)。

---

## 16. 验收标准(v1)

1. Owner 用飞书登录后,能发布一个 MCP 能力(endpoint + 工具清单 + 可见范围),经 admin 审核上线。
2. 另一位有权限用户能在列表(卡片/表格可切)发现它、进详情、点「获取接入凭证」拿到 endpoint + 凭证,且该动作写入审计。
3. 无权限用户看不到该能力,discovery API 也不返回。
4. Skill 能上传 SKILL.md + 附件、审核上线、导出为 Claude Skills zip。
5. 健康检查能标出已上线 MCP 的存活状态。
6. 游乐场能用用户自行配置的 API 地址、API Key 和模型 ID 与 OpenAI 兼容服务对话,无需 Token Hub 源码或私有接口。
7. 全流程满足项目规约：按变更范围使用 goose/sqlc、River 异步、显式错误；平台配置秘密走 env，MCP 凭证与用户 Token Hub key 按 §6/§8 加密存储，不写入 Git 或日志。验证范围遵循 AGENTS.md；以上 v1 产品验收不作为每次维护修改的全量门禁。
