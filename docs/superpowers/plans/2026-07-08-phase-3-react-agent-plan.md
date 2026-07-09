# 阶段 3 · ReAct Agent 模式实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 会话可切换 RAG / Agent 模式；Agent 模式下 LLM 通过 OpenAI tool calling 自主调用 kb_retrieval 与 list_documents，工具轨迹实时推送并持久化，回答携带可点击 citations。

**Architecture:** 不引入 Eino（spec D1），手写 ReAct 循环。`ports.LLMClient` 扩展 tool calling（infra 层聚合流式 delta），新增 `ports.Tool` 接口与 `internal/agent/` 包（循环 + 工具）。chat_service 按 `conversations.mode` 分发；工具由 main.go 注入的 ToolFactory 按会话构造（kbID 闭包注入），避免 service ↔ tools 包循环依赖。SSE 新增 `tool_call`/`tool_result` 事件，轨迹存 assistant 消息 `tool_calls` JSONB。

**Tech Stack:** Go 1.22 · chi · sqlc（仅加 query，无 schema 迁移）· Next.js 15 · TanStack Query · zod · vitest

**权威 spec:** [docs/superpowers/specs/2026-07-08-phase-3-react-agent-design.md](../specs/2026-07-08-phase-3-react-agent-design.md)

---

## 文件结构

后端（`backend/internal/`）：

| 文件 | 动作 | 职责 |
|---|---|---|
| `domain/ports/llm.go` | 改 | Message/ChatOptions/StreamChunk 增加 tool 字段；ToolDefinition/ToolCall 类型 |
| `domain/ports/tool.go` | 新建 | Tool 接口 |
| `domain/chat.go` | 改 | ConversationModeReAct/RoleTool 常量、ToolCallStep、ToolCallEvent/ToolResultEvent；ChatMessage.ToolCalls 收紧类型 |
| `infra/llm/openai_compat.go` | 改 | 请求侧 tools/tool_calls 序列化（wire 类型）；流式 tool_call delta 聚合 |
| `repo/queries/conversations.sql` | 改 | UpdateConversationMode query（无 migration，mode 列已存在） |
| `agent/react_agent.go` | 新建 | ReAct 循环、EventSink、Result |
| `agent/prompt.go` | 新建 | Agent 模式 system prompt + BuildReActMessages |
| `agent/tools/kb_retrieval.go` | 新建 | 检索工具（包装 service.Retrieval，onRetrieval 回调） |
| `agent/tools/list_documents.go` | 新建 | 文档清单工具（包装 service.Document） |
| `service/chat_service.go` | 改 | mode 分发、CreateConversation 带 mode、UpdateMode、createMessage 带 steps、sink 接口扩展 |
| `service/chat_react.go` | 新建 | askReAct 路径 + citation 收集回调 |
| `http/chat_handler.go` | 改 | PATCH handler、CreateConversation body、sink SendToolCall/SendToolResult |
| `http/router.go` | 改 | PATCH 路由 |
| `cmd/server/main.go` | 改 | agent + ToolFactory 接线 |

前端（`frontend/`）：

| 文件 | 动作 | 职责 |
|---|---|---|
| `lib/schemas/index.ts` | 改 | mode 枚举、toolCallStepSchema、tool_calls 类型收紧 |
| `lib/api/chat.ts` | 改 | ChatStreamEvent 增 tool_call/tool_result、updateConversationMode |
| `lib/hooks/use-chat-stream.ts` | 改 | LocalToolStep、tool_call/tool_result 事件处理（content 挪 thought） |
| `components/chat/tool-call-trace.tsx` | 新建 | 可折叠轨迹组件 |
| `components/chat/message-bubble.tsx` | 改 | assistant 消息渲染轨迹 |
| `components/chat/chat-header.tsx` | 改 | RAG/Agent 分段切换 + PATCH mutation |
| `app/kbs/[kbId]/chats/[conversationId]/page.tsx` | 改 | 给 header 传 isStreaming |

文档：主 spec、阶段 3 spec §6.1（SSE arguments 示例改为对象）、CLAUDE.md §2/§3/§5.4。

**分支**：从 main 切 `phase-3-react-agent`，全程在该分支提交。

---

### Task 1: ports 层 tool 类型

**Files:**
- Modify: `backend/internal/domain/ports/llm.go`
- Create: `backend/internal/domain/ports/tool.go`

纯类型定义，无行为逻辑，以编译代替测试（行为由 Task 2/4 的测试覆盖）。

- [x] **Step 1: 扩展 llm.go**

将 `backend/internal/domain/ports/llm.go` 中的 `Message`、`ChatOptions`、`StreamChunk` 替换为以下内容，并新增 `ToolDefinition`/`ToolCall`（文件其余部分不动，需要新增 import `encoding/json`）：

```go
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  json.RawMessage // JSON Schema
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // raw JSON string
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // role=assistant 发起的调用
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
	ToolCalls []ToolCall // infra 层聚合完成后一次性给出，上层不处理增量
	Done      bool
	Usage     *TokenUsage
	Err       error
}
```

- [x] **Step 2: 新建 tool.go**

创建 `backend/internal/domain/ports/tool.go`：

```go
package ports

import (
	"context"
	"encoding/json"
)

// Tool 是 Agent 可调用的工具。Invoke 返回的字符串直接作为
// role=tool 消息内容回喂 LLM；实现必须尊重 ctx 取消。
type Tool interface {
	Name() string
	Description() string
	ParametersSchema() json.RawMessage
	Invoke(ctx context.Context, argsJSON string) (string, error)
}
```

- [x] **Step 3: 编译验证**

Run: `cd backend && go build ./...`
Expected: 编译通过（现有代码只读 Message.Role/Content，新增字段向后兼容）

- [x] **Step 4: Commit**

```bash
git add internal/domain/ports/llm.go internal/domain/ports/tool.go
git commit -m "feat(ports): add tool calling types to LLM port and Tool interface"
```

---

### Task 2: openai_compat 支持 tool calling

**Files:**
- Modify: `backend/internal/infra/llm/openai_compat.go`
- Test: `backend/internal/infra/llm/openai_compat_test.go`

两部分：① 请求侧把 `ports.Message`/`ports.ToolDefinition` 转成 OpenAI wire 格式（tool_calls 是嵌套 `function` 对象，不能直接 marshal ports 类型）；② 流式响应侧把 tool_call 增量 delta（按 index 分片的 id/name/arguments）聚合成完整 `ports.ToolCall`，在 finish_reason 到达时随 chunk 一次性发出。

- [x] **Step 1: 写失败测试——流式 delta 聚合**

在 `openai_compat_test.go` 追加（沿用文件现有的 httptest 风格；`sseHandler` 若已有同名 helper 则复用）：

```go
func TestChatStreamAggregatesToolCallDeltas(t *testing.T) {
	lines := []string{
		`data: {"choices":[{"delta":{"content":"让我查一下。"},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"kb_retrieval","arguments":""}}]},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"query\":"}}]},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"部署\"}"}}]},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, l := range lines {
			_, _ = io.WriteString(w, l+"\n\n")
		}
	}))
	defer srv.Close()

	client := New(Config{BaseURL: srv.URL, Model: "test"})
	stream, err := client.ChatStream(context.Background(), []ports.Message{{Role: "user", Content: "hi"}}, ports.ChatOptions{})
	if err != nil {
		t.Fatalf("ChatStream() error = %v", err)
	}

	var text string
	var calls []ports.ToolCall
	for chunk := range stream {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error: %v", chunk.Err)
		}
		text += chunk.Text
		calls = append(calls, chunk.ToolCalls...)
	}

	if text != "让我查一下。" {
		t.Errorf("text = %q, want 让我查一下。", text)
	}
	if len(calls) != 1 {
		t.Fatalf("aggregated calls = %d, want 1", len(calls))
	}
	want := ports.ToolCall{ID: "call_1", Name: "kb_retrieval", Arguments: `{"query":"部署"}`}
	if calls[0] != want {
		t.Errorf("call = %+v, want %+v", calls[0], want)
	}
}

