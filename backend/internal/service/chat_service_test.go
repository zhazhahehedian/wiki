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
	svc := NewChat(queries, retrieval, llm, "phase-model", 10)

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
	svc := NewChat(queries, retrieval, &fakeLLM{chunks: []ports.StreamChunk{{Err: llmErr}}}, "phase-model", 10)
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
	svc := NewChat(queries, nil, &fakeLLM{}, "phase-model", 10)
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

func (s *recordingSink) Started() bool {
	return s.started
}
