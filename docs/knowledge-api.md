# K1 知识服务 API

人员入口：`/hub/knowledge`、`/hub/knowledge/search`。管理员入口：`/hub/knowledge/applications`。业务系统负责对话历史、多轮上下文问题整理与回答生成；平台负责单次问题检索优化。

## 机器检索

```http
POST /api/v1/knowledge/search
Authorization: Bearer <application_token>
Content-Type: application/json

{"knowledge_base_ids":["<uuid>"],"query":"报销需要哪些材料？","top_k":5}
```

凭证在管理员签发/轮换时显示一次，服务端只存 SHA-256 摘要和前缀；有效期 90 天。轮换撤销该应用现存凭证，禁用、撤销、过期和移除 scope 后立即失效。业务后端持有凭证，不放到前端或 URL 中。

- `knowledge_base_ids`：1–10 个 UUID，服务端去重；混入任何无权库整次 403。全公司可见不等于应用授权；Owner 须先确认整库可共享，Admin 再显式授权。
- `query`：1–4000 个 Unicode 字符；`top_k` 默认 5，范围 1–20，所有库合计。未知字段（含 `user_id`、`department`、聊天历史）拒绝。
- 请求体上限 32 KiB；每个片段最多 1200 字符，最多 20 个片段，内容量最多约 96 KiB UTF-8。客户端不能覆盖模型、scope 或授权主体。
- 每个应用每分钟最多 60 次、最多 4 个并发；人员检索单独按用户限制。当前沿用单 Go 实例的进程内限制，重启重置；多副本部署前须改为共享配额，不能将此限制宣传为跨实例配额。
- 检索超时 25 秒并传递取消。查询前校验完整授权范围，候选 SQL 只读取该范围和活动版本；返回前重新校验凭证、授权及活动文档状态。

```json
{
  "request_id": "...",
  "matches": [{
    "knowledge_base_id": "...",
    "document_id": "...",
    "document_version_id": "...",
    "chunk_id": "...",
    "title": "报销制度.md",
    "content": "报销需要发票和审批单。",
    "score": 0.81,
    "locator": {"heading":"财务制度 / 报销材料","line":3,"line_end":5,"part":1}
  }]
}
```

score 为最终排序信号，不是答案正确率，也不再统一表示余弦相似度。基础排序使用融合与词项覆盖分，模型重排成功时使用模型相关性分；不同模式分数不可直接比较。无匹配返回 200 和空数组。PDF 使用真实页码；TXT/Markdown 使用 1 起算行范围，DOCX 使用正文 XML 顺序中的段落范围（包括表格段落），不编造 Word 页码。`part` 标识长段切分序号。原文预览与引用共享不可变版本，接口不返回对象存储路径、密钥或未鉴权下载地址。

### 单次问题检索优化（2026-09-22）

原问题始终保留。配置优化模型后，先尝试生成一个保留条件的检索问题，再分别获取原问题、改写问题的向量候选。改写的数字/拉丁标识及常见否定、地域、单位条件变动时拒绝改写；该检查是保守保护，不是语义等价证明。中文连续双字词及拉丁完整词提供额外关键词召回，不依赖额外 PostgreSQL 扩展。关键词覆盖至少 50%，向量候选仍使用 `KNOWLEDGE_MIN_SCORE`；这些召回阈值均需真实语料校准。关键词重合不作为有答案的证据：模型重排未成功（未配置或降级）时，仅返回达到向量阈值的片段，关键词通道只参与这些片段的融合排序；模型重排成功时，以模型相关性 ≥ 0.5 取代向量阈值，仅关键词召回的片段可经模型判定后返回。

各路最多 `max(20, top_k*3)` 个候选，只查询已授权库的活动版本。以 chunk_id 去重，使用 RRF（k=60）融合；基础重排权重为融合 0.65、原问题正文词项覆盖 0.25、标题/章节覆盖 0.10。最多 60 个候选送模型语义重排，按原问题评估适用范围，模型相关性低于 0.5 的候选被排除，允许全部无命中。重排只能评分已有候选，不能创造内容或改写引用。