func TestChatStreamSendsToolsAndToolMessagesOnWire(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	client := New(Config{BaseURL: srv.URL, Model: "test"})
	msgs := []ports.Message{
		{Role: "assistant", Content: "查一下", ToolCalls: []ports.ToolCall{{ID: "call_1", Name: "kb_retrieval", Arguments: `{"query":"x"}`}}},
		{Role: "tool", Content: "result text", ToolCallID: "call_1"},
	}
	opts := ports.ChatOptions{Tools: []ports.ToolDefinition{{
		Name: "kb_retrieval", Description: "search kb",
		Parameters: json.RawMessage(`{"type":"object"}`),
	}}}
	stream, err := client.ChatStream(context.Background(), msgs, opts)
	if err != nil {
		t.Fatalf("ChatStream() error = %v", err)
	}
	for range stream {
	}

	body := string(gotBody)
	for _, want := range []string{
		`"tools":[{"type":"function","function":{"name":"kb_retrieval","description":"search kb","parameters":{"type":"object"}}}]`,
		`"tool_calls":[{"id":"call_1","type":"function","function":{"name":"kb_retrieval","arguments":"{\"query\":\"x\"}"}}]`,
		`"tool_call_id":"call_1"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("request body missing %s\nbody: %s", want, body)
		}
	}
}
```

- [x] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/infra/llm/ -run 'TestChatStreamAggregates|TestChatStreamSendsTools' -v`
Expected: FAIL（ToolCalls 恒空、请求体无 tools 字段）

- [x] **Step 3: 实现 wire 转换与聚合**

修改 `openai_compat.go`。① 替换 `chatRequest` 并新增 wire 类型与转换函数：

```go
type wireFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type wireToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireFunctionCall `json:"function"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type wireTool struct {
	Type     string           `json:"type"`
	Function wireToolFunction `json:"function"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []wireMessage `json:"messages"`
	Temperature float32       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream"`
	Tools       []wireTool    `json:"tools,omitempty"`
}

func toWireMessages(msgs []ports.Message) []wireMessage {
	out := make([]wireMessage, 0, len(msgs))
	for _, m := range msgs {
		wm := wireMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, c := range m.ToolCalls {
			wm.ToolCalls = append(wm.ToolCalls, wireToolCall{
				ID: c.ID, Type: "function",
				Function: wireFunctionCall{Name: c.Name, Arguments: c.Arguments},
			})
		}
		out = append(out, wm)
	}
	return out
}

func toWireTools(defs []ports.ToolDefinition) []wireTool {
	if len(defs) == 0 {
		return nil
	}
	out := make([]wireTool, 0, len(defs))
	for _, d := range defs {
		out = append(out, wireTool{Type: "function", Function: wireToolFunction{
			Name: d.Name, Description: d.Description, Parameters: d.Parameters,
		}})
	}
	return out
}
```

`Chat` 与 `ChatStream` 中构造 reqBody 处改为：

```go
	reqBody := chatRequest{
		Model:       modelOrDefault(opts.Model, c.model),
		Messages:    toWireMessages(msgs),
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
		Stream:      false, // ChatStream 中为 true
		Tools:       toWireTools(opts.Tools),
	}
```

② 非流式响应解析：`chatResponse` 的 Message 字段改为 wire 类型并转换回 ports（`Chat` 末尾）：

```go
type chatResponse struct {
	Choices []struct {
		Message wireMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func fromWireMessage(m wireMessage) *ports.Message {
	out := &ports.Message{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
	for _, c := range m.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, ports.ToolCall{ID: c.ID, Name: c.Function.Name, Arguments: c.Function.Arguments})
	}
	return out
}
```

`Chat` 最后一行改为 `return fromWireMessage(parsed.Choices[0].Message), nil`。

③ 流式 delta 类型与聚合器：

```go
type streamToolCallDelta struct {
	Index    int `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type streamChoice struct {
	Delta struct {
		Content   string                `json:"content"`
		ToolCalls []streamToolCallDelta `json:"tool_calls"`
	} `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

type toolCallAggregator struct {
	order []int
	calls map[int]*ports.ToolCall
}

func newToolCallAggregator() *toolCallAggregator {
	return &toolCallAggregator{calls: map[int]*ports.ToolCall{}}
}

func (a *toolCallAggregator) ingest(deltas []streamToolCallDelta) {
	for _, d := range deltas {
		c, ok := a.calls[d.Index]
		if !ok {
			c = &ports.ToolCall{}
			a.calls[d.Index] = c
			a.order = append(a.order, d.Index)
		}
		if d.ID != "" {
			c.ID = d.ID
		}
		if d.Function.Name != "" {
			c.Name = d.Function.Name
		}
		c.Arguments += d.Function.Arguments
	}
}

// flush 返回聚合完成的调用并清空聚合器。
func (a *toolCallAggregator) flush() []ports.ToolCall {
	if len(a.order) == 0 {
		return nil
	}
	out := make([]ports.ToolCall, 0, len(a.order))
	for _, idx := range a.order {
		out = append(out, *a.calls[idx])
	}
	a.order = nil
	a.calls = map[int]*ports.ToolCall{}
	return out
}
```

④ 重写 `ChatStream` 的 goroutine 循环体（替换原 scan 循环与 `parseStreamPayload`，删除旧的 `parseStreamPayload` 函数）：

```go
	out := make(chan ports.StreamChunk)
	go func() {
		defer close(out)
		defer resp.Body.Close()

		emit := func(chunk ports.StreamChunk) bool {
			select {
			case out <- chunk:
				return true
			case <-ctx.Done():
				return false
			}
		}
		agg := newToolCallAggregator()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "[DONE]" {
				// 有的兼容实现不发 finish_reason，兜底 flush
				if calls := agg.flush(); len(calls) > 0 {
					if !emit(ports.StreamChunk{ToolCalls: calls}) {
						return
					}
				}
				emit(ports.StreamChunk{Done: true})
				return
			}

			var parsed streamResponse
			if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
				emit(ports.StreamChunk{Err: err})
				return
			}
			if parsed.Error != nil {
				emit(ports.StreamChunk{Err: fmt.Errorf("llm stream error: %s", parsed.Error.Message)})
				return
			}

			chunk := ports.StreamChunk{Usage: parsed.Usage}
			if len(parsed.Choices) > 0 {
				chunk.Text = parsed.Choices[0].Delta.Content
				agg.ingest(parsed.Choices[0].Delta.ToolCalls)
				if parsed.Choices[0].FinishReason != nil {
					chunk.ToolCalls = agg.flush()
					chunk.Done = true
				}
			}
			if chunk.Text == "" && !chunk.Done && chunk.Usage == nil && len(chunk.ToolCalls) == 0 {
				continue
			}
			if !emit(chunk) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			emit(ports.StreamChunk{Err: err})
		}
	}()
	return out, nil
```

- [x] **Step 4: 跑测试确认通过（含既有用例不回归）**

Run: `cd backend && go test ./internal/infra/llm/ -v`
Expected: 全部 PASS

- [x] **Step 5: Commit**

```bash
git add internal/infra/llm/openai_compat.go internal/infra/llm/openai_compat_test.go
git commit -m "feat(llm): support OpenAI tool calling with streaming delta aggregation"
```

---

### Task 3: domain 类型 + UpdateConversationMode query

**Files:**
- Modify: `backend/internal/domain/chat.go`
- Modify: `backend/internal/service/chat_service.go`（一处编译修正）
- Modify: `backend/internal/repo/queries/conversations.sql`
- Modify: `backend/internal/repo/generated/`（sqlc generate 产物）

无 schema 迁移（`conversations.mode` 列阶段 2 已建）。纯类型 + query，以编译和既有测试验证。

- [x] **Step 1: 扩展 domain/chat.go**

常量块改为：

```go
const (
	ConversationModeRAG   = "rag"
	ConversationModeReAct = "react"
	RoleUser              = "user"
	RoleAssistant         = "assistant"
	RoleTool              = "tool"
	EvidenceNone          = "none"
	EvidenceWeak          = "weak"
	EvidenceSufficient    = "sufficient"
)
```

新增类型（放在 `Citation` 定义之后；文件需新增 import `encoding/json`）：

```go
// ToolCallStep 是持久化在 messages.tool_calls JSONB 里的单步轨迹（spec §6.2）。
type ToolCallStep struct {
	Step       int             `json:"step"`
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Thought    string          `json:"thought,omitempty"`
	Arguments  json.RawMessage `json:"arguments"`
	Result     string          `json:"result"`
	DurationMs int64           `json:"duration_ms"`
	Error      string          `json:"error,omitempty"`
}

// ToolCallEvent / ToolResultEvent 是 SSE tool_call / tool_result 事件载荷（spec §6.1）。
type ToolCallEvent struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolResultEvent struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Result     string `json:"result"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}
```

`ChatMessage` 的 ToolCalls 字段改为：

```go
	ToolCalls  []ToolCallStep `json:"tool_calls"`
```

- [x] **Step 2: 修 chat_service.go 编译错**

`rowToChatMessage` 中 `ToolCalls: []any{}` 改为 `ToolCalls: []domain.ToolCallStep{}`（`json.Unmarshal(r.ToolCalls, &msg.ToolCalls)` 行不变，历史数据 `[]` 解出空 slice）。

- [x] **Step 3: 加 UpdateConversationMode query**

`backend/internal/repo/queries/conversations.sql` 末尾追加：

```sql
-- name: UpdateConversationMode :one
UPDATE conversations
SET mode = $2, updated_at = now()
WHERE id = $1
RETURNING *;
```

- [x] **Step 4: sqlc generate + 编译 + 既有测试**

Run: `cd backend && sqlc generate && go build ./... && go test ./internal/service/`
Expected: generated 代码出现 `UpdateConversationMode`；编译与既有测试全通过

- [x] **Step 5: Commit**

```bash
git add internal/domain/chat.go internal/service/chat_service.go internal/repo/queries/conversations.sql internal/repo/generated/
git commit -m "feat(domain): add react mode, tool call step types and UpdateConversationMode query"
```

---

### Task 4: agent 包——ReAct 循环

**Files:**
- Create: `backend/internal/agent/prompt.go`
- Create: `backend/internal/agent/react_agent.go`
- Test: `backend/internal/agent/react_agent_test.go`

循环持有 llm/model/maxIterations；tools 与 sink 由每次 `Run` 传入（工具按会话构造、kbID 闭包注入，见 Task 7）。`EventSink` 是 `service.ChatStreamSink` 的结构化子集——service 的 sink 自动满足它，无需适配器，也不产生 agent→service 依赖。

- [x] **Step 1: 写 prompt.go**

```go
package agent

import (
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

const reactSystemPrompt = `You are a knowledge base agent with tools.
- Before answering any content question, call kb_retrieval to search the knowledge base. Call it again with a refined query if the first results are insufficient.
- When the user asks what documents exist in the knowledge base, call list_documents.
- Never invent facts that are not supported by tool results. If the knowledge base lacks the answer, say so.
- When you use retrieved content, include inline markers like [1] that match the numbered retrieval results.
- Answer concisely in the same language as the user.`

func BuildReActMessages(history []*domain.ChatMessage, question string) []ports.Message {
	msgs := []ports.Message{{Role: "system", Content: reactSystemPrompt}}
	for _, m := range history {
		if m.Role == domain.RoleUser || m.Role == domain.RoleAssistant {
			msgs = append(msgs, ports.Message{Role: m.Role, Content: m.Content})
		}
	}
	msgs = append(msgs, ports.Message{Role: domain.RoleUser, Content: question})
	return msgs
}
```

- [x] **Step 2: 写失败测试**

创建 `react_agent_test.go`（fake 全在本文件，不依赖 service 包）：

```go
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

// scriptedLLM 每次 ChatStream 按脚本弹出一轮 chunks，并记录收到的 msgs/opts。
type scriptedLLM struct {
	rounds  [][]ports.StreamChunk
	msgsLog [][]ports.Message
	optsLog []ports.ChatOptions
}

func (f *scriptedLLM) Chat(context.Context, []ports.Message, ports.ChatOptions) (*ports.Message, error) {
	return nil, errors.New("not implemented")
}

func (f *scriptedLLM) ChatStream(_ context.Context, msgs []ports.Message, opts ports.ChatOptions) (<-chan ports.StreamChunk, error) {
	f.msgsLog = append(f.msgsLog, msgs)
	f.optsLog = append(f.optsLog, opts)
	if len(f.rounds) == 0 {
		return nil, errors.New("scriptedLLM: no rounds left")
	}
	round := f.rounds[0]
	f.rounds = f.rounds[1:]
	out := make(chan ports.StreamChunk, len(round))
	for _, c := range round {
		out <- c
	}
	close(out)
	return out, nil
}

type fakeTool struct {
	name    string
	result  string
	err     error
	invoked []string
}

func (t *fakeTool) Name() string                      { return t.name }
func (t *fakeTool) Description() string               { return "fake " + t.name }
func (t *fakeTool) ParametersSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t *fakeTool) Invoke(_ context.Context, args string) (string, error) {
	t.invoked = append(t.invoked, args)
	return t.result, t.err
}

type recordingSink struct {
	events []string
	tokens strings.Builder
}

func (s *recordingSink) SendToken(_ context.Context, text string) error {
	s.events = append(s.events, "token")
	s.tokens.WriteString(text)
	return nil
}

func (s *recordingSink) SendToolCall(_ context.Context, ev domain.ToolCallEvent) error {
	s.events = append(s.events, "tool_call:"+ev.Name)
	return nil
}

func (s *recordingSink) SendToolResult(_ context.Context, ev domain.ToolResultEvent) error {
	s.events = append(s.events, "tool_result:"+ev.Name)
	return nil
}

func TestRunSingleToolRoundThenAnswer(t *testing.T) {
	llm := &scriptedLLM{rounds: [][]ports.StreamChunk{
		{
			{Text: "让我查一下。"},
			{ToolCalls: []ports.ToolCall{{ID: "call_1", Name: "kb_retrieval", Arguments: `{"query":"部署"}`}}, Done: true},
		},
		{
			{Text: "部署步骤是……[1]"},
			{Done: true, Usage: &ports.TokenUsage{TotalTokens: 10}},
		},
	}}
	tool := &fakeTool{name: "kb_retrieval", result: "[1] chunk content"}
	sink := &recordingSink{}

	res, err := New(llm, "test-model", 5).Run(context.Background(),
		BuildReActMessages(nil, "怎么部署？"), []ports.Tool{tool}, sink)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if res.Content != "部署步骤是……[1]" {
		t.Errorf("Content = %q", res.Content)
	}
	if len(res.Steps) != 1 {
		t.Fatalf("Steps = %d, want 1", len(res.Steps))
	}
	if res.Steps[0].Thought != "让我查一下。" {
		t.Errorf("Thought = %q", res.Steps[0].Thought)
	}
	if res.Steps[0].Result != "[1] chunk content" {
		t.Errorf("Result = %q", res.Steps[0].Result)
	}
	if res.Usage == nil || res.Usage.TotalTokens != 10 {
		t.Errorf("Usage = %+v", res.Usage)
	}

	// 第二轮 LLM 请求必须带 assistant tool_calls + tool 结果消息
	second := llm.msgsLog[1]
	last, prev := second[len(second)-1], second[len(second)-2]
	if prev.Role != domain.RoleAssistant || len(prev.ToolCalls) != 1 {
		t.Errorf("second round assistant msg = %+v", prev)
	}
	if last.Role != domain.RoleTool || last.ToolCallID != "call_1" || last.Content != "[1] chunk content" {
		t.Errorf("second round tool msg = %+v", last)
	}

	wantEvents := []string{"token", "tool_call:kb_retrieval", "tool_result:kb_retrieval", "token"}
	if strings.Join(sink.events, ",") != strings.Join(wantEvents, ",") {
		t.Errorf("events = %v, want %v", sink.events, wantEvents)
	}
	// thought 文本会作为 token 流出，但不进最终 Content
	if !strings.Contains(sink.tokens.String(), "让我查一下。") {
		t.Errorf("thought tokens not streamed: %q", sink.tokens.String())
	}
}

func TestRunFeedsToolErrorBackAndContinues(t *testing.T) {
	llm := &scriptedLLM{rounds: [][]ports.StreamChunk{
		{{ToolCalls: []ports.ToolCall{{ID: "c1", Name: "kb_retrieval", Arguments: `{}`}}, Done: true}},
		{{Text: "工具失败了，基于已知信息回答。"}, {Done: true}},
	}}
	tool := &fakeTool{name: "kb_retrieval", err: errors.New("boom")}

	res, err := New(llm, "m", 5).Run(context.Background(), BuildReActMessages(nil, "q"), []ports.Tool{tool}, &recordingSink{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if res.Steps[0].Error != "boom" {
		t.Errorf("step error = %q", res.Steps[0].Error)
	}
	toolMsg := llm.msgsLog[1][len(llm.msgsLog[1])-1]
	if !strings.Contains(toolMsg.Content, "boom") {
		t.Errorf("error not fed back to llm: %q", toolMsg.Content)
	}
}

func TestRunUnknownToolNameFeedsErrorBack(t *testing.T) {
	llm := &scriptedLLM{rounds: [][]ports.StreamChunk{
		{{ToolCalls: []ports.ToolCall{{ID: "c1", Name: "no_such_tool", Arguments: `{}`}}, Done: true}},
		{{Text: "ok"}, {Done: true}},
	}}
	res, err := New(llm, "m", 5).Run(context.Background(), BuildReActMessages(nil, "q"), []ports.Tool{}, &recordingSink{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(res.Steps[0].Error, "unknown tool") {
		t.Errorf("step error = %q", res.Steps[0].Error)
	}
}

func TestRunMaxIterationsForcesFinalAnswerWithoutTools(t *testing.T) {
	llm := &scriptedLLM{rounds: [][]ports.StreamChunk{
		{{ToolCalls: []ports.ToolCall{{ID: "c1", Name: "t", Arguments: `{}`}}, Done: true}},
		{{Text: "final"}, {Done: true}},
	}}
	tool := &fakeTool{name: "t", result: "r"}

	res, err := New(llm, "m", 2).Run(context.Background(), BuildReActMessages(nil, "q"), []ports.Tool{tool}, &recordingSink{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if res.Content != "final" {
		t.Errorf("Content = %q", res.Content)
	}
	if len(llm.optsLog[1].Tools) != 0 {
		t.Errorf("final round should not offer tools, got %d", len(llm.optsLog[1].Tools))
	}
	lastMsgs := llm.msgsLog[1]
	if !strings.Contains(lastMsgs[len(lastMsgs)-1].Content, "Tool call limit reached") {
		t.Errorf("missing limit notice, last msg = %+v", lastMsgs[len(lastMsgs)-1])
	}
}

// cancelingLLM 发出一个 token 后触发取消并结束流。
type cancelingLLM struct{ cancel context.CancelFunc }

func (f *cancelingLLM) Chat(context.Context, []ports.Message, ports.ChatOptions) (*ports.Message, error) {
	return nil, errors.New("not implemented")
}

func (f *cancelingLLM) ChatStream(context.Context, []ports.Message, ports.ChatOptions) (<-chan ports.StreamChunk, error) {
	out := make(chan ports.StreamChunk, 1)
	out <- ports.StreamChunk{Text: "partial"}
	f.cancel()
	close(out)
	return out, nil
}

func TestRunCancelDuringStreamReturnsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	llm := &cancelingLLM{cancel: cancel}
	_, err := New(llm, "m", 5).Run(ctx, BuildReActMessages(nil, "q"), nil, &recordingSink{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}
```

- [x] **Step 3: 跑测试确认失败**

Run: `cd backend && go test ./internal/agent/ -v`
Expected: 编译失败（react_agent.go 尚不存在，`New`/`Run` 未定义）

- [x] **Step 4: 实现 react_agent.go**

```go
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

// EventSink 是 service.ChatStreamSink 的结构化子集，service 的实现自动满足。
type EventSink interface {
	SendToken(ctx context.Context, text string) error
	SendToolCall(ctx context.Context, ev domain.ToolCallEvent) error
	SendToolResult(ctx context.Context, ev domain.ToolResultEvent) error
}

type Result struct {
	Content string
	Steps   []domain.ToolCallStep
	Usage   *ports.TokenUsage
}

type ReactAgent struct {
	llm           ports.LLMClient
	model         string
	maxIterations int
}

const (
	maxIterationsNotice = "Tool call limit reached. Answer now using the information you already have; do not request any more tool calls."
	resultMaxRunes      = 2000 // SSE 与持久化统一截断长度（spec §6.1/§6.2）
)

func New(llm ports.LLMClient, model string, maxIterations int) *ReactAgent {
	if maxIterations < 1 {
		maxIterations = 5
	}
	return &ReactAgent{llm: llm, model: model, maxIterations: maxIterations}
}

func (a *ReactAgent) Run(ctx context.Context, msgs []ports.Message, tools []ports.Tool, sink EventSink) (*Result, error) {
	res := &Result{}
	var usage ports.TokenUsage
	var usageSeen bool

	for iter := 1; iter <= a.maxIterations; iter++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		opts := ports.ChatOptions{Model: a.model, Temperature: 0.2}
		if iter < a.maxIterations {
			opts.Tools = definitions(tools)
		} else {
			// 最终轮不给工具，强制文字收尾（spec §4.1）
			msgs = append(msgs, ports.Message{Role: "system", Content: maxIterationsNotice})
		}

		stream, err := a.llm.ChatStream(ctx, msgs, opts)
		if err != nil {
			return nil, err
		}
		var text strings.Builder
		var calls []ports.ToolCall
		for chunk := range stream {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if chunk.Err != nil {
				return nil, chunk.Err
			}
			if chunk.Usage != nil {
				usageSeen = true
				usage.PromptTokens += chunk.Usage.PromptTokens
				usage.CompletionTokens += chunk.Usage.CompletionTokens
				usage.TotalTokens += chunk.Usage.TotalTokens
			}
			if chunk.Text != "" {
				text.WriteString(chunk.Text)
				if err := sink.SendToken(ctx, chunk.Text); err != nil {
					return nil, err
				}
			}
			calls = append(calls, chunk.ToolCalls...)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if len(calls) == 0 {
			res.Content = strings.TrimSpace(text.String())
			if res.Content == "" {
				return nil, fmt.Errorf("llm stream completed without content")
			}
			if usageSeen {
				res.Usage = &usage
			}
			return res, nil
		}

		thought := strings.TrimSpace(text.String())
		msgs = append(msgs, ports.Message{Role: domain.RoleAssistant, Content: thought, ToolCalls: calls})
		for i, call := range calls {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if err := sink.SendToolCall(ctx, domain.ToolCallEvent{ID: call.ID, Name: call.Name, Arguments: call.Arguments}); err != nil {
				return nil, err
			}

			start := time.Now()
			resultText, invokeErr := invokeTool(ctx, tools, call)
			elapsed := time.Since(start).Milliseconds()
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			step := domain.ToolCallStep{
				Step:       len(res.Steps) + 1,
				ID:         call.ID,
				Name:       call.Name,
				Arguments:  normalizeArguments(call.Arguments),
				DurationMs: elapsed,
			}
			if i == 0 {
				step.Thought = thought
			}
			ev := domain.ToolResultEvent{ID: call.ID, Name: call.Name, DurationMs: elapsed}
			if invokeErr != nil {
				// 工具失败不中断循环：错误作为 result 回喂 LLM（spec §4.1）
				step.Error = invokeErr.Error()
				ev.Error = invokeErr.Error()
				resultText = fmt.Sprintf(`{"error":%q}`, invokeErr.Error())
			} else {
				step.Result = truncateRunes(resultText, resultMaxRunes)
				ev.Result = step.Result
			}
			if err := sink.SendToolResult(ctx, ev); err != nil {
				return nil, err
			}
			res.Steps = append(res.Steps, step)
			msgs = append(msgs, ports.Message{Role: domain.RoleTool, Content: resultText, ToolCallID: call.ID})
		}
	}
	// 最终轮不提供 tools，正常情况下不可达
	return nil, fmt.Errorf("react loop exceeded %d iterations", a.maxIterations)
}

func definitions(tools []ports.Tool) []ports.ToolDefinition {
	defs := make([]ports.ToolDefinition, 0, len(tools))
	for _, t := range tools {
		defs = append(defs, ports.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.ParametersSchema(),
		})
	}
	return defs
}

func invokeTool(ctx context.Context, tools []ports.Tool, call ports.ToolCall) (string, error) {
	for _, t := range tools {
		if t.Name() == call.Name {
			return t.Invoke(ctx, call.Arguments)
		}
	}
	return "", fmt.Errorf("unknown tool: %s", call.Name)
}

// normalizeArguments 保证写入 JSONB 的 arguments 一定是合法 JSON：
// LLM 偶发输出坏 JSON 时降级为带引号的字符串。
func normalizeArguments(raw string) json.RawMessage {
	if raw == "" {
		return json.RawMessage(`{}`)
	}
	if json.Valid([]byte(raw)) {
		return json.RawMessage(raw)
	}
	quoted, _ := json.Marshal(raw)
	return quoted
}

func truncateRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes-3]) + "..."
}
```

- [x] **Step 5: 跑测试确认通过**

Run: `cd backend && go test ./internal/agent/ -v`
Expected: 5 个用例全 PASS

- [x] **Step 6: Commit**

```bash
git add internal/agent/
git commit -m "feat(agent): add hand-rolled ReAct loop with tool trace and cancellation"
```

---

### Task 5: kb_retrieval 工具

**Files:**
- Create: `backend/internal/agent/tools/kb_retrieval.go`
- Test: `backend/internal/agent/tools/kb_retrieval_test.go`

包装 `service.Retrieval`。kbID 构造时闭包注入（不让 LLM 传）；每次成功检索后调 `onRetrieval` 回调（Task 7 中 chat_service 用它做 citation 去重收集 + 推 retrieval SSE 事件）。**依赖方向**：tools → service（允许；service 不 import tools，工厂在 main.go 组装，无循环）。

- [x] **Step 1: 写失败测试**

创建 `kb_retrieval_test.go`：

```go
package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

func newTestKBRetrieval(t *testing.T, hits []ports.VectorSearchHit, onRetrieval func(context.Context, *service.RetrievalResult) error) *KBRetrieval {
	t.Helper()
	retrieval := service.NewRetrieval(
		fakeEmbedder{dim: 1},
		fakeVectorStore{hits: hits},
		8, 0,
	)
	return NewKBRetrieval(retrieval, "kb-1", onRetrieval)
}

type fakeEmbedder struct{ dim int }

func (f fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1}
	}
	return out, nil
}
func (f fakeEmbedder) Dim() int { return f.dim }

type fakeVectorStore struct{ hits []ports.VectorSearchHit }

func (f fakeVectorStore) Upsert(context.Context, string, []ports.VectorItem) error { return nil }
func (f fakeVectorStore) Search(context.Context, string, []float32, ports.VectorSearchOptions) ([]ports.VectorSearchHit, error) {
	return f.hits, nil
}
func (f fakeVectorStore) Delete(context.Context, string, []string) error { return nil }

func TestKBRetrievalInvokeFormatsHitsAndFiresCallback(t *testing.T) {
	hits := []ports.VectorSearchHit{{
		KBID: "kb-1", ChunkID: "ch-1", DocumentID: "doc-1",
		DocumentTitle: "Runbook.md", Seq: 3, Score: 0.91,
		Content: "重启服务的步骤",
	}}
	var got *service.RetrievalResult
	tool := newTestKBRetrieval(t, hits, func(_ context.Context, r *service.RetrievalResult) error {
		got = r
		return nil
	})

	if tool.Name() != "kb_retrieval" {
		t.Errorf("Name() = %q", tool.Name())
	}
	result, err := tool.Invoke(context.Background(), `{"query":"怎么重启"}`)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	for _, want := range []string{"[1]", "Runbook.md", "重启服务的步骤"} {
		if !strings.Contains(result, want) {
			t.Errorf("result missing %q\nresult: %s", want, result)
		}
	}
	if got == nil || len(got.Hits) != 1 {
		t.Fatalf("onRetrieval not fired or wrong hits: %+v", got)
	}
}

func TestKBRetrievalInvokeRejectsBadArguments(t *testing.T) {
	tool := newTestKBRetrieval(t, nil, nil)
	for _, args := range []string{``, `not-json`, `{}`, `{"query":"  "}`} {
		if _, err := tool.Invoke(context.Background(), args); err == nil {
			t.Errorf("Invoke(%q) expected error", args)
		}
	}
}

func TestKBRetrievalInvokeNoHitsReturnsExplicitText(t *testing.T) {
	tool := newTestKBRetrieval(t, nil, func(context.Context, *service.RetrievalResult) error { return nil })
	result, err := tool.Invoke(context.Background(), `{"query":"不存在的内容"}`)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !strings.Contains(result, "no relevant content") {
		t.Errorf("result = %q, want no-hit notice", result)
	}
}
```

注意：`ports.VectorSearchHit`/`ports.VectorItem`/`ports.VectorSearchOptions` 的字段以 `internal/domain/ports/vectorstore.go` 现有定义为准（`KBID/ChunkID/DocumentID/DocumentTitle/Seq/Score/Content` 已在 citation_assembler 中使用过，签名如不符以现文件为准微调 fake）。

- [x] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/agent/tools/ -v`
Expected: 编译失败（`NewKBRetrieval` 未定义）

- [x] **Step 3: 实现 kb_retrieval.go**

```go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

// KBRetrieval 让 Agent 检索当前会话所属 KB。kbID 闭包注入，不暴露给 LLM。
// onRetrieval 在每次成功检索后触发（citation 收集 + retrieval SSE 推送由调用方实现）。
type KBRetrieval struct {
	retrieval   *service.Retrieval
	kbID        string
	onRetrieval func(ctx context.Context, r *service.RetrievalResult) error
}

func NewKBRetrieval(retrieval *service.Retrieval, kbID string, onRetrieval func(ctx context.Context, r *service.RetrievalResult) error) *KBRetrieval {
	return &KBRetrieval{retrieval: retrieval, kbID: kbID, onRetrieval: onRetrieval}
}

func (t *KBRetrieval) Name() string { return "kb_retrieval" }

func (t *KBRetrieval) Description() string {
	return "Search the knowledge base for content relevant to a query. Returns numbered passages with document titles. Call again with a refined query if results are insufficient."
}

func (t *KBRetrieval) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Search query in the same language as the documents"}
		},
		"required": ["query"]
	}`)
}

type kbRetrievalArgs struct {
	Query string `json:"query"`
}

func (t *KBRetrieval) Invoke(ctx context.Context, argsJSON string) (string, error) {
	var args kbRetrievalArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	args.Query = strings.TrimSpace(args.Query)
	if args.Query == "" {
		return "", fmt.Errorf("query is required")
	}

	r, err := t.retrieval.Retrieve(ctx, t.kbID, args.Query)
	if err != nil {
		return "", err
	}
	if t.onRetrieval != nil {
		if err := t.onRetrieval(ctx, r); err != nil {
			return "", err
		}
	}
	if len(r.Hits) == 0 {
		return "The knowledge base has no relevant content for this query.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "evidence_level: %s\n", r.EvidenceLevel)
	for i, hit := range r.Hits {
		fmt.Fprintf(&b, "[%d] document: %s (seq %d, score %.4f)\n%s\n\n",
			i+1, hit.DocumentTitle, hit.Seq, hit.Score, hit.Content)
	}
	return b.String(), nil
}
```

