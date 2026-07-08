# 阶段 3 · ReAct Agent 模式设计

- 起草日期：2026-07-08
- 作者：Zenith Wang · 与 Claude 共建
- 状态：设计已确认，待实施
- 上游文档：[主 spec §5.3 / §7 阶段 3](2026-05-19-it-wiki-agent-design.md) · [阶段 2 spec](2026-06-02-phase-2-deterministic-rag-design.md) · [阶段 2.5 spec](2026-07-07-phase-2.5-retrieval-quality-design.md)

---

## 0. 背景与目标

阶段 2/2.5 已交付 deterministic RAG 对话闭环（检索 → 拼 prompt → 流式回答 → citations），流式取消已硬化，检索评测基线满分。阶段 3 交付**ReAct Agent 模式**：用户可切换对话模式，让 LLM 自主决定是否调用工具、调用哪个工具、调用几次，前端展示完整工具调用轨迹。

**目标**：

1. 会话可在 RAG / Agent 两种模式间切换（中途可切，只影响后续消息）
2. Agent 模式下 LLM 通过 OpenAI tool calling 自主调用 `kb_retrieval`、`list_documents`
3. 工具轨迹（含中间思考文本）实时推送并持久化，刷新后可回看
4. Agent 模式回答与 RAG 模式一样携带可点击 citations
5. 保持阶段 2.5 的流式取消保障不回退

## 1. 范围

### 1.1 本阶段做

- `ports.LLMClient` 扩展 tool calling（定义、流式 delta 聚合）
- 新增 `ports.Tool` 接口 + `internal/agent/`（ReAct 循环 + 2 个工具）
- SSE 协议新增 `tool_call` / `tool_result` 事件
- `PATCH /conversations/{id}` 切换 mode；创建会话可指定 mode
- 前端 mode 切换、tool-call-trace 组件、use-chat-stream 扩展
- 主 spec 与 CLAUDE.md 的决策同步（见 §9）

### 1.2 本阶段不做

- **Eino 集成**（见 §2 决策 D1）
- **calculator 工具**（见 §2 决策 D5）
- **UI 视觉升级**：现有界面偏单薄，将作为**阶段 3.5**紧随本阶段单独立项（参考 argus 与 do-write 项目的 UI，届时单独 brainstorm 出 spec）。本阶段新组件按现有风格实现，3.5 统一刷新
- 工具并行执行（MVP 顺序执行）、`thinking` 独立 SSE 事件、多 KB 工具、web search 等扩展工具

## 2. 决策记录

| # | 决策 | 理由 |
|---|---|---|
| D1 | **不引入 Eino，手写 ReAct 循环** | MVP 仅 2 个工具、单层循环，编排框架价值发挥不出来；避免 ports.Message 与 eino schema.Message 双体系；阶段 2.5 刚硬化的流式取消行为无需重新验证；主 spec §9 本就预留"必要时降级用裸 OpenAI SDK"的出口。Eino 推迟到 V1.5 出现真实编排需求（multi-agent / workflow）时再评估 |
| D2 | **mode 是 conversation 的可变属性，中途可切** | 交互自然，便于验收时同一会话对比两种模式；历史消息不受影响 |
| D3 | **工具轨迹持久化到 assistant 消息的 `tool_calls` JSONB**（列已存在） | 刷新/重开对话后轨迹可回看；不新增消息行，列表渲染与分页不受影响 |
| D4 | **Agent 模式同样生成 citations** | kb_retrieval 各次命中去重后走现有 `BuildCitations`，前端复用 citation-card + 抽屉，两种模式回答体验一致 |
| D5 | **calculator 换成 list_documents** | calculator 对知识库场景无实际用途；list_documents 能回答"知识库里有哪些文档"这类向量检索答不了的元问题，恰好展示 Agent 模式相对 RAG 的真实差异，同时验证多工具注册/选择路径 |
| D6 | **UI 升级独立为阶段 3.5，顺序在阶段 3 之后** | 横切全站的视觉工作与纵向功能闭环分开验收、分开 review；先完成功能组件再统一刷风格，避免返工 |