新增可选配置 `KNOWLEDGE_OPTIMIZER_BASE_URL`、`KNOWLEDGE_OPTIMIZER_API_KEY`、`KNOWLEDGE_OPTIMIZER_MODEL`，三项同时设置，使用 Chat Completions 兼容端点（base 通常以 `/v1` 结尾），要求 JSON 文本及 `finish_reason=stop`。平台运营方须选择能处理内部问题和授权文档的模型服务，配置独立于人员游乐场。问题与候选仅用于本次请求，不持久化、不写日志，不发会话历史、不启用工具。发送候选前再次复核授权与活动版本。

改写预算 3 秒，单路 embedding 8 秒，重排 5 秒，总请求仍为 25 秒。响应上限 64 KiB，禁重定向和自动重试。模型缺配时使用原问题混合召回及基础重排；模型超时/格式错误时退回对应基础路径，保留原始查询。原始 embedding 或数据库故障仍返回错误，不伪装成无答案。

成功响应新增 `retrieval`：`strategy` 为 `hybrid_rrf`（空库未执行为 `none`）；`rewrite` 为 `applied/unchanged/not_configured/fallback/skipped`；`rerank` 为 `model/local/fallback/skipped`；`warnings` 为安全诊断代码数组。人员测试页展示模型未配置和降级状态。调用方不得把 `fallback` 或 `not_configured` 当作模型优化验收通过。

当前关键词通道在授权活动片段内进行有界词项扫描，不是 BM25，也尚无专用全文索引。无需 schema 迁移或重新索引；数据量增长后的真实延迟、召回阈值与模型效果需在试用语料验收，不能仅凭小样本通过推断大库性能。

优化验收使用 `KNOWLEDGE_EVAL_SUITE=optimization KNOWLEDGE_EVAL_REQUIRE_OPTIMIZER=1 go run ./cmd/evalknowledge`，仍需上述三个 fixture 文档完成真实索引。新增六题覆盖口语表达、否定地区、紧急病假例外、相似但无答案和不存在的制度编号；默认 `baseline` 保留原八题。两套均需通过，不能只挑优化集。报告记录每题实际检索阶段、模型配置标签及版本；强制优化模式拒绝缺配、降级或非模型重排的非空结果。该命令不自动写入或重建评测资料。

错误沿用 `{"error":{"code":"...","message":"...","request_id":"..."}}`：400 无效输入；401 无效/失效凭证；403 scope/库授权不足；409 `knowledge_not_ready`（请求库有文档但无活动版本，或模型配置不匹配）；429 限流；503 依赖不可用；504 超时。空库为正常无命中，不以失败文档伪装空库。

## 人员管理

所有下列接口使用现有 session；写请求必须带正确 Origin 和 CSRF。应用 Bearer token 不能调用它们。

| 路由 | 操作 |
|---|---|
| `GET/POST /api/v1/knowledge/bases` | 可见库列表 / 创建 |
| `PUT/DELETE /api/v1/knowledge/bases/{id}` | Owner/Admin 编辑范围、成员、应用共享 / 逻辑删除 |
| `GET/POST /api/v1/knowledge/bases/{id}/documents` | 文档列表 / multipart `file` 上传 |
| `POST /api/v1/knowledge/bases/{id}/documents/{documentID}/versions` | 上传替换版本 |
| `DELETE /api/v1/knowledge/documents/{id}` | 立即逻辑不可见 |
| `POST /api/v1/knowledge/documents/{id}/retry` | 重试当前失败待处理版本 |
| `GET /api/v1/knowledge/versions/{id}` | 预览、定位和质量诊断；非活动版本仅管理者可见 |
| `POST /api/v1/knowledge/test-search` | 当前人员权限检索，参数与机器检索一致 |
| `GET/POST /api/v1/knowledge/applications` | Admin 列表 / 创建 |
| `PUT /api/v1/knowledge/applications/{id}` | Admin 名称、启停和显式授权集合 |
| `POST/DELETE /api/v1/knowledge/applications/{id}/credentials` | Admin 签发轮换 / 撤销；应用不存在返回 404 |
| `GET /api/v1/knowledge/audit` | Admin 最近 100 次调用元数据 |