- [x] **Step 4: 跑测试确认通过**

Run: `cd backend && go test ./internal/agent/tools/ -v`
Expected: 3 个用例全 PASS

- [x] **Step 5: Commit**

```bash
git add internal/agent/tools/
git commit -m "feat(agent): add kb_retrieval tool wrapping retrieval service"
```

---

### Task 6: list_documents 工具

**Files:**
- Create: `backend/internal/agent/tools/list_documents.go`
- Test: `backend/internal/agent/tools/list_documents_test.go`

包装 `service.Document.ListByKB`（签名：`ListByKB(ctx, kbID string, statusFilter *string, limit, offset int) ([]*domain.Document, int, error)`）。用本包定义的窄接口解耦，测试无需真 DB。上限 50 条，超出在结果中注明总数（spec §5.2）。

- [x] **Step 1: 写失败测试**

创建 `list_documents_test.go`：

```go
package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

type fakeDocLister struct {
	docs      []*domain.Document
	total     int
	gotKBID   string
	gotStatus *string
}

func (f *fakeDocLister) ListByKB(_ context.Context, kbID string, statusFilter *string, _, _ int) ([]*domain.Document, int, error) {
	f.gotKBID = kbID
	f.gotStatus = statusFilter
	return f.docs, f.total, nil
}

func TestListDocumentsFormatsDocs(t *testing.T) {
	lister := &fakeDocLister{
		docs: []*domain.Document{{
			Title: "Runbook.md", Status: domain.StatusReady, Bytes: 2048,
			UpdatedAt: time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC),
		}},
		total: 1,
	}
	tool := NewListDocuments(lister, "kb-1")

	if tool.Name() != "list_documents" {
		t.Errorf("Name() = %q", tool.Name())
	}
	result, err := tool.Invoke(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if lister.gotKBID != "kb-1" {
		t.Errorf("kbID = %q", lister.gotKBID)
	}
	for _, want := range []string{"Runbook.md", "ready", "2.0 KB"} {
		if !strings.Contains(result, want) {
			t.Errorf("result missing %q\nresult: %s", want, result)
		}
	}
}

func TestListDocumentsPassesStatusFilter(t *testing.T) {
	lister := &fakeDocLister{}
	tool := NewListDocuments(lister, "kb-1")
	if _, err := tool.Invoke(context.Background(), `{"status":"failed"}`); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if lister.gotStatus == nil || *lister.gotStatus != "failed" {
		t.Errorf("statusFilter = %v, want failed", lister.gotStatus)
	}
}

func TestListDocumentsRejectsInvalidStatus(t *testing.T) {
	tool := NewListDocuments(&fakeDocLister{}, "kb-1")
	if _, err := tool.Invoke(context.Background(), `{"status":"bogus"}`); err == nil {
		t.Error("expected error for invalid status")
	}
}

func TestListDocumentsNotesTruncationWhenTotalExceedsLimit(t *testing.T) {
	docs := make([]*domain.Document, 50)
	for i := range docs {
		docs[i] = &domain.Document{Title: "d", Status: domain.StatusReady, UpdatedAt: time.Now()}
	}
	lister := &fakeDocLister{docs: docs, total: 120}
	result, err := NewListDocuments(lister, "kb-1").Invoke(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !strings.Contains(result, "120") {
		t.Errorf("result should mention total 120: %s", result)
	}
}
```