## 3. 端口层扩展（`internal/domain/ports/`）

### 3.1 llm.go（向后兼容地加字段）

```go
type ToolDefinition struct {
    Name        string
    Description string
    Parameters  json.RawMessage // JSON Schema
}

type ToolCall struct {
    ID        string
    Name      string
    Arguments string // raw JSON
}

type Message struct {
    Role       string     `json:"role"`
    Content    string     `json:"content"`
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // role=assistant 发起调用
    ToolCallID string     `json:"tool_call_id,omitempty"` // role=tool 回填结果
}

type ChatOptions struct {
    Model       string
    Temperature float32
    MaxTokens   int
    Tools       []ToolDefinition // 非空时启用 tool calling
}

type StreamChunk struct {
    Text      string
    ToolCalls []ToolCall // 完整聚合后的调用，仅在该轮流结束前发出一次
    Done      bool
    Usage     *TokenUsage
    Err       error
}
```

**约定**：OpenAI 流式 tool_call 的增量 delta（index/id/name/arguments 分片）由 `infra/llm/openai_compat.go` 内部聚合，聚合完成后以完整 `ToolCalls` 一次性放入 StreamChunk。上层永远看不到半截 arguments。文本 token 照常逐块流出，两者互不影响。

### 3.2 tool.go（新增）

```go
type Tool interface {
    Name() string
    Description() string
    ParametersSchema() json.RawMessage // JSON Schema，序列化进 ToolDefinition
    Invoke(ctx context.Context, argsJSON string) (result string, err error)
}
```

- `Invoke` 返回的 string 直接作为 role=tool 消息内容回喂 LLM
- `Invoke` 必须尊重 ctx 取消（工具内部的 DB/embedding 调用都已带 ctx）

## 4. ReAct 循环（`internal/agent/react_agent.go`）

新增 `internal/agent/` 包（目录符合主 spec §3 约定）：

```
internal/agent/
├── react_agent.go        # 循环本体
└── tools/
    ├── kb_retrieval.go
    └── list_documents.go
```

### 4.1 循环逻辑

```
输入：system prompt + 最近历史 + 用户问题；已注册 tools；sink
maxIterations = 5

for iter := 1; ; iter++ {
    ctx 取消检查
    tools := 已注册工具；若 iter == maxIterations 则 tools = nil（强制文字收尾，
             prompt 追加"工具调用次数已达上限，请基于已有信息回答"）
    stream := llm.ChatStream(msgs, opts{Tools: tools})
    实时转发 Text 为 SSE token 事件，同时缓存该轮全文
    if 流中出现 ToolCalls:
        该轮缓存文本 = thought（不进最终答案）
        对每个 call 顺序执行:
            SSE: tool_call {id, name, arguments}
            result, err := tool.Invoke(ctx, arguments)
            SSE: tool_result {id, name, result 截断, duration_ms, error?}
            err != nil → result = `{"error":"..."}` 照常回喂（不中断循环）
            未知工具名 / 参数解析失败 → 同样以 error result 回喂
        msgs 追加 assistant(ToolCalls) + 各 tool 消息，continue
    else:
        该轮文本 = 最终答案，break
}
ctx 取消检查（持久化前，沿用 2.5 模式）
持久化 assistant 消息（content + citations + tool_calls 轨迹 + 累计 usage）
SSE: done
```

**要点**：

- 各轮 `TokenUsage` 累加后存入 `token_usage`
- LLM 流错误 → SSE error 事件 + 终止，行为与 RAG 路径一致
- 最终轮 content 为空 → 与现有 `llm stream completed without content` 同样报错
- 取消语义：每轮进入前、每个工具执行前、持久化前检查 `ctx.Err()`；客户端断连不落半截消息（阶段 2.5 保障延续）