知识库可见范围 `private/company/department/members`；部门从可信平台身份读取，成员使用飞书 Open ID。应用共享与人员可见范围独立，私有库不允许应用共享。关闭应用共享或删除库会在同一事务撤销该库的全部应用授权，重新开启共享后须由 Admin 再次显式授权。旧资料默认私有，未完成 K1 质量核验的旧索引不作为 K1 可检索版本；旧远程来源 ACL 未核验前不得共享给应用。

## 摄入和质量边界

上传上限 20 MiB，提取正文上限 2 MiB，PDF 上限 500 页，最多 3000 个片段。仅 UTF-8 TXT/Markdown、DOCX、文字型 PDF；旧 DOC、Excel、CSV、Sheets、多维表格均拒绝。

解析预览保留原始位置；同章节相邻段落可合并以保留适用条件、单位和表头，最长 1200 字符，长段重叠 120 字符，PDF 不跨页合并。超大片段表格阻止索引，不截断表头。DOCX 合并/嵌套表格、未接受修订、无法核验的图片/嵌入内容，PDF 空白/扫描/失败页、多栏或明显表格间隔等保守阻止发布。页眉页脚不参与 DOCX 正文检索并明确警告。当前没有 OCR；不能承诺任意 DOCX/PDF 的版式都可可靠解析。

上传写入 MinIO 独立对象，并在同一个数据库事务内创建待处理版本和 River 任务。checksum 去重限制在库内活动文档；版本对象校验 SHA-256 后才能解析。River `knowledge` 队列、4 个 worker、最多 3 次尝试、每次最长 10 分钟：依赖/写入失败采用 River 默认退避；永久质量失败记录诊断并结束任务，用户可上传修复版或显式重试。

只有全部片段 embedding 成功且数量、维度、有限非零向量均有效时，才事务性写入新版本全部片段并切换活动指针。失败保留旧活动版本；删除或新任务取代旧任务后，旧任务不能发布。旧对象/版本保留供追溯，物理清理不在 K1 自动执行；不存在自动删除历史知识数据的任务。

## 配置、迁移与评测

使用平台 `EMBEDDING_BASE_URL`（包含提供商 API 前缀，如 `/v1`）、`EMBEDDING_API_KEY`、`EMBEDDING_MODEL`、`EMBEDDING_DIM`，不借用用户游乐场密钥。四项均未设置时知识服务不装配，`/api/v1/knowledge/*` 路由不存在；部分设置则拒绝启动。`KNOWLEDGE_MIN_SCORE` 默认 0.5，必须通过真实模型评测校准。

新增 0016 权限/版本表和 0017 向量维度迁移。旧 `chunks` 原样保留，K1 版本片段使用 `knowledge_chunks`；0017 固定其 vector(N) 类型，运行时校验维度。库和版本记录模型/维度/parser version。模型或维度改变不混写；需要显式重建和迁移方案，不能自动改列或删除旧知识数据。

服务启动仍会执行项目 goose/River 迁移，**不要为了查看页面直接启动并迁移共享业务库**。须先确认环境与该库迁移授权。0016 回滚在旧 checksum 唯一约束无法表示 K1 删除后重传的数据时会拒绝，避免自动删数据。

固定评测集见 `backend/internal/knowledge/testdata/`，覆盖材料、跨段要求、时限、表格单位、境内/境外适用性及两个无答案问题。通过标准：Recall@5 ≥ 0.9、命中引用位置正确率 100%、无答案误召回 0；境内/境外用例额外要求首条命中正确。8 例是开发基线，不代表所有业务语料质量。

先在隔离评测库上传三个 fixture 文件并完成真实 embedding 索引，再在业务系统调用环境配置 `KNOWLEDGE_API_BASE`、`KNOWLEDGE_APPLICATION_TOKEN`、`KNOWLEDGE_EVAL_BASE_IDS`，执行 `go run ./cmd/evalknowledge`。命令只检索，不创建/修改资料，不打印 token。输出包括每题命中版本和指标；模型名通过 `EMBEDDING_MODEL` 记录。模拟词项模型报告不能替代这一步。