（`fakeDocLister` 用了 `time.Now()`——这是 Go 测试代码，允许。）

- [x] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/agent/tools/ -run TestListDocuments -v`
Expected: 编译失败（`NewListDocuments` 未定义）

- [x] **Step 3: 实现 list_documents.go**

```go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

const listDocumentsLimit = 50

// DocumentLister 是 service.Document 的窄接口（*service.Document 自动满足）。
type DocumentLister interface {
	ListByKB(ctx context.Context, kbID string, statusFilter *string, limit, offset int) ([]*domain.Document, int, error)
}

// ListDocuments 让 Agent 列出当前 KB 内的文档清单——回答
// “知识库里有哪些文档”这类向量检索无法回答的元问题。
type ListDocuments struct {
	docs DocumentLister
	kbID string
}

func NewListDocuments(docs DocumentLister, kbID string) *ListDocuments {
	return &ListDocuments{docs: docs, kbID: kbID}
}

func (t *ListDocuments) Name() string { return "list_documents" }

func (t *ListDocuments) Description() string {
	return "List the documents in the knowledge base with title, ingestion status, size and last update time. Use when the user asks what documents exist."
}

func (t *ListDocuments) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"status": {
				"type": "string",
				"enum": ["pending", "parsing", "chunking", "embedding", "ready", "failed"],
				"description": "Optional: only list documents with this ingestion status"
			}
		}
	}`)
}

type listDocumentsArgs struct {
	Status string `json:"status"`
}

var validDocStatuses = map[string]bool{
	"pending": true, "parsing": true, "chunking": true,
	"embedding": true, "ready": true, "failed": true,
}

func (t *ListDocuments) Invoke(ctx context.Context, argsJSON string) (string, error) {
	var args listDocumentsArgs
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
	}
	var statusFilter *string
	if s := strings.TrimSpace(args.Status); s != "" {
		if !validDocStatuses[s] {
			return "", fmt.Errorf("invalid status: %s", s)
		}
		statusFilter = &s
	}

	docs, total, err := t.docs.ListByKB(ctx, t.kbID, statusFilter, listDocumentsLimit, 0)
	if err != nil {
		return "", err
	}
	if len(docs) == 0 {
		return "The knowledge base has no documents matching the filter.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d document(s) in the knowledge base:\n", total)
	for i, d := range docs {
		fmt.Fprintf(&b, "%d. %s — status: %s, size: %s, updated: %s\n",
			i+1, d.Title, d.Status, humanBytes(d.Bytes), d.UpdatedAt.Format("2006-01-02 15:04"))
	}
	if total > len(docs) {
		fmt.Fprintf(&b, "(showing first %d of %d)\n", len(docs), total)
	}
	return b.String(), nil
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
```

- [x] **Step 4: 跑测试确认通过**

Run: `cd backend && go test ./internal/agent/tools/ -v`
Expected: kb_retrieval + list_documents 用例全 PASS

- [x] **Step 5: Commit**

```bash
git add internal/agent/tools/
git commit -m "feat(agent): add list_documents tool for KB meta questions"
```

---

### Task 7: chat_service mode 分发 + main.go 接线

**Files:**
- Modify: `backend/internal/service/chat_service.go`
- Create: `backend/internal/service/chat_react.go`
- Modify: `backend/cmd/server/main.go`
- Test: `backend/internal/service/chat_service_test.go`（扩展既有 fakes + 新用例）

service 层收口：`ChatStreamSink` 扩展两个 tool 事件方法（从而自动满足 `agent.EventSink`）；`AskStream` 按 mode 分发；react 路径在新文件 `chat_react.go`（citation 去重收集 + 委托 agent.Run + 持久化）；工具经 `AgentToolFactory` 注入（main.go 组装，service 不 import agent/tools，避免循环依赖）。

- [x] **Step 1: 写失败测试**

在 `chat_service_test.go` 追加（既有 fakes 的适配放 Step 2）：