### 4.2 思考文本（thought）的流式处理

DeepSeek 等模型有时在发起 tool_call 前先输出一段 content。处理约定：

- 后端：文本 token **照常实时转发**；该轮若以 ToolCalls 结束，缓存的该轮文本写入第一个 step 的 `thought` 字段，不拼进最终 content
- 前端：收到 `tool_call` 事件时，把 draft 气泡当前已积累的 content 挪为该 step 的 thought 并清空气泡

最终答案的流式体验不受影响；带工具调用的轮次，其文本以"思考"形式折叠进轨迹。

### 4.3 system prompt

Agent 模式独立 system prompt（`internal/agent/` 内维护），要点：优先用 kb_retrieval 查证再回答、不得编造知识库不存在的内容、回答中沿用 `[1]` 风格的引文标记（与 `BuildRAGMessages` 的约定对齐）、列举知识库文档清单时用 list_documents。

## 5. 工具设计（`internal/agent/tools/`）

### 5.1 kb_retrieval

- 参数：`{"query": string}`（kbID 由构造时闭包注入，**不让 LLM 传**）
- 实现：薄包装现有 `service.Retrieval.Retrieve()`
- 回喂 LLM 的 result：编号列表，含 document_title / seq / score / content（单条截断），与 `buildContextBlock` 风格一致
- 副作用：把本次 hits 交给 agent 的 citation collector；按 chunk_id 去重、重新编号 c1..cN，每次调用后立即推 `retrieval` SSE 事件（携带**累计去重后的全量 citations**）

### 5.2 list_documents

- 参数：`{"status": string}`（可选，枚举过滤；缺省列全部）
- 实现：薄包装现有 document 查询，返回标题/状态/大小/更新时间，上限 50 条，超出时在 result 中注明总数
- kbID 同样闭包注入

## 6. SSE 协议与持久化

### 6.1 SSE 事件（新增 2 个，其余不变）

```
event: tool_call    data: {"id":"call_x","name":"kb_retrieval","arguments":"{...}"}
event: tool_result  data: {"id":"call_x","name":"kb_retrieval","result":"...(截断)","duration_ms":840,"error":"..."?}
```

- `retrieval` 事件在 Agent 模式下**可出现多次**（每次 kb_retrieval 后，全量替换语义）——前端现有 retrieval 分支天然兼容
- `tool_result.result` 截断长度与持久化一致（2000 字符）
- `token` / `done` / `error` 语义不变
- `ChatStreamSink` 接口增加 `SendToolCall` / `SendToolResult`

### 6.2 持久化格式（messages.tool_calls JSONB）

```json
[
  {
    "step": 1,
    "id": "call_x",
    "name": "kb_retrieval",
    "thought": "需要先查一下部署文档……",
    "arguments": {"query": "部署 QPS"},
    "result": "…（截断至 2000 字符）",
    "duration_ms": 840,
    "error": null
  }
]
```

- 同一轮多个 tool call 共享 thought（thought 只写在该轮第一个 step 上）
- citations 与 RAG 模式同结构同列；`token_usage` 存各轮累加值

## 7. API 变更

| 端点 | 变更 |
|---|---|
| `PATCH /api/v1/conversations/{conversationID}` | **新增**。body `{"mode":"rag"\|"react"}`，仅开放 mode 字段，枚举校验，404/422 语义与现有 handler 一致 |
| `POST /api/v1/kbs/{kbID}/conversations` | body 增加可选 `mode`（默认 `rag`） |
| `POST /api/v1/conversations/{conversationID}/messages/stream` | 行为按 `conv.Mode` 分发：`rag` → 现有路径不动；`react` → ReAct 循环 |

`chat_service.AskStream` 中现有的 mode 拒绝逻辑改为分发；RAG 路径零改动。

## 8. 前端设计

