package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/zenith-wang/it-wiki/backend/internal/agent"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

func TestChatAskStreamPersistsUserThenRetrievalThenAssistant(t *testing.T) {
	convID := uuid.New()
	kbID := uuid.New()
	events := &eventLog{}
	queries := &fakeChatQueries{
		events: events,
		conversation: generated.Conversation{
			ID:        convID,
			KbID:      kbID,
			Title:     "New chat",
			Mode:      domain.ConversationModeRAG,
			UserID:    localUserID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	retrieval := NewRetrieval(
		&fakeEmbedder{dim: 1, vectors: [][]float32{{1}}},
		&fakeVectorStore{hits: []ports.VectorSearchHit{{
			KBID:          kbID.String(),
			ChunkID:       "chunk-1",
			DocumentID:    "doc-1",
			DocumentTitle: "Runbook.md",
			Seq:           1,
			Content:       "rotate password",
			Score:         0.95,
		}}},
		8,
		0,
	)
	llm := &fakeLLM{chunks: []ports.StreamChunk{
		{Text: "Use "},
		{Text: "the runbook.", Usage: &ports.TokenUsage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7}},
	}}
	sink := &recordingSink{events: events}
	svc := NewChat(queries, retrieval, llm, "phase-model", 10, testAgentResolver(nil), nil)

	err := svc.AskStream(context.Background(), convID.String(), " how rotate? ", sink)
	if err != nil {
		t.Fatalf("AskStream() error = %v", err)
	}

	wantEvents := []string{"message:user", "touch", "retrieval", "token", "token", "message:assistant", "touch", "done"}
	if !reflect.DeepEqual(events.items, wantEvents) {
		t.Fatalf("events = %#v, want %#v", events.items, wantEvents)
	}
	if len(queries.createdMessages) != 2 {
		t.Fatalf("created messages = %d, want 2", len(queries.createdMessages))
	}
	if queries.createdMessages[0].Role != domain.RoleUser || queries.createdMessages[0].Content != "how rotate?" {
		t.Fatalf("user message = %+v", queries.createdMessages[0])
	}
	assistant := queries.createdMessages[1]
	if assistant.Role != domain.RoleAssistant || assistant.Content != "Use the runbook." {
		t.Fatalf("assistant message = %+v", assistant)
	}
	if string(assistant.ToolCalls) != "[]" {
		t.Fatalf("tool_calls = %s, want []", assistant.ToolCalls)
	}
	var citations []domain.Citation
	if err := json.Unmarshal(assistant.Citations, &citations); err != nil {
		t.Fatalf("unmarshal citations: %v", err)
	}
	if len(citations) != 1 || citations[0].ChunkID != "chunk-1" {
		t.Fatalf("citations = %#v", citations)
	}
	var usage ports.TokenUsage
	if err := json.Unmarshal(assistant.TokenUsage, &usage); err != nil {
		t.Fatalf("unmarshal usage: %v", err)
	}
	if usage.TotalTokens != 7 {
		t.Fatalf("usage = %+v, want total 7", usage)
	}
	if llm.opts.Model != "phase-model" || llm.opts.Temperature != 0.2 {
		t.Fatalf("llm opts = %+v", llm.opts)
	}
	if len(llm.messages) < 2 || !strings.Contains(llm.messages[1].Content, "chunk_id: chunk-1") {
		t.Fatalf("llm messages missing context: %#v", llm.messages)
	}
	if sink.done.MessageID == "" || sink.done.ConversationID != convID.String() {
		t.Fatalf("done = %+v", sink.done)
	}
}

func TestChatAskStreamDoesNotPersistAssistantOnLLMFailure(t *testing.T) {
	convID := uuid.New()
	kbID := uuid.New()
	events := &eventLog{}
	queries := &fakeChatQueries{
		events: events,
		conversation: generated.Conversation{
			ID:     convID,
			KbID:   kbID,
			Mode:   domain.ConversationModeRAG,
			UserID: localUserID,
		},
	}
	retrieval := NewRetrieval(
		&fakeEmbedder{dim: 1, vectors: [][]float32{{1}}},
		&fakeVectorStore{hits: []ports.VectorSearchHit{{KBID: kbID.String(), ChunkID: "chunk-1", Score: 0.9, Content: "context"}}},
		8,
		0,
	)
	llmErr := errors.New("provider disconnected")
	svc := NewChat(queries, retrieval, &fakeLLM{chunks: []ports.StreamChunk{{Err: llmErr}}}, "phase-model", 10, testAgentResolver(nil), nil)
	sink := &recordingSink{events: events}

	err := svc.AskStream(context.Background(), convID.String(), "question", sink)
	if !errors.Is(err, llmErr) {
		t.Fatalf("AskStream() error = %v, want %v", err, llmErr)
	}

	wantEvents := []string{"message:user", "touch", "retrieval", "error:llm_stream_failed"}
	if !reflect.DeepEqual(events.items, wantEvents) {
		t.Fatalf("events = %#v, want %#v", events.items, wantEvents)
	}
	if len(queries.createdMessages) != 1 || queries.createdMessages[0].Role != domain.RoleUser {
		t.Fatalf("created messages = %#v, want only user", queries.createdMessages)
	}
}

func TestChatAskStreamReturnsNotFoundBeforeSSEStarts(t *testing.T) {
	convID := uuid.New()
	events := &eventLog{}
	queries := &fakeChatQueries{events: events, getConversationErr: pgx.ErrNoRows}
	svc := NewChat(queries, nil, &fakeLLM{}, "phase-model", 10, nil, nil)
	sink := &recordingSink{events: events}

	err := svc.AskStream(context.Background(), convID.String(), "question", sink)
	var notFound *ErrConversationNotFound
	if !errors.As(err, &notFound) {
		t.Fatalf("AskStream() error = %v, want ErrConversationNotFound", err)
	}
	if sink.Started() {
		t.Fatal("sink started before not found error")
	}
	if len(events.items) != 0 {
		t.Fatalf("events = %#v, want none", events.items)
	}
}

func TestChatAskStreamRejectsUnknownAgentBeforePersistingOrStreaming(t *testing.T) {
	convID := uuid.New()
	events := &eventLog{}
	queries := &fakeChatQueries{
		events: events,
		conversation: generated.Conversation{
			ID: convID, KbID: uuid.New(), Mode: domain.ConversationModeRAG, AgentID: "missing-agent",
		},
	}
	resolver := agent.NewRegistry()
	svc := NewChat(queries, nil, &fakeLLM{}, "phase-model", 10, resolver, nil)
	sink := &recordingSink{events: events}

	err := svc.AskStream(context.Background(), convID.String(), "question", sink)
	var unknown *agent.ErrUnknownAgent
	if !errors.As(err, &unknown) || unknown.AgentID != "missing-agent" {
		t.Fatalf("AskStream() error = %#v, want ErrUnknownAgent", err)
	}
	if sink.Started() {
		t.Fatal("sink started for unknown agent")
	}
	if len(queries.createdMessages) != 0 || len(events.items) != 0 {
		t.Fatalf("unknown agent persisted/streamed: messages=%#v events=%#v", queries.createdMessages, events.items)
	}
}

func TestChatAskStreamRejectsMissingToolProfileBeforePersistingOrStreaming(t *testing.T) {
	convID := uuid.New()
	events := &eventLog{}
	queries := &fakeChatQueries{
		events: events,
		conversation: generated.Conversation{
			ID: convID, KbID: uuid.New(), Mode: domain.ConversationModeReAct, AgentID: "known-agent",
		},
	}
	runner := &recordingAgentRunner{}
	resolver := agent.NewRegistry()
	if err := resolver.Register("known-agent", runner); err != nil {
		t.Fatal(err)
	}
	svc := NewChat(queries, nil, &fakeLLM{}, "phase-model", 10, resolver, agent.NewToolRegistry())
	sink := &recordingSink{events: events}

	err := svc.AskStream(context.Background(), convID.String(), "question", sink)
	var unknown *agent.ErrUnknownAgent
	if !errors.As(err, &unknown) || unknown.AgentID != "known-agent" {
		t.Fatalf("AskStream() error = %#v, want ErrUnknownAgent", err)
	}
	if runner.called {
		t.Fatal("runner called without a registered tool profile")
	}
	if sink.Started() || len(queries.createdMessages) != 0 || len(queries.touched) != 0 || len(events.items) != 0 {
		t.Fatalf("missing tool profile caused side effects: started=%v messages=%#v touches=%#v events=%#v",
			sink.Started(), queries.createdMessages, queries.touched, events.items)
	}
}

type recordingAgentRunner struct {
	called bool
}

func (r *recordingAgentRunner) Run(context.Context, []ports.Message, []ports.Tool, ports.AgentEventSink) (*ports.AgentResult, error) {
	r.called = true
	return &ports.AgentResult{Content: "selected agent response"}, nil
}

func testAgentResolver(runner ports.AgentRunner) *agent.Registry {
	if runner == nil {
		runner = &recordingAgentRunner{}
	}
	resolver := agent.NewRegistry()
	if err := resolver.Register(ports.DefaultAgentID, runner); err != nil {
		panic(err)
	}
	return resolver
}

func TestChatAskStreamUsesConversationAgentID(t *testing.T) {
	convID := uuid.New()
	events := &eventLog{}
	queries := &fakeChatQueries{
		events: events,
		conversation: generated.Conversation{
			ID: convID, KbID: uuid.New(), Mode: domain.ConversationModeReAct, AgentID: "special-agent",
		},
	}
	runner := &recordingAgentRunner{}
	resolver := agent.NewRegistry()
	if err := resolver.Register("special-agent", runner); err != nil {
		t.Fatal(err)
	}
	toolRegistry := agent.NewToolRegistry()
	if err := toolRegistry.RegisterAgent("special-agent"); err != nil {
		t.Fatal(err)
	}
	svc := NewChat(queries, nil, &fakeLLM{}, "phase-model", 10, resolver, toolRegistry)

	if err := svc.AskStream(context.Background(), convID.String(), "question", &recordingSink{events: events}); err != nil {
		t.Fatalf("AskStream() error = %v", err)
	}
	if !runner.called {
		t.Fatal("conversation agent runner was not called")
	}
	if len(queries.createdMessages) != 2 || queries.createdMessages[1].Content != "selected agent response" {
		t.Fatalf("created messages = %#v", queries.createdMessages)
	}
}
func TestBuildRAGMessagesIncludesNoEvidenceContext(t *testing.T) {
	msgs := BuildRAGMessages(nil, []*domain.ChatMessage{
		{Role: "tool", Content: "ignored"},
		{Role: domain.RoleUser, Content: "previous question"},
	}, "current question")

	if len(msgs) != 4 {
		t.Fatalf("len(messages) = %d, want 4: %#v", len(msgs), msgs)
	}
	if msgs[0].Role != "system" || !strings.Contains(msgs[0].Content, "deterministic knowledge base assistant") {
		t.Fatalf("system message = %+v", msgs[0])
	}
	if msgs[1].Role != "system" || !strings.Contains(msgs[1].Content, "evidence_level: none") {
		t.Fatalf("context message = %+v", msgs[1])
	}
	if msgs[2].Content != "previous question" || msgs[3].Content != "current question" {
		t.Fatalf("history/current messages = %#v", msgs[2:])
	}
}

// blockingLLM 先吐一个 token, 然后等 ctx 取消才关流,
// 模拟真实 LLM 客户端在客户端断连时结束流的行为。
type blockingLLM struct{}

func (blockingLLM) Chat(context.Context, []ports.Message, ports.ChatOptions) (*ports.Message, error) {
	return nil, errors.New("not implemented")
}

func (blockingLLM) ChatStream(ctx context.Context, _ []ports.Message, _ ports.ChatOptions) (<-chan ports.StreamChunk, error) {
	out := make(chan ports.StreamChunk, 1)
	out <- ports.StreamChunk{Text: "部分回答"}
	go func() {
		<-ctx.Done()
		close(out)
	}()
	return out, nil
}

// cancelOnTokenSink 在收到第一个 token 时取消 ctx, 模拟客户端中途断连。
type cancelOnTokenSink struct {
	recordingSink
	cancel context.CancelFunc
}

func (s *cancelOnTokenSink) SendToken(ctx context.Context, text string) error {
	s.cancel()
	return s.recordingSink.SendToken(ctx, text)
}

func TestAskStreamClientCancelDoesNotPersistAssistant(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	convID := uuid.New()
	queries := &fakeChatQueries{
		events: &eventLog{},
		conversation: generated.Conversation{
			ID:   convID,
			KbID: uuid.New(),
			Mode: domain.ConversationModeRAG,
		},
	}
	retrieval := NewRetrieval(&fakeEmbedder{dim: 1, vectors: [][]float32{{1}}}, &fakeVectorStore{}, 8, 0)
	svc := NewChat(queries, retrieval, blockingLLM{}, "test-model", 0, testAgentResolver(nil), nil)
	sink := &cancelOnTokenSink{
		recordingSink: recordingSink{events: queries.events},
		cancel:        cancel,
	}

	err := svc.AskStream(ctx, convID.String(), "问题", sink)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("AskStream() error = %v, want context.Canceled", err)
	}
	for _, m := range queries.createdMessages {
		if m.Role == domain.RoleAssistant {
			t.Fatalf("assistant message persisted after cancel: %q", m.Content)
		}
	}
	for _, e := range queries.events.items {
		if e == "done" {
			t.Fatal("done event sent after cancel")
		}
	}
}