```go
// scriptedChatLLM 与 agent 包测试里的 scriptedLLM 相同思路：按轮弹出 chunks。
type scriptedChatLLM struct {
	rounds [][]ports.StreamChunk
}

func (f *scriptedChatLLM) Chat(context.Context, []ports.Message, ports.ChatOptions) (*ports.Message, error) {
	return nil, errors.New("not implemented")
}

func (f *scriptedChatLLM) ChatStream(context.Context, []ports.Message, ports.ChatOptions) (<-chan ports.StreamChunk, error) {
	if len(f.rounds) == 0 {
		return nil, errors.New("no rounds left")
	}
	round := f.rounds[0]
	f.rounds = f.rounds[1:]
	out := make(chan ports.StreamChunk, len(round))
	for _, c := range round {
		out <- c
	}
	close(out)
	return out, nil
}

// callbackTool 模拟 kb_retrieval：Invoke 时触发 onRetrieval。
type callbackTool struct{ onRetrieval RetrievalCallback }

func (t *callbackTool) Name() string                      { return "kb_retrieval" }
func (t *callbackTool) Description() string               { return "fake" }
func (t *callbackTool) ParametersSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t *callbackTool) Invoke(ctx context.Context, _ string) (string, error) {
	err := t.onRetrieval(ctx, &RetrievalResult{
		EvidenceLevel: domain.EvidenceSufficient,
		Hits: []ports.VectorSearchHit{{
			KBID: "kb", ChunkID: "ch-1", DocumentID: "d1",
			DocumentTitle: "T.md", Seq: 1, Score: 0.9, Content: "内容",
		}},
	})
	return "[1] 内容", err
}

func newReActChat(queries *fakeChatQueries, llm ports.LLMClient) *Chat {
	factory := func(kbID string, onRetrieval RetrievalCallback) []ports.Tool {
		return []ports.Tool{&callbackTool{onRetrieval: onRetrieval}}
	}
	return NewChat(queries, nil, llm, "test-model", 0, agent.New(llm, "test-model", 5), factory)
}

func TestChatAskStreamReActPersistsStepsAndCitations(t *testing.T) {
	convID := uuid.New()
	queries := &fakeChatQueries{
		events: &eventLog{},
		conversation: generated.Conversation{
			ID: convID, KbID: uuid.New(), Mode: domain.ConversationModeReAct,
		},
	}
	llm := &scriptedChatLLM{rounds: [][]ports.StreamChunk{
		{{ToolCalls: []ports.ToolCall{{ID: "call_1", Name: "kb_retrieval", Arguments: `{"query":"部署"}`}}, Done: true}},
		{{Text: "答案 [1]"}, {Done: true, Usage: &ports.TokenUsage{TotalTokens: 7}}},
	}}
	svc := newReActChat(queries, llm)
	sink := &recordingSink{events: queries.events}

	if err := svc.AskStream(context.Background(), convID.String(), "怎么部署", sink); err != nil {
		t.Fatalf("AskStream() error = %v", err)
	}

	if len(queries.createdMessages) != 2 {
		t.Fatalf("created %d messages, want 2", len(queries.createdMessages))
	}
	assistant := queries.createdMessages[1]
	if assistant.Role != domain.RoleAssistant || assistant.Content != "答案 [1]" {
		t.Errorf("assistant = %+v", assistant)
	}
	toolCalls := string(assistant.ToolCalls)
	for _, want := range []string{`"name":"kb_retrieval"`, `"step":1`, `"result":"[1] 内容"`} {
		if !strings.Contains(toolCalls, want) {
			t.Errorf("tool_calls missing %s\ngot: %s", want, toolCalls)
		}
	}
	if !strings.Contains(string(assistant.Citations), `"chunk_id":"ch-1"`) {
		t.Errorf("citations = %s", assistant.Citations)
	}

	got := strings.Join(queries.events.items, ",")
	for _, want := range []string{"tool_call:kb_retrieval", "tool_result:kb_retrieval", "retrieval", "done"} {
		if !strings.Contains(got, want) {
			t.Errorf("events missing %s\ngot: %s", want, got)
		}
	}
}

func TestChatAskStreamReActCancelDoesNotPersistAssistant(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	convID := uuid.New()
	queries := &fakeChatQueries{
		events: &eventLog{},
		conversation: generated.Conversation{
			ID: convID, KbID: uuid.New(), Mode: domain.ConversationModeReAct,
		},
	}
	svc := newReActChat(queries, blockingLLM{})
	sink := &cancelOnTokenSink{recordingSink: recordingSink{events: queries.events}, cancel: cancel}

	err := svc.AskStream(ctx, convID.String(), "q", sink)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("AskStream() error = %v, want context.Canceled", err)
	}
	for _, m := range queries.createdMessages {
		if m.Role == domain.RoleAssistant {
			t.Fatalf("assistant persisted after cancel: %q", m.Content)
		}
	}
}

func TestCreateConversationWithMode(t *testing.T) {
	queries := &fakeChatQueries{}
	svc := NewChat(queries, nil, &fakeLLM{}, "m", 0, nil, nil)

	conv, err := svc.CreateConversation(context.Background(), uuid.NewString(), domain.ConversationModeReAct)
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	if conv.Mode != domain.ConversationModeReAct {
		t.Errorf("mode = %q", conv.Mode)
	}

	if _, err := svc.CreateConversation(context.Background(), uuid.NewString(), "bogus"); !errors.Is(err, ErrInvalidMode) {
		t.Errorf("expected ErrInvalidMode, got %v", err)
	}
	// 空 mode 默认 rag
	conv, err = svc.CreateConversation(context.Background(), uuid.NewString(), "")
	if err != nil || conv.Mode != domain.ConversationModeRAG {
		t.Errorf("default mode = %q, err = %v", conv.Mode, err)
	}
}

func TestUpdateModeValidatesEnum(t *testing.T) {
	queries := &fakeChatQueries{conversation: generated.Conversation{ID: uuid.New()}}
	svc := NewChat(queries, nil, &fakeLLM{}, "m", 0, nil, nil)

	if _, err := svc.UpdateMode(context.Background(), queries.conversation.ID.String(), "bogus"); !errors.Is(err, ErrInvalidMode) {
		t.Errorf("expected ErrInvalidMode, got %v", err)
	}
	conv, err := svc.UpdateMode(context.Background(), queries.conversation.ID.String(), domain.ConversationModeReAct)
	if err != nil {
		t.Fatalf("UpdateMode() error = %v", err)
	}
	if conv.Mode != domain.ConversationModeReAct {
		t.Errorf("mode = %q", conv.Mode)
	}
}
```

- [x] **Step 2: 适配既有 fakes**

同文件：

1. `fakeChatQueries` 增加方法：

```go
func (f *fakeChatQueries) UpdateConversationMode(_ context.Context, arg generated.UpdateConversationModeParams) (generated.Conversation, error) {
	f.conversation.Mode = arg.Mode
	return f.conversation, nil
}
```

2. `recordingSink` 增加两个方法（满足扩展后的 ChatStreamSink）：

```go
func (s *recordingSink) SendToolCall(_ context.Context, ev domain.ToolCallEvent) error {
	s.events.add("tool_call:" + ev.Name)
	return nil
}

func (s *recordingSink) SendToolResult(_ context.Context, ev domain.ToolResultEvent) error {
	s.events.add("tool_result:" + ev.Name)
	return nil
}
```

3. 既有 `NewChat(queries, retrieval, llm, "...", n)` 调用点全部追加 `, nil, nil`（RAG 用例不需要 agent）；`CreateConversation(ctx, kbID)` 调用点追加 `, ""`。
4. 测试文件 import 增加 `"github.com/zenith-wang/it-wiki/backend/internal/agent"`。

- [x] **Step 3: 跑测试确认失败**

Run: `cd backend && go test ./internal/service/ -v`
Expected: 编译失败（NewChat 签名、UpdateMode、ErrInvalidMode、RetrievalCallback 均未定义）

- [x] **Step 4: 改 chat_service.go**

1. `ChatStreamSink` 接口增加（`SendToken` 之后）：

```go
	SendToolCall(ctx context.Context, ev domain.ToolCallEvent) error
	SendToolResult(ctx context.Context, ev domain.ToolResultEvent) error
```

2. `ChatQueries` 接口增加：

```go
	UpdateConversationMode(ctx context.Context, arg generated.UpdateConversationModeParams) (generated.Conversation, error)
```

3. 新类型与错误（`ErrConversationNotFound` 附近）：

```go
// ErrInvalidMode 表示 mode 不在 rag/react 枚举内。
var ErrInvalidMode = errors.New("mode must be 'rag' or 'react'")

// RetrievalCallback 在 Agent 每次 kb_retrieval 成功后触发。
type RetrievalCallback func(ctx context.Context, r *RetrievalResult) error

// AgentToolFactory 按会话构造工具集（kbID 闭包注入）。由 main.go 组装，
// 避免 service ↔ agent/tools 循环依赖。
type AgentToolFactory func(kbID string, onRetrieval RetrievalCallback) []ports.Tool
```

4. `Chat` 结构体与构造函数：

```go
type Chat struct {
	queries         ChatQueries
	retrieval       *Retrieval
	llm             ports.LLMClient
	llmModel        string
	historyMessages int
	reactAgent      *agent.ReactAgent
	toolFactory     AgentToolFactory
}

func NewChat(q ChatQueries, retrieval *Retrieval, llm ports.LLMClient, llmModel string, historyMessages int, reactAgent *agent.ReactAgent, toolFactory AgentToolFactory) *Chat {
	if historyMessages < 0 {
		historyMessages = 10
	}
	return &Chat{queries: q, retrieval: retrieval, llm: llm, llmModel: llmModel,
		historyMessages: historyMessages, reactAgent: reactAgent, toolFactory: toolFactory}
}
```

（import 增加 `"github.com/zenith-wang/it-wiki/backend/internal/agent"`。）

5. `CreateConversation` 带 mode：

```go
func (s *Chat) CreateConversation(ctx context.Context, kbID, mode string) (*domain.Conversation, error) {
	if mode == "" {
		mode = domain.ConversationModeRAG
	}
	if mode != domain.ConversationModeRAG && mode != domain.ConversationModeReAct {
		return nil, ErrInvalidMode
	}
	kbUUID, err := uuid.Parse(kbID)
	if err != nil {
		return nil, &ErrKBNotFound{ID: kbID}
	}
	row, err := s.queries.CreateConversation(ctx, generated.CreateConversationParams{
		KbID:   kbUUID,
		Title:  "New chat",
		Mode:   mode,
		UserID: localUserID,
	})
	if err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	return rowToConversation(row), nil
}
```

6. 新增 `UpdateMode`：

```go
func (s *Chat) UpdateMode(ctx context.Context, conversationID, mode string) (*domain.Conversation, error) {
	if mode != domain.ConversationModeRAG && mode != domain.ConversationModeReAct {
		return nil, ErrInvalidMode
	}
	convID, err := uuid.Parse(conversationID)
	if err != nil {
		return nil, &ErrConversationNotFound{ID: conversationID}
	}
	row, err := s.queries.UpdateConversationMode(ctx, generated.UpdateConversationModeParams{ID: convID, Mode: mode})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ErrConversationNotFound{ID: conversationID}
		}
		return nil, fmt.Errorf("update conversation mode: %w", err)
	}
	return rowToConversation(row), nil
}
```

7. `AskStream` 改为分发（用户消息持久化留在公共段，之后按 mode 走分支；原 mode 检查删除）：

```go
func (s *Chat) AskStream(ctx context.Context, conversationID, content string, sink ChatStreamSink) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("content is required")
	}
	convID, err := uuid.Parse(conversationID)
	if err != nil {
		return &ErrConversationNotFound{ID: conversationID}
	}
	conv, err := s.queries.GetConversation(ctx, convID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &ErrConversationNotFound{ID: conversationID}
		}
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv.Mode != domain.ConversationModeRAG && conv.Mode != domain.ConversationModeReAct {
		return fmt.Errorf("unsupported conversation mode: %s", conv.Mode)
	}

	if _, err := s.createMessage(ctx, conv.ID, domain.RoleUser, content, nil, nil, nil); err != nil {
		return err
	}
	if err := s.queries.TouchConversation(ctx, conv.ID); err != nil {
		return fmt.Errorf("touch conversation after user message: %w", err)
	}

	if conv.Mode == domain.ConversationModeReAct {
		return s.askReAct(ctx, conv, content, sink)
	}
	return s.askRAG(ctx, conv, content, sink)
}
```

8. 原 AskStream 中检索之后的整段逻辑原样抽为 `askRAG(ctx context.Context, conv generated.Conversation, content string, sink ChatStreamSink) error`（内容不变，仅 `s.createMessage(...)` 调用处适配新签名：assistant 持久化传 `retrieval.Citations, nil, usage`）。

9. `createMessage` 增加 steps 参数：

```go
func (s *Chat) createMessage(ctx context.Context, convID uuid.UUID, role, content string, citations []domain.Citation, steps []domain.ToolCallStep, usage *ports.TokenUsage) (*domain.ChatMessage, error) {
```

内部在 usageJSON 之前加：

```go
	toolCallsJSON := []byte("[]")
	if len(steps) > 0 {
		b, err := json.Marshal(steps)
		if err != nil {
			return nil, fmt.Errorf("marshal tool calls: %w", err)
		}
		toolCallsJSON = b
	}
```

`CreateMessageParams` 的 `ToolCalls: []byte("[]")` 改为 `ToolCalls: toolCallsJSON`。

- [x] **Step 5: 新建 chat_react.go**

```go
package service

import (
	"context"
	"fmt"

	"github.com/zenith-wang/it-wiki/backend/internal/agent"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

func (s *Chat) askReAct(ctx context.Context, conv generated.Conversation, content string, sink ChatStreamSink) error {
	if s.reactAgent == nil || s.toolFactory == nil {
		return fmt.Errorf("react mode is not configured")
	}
	history, err := s.recentHistory(ctx, conv.ID)
	if err != nil {
		return err
	}
	msgs := agent.BuildReActMessages(history, content)

	// 跨多次 kb_retrieval 按 chunk_id 去重，citations 全量重编号（spec D4）
	seen := map[string]bool{}
	var allHits []ports.VectorSearchHit
	var citations []domain.Citation
	onRetrieval := func(cbCtx context.Context, r *RetrievalResult) error {
		for _, hit := range r.Hits {
			if seen[hit.ChunkID] {
				continue
			}
			seen[hit.ChunkID] = true
			allHits = append(allHits, hit)
		}
		citations = BuildCitations(allHits)
		return sink.SendRetrieval(cbCtx, &RetrievalResult{
			EvidenceLevel: r.EvidenceLevel,
			Citations:     citations,
		})
	}
	tools := s.toolFactory(conv.KbID.String(), onRetrieval)

	result, err := s.reactAgent.Run(ctx, msgs, tools, sink)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		_ = sink.SendError(ctx, ChatStreamError{Code: "llm_stream_failed", Message: err.Error()})
		return err
	}
	// 持久化前必须再查取消状态，避免落半截消息（spec §4.1，沿用 2.5 保障）
	if err := ctx.Err(); err != nil {
		return err
	}

	assistant, err := s.createMessage(ctx, conv.ID, domain.RoleAssistant, result.Content, citations, result.Steps, result.Usage)
	if err != nil {
		_ = sink.SendError(ctx, ChatStreamError{Code: "assistant_persist_failed", Message: err.Error()})
		return err
	}
	if err := s.queries.TouchConversation(ctx, conv.ID); err != nil {
		return fmt.Errorf("touch conversation after assistant message: %w", err)
	}
	return sink.SendDone(ctx, ChatDone{MessageID: assistant.ID, ConversationID: conv.ID.String(), Usage: result.Usage})
}
```

