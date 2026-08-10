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
	events      []string
	tokens      strings.Builder
	toolResults []domain.ToolResultEvent
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
	s.toolResults = append(s.toolResults, ev)
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
	secret := "oauth_token=secret provider_body=private-content"
	llm := &scriptedLLM{rounds: [][]ports.StreamChunk{
		{{ToolCalls: []ports.ToolCall{{ID: "c1", Name: "kb_retrieval", Arguments: `{}`}}, Done: true}},
		{{Text: "工具失败了，基于已知信息回答。"}, {Done: true}},
	}}
	tool := &fakeTool{name: "kb_retrieval", err: errors.New(secret)}
	sink := &recordingSink{}

	res, err := New(llm, "m", 5).Run(context.Background(), BuildReActMessages(nil, "q"), []ports.Tool{tool}, sink)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if res.Steps[0].Error != "tool execution failed" {
		t.Errorf("step error = %q", res.Steps[0].Error)
	}
	toolMsg := llm.msgsLog[1][len(llm.msgsLog[1])-1]
	if !strings.Contains(toolMsg.Content, "tool execution failed") {
		t.Errorf("safe error not fed back to llm: %q", toolMsg.Content)
	}
	if len(sink.toolResults) != 1 || sink.toolResults[0].Error != "tool execution failed" {
		t.Fatalf("tool result events = %#v", sink.toolResults)
	}
	if strings.Contains(res.Steps[0].Error, secret) || strings.Contains(toolMsg.Content, secret) || strings.Contains(sink.toolResults[0].Error, secret) {
		t.Fatalf("tool cause leaked: step=%q llm=%q event=%q", res.Steps[0].Error, toolMsg.Content, sink.toolResults[0].Error)
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
	if res.Steps[0].Error != domain.ToolExecutionFailed {
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
