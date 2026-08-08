package http

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
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

func (t *callbackTool) Name() string        { return "kb_retrieval" }
func (t *callbackTool) Description() string { return "fake" }
func (t *callbackTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
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

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newReActTestServer(t *testing.T, queries service.ChatQueries, llm ports.LLMClient) *httptest.Server {
	t.Helper()
	resolver := agent.NewRegistry()
	if err := resolver.Register(ports.DefaultAgentID, agent.New(llm, "test-model", 5)); err != nil {
		t.Fatal(err)
	}
	tools := agent.NewToolRegistry()
	if err := tools.Register("kb_retrieval", func(_ context.Context, _ string, callback ports.RetrievalCallback) (ports.Tool, error) {
		return &callbackTool{onRetrieval: callback}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := tools.RegisterAgent(ports.DefaultAgentID, "kb_retrieval"); err != nil {
		t.Fatal(err)
	}
	chatSvc := service.NewChat(queries, nil, llm, "test-model", 0, resolver, tools)
	csrf := "test-csrf"
	csrfHash := sha256.Sum256([]byte(csrf))
	authHandler := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{
		session: domain.Session{UserID: "test-user", CSRFTokenHash: csrfHash[:]},
	}, fakeUserResolver{user: domain.User{ID: "test-user"}})
	router := NewRouter(Handlers{Auth: authHandler, Chat: NewChatHandler(chatSvc)})
	srv := httptest.NewServer(router)
	baseTransport := srv.Client().Transport
	srv.Client().Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.Header = req.Header.Clone()
		clone.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "test-session"})
		if !isSafeMethod(clone.Method) {
			clone.Header.Set("Origin", "https://app.example.test")
			clone.Header.Set(CSRFHeaderName, csrf)
		}
		return baseTransport.RoundTrip(clone)
	})
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

	resp, err := srv.Client().Post(
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
	resp, err := srv.Client().Do(req)
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
	resp2, err := srv.Client().Do(req2)
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
	resp, err := srv.Client().Post(srv.URL+"/api/v1/kbs/"+kbID+"/conversations", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("no-body status = %d, want 201", resp.StatusCode)
	}

	// 带 mode=react → 201
	resp2, err := srv.Client().Post(srv.URL+"/api/v1/kbs/"+kbID+"/conversations", "application/json",
		strings.NewReader(`{"mode":"react"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		t.Errorf("react status = %d, want 201", resp2.StatusCode)
	}
}

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
	resp, err := srv.Client().Do(req)
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