- [x] **Step 6: main.go 接线**

`cmd/server/main.go` import 增加：

```go
	"github.com/zenith-wang/it-wiki/backend/internal/agent"
	agenttools "github.com/zenith-wang/it-wiki/backend/internal/agent/tools"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
```

`chatSvc :=` 一行替换为：

```go
	reactAgent := agent.New(llmClient, cfg.LLMModel, 5)
	agentToolFactory := func(kbID string, onRetrieval service.RetrievalCallback) []ports.Tool {
		return []ports.Tool{
			agenttools.NewKBRetrieval(retrievalSvc, kbID, onRetrieval),
			agenttools.NewListDocuments(docSvc, kbID),
		}
	}
	chatSvc := service.NewChat(queries, retrievalSvc, llmClient, cfg.LLMModel, cfg.RAGHistoryMessages, reactAgent, agentToolFactory)
```

- [x] **Step 7: 补 httpChatSink 两个方法（保持全仓可编译）**

`ChatStreamSink` 接口扩展会让 `internal/http` 编译失败。在 `backend/internal/http/chat_handler.go` 的 `httpChatSink` 追加（SSE 事件名即 spec §6.1 的两个新事件）：

```go
func (s *httpChatSink) SendToolCall(_ context.Context, ev domain.ToolCallEvent) error {
	s.ensureStarted()
	return WriteSSEEvent(s.w, "tool_call", ev)
}

func (s *httpChatSink) SendToolResult(_ context.Context, ev domain.ToolResultEvent) error {
	s.ensureStarted()
	return WriteSSEEvent(s.w, "tool_result", ev)
}
```

（import 增加 `"github.com/zenith-wang/it-wiki/backend/internal/domain"`。）

同文件 `CreateConversation` handler 因 service 签名变更需同步：`h.svc.CreateConversation(r.Context(), kbID)` 临时改为 `h.svc.CreateConversation(r.Context(), kbID, "")`（Task 8 再升级为解析可选 body）。

- [x] **Step 8: 跑测试确认通过**

Run: `cd backend && go build ./... && go test ./internal/service/ -v`
Expected: 全仓编译通过，新旧用例全 PASS

- [x] **Step 9: Commit**

```bash
git add internal/service/ internal/http/chat_handler.go cmd/server/main.go
git commit -m "feat(chat): dispatch by conversation mode with ReAct path and tool factory"
```

---

### Task 8: HTTP 层——PATCH mode + 端到端 SSE 验证

**Files:**
- Modify: `backend/internal/http/chat_handler.go`
- Modify: `backend/internal/http/router.go`
- Create: `backend/internal/http/chat_react_e2e_test.go`

PATCH `/api/v1/conversations/{conversationID}`（仅 mode 字段）；POST 创建会话支持可选 body `{mode}`（现前端不传 body，须容忍空 body）。端到端测试用 httptest 起真 router，fakes 全部实现 service 导出接口（`service.ChatQueries`、`ports.LLMClient` 等都是导出的，http 包测试可直接实现）。

- [x] **Step 1: 写失败测试**

创建 `chat_react_e2e_test.go`（package `http`；fakes 与 service 包测试同构但跨包不可复用，此处独立完整实现）：

```go
package http

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/zenith-wang/it-wiki/backend/internal/agent"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

// fakeE2EQueries 实现 service.ChatQueries（HTTP 测试有并发访问，加锁）。
type fakeE2EQueries struct {
	mu              sync.Mutex
	conversation    generated.Conversation
	createdMessages []generated.CreateMessageParams
}

func (f *fakeE2EQueries) CreateConversation(_ context.Context, arg generated.CreateConversationParams) (generated.Conversation, error) {
	return generated.Conversation{
		ID: uuid.New(), KbID: arg.KbID, Title: arg.Title, Mode: arg.Mode,
		UserID: arg.UserID, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}, nil
}

func (f *fakeE2EQueries) GetConversation(context.Context, uuid.UUID) (generated.Conversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.conversation, nil
}

func (f *fakeE2EQueries) ListConversationsByKB(context.Context, generated.ListConversationsByKBParams) ([]generated.Conversation, error) {
	return nil, nil
}

func (f *fakeE2EQueries) CountConversationsByKB(context.Context, generated.CountConversationsByKBParams) (int64, error) {
	return 0, nil
}

func (f *fakeE2EQueries) CreateMessage(_ context.Context, arg generated.CreateMessageParams) (generated.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createdMessages = append(f.createdMessages, arg)
	return generated.Message{
		ID: uuid.New(), ConversationID: arg.ConversationID, Role: arg.Role,
		Content: arg.Content, Citations: arg.Citations, ToolCalls: arg.ToolCalls,
		TokenUsage: arg.TokenUsage, CreatedAt: time.Now(),
	}, nil
}

func (f *fakeE2EQueries) ListMessagesByConversation(context.Context, generated.ListMessagesByConversationParams) ([]generated.Message, error) {
	return nil, nil
}

func (f *fakeE2EQueries) CountMessagesByConversation(context.Context, uuid.UUID) (int64, error) {
	return 0, nil
}

func (f *fakeE2EQueries) ListRecentMessagesByConversation(context.Context, generated.ListRecentMessagesByConversationParams) ([]generated.Message, error) {
	return nil, nil
}

func (f *fakeE2EQueries) TouchConversation(context.Context, uuid.UUID) error { return nil }

func (f *fakeE2EQueries) UpdateConversationMode(_ context.Context, arg generated.UpdateConversationModeParams) (generated.Conversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.conversation.Mode = arg.Mode
	return f.conversation, nil
}

func (f *fakeE2EQueries) assistantMessages() []generated.CreateMessageParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []generated.CreateMessageParams
	for _, m := range f.createdMessages {
		if m.Role == domain.RoleAssistant {
			out = append(out, m)
		}
	}
	return out
}

// scriptedChatLLM 按轮弹出 chunks（并发安全）。
type scriptedChatLLM struct {
	mu     sync.Mutex
	rounds [][]ports.StreamChunk
}

func (f *scriptedChatLLM) Chat(context.Context, []ports.Message, ports.ChatOptions) (*ports.Message, error) {
	return nil, errors.New("not implemented")
}

func (f *scriptedChatLLM) ChatStream(context.Context, []ports.Message, ports.ChatOptions) (<-chan ports.StreamChunk, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.rounds) == 0 {
		return nil, errors.New("no rounds left")
	}
	round := f.rounds[0]
	f.rounds = f.rounds[1:]
	out := make(chan ports.StreamChunk, len(round))
	for _, c := range round {
		out <- c
	}
	close(out)
	return out, nil
}

// callbackTool 模拟 kb_retrieval：Invoke 时触发 onRetrieval。
type callbackTool struct{ onRetrieval service.RetrievalCallback }

func (t *callbackTool) Name() string                      { return "kb_retrieval" }
func (t *callbackTool) Description() string               { return "fake" }
func (t *callbackTool) ParametersSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t *callbackTool) Invoke(ctx context.Context, _ string) (string, error) {
	err := t.onRetrieval(ctx, &service.RetrievalResult{
		EvidenceLevel: domain.EvidenceSufficient,
		Hits: []ports.VectorSearchHit{{
			KBID: "kb", ChunkID: "ch-1", DocumentID: "d1",
			DocumentTitle: "T.md", Seq: 1, Score: 0.9, Content: "内容",
		}},
	})
	return "[1] 内容", err
}
```

（import 补 `"encoding/json"`。）测试主体：

```go
func newReActTestServer(t *testing.T, queries service.ChatQueries, llm ports.LLMClient) *httptest.Server {
	t.Helper()
	factory := func(kbID string, onRetrieval service.RetrievalCallback) []ports.Tool {
		return []ports.Tool{&callbackTool{onRetrieval: onRetrieval}}
	}
	chatSvc := service.NewChat(queries, nil, llm, "test-model", 0, agent.New(llm, "test-model", 5), factory)
	router := NewRouter(Handlers{Chat: NewChatHandler(chatSvc)})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func TestStreamReActEmitsToolEventsEndToEnd(t *testing.T) {
	convID := uuid.New()
	queries := &fakeE2EQueries{conversation: generated.Conversation{
		ID: convID, KbID: uuid.New(), Mode: domain.ConversationModeReAct,
	}}
	llm := &scriptedChatLLM{rounds: [][]ports.StreamChunk{
		{{ToolCalls: []ports.ToolCall{{ID: "call_1", Name: "kb_retrieval", Arguments: `{"query":"x"}`}}, Done: true}},
		{{Text: "答案 [1]"}, {Done: true}},
	}}
	srv := newReActTestServer(t, queries, llm)

	resp, err := http.Post(
		srv.URL+"/api/v1/conversations/"+convID.String()+"/messages/stream",
		"application/json",
		strings.NewReader(`{"content":"怎么部署"}`),
	)
	if err != nil {
		t.Fatalf("POST stream: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}

	var events []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			events = append(events, strings.TrimPrefix(line, "event: "))
		}
	}
	got := strings.Join(events, ",")
	for _, want := range []string{"tool_call", "tool_result", "retrieval", "token", "done"} {
		if !strings.Contains(got, want) {
			t.Errorf("events missing %s\ngot: %s", want, got)
		}
	}
	// tool_call 必须先于 done 出现
	if strings.Index(got, "tool_call") > strings.Index(got, "done") {
		t.Errorf("tool_call after done: %s", got)
	}
}

func TestPatchConversationModeEndToEnd(t *testing.T) {
	convID := uuid.New()
	queries := &fakeE2EQueries{conversation: generated.Conversation{
		ID: convID, KbID: uuid.New(), Mode: domain.ConversationModeRAG,
	}}
	srv := newReActTestServer(t, queries, &scriptedChatLLM{})

	// 合法切换
	req, _ := http.NewRequest(http.MethodPatch,
		srv.URL+"/api/v1/conversations/"+convID.String(),
		strings.NewReader(`{"mode":"react"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// 非法 mode → 400 validation_failed
	req2, _ := http.NewRequest(http.MethodPatch,
		srv.URL+"/api/v1/conversations/"+convID.String(),
		strings.NewReader(`{"mode":"bogus"}`))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp2.StatusCode)
	}
}