| 单元 | 变更 |
|---|---|
| `chat-header.tsx` | 加 RAG / Agent 分段切换（shadcn tabs 或 select），PATCH mutation + invalidate conversation query；`isStreaming` 期间禁用 |
| `tool-call-trace.tsx` | **新增**。按 step 渲染可折叠条目（shadcn collapsible）：折叠态 = 工具图标 + 名称 + 参数摘要 + 耗时/错误标记；展开 = thought + 完整 arguments + result。流式期间 `tool_call` 先渲染运行中 spinner，`tool_result` 到达后落定 |
| `message-bubble.tsx` | assistant 消息 `tool_calls` 非空时渲染 tool-call-trace（历史消息刷新后同样可见） |
| `use-chat-stream.ts` | `LocalChatMessage` 增加 steps；`applyStreamEvent` 新增 `tool_call`（含把 draft content 挪为 thought 并清空）与 `tool_result` 分支；`retrieval` 分支不动 |
| `lib/schemas` | `tool_calls` 从 `any[]` 收紧为 §6.2 的 step 结构 |
| shadcn 组件 | 需要新组件一律 `pnpm dlx shadcn@latest add`（collapsible、tabs 等） |

视觉风格按现状实现，阶段 3.5 统一升级。

## 9. 文档同步清单

本阶段两处偏离主 spec，实施时一并落库：

- 主 spec `2026-05-19-it-wiki-agent-design.md`：
  - §1 技术选型表 Eino 行标注"阶段 3 决策：MVP 手写 ReAct 循环，Eino 推迟到 V1.5 再评估（见阶段 3 spec D1）"
  - §3 "Eino 的定位" 同步更新
  - §7 阶段 3 工具清单与验收描述：calculator → list_documents
- `CLAUDE.md`：
  - §3 版本表 Eino 行同步标注
  - §5.4 "添加 Eino tool" 改为 "添加 agent tool"（实现 `ports.Tool` + 在 react_agent 注册 + 前端 trace 分支可选）
  - §2 当前阶段（完成后照例推进，并注明下一步为阶段 3.5 UI 升级）

## 10. 测试策略

### 10.1 后端单测

- `infra/llm/openai_compat`：httptest 模拟含 tool_call delta 的 SSE 流，断言聚合出完整 ToolCall（多工具、arguments 分片、与文本 token 交错）
- `agent/react_agent`：fake LLM 脚本化驱动，覆盖——
  - 单轮工具 → 文字收尾的事件序列（tool_call → tool_result → retrieval → token → done）
  - 多轮/多工具调用
  - thought 归轨迹、不进最终 content
  - 工具报错回喂后循环继续
  - maxIterations 强制文字收尾
  - **中途取消不落半截消息**（沿用阶段 2.5 测试模式）
- `agent/tools`：kb_retrieval 参数校验与 result 格式、citation 去重编号；list_documents 过滤与截断
- `service/chat_service`：mode 分发、PATCH mode 校验

### 10.2 端到端 + 前端

- HTTP 端到端：PATCH mode + react 模式全流程 SSE（fake LLM 注入），含断连取消验证（对齐 2.5）
- 前端 `use-chat-stream.test.ts`：tool_call/tool_result 事件用例（含 content 挪 thought）、多次 retrieval 全量替换

## 11. 验收标准

1. ChatHeader 切到 Agent 模式提问，能看到 LLM 调用 kb_retrieval（必要时多次）+ list_documents 的完整轨迹，最终答案流式输出且带可点击 citations（点击开抽屉）
2. 问"知识库里有哪些文档"，Agent 模式经 list_documents 正确回答（RAG 模式答不了的元问题）
3. 同一会话中途 RAG ↔ Agent 切换，后续消息按新模式处理，历史消息展示不变
4. 刷新页面后，历史消息的工具轨迹仍可展开查看
5. 流式期间停止/断连，服务端取消 LLM 流且不落半截消息（单测 + 端到端证明）
6. `go test ./...`、`pnpm lint && pnpm typecheck` 全绿

工期估算：与主 spec 一致，约 1.5~2 天。
