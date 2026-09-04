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
6. 一个**游乐场**:网页端对话,用用户自己的 Token Hub `sk-` key 反代到 Token Hub,测模型。

### 2.2 非目标(v1 明确不做,见 §13)

- ❌ 运行时网关 / 代理(平台**不进 MCP 调用链路**)
- ❌ broker / 委托身份鉴权(v1 用共享凭证保险箱,见 §8)
- ❌ Skill 在线执行 / sandbox 运行
- ❌ Skill 自然语言**生成引导**(押后到阶段二产品)
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

遵循项目 DB 规约:所有 schema 走 **goose migration** → **sqlc queries** → 生成 Go 代码,禁止裸 SQL(pgvector 特有语法除外,v1 不需要)。v1 不再需要 embedding/pgvector(chunks 表退役,见 §12),`vector` 扩展可保留备用。

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
| current_version_id | uuid null | 当前上线版本 |
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
| **— MCP —** | | |
| mcp_endpoint | text null | Streamable HTTP endpoint |
| mcp_transport | text null | 默认 `streamable-http` |
| mcp_auth_scheme | text null | 如 `bearer` |
| tools | jsonb null | 工具清单(名称/描述/入参 schema),作者声明或 introspect 回填 |
| **— Skill —** | | |
| skill_bundle_key | text null | MinIO 对象 key(SKILL.md + 附件打包) |
| skill_manifest | jsonb null | name/description/when-to-use 等元信息 |

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
- `visibility=department`:同 `department` 用户可见。部门来源:飞书组织 API(复用 it-wiki 飞书集成)或用户 profile,v1 可先用用户自报/飞书部门字段。
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
| open_id | text pk | |
| base_url | text | 默认 Token Hub `https://tokenhub.robosense.cn/v1` |
| key_ciphertext / nonce | bytea | 用户 `sk-` key 加密存 |
| default_model | text null | |
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
- 基调:**深色侧栏 + 亮色内容 + 蓝色强调(`#2563eb` 系)**,neutral 灰阶,圆角柔和,卡片带轻阴影。
- 状态色:上线=绿、待审核=琥珀、草稿/下线=灰。

### 11.2 导航(左侧深色栏)

`仪表盘 · 注册中心 · 游乐场 · 我的发布 · 审核队列(admin) · 审计日志(admin) · 设置`

### 11.3 关键页面

- **注册中心列表**:默认**卡片网格(发现向)**,右上角「卡片 / 表格」**视图开关**切换到**表格(治理向)**;顶部搜索 + 筛选(类型 / 部门 / 状态) + 「发布」按钮。
- **能力详情(两栏)**:
  - 左主栏:描述、**工具清单**(每个 tool 名称 + 说明 + 入参)、版本历史。
  - 右栏:**接入信息**(endpoint / 传输 / 鉴权 / 「获取接入凭证」)、可见范围(部门 + 白名单)、健康 / 元信息(负责人 / 更新时间 / 仓库)。
- **发布 / 编辑**(标准表单/向导):基本信息(名称/类型/部门/描述)→ 类型专属(MCP:endpoint/传输/鉴权/工具清单;Skill:上传 SKILL.md + 附件)→ 可见范围/权限 → 提交审核。
- **游乐场**:new-api 游乐场式的网页对话页,套 B 风格;顶部选模型,输入框对话;首次需在「设置」录入 Token Hub `sk-` key。
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

- **阶段 A · 平台骨架**:改造 it-wiki,退役 KB;保留并跑通飞书登录 + shell + 深色侧栏导航。
- **阶段 B · 注册中心核心**:capabilities/versions 数据模型;发布/编辑;列表(卡片 + 表格开关);详情页。Owner 视角闭环。
- **阶段 C · 治理**:审核状态机(draft→in_review→published→offline);admin 审核队列;可见范围/权限;审计日志。
- **阶段 D · 凭证保险箱 + 发现**:加密凭证存取;「获取接入凭证」(权限 + 审计);只读 discovery API。
- **阶段 E · 健康检查**:river job 探测 MCP endpoint,回填健康状态。
- **阶段 F · 游乐场**:设置里录入 `sk-` key(加密);对话页反代 Token Hub。

### V1.5+ 推迟项(别在 v1 做)

- 接入鉴权从「共享凭证」升级到「委托身份(server 自校验飞书/OAuth)」或「短时令牌 broker」。
- 系统对系统的 discovery/凭证访问的平台专用 token。
- Skill 自然语言**生成引导**(阶段二产品)。
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
- **D6** UI = **B 风格**(对齐 micuapi/新版 new-api),深色侧栏 + 亮色内容 + 蓝强调;列表卡片默认 + 表格开关;详情两栏。
- **D7** v1 范围 = 注册中心 + 治理(核心) + 薄游乐场;Skill 生成引导押后。

---

## 16. 验收标准(v1)

1. Owner 用飞书登录后,能发布一个 MCP 能力(endpoint + 工具清单 + 可见范围),经 admin 审核上线。
2. 另一位有权限用户能在列表(卡片/表格可切)发现它、进详情、点「获取接入凭证」拿到 endpoint + 凭证,且该动作写入审计。
3. 无权限用户看不到该能力,discovery API 也不返回。
4. Skill 能上传 SKILL.md + 附件、审核上线、导出为 Claude Skills zip。
5. 健康检查能标出已上线 MCP 的存活状态。
6. 游乐场能用用户自己的 `sk-` key 与 Token Hub 对话。
7. 全流程满足项目规约:goose+sqlc、river 异步、显式错误、secret 走 env。