func TestCreateConversationAcceptsOptionalMode(t *testing.T) {
	queries := &fakeE2EQueries{}
	srv := newReActTestServer(t, queries, &scriptedChatLLM{})
	kbID := uuid.NewString()

	// 无 body（现有前端行为）→ 201 默认 rag
	resp, err := http.Post(srv.URL+"/api/v1/kbs/"+kbID+"/conversations", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("no-body status = %d, want 201", resp.StatusCode)
	}

	// 带 mode=react → 201
	resp2, err := http.Post(srv.URL+"/api/v1/kbs/"+kbID+"/conversations", "application/json",
		strings.NewReader(`{"mode":"react"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		t.Errorf("react status = %d, want 201", resp2.StatusCode)
	}
}
```

另外补：本文件里的 e2e 取消验证（对齐 2.5 保障，客户端断连 → 不落 assistant）：

```go
func TestStreamReActClientDisconnectDoesNotPersistAssistant(t *testing.T) {
	convID := uuid.New()
	queries := &fakeE2EQueries{conversation: generated.Conversation{
		ID: convID, KbID: uuid.New(), Mode: domain.ConversationModeReAct,
	}}
	llm := &tokenThenBlockLLM{unblock: make(chan struct{})}
	srv := newReActTestServer(t, queries, llm)

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		srv.URL+"/api/v1/conversations/"+convID.String()+"/messages/stream",
		strings.NewReader(`{"content":"q"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	// 读到第一个 token 后断开
	buf := make([]byte, 1)
	_, _ = resp.Body.Read(buf)
	cancel()
	resp.Body.Close()
	close(llm.unblock)
	llm.wait()
	time.Sleep(50 * time.Millisecond) // 给 handler 收尾余量；确定性保障由服务级取消测试覆盖

	for _, m := range queries.assistantMessages() {
		t.Fatalf("assistant persisted after disconnect: %q", m.Content)
	}
}

// tokenThenBlockLLM 发一个 token 后阻塞在 ctx 上，模拟慢速 LLM；
// done 供测试等待流 goroutine 结束。
type tokenThenBlockLLM struct {
	unblock chan struct{}
	done    chan struct{}
}

func (f *tokenThenBlockLLM) Chat(context.Context, []ports.Message, ports.ChatOptions) (*ports.Message, error) {
	return nil, nil
}

func (f *tokenThenBlockLLM) ChatStream(ctx context.Context, _ []ports.Message, _ ports.ChatOptions) (<-chan ports.StreamChunk, error) {
	f.done = make(chan struct{})
	out := make(chan ports.StreamChunk)
	go func() {
		defer close(out)
		defer close(f.done)
		select {
		case out <- ports.StreamChunk{Text: "partial"}:
		case <-ctx.Done():
			return
		}
		select {
		case <-ctx.Done():
		case <-f.unblock:
		}
	}()
	return out, nil
}

func (f *tokenThenBlockLLM) wait() {
	if f.done != nil {
		<-f.done
	}
}
```

（`fakeE2EQueries`/`scriptedChatLLM`/`callbackTool` 的完整实现见本 Step 开头的第一个代码块。）

- [x] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/http/ -v`
Expected: 编译失败（`UpdateConversation` handler、PATCH 路由、createConversationRequest 未实现；`NewRouter(Handlers{Chat: ...})` 若 Handlers 其他字段为 nil 导致 panic，则在 router 注册处允许 nil handler 或测试里补零值 handler——以现 router.go 实现为准，必要时给 Handlers 补齐空实现）

- [x] **Step 3: 实现 handler + 路由**

`chat_handler.go`：

1. `CreateConversation` 支持可选 body：

```go
type createConversationRequest struct {
	Mode string `json:"mode"`
}

func (h *ChatHandler) CreateConversation(w http.ResponseWriter, r *http.Request) {
	var req createConversationRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			WriteError(w, r, NewAPIError(http.StatusBadRequest, CodeValidationFailed, "invalid JSON body"))
			return
		}
	}
	kbID := chi.URLParam(r, "kbID")
	conv, err := h.svc.CreateConversation(r.Context(), kbID, req.Mode)
	if err != nil {
		WriteError(w, r, mapChatError(err))
		return
	}
	WriteJSON(w, http.StatusCreated, conv)
}
```

（import 增加 `"io"`。）

2. 新增 `UpdateConversation`：

```go
type updateConversationRequest struct {
	Mode string `json:"mode"`
}

func (h *ChatHandler) UpdateConversation(w http.ResponseWriter, r *http.Request) {
	var req updateConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, r, NewAPIError(http.StatusBadRequest, CodeValidationFailed, "invalid JSON body"))
		return
	}
	conv, err := h.svc.UpdateMode(r.Context(), chi.URLParam(r, "conversationID"), req.Mode)
	if err != nil {
		WriteError(w, r, mapChatError(err))
		return
	}
	WriteJSON(w, http.StatusOK, conv)
}
```

3. `mapChatError` 增加 mode 校验分支（`errors.As` 之前）：

```go
	if errors.Is(err, service.ErrInvalidMode) {
		return NewAPIError(http.StatusBadRequest, CodeValidationFailed, err.Error())
	}
```

`router.go` 会话路由组追加：

```go
		r.Patch("/conversations/{conversationID}", h.Chat.UpdateConversation)
```

- [x] **Step 4: 跑测试确认通过**

Run: `cd backend && go test ./internal/http/ -v && go test ./...`
Expected: http 新用例 + 全仓测试 PASS

- [x] **Step 5: Commit**

```bash
git add internal/http/
git commit -m "feat(api): add PATCH conversation mode and react SSE end-to-end coverage"
```

---

### Task 9: 前端 schemas + API 客户端

**Files:**
- Modify: `frontend/lib/schemas/index.ts`
- Modify: `frontend/lib/api/chat.ts`

纯类型/客户端方法，以 `pnpm typecheck` 验证。

- [x] **Step 1: schemas 扩展**

`lib/schemas/index.ts`：

1. `conversationSchema` 的 mode 从 `z.literal("rag")` 改为：

```ts
  mode: z.enum(["rag", "react"]),
```

2. `chatMessageSchema` 之前新增 step schema，并把 `tool_calls` 收紧：

```ts
export const toolCallStepSchema = z.object({
  step: z.number(),
  id: z.string(),
  name: z.string(),
  thought: z.string().optional(),
  arguments: z.unknown().optional(),
  result: z.string().optional(),
  duration_ms: z.number().optional(),
  error: z.string().optional(),
});
export type ToolCallStep = z.infer<typeof toolCallStepSchema>;
```

`chatMessageSchema` 中：

```ts
  tool_calls: z.array(toolCallStepSchema).default([]),
```

- [x] **Step 2: chat.ts 扩展**

1. `ChatStreamEvent` 联合类型追加两个成员（spec §6.1）：

```ts
  | { event: "tool_call"; data: { id: string; name: string; arguments: string } }
  | {
      event: "tool_result";
      data: { id: string; name: string; result?: string; duration_ms: number; error?: string };
    }
```

2. `chatApi` 追加：

```ts
  updateConversationMode(conversationId: string, mode: "rag" | "react") {
    return apiFetch<Conversation>(`/api/v1/conversations/${conversationId}`, {
      method: "PATCH",
      body: JSON.stringify({ mode }),
    });
  },
```

（`apiFetch` 对非 FormData body 自动加 `Content-Type: application/json`，无需手动传 headers。）

- [x] **Step 3: 类型检查**

Run: `cd frontend && pnpm typecheck`
Expected: PASS（`use-chat-stream.ts` 若因 `tool_calls` 类型收紧报错，属预期，Task 10 消除；若报错则本步只要求错误仅来自该文件）

- [x] **Step 4: Commit**

```bash
git add lib/schemas/index.ts lib/api/chat.ts
git commit -m "feat(frontend): add react mode schema and tool call stream events"
```

---

### Task 10: use-chat-stream 处理 tool 事件

**Files:**
- Modify: `frontend/lib/hooks/use-chat-stream.ts`
- Test: `frontend/lib/hooks/use-chat-stream.test.ts`

核心行为（spec §4.2/§8）：`tool_call` 事件把 draft 已积累的 content 挪为该 step 的 thought 并清空气泡（thought 是流式转发过的思考文本）；`tool_result` 按 id 落定对应 step；`retrieval` 在 Agent 模式下会多次到达，沿用整体替换（零改动）。

- [x] **Step 1: 写失败测试**

`use-chat-stream.test.ts` 追加（沿用文件现有 `baseState`/`applyStreamEvent` 风格）：

```ts
describe("applyStreamEvent tool events", () => {
  it("moves draft content to thought and appends running step on tool_call", () => {
    const withContent = applyStreamEvent(baseState, "draft-assistant", {
      event: "token",
      data: { text: "让我查一下。" },
    });
    const next = applyStreamEvent(withContent, "draft-assistant", {
      event: "tool_call",
      data: { id: "call_1", name: "kb_retrieval", arguments: '{"query":"部署"}' },
    });

    const draft = next.messages[0];
    expect(draft.content).toBe("");
    expect(draft.tool_calls).toHaveLength(1);
    const step = draft.tool_calls[0] as LocalToolStep;
    expect(step).toMatchObject({
      step: 1,
      id: "call_1",
      name: "kb_retrieval",
      thought: "让我查一下。",
      running: true,
    });
    expect(step.arguments).toEqual({ query: "部署" });
  });

  it("keeps thought empty for subsequent calls in the same round", () => {
    let state = applyStreamEvent(baseState, "draft-assistant", {
      event: "tool_call",
      data: { id: "call_1", name: "kb_retrieval", arguments: "{}" },
    });
    state = applyStreamEvent(state, "draft-assistant", {
      event: "tool_call",
      data: { id: "call_2", name: "list_documents", arguments: "{}" },
    });
    const steps = state.messages[0].tool_calls as LocalToolStep[];
    expect(steps).toHaveLength(2);
    expect(steps[1].thought).toBeUndefined();
    expect(steps[1].step).toBe(2);
  });

  it("settles the matching step on tool_result", () => {
    let state = applyStreamEvent(baseState, "draft-assistant", {
      event: "tool_call",
      data: { id: "call_1", name: "kb_retrieval", arguments: "{}" },
    });
    state = applyStreamEvent(state, "draft-assistant", {
      event: "tool_result",
      data: { id: "call_1", name: "kb_retrieval", result: "[1] 内容", duration_ms: 840 },
    });
    const step = (state.messages[0].tool_calls as LocalToolStep[])[0];
    expect(step).toMatchObject({ result: "[1] 内容", duration_ms: 840, running: false });
  });

  it("records tool error on tool_result with error", () => {
    let state = applyStreamEvent(baseState, "draft-assistant", {
      event: "tool_call",
      data: { id: "call_1", name: "kb_retrieval", arguments: "{}" },
    });
    state = applyStreamEvent(state, "draft-assistant", {
      event: "tool_result",
      data: { id: "call_1", name: "kb_retrieval", duration_ms: 5, error: "boom" },
    });
    const step = (state.messages[0].tool_calls as LocalToolStep[])[0];
    expect(step.error).toBe("boom");
    expect(step.running).toBe(false);
  });

  it("keeps non-JSON arguments as raw string", () => {
    const state = applyStreamEvent(baseState, "draft-assistant", {
      event: "tool_call",
      data: { id: "call_1", name: "kb_retrieval", arguments: "not-json" },
    });
    const step = (state.messages[0].tool_calls as LocalToolStep[])[0];
    expect(step.arguments).toBe("not-json");
  });
});
```

（import 增加 `type LocalToolStep`。）

- [x] **Step 2: 跑测试确认失败**

Run: `cd frontend && pnpm test`
Expected: 新用例 FAIL（tool_call/tool_result 事件落进 applyStreamEvent 兜底分支被忽略）

- [x] **Step 3: 实现**

`use-chat-stream.ts`：

1. import 类型改为 `import type { ChatMessage, Citation, ToolCallStep } from "@/lib/schemas";`，新增导出类型：

```ts
export interface LocalToolStep extends ToolCallStep {
  running?: boolean;
}
```

`LocalChatMessage` 增加字段覆盖（保持 `Omit` 写法）：

```ts
export interface LocalChatMessage extends Omit<ChatMessage, "id" | "created_at" | "tool_calls"> {
  id: string;
  created_at: string;
  tool_calls: LocalToolStep[];
  pending?: boolean;
  error?: string;
}
```

2. `applyStreamEvent` 在 `token` 分支之后追加两个分支：

```ts
  if (event.event === "tool_call") {
    return {
      ...state,
      messages: state.messages.map((message) => {
        if (message.id !== draftId) return message;
        const thought = message.content.trim();
        const step: LocalToolStep = {
          step: message.tool_calls.length + 1,
          id: event.data.id,
          name: event.data.name,
          arguments: safeParseJson(event.data.arguments),
          ...(thought ? { thought } : {}),
          running: true,
        };
        // 思考文本已流式展示过，收进轨迹后清空气泡（spec §4.2）
        return { ...message, content: "", tool_calls: [...message.tool_calls, step] };
      }),
    };
  }
  if (event.event === "tool_result") {
    return {
      ...state,
      messages: state.messages.map((message) => {
        if (message.id !== draftId) return message;
        return {
          ...message,
          tool_calls: message.tool_calls.map((step) =>
            step.id === event.data.id
              ? {
                  ...step,
                  result: event.data.result,
                  duration_ms: event.data.duration_ms,
                  error: event.data.error,
                  running: false,
                }
              : step,
          ),
        };
      }),
    };
  }
```

3. 文件底部加 helper：

```ts
function safeParseJson(raw: string): unknown {
  try {
    return JSON.parse(raw);
  } catch {
    return raw;
  }
}
```

4. 若 `localMessage` 或初始化处对 `tool_calls: []` 的类型有报错，按 `LocalToolStep[]` 空数组处理（字面量 `[]` 即可）。

- [x] **Step 4: 跑测试 + 类型检查确认通过**

Run: `cd frontend && pnpm test && pnpm typecheck`
Expected: 全 PASS

- [x] **Step 5: Commit**

```bash
git add lib/hooks/use-chat-stream.ts lib/hooks/use-chat-stream.test.ts
git commit -m "feat(frontend): handle tool_call/tool_result stream events with thought capture"
```

---

### Task 11: ChatHeader mode 切换

**Files:**
- Modify: `frontend/components/chat/chat-header.tsx`
- Modify: `frontend/app/kbs/[kbId]/chats/[conversationId]/page.tsx`

分段控件（两个按钮，沿用现有 border/bg-muted 风格，不引新 shadcn 组件）；PATCH mutation 成功后 invalidate `["conversations", kbId]`（页面的 conversation 即来自该 query）；流式期间禁用；失败走 sonner toast（spec §6.2 错误显式）。

- [x] **Step 1: 改 chat-header.tsx**

整文件替换为：

```tsx
"use client";

import Link from "next/link";
import { ArrowLeft, Files } from "lucide-react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import { chatApi } from "@/lib/api/chat";
import { cn } from "@/lib/utils";
import type { Conversation } from "@/lib/schemas";

const MODES: { value: Conversation["mode"]; label: string }[] = [
  { value: "rag", label: "RAG" },
  { value: "react", label: "Agent" },
];

export function ChatHeader({
  kbId,
  conversation,
  disabled = false,
}: {
  kbId: string;
  conversation?: Conversation | null;
  disabled?: boolean;
}) {
  const queryClient = useQueryClient();
  const modeMutation = useMutation({
    mutationFn: (mode: Conversation["mode"]) =>
      chatApi.updateConversationMode(conversation!.id, mode),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["conversations", kbId] }),
    onError: (error) => toast.error(`Failed to switch mode: ${(error as Error).message}`),
  });

  const subtitle = conversation
    ? conversation.mode === "react"
      ? "ReAct Agent"
      : "Deterministic RAG"
    : "Select or create a chat";

  return (
    <header className="flex min-h-14 items-center justify-between gap-3 border-b px-4">
      <div className="min-w-0">
        <h1 className="truncate text-base font-semibold">{conversation?.title ?? "Chat"}</h1>
        <p className="truncate text-xs text-muted-foreground">{subtitle}</p>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        {conversation && (
          <div className="flex items-center rounded-md border border-border p-0.5" role="group" aria-label="Chat mode">
            {MODES.map(({ value, label }) => (
              <button
                key={value}
                type="button"
                disabled={disabled || modeMutation.isPending || conversation.mode === value}
                onClick={() => modeMutation.mutate(value)}
                className={cn(
                  "h-7 rounded px-2.5 text-xs font-medium transition-colors",
                  conversation.mode === value
                    ? "bg-primary text-primary-foreground"
                    : "text-muted-foreground hover:bg-muted disabled:opacity-50",
                )}
              >
                {label}
              </button>
            ))}
          </div>
        )}
        <Link
          href={`/kbs/${kbId}/docs`}
          className="inline-flex h-8 items-center gap-1 rounded-md border border-border bg-background px-2.5 text-sm font-medium hover:bg-muted"
        >
          <Files className="size-4" />
          Docs
        </Link>
        <Link
          href="/"
          className="inline-flex h-8 items-center gap-1 rounded-md border border-border bg-background px-2.5 text-sm font-medium hover:bg-muted"
        >
          <ArrowLeft className="size-4" />
          KBs
        </Link>
      </div>
    </header>
  );
}
```

- [x] **Step 2: 页面传 disabled**

`app/kbs/[kbId]/chats/[conversationId]/page.tsx` 中：

```tsx
          <ChatHeader kbId={kbId} conversation={conversation} disabled={chat.isStreaming} />
```

- [x] **Step 3: 验证**

Run: `cd frontend && pnpm typecheck && pnpm lint`
Expected: PASS

- [x] **Step 4: Commit**

```bash
git add components/chat/chat-header.tsx "app/kbs/[kbId]/chats/[conversationId]/page.tsx"
git commit -m "feat(frontend): add RAG/Agent mode switch in chat header"
```

---

### Task 12: tool-call-trace 组件 + 消息渲染

**Files:**
- Create: `frontend/components/chat/tool-call-trace.tsx`
- Modify: `frontend/components/chat/message-bubble.tsx`

轨迹用原生 `<details>/<summary>` 做折叠（零新依赖，样式贴现有风格；阶段 3.5 统一刷视觉）。折叠态：图标 + 工具名 + 参数摘要 + 耗时/spinner/错误标记；展开：thought、arguments、result。历史消息 `tool_calls` 非空即渲染，刷新后可回看（spec D3）。

- [x] **Step 1: 新建 tool-call-trace.tsx**

```tsx
"use client";

import { CircleAlert, LoaderCircle, Wrench } from "lucide-react";

import { cn } from "@/lib/utils";
import type { LocalToolStep } from "@/lib/hooks/use-chat-stream";

function argsSummary(args: unknown): string {
  const text = typeof args === "string" ? args : JSON.stringify(args ?? {});
  return text.length > 60 ? text.slice(0, 57) + "..." : text;
}

function StepRow({ step }: { step: LocalToolStep }) {
  return (
    <details className="group rounded-md border bg-muted/40 text-xs">
      <summary className="flex cursor-pointer list-none items-center gap-2 px-2.5 py-1.5 [&::-webkit-details-marker]:hidden">
        {step.running ? (
          <LoaderCircle className="size-3.5 shrink-0 animate-spin text-muted-foreground" />
        ) : step.error ? (
          <CircleAlert className="size-3.5 shrink-0 text-destructive" />
        ) : (
          <Wrench className="size-3.5 shrink-0 text-muted-foreground" />
        )}
        <span className="font-medium">{step.name}</span>
        <span className="truncate text-muted-foreground">{argsSummary(step.arguments)}</span>
        <span className={cn("ml-auto shrink-0 text-muted-foreground", step.error && "text-destructive")}>
          {step.running ? "running..." : step.error ? "failed" : `${step.duration_ms ?? 0} ms`}
        </span>
      </summary>
      <div className="space-y-2 border-t px-2.5 py-2">
        {step.thought && <p className="whitespace-pre-wrap text-muted-foreground">{step.thought}</p>}
        <div>
          <p className="mb-1 font-medium text-muted-foreground">arguments</p>
          <pre className="overflow-x-auto rounded bg-muted p-2">{JSON.stringify(step.arguments, null, 2)}</pre>
        </div>
        {step.error ? (
          <div>
            <p className="mb-1 font-medium text-destructive">error</p>
            <pre className="overflow-x-auto rounded bg-muted p-2 text-destructive">{step.error}</pre>
          </div>
        ) : (
          step.result !== undefined && (
            <div>
              <p className="mb-1 font-medium text-muted-foreground">result</p>
              <pre className="max-h-48 overflow-auto whitespace-pre-wrap rounded bg-muted p-2">{step.result}</pre>
            </div>
          )
        )}
      </div>
    </details>
  );
}

export function ToolCallTrace({ steps }: { steps: LocalToolStep[] }) {
  if (steps.length === 0) return null;
  return (
    <div className="mb-2 space-y-1">
      {steps.map((step) => (
        <StepRow key={step.id || step.step} step={step} />
      ))}
    </div>
  );
}
```

- [x] **Step 2: message-bubble 接入**

`message-bubble.tsx` 的 assistant 分支，在 Markdown 容器 `<div className="leading-relaxed ...">` 之前插入：

```tsx
            <ToolCallTrace steps={message.tool_calls} />
```

（assistant 分支需把原来的单一子元素包成 fragment：`<>` `<ToolCallTrace .../>` `<div ...>...</div>` `</>`；import 增加 `import { ToolCallTrace } from "@/components/chat/tool-call-trace";`。）

注意 pending 占位：Agent 模式下 content 被挪进 thought 后气泡可能为空，`message.pending ? "Thinking..." : ""` 的现有兜底保持不变即可。

- [x] **Step 3: 验证**

Run: `cd frontend && pnpm typecheck && pnpm lint && pnpm test`
Expected: 全 PASS

- [x] **Step 4: Commit**

```bash
git add components/chat/tool-call-trace.tsx components/chat/message-bubble.tsx
git commit -m "feat(frontend): render collapsible tool call trace in messages"
```

---

### Task 13: 文档同步 + 全量验证

**Files:**
- Modify: `docs/superpowers/specs/2026-05-19-it-wiki-agent-design.md`
- Modify: `docs/superpowers/specs/2026-07-08-phase-3-react-agent-design.md`
- Modify: `CLAUDE.md`

- [x] **Step 1: 主 spec 三处标注**

`2026-05-19-it-wiki-agent-design.md`：

1. §1 后端技术栈表 Eino 行，理由列追加：`（阶段 3 决策：MVP 手写 ReAct 循环未引入 Eino，推迟到 V1.5 再评估，见阶段 3 spec D1）`
2. §3 「Eino 的定位」标题下第一行加：`> 阶段 3 实施注记（2026-07）：未引入 Eino。react_agent 为手写循环（internal/agent/），tools 实现 ports.Tool 接口。本节保留原设想供 V1.5 评估。`
3. §7 阶段 3 小节：`tools 注册：kb_retrieval、calculator` 改为 `tools 注册：kb_retrieval、list_documents（阶段 3 spec D5：calculator 无业务价值，换为可回答"知识库里有哪些文档"的 list_documents）`；验收行 `kb_retrieval + calculator` 同步改为 `kb_retrieval + list_documents`。§10 验收清单不动（表述仍适用）。

- [x] **Step 2: 阶段 3 spec 校正 SSE 示例**

`2026-07-08-phase-3-react-agent-design.md` §6.1 的 `tool_call` 示例中 `"arguments":"{...}"`（字符串）改为与实现一致的原样字符串透传说明：`data: {"id":"call_x","name":"kb_retrieval","arguments":"{\"query\":\"部署\"}"}`，并加一句 `arguments 为 LLM 原样输出的 JSON 字符串，前端负责 parse（坏 JSON 降级原文展示）`。

- [x] **Step 3: CLAUDE.md 三处**

1. §2 当前阶段：更新为阶段 3 已完成（附本 spec/plan 链接与验收结论），下一阶段改为 **阶段 3.5（UI 视觉升级，参考 argus 与 do-write 的 UI，需先 brainstorm 出 spec）**
2. §3 版本表 Eino 行改为：`| Eino | 未引入（阶段 3 D1 决策：手写 ReAct 循环，V1.5 再评估） |`
3. §5.4 标题「添加 Eino tool」改为「添加 agent tool」，流程改为：

```
1. 在 backend/internal/agent/tools/ 新建 xxx.go，实现 ports.Tool 接口
2. 在 cmd/server/main.go 的 agentToolFactory 注册
3. 前端 tool-call-trace 无需改动（按 name/arguments/result 通用渲染）；如需专属展示再加分支
```

- [x] **Step 4: 全量验证（后端 + 前端）**

```bash
cd backend && go build ./... && go test ./...
cd ../frontend && pnpm lint && pnpm typecheck && pnpm test && pnpm build
```

Expected: 全绿。

- [x] **Step 5: 手动验收（对照 spec §11，需 .env 与远程 DB/MinIO 可用）**

```bash
cd backend && set -a; . ../.env; set +a && go run ./cmd/server   # PORT=8088
cd frontend && pnpm dev
```

1. 已有文档的 KB 中新建对话 → header 切到 Agent → 提问内容类问题 → 看到 kb_retrieval 轨迹（spinner→落定）+ 流式答案 + 可点击 citations
2. 问「知识库里有哪些文档」→ 走 list_documents 正确回答
3. 中途切回 RAG 再提问 → 行为回到 RAG；历史 Agent 消息轨迹不变
4. 刷新页面 → 历史轨迹仍可展开
5. 流式中点 Stop → 无半截 assistant 消息落库

- [x] **Step 6: Commit**

```bash
git add docs/ CLAUDE.md
git commit -m "docs: sync phase 3 decisions into main spec and CLAUDE.md"
```

---

## 自审结论（spec 覆盖对照）

| spec 章节 | 对应任务 |
|---|---|
| §3.1/§3.2 端口扩展 | Task 1 |
| §4.1 循环/§4.2 thought/§4.3 prompt | Task 4（thought 前端侧在 Task 10） |
| §5.1 kb_retrieval / §5.2 list_documents | Task 5 / Task 6 |
| §6.1 SSE / §6.2 持久化 | Task 2、4、7、8 |
| §7 API（PATCH/POST mode/分发） | Task 7、8 |
| §8 前端 | Task 9~12 |
| §9 文档同步 | Task 13 |
| §10 测试策略 | Task 2/4/5/6/7/8/10 内嵌 |
| §11 验收标准 | Task 13 Step 4~5 |