type eventLog struct {
	items []string
}

func (l *eventLog) add(event string) {
	l.items = append(l.items, event)
}

type fakeChatQueries struct {
	events             *eventLog
	conversation       generated.Conversation
	getConversationErr error
	createdMessages    []generated.CreateMessageParams
	recentMessages     []generated.Message
	touched            []uuid.UUID
}

func (f *fakeChatQueries) CreateConversation(_ context.Context, arg generated.CreateConversationParams) (generated.Conversation, error) {
	return generated.Conversation{
		ID:        uuid.New(),
		KbID:      arg.KbID,
		Title:     arg.Title,
		Mode:      arg.Mode,
		UserID:    arg.UserID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

func (f *fakeChatQueries) GetConversation(context.Context, uuid.UUID) (generated.Conversation, error) {
	if f.getConversationErr != nil {
		return generated.Conversation{}, f.getConversationErr
	}
	return f.conversation, nil
}

func (f *fakeChatQueries) ListConversationsByKB(context.Context, generated.ListConversationsByKBParams) ([]generated.Conversation, error) {
	return []generated.Conversation{f.conversation}, nil
}

func (f *fakeChatQueries) CountConversationsByKB(context.Context, generated.CountConversationsByKBParams) (int64, error) {
	return 1, nil
}

func (f *fakeChatQueries) CreateMessage(_ context.Context, arg generated.CreateMessageParams) (generated.Message, error) {
	f.createdMessages = append(f.createdMessages, arg)
	if f.events != nil {
		f.events.add("message:" + arg.Role)
	}
	return generated.Message{
		ID:             uuid.New(),
		ConversationID: arg.ConversationID,
		Role:           arg.Role,
		Content:        arg.Content,
		Citations:      arg.Citations,
		ToolCalls:      arg.ToolCalls,
		TokenUsage:     arg.TokenUsage,
		CreatedAt:      time.Now(),
	}, nil
}

func (f *fakeChatQueries) ListMessagesByConversation(context.Context, generated.ListMessagesByConversationParams) ([]generated.Message, error) {
	return f.recentMessages, nil
}

func (f *fakeChatQueries) CountMessagesByConversation(context.Context, uuid.UUID) (int64, error) {
	return int64(len(f.recentMessages)), nil
}

func (f *fakeChatQueries) ListRecentMessagesByConversation(context.Context, generated.ListRecentMessagesByConversationParams) ([]generated.Message, error) {
	return f.recentMessages, nil
}

func (f *fakeChatQueries) TouchConversation(_ context.Context, id uuid.UUID) error {
	f.touched = append(f.touched, id)
	if f.events != nil {
		f.events.add("touch")
	}
	return nil
}

func (f *fakeChatQueries) UpdateConversationMode(_ context.Context, arg generated.UpdateConversationModeParams) (generated.Conversation, error) {
	f.conversation.Mode = arg.Mode
	return f.conversation, nil
}

type fakeLLM struct {
	messages []ports.Message
	opts     ports.ChatOptions
	chunks   []ports.StreamChunk
	err      error
}

func (f *fakeLLM) Chat(context.Context, []ports.Message, ports.ChatOptions) (*ports.Message, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLLM) ChatStream(_ context.Context, msgs []ports.Message, opts ports.ChatOptions) (<-chan ports.StreamChunk, error) {
	f.messages = msgs
	f.opts = opts
	if f.err != nil {
		return nil, f.err
	}
	out := make(chan ports.StreamChunk, len(f.chunks))
	for _, chunk := range f.chunks {
		out <- chunk
	}
	close(out)
	return out, nil
}

type recordingSink struct {
	events  *eventLog
	started bool
	done    ChatDone
	err     ChatStreamError
}

func (s *recordingSink) SendRetrieval(context.Context, *RetrievalResult) error {
	s.started = true
	s.events.add("retrieval")
	return nil
}

func (s *recordingSink) SendToken(context.Context, string) error {
	s.started = true
	s.events.add("token")
	return nil
}

func (s *recordingSink) SendDone(_ context.Context, done ChatDone) error {
	s.started = true
	s.done = done
	s.events.add("done")
	return nil
}

func (s *recordingSink) SendError(_ context.Context, err ChatStreamError) error {
	s.started = true
	s.err = err
	s.events.add("error:" + err.Code)
	return nil
}

func (s *recordingSink) SendToolCall(_ context.Context, ev domain.ToolCallEvent) error {
	s.started = true
	s.events.add("tool_call:" + ev.Name)
	return nil
}

func (s *recordingSink) SendToolResult(_ context.Context, ev domain.ToolResultEvent) error {
	s.started = true
	s.events.add("tool_result:" + ev.Name)
	return nil
}

func (s *recordingSink) Started() bool {
	return s.started
}

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

func (t *callbackTool) Name() string        { return "kb_retrieval" }
func (t *callbackTool) Description() string { return "fake" }
func (t *callbackTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
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
	resolver := testAgentResolver(agent.New(llm, "test-model", 5))
	tools := agent.NewToolRegistry()
	if err := tools.Register("kb_retrieval", func(_ context.Context, _ string, callback ports.RetrievalCallback) (ports.Tool, error) {
		return &callbackTool{onRetrieval: callback}, nil
	}); err != nil {
		panic(err)
	}
	if err := tools.RegisterAgent(ports.DefaultAgentID, "kb_retrieval"); err != nil {
		panic(err)
	}
	return NewChat(queries, nil, llm, "test-model", 0, resolver, tools)
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
