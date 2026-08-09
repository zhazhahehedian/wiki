package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type ErrConversationNotFound struct{ ID string }

func (e *ErrConversationNotFound) Error() string { return "conversation not found: " + e.ID }

// ErrInvalidMode 表示 mode 不在 rag/react 枚举内。
var ErrInvalidMode = errors.New("mode must be 'rag' or 'react'")

type RetrievalCallback = ports.RetrievalCallback
type ChatQueries interface {
	GetKnowledgeBaseForOwner(context.Context, generated.GetKnowledgeBaseForOwnerParams) (generated.KnowledgeBase, error)
	CreateConversationForOwner(context.Context, generated.CreateConversationForOwnerParams) (generated.Conversation, error)
	GetConversationForOwner(context.Context, generated.GetConversationForOwnerParams) (generated.Conversation, error)
	ListConversationsByKBForOwner(context.Context, generated.ListConversationsByKBForOwnerParams) ([]generated.Conversation, error)
	CountConversationsByKBForOwner(context.Context, generated.CountConversationsByKBForOwnerParams) (int64, error)
	CreateMessageForOwner(context.Context, generated.CreateMessageForOwnerParams) (generated.Message, error)
	ListMessagesByConversationForOwner(context.Context, generated.ListMessagesByConversationForOwnerParams) ([]generated.Message, error)
	CountMessagesByConversationForOwner(context.Context, generated.CountMessagesByConversationForOwnerParams) (int64, error)
	ListRecentMessagesByConversationForOwner(context.Context, generated.ListRecentMessagesByConversationForOwnerParams) ([]generated.Message, error)
	TouchConversationForOwner(context.Context, generated.TouchConversationForOwnerParams) error
	UpdateConversationModeForOwner(context.Context, generated.UpdateConversationModeForOwnerParams) (generated.Conversation, error)
}

type ChatStreamSink interface {
	SendRetrieval(ctx context.Context, result *RetrievalResult) error
	SendToken(ctx context.Context, text string) error
	SendToolCall(ctx context.Context, ev domain.ToolCallEvent) error
	SendToolResult(ctx context.Context, ev domain.ToolResultEvent) error
	SendDone(ctx context.Context, done ChatDone) error
	SendError(ctx context.Context, err ChatStreamError) error
	Started() bool
}

type ChatDone struct {
	MessageID      string            `json:"message_id"`
	ConversationID string            `json:"conversation_id"`
	Usage          *ports.TokenUsage `json:"usage,omitempty"`
}

type ChatStreamError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Chat struct {
	queries         ChatQueries
	retrieval       *Retrieval
	llm             ports.LLMClient
	llmModel        string
	historyMessages int
	agentResolver   ports.AgentResolver
	toolRegistry    ports.ToolRegistry
}

func NewChat(q ChatQueries, retrieval *Retrieval, llm ports.LLMClient, llmModel string, historyMessages int, agentResolver ports.AgentResolver, toolRegistry ports.ToolRegistry) *Chat {
	if historyMessages < 0 {
		historyMessages = 10
	}
	return &Chat{queries: q, retrieval: retrieval, llm: llm, llmModel: llmModel,
		historyMessages: historyMessages, agentResolver: agentResolver, toolRegistry: toolRegistry}
}
func (s *Chat) CreateConversation(ctx context.Context, userID, kbID, mode string) (*domain.Conversation, error) {
	ownerID, err := ownerUUID(userID)
	if err != nil {
		return nil, err
	}
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
	row, err := s.queries.CreateConversationForOwner(ctx, generated.CreateConversationForOwnerParams{
		KbID: kbUUID, Title: "New chat", Mode: mode,
		OwnerUserID: ownerID, AgentID: ports.DefaultAgentID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ErrKBNotFound{ID: kbID}
		}
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	return rowToConversation(row), nil
}

func (s *Chat) UpdateMode(ctx context.Context, userID, conversationID, mode string) (*domain.Conversation, error) {
	ownerID, err := ownerUUID(userID)
	if err != nil {
		return nil, err
	}
	if mode != domain.ConversationModeRAG && mode != domain.ConversationModeReAct {
		return nil, ErrInvalidMode
	}
	convID, err := uuid.Parse(conversationID)
	if err != nil {
		return nil, &ErrConversationNotFound{ID: conversationID}
	}
	row, err := s.queries.UpdateConversationModeForOwner(ctx, generated.UpdateConversationModeForOwnerParams{ID: convID, Mode: mode, OwnerUserID: ownerID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ErrConversationNotFound{ID: conversationID}
		}
		return nil, fmt.Errorf("update conversation mode: %w", err)
	}
	return rowToConversation(row), nil
}

func (s *Chat) ListConversations(ctx context.Context, userID, kbID string, limit, offset int) ([]*domain.Conversation, int, error) {
	ownerID, err := ownerUUID(userID)
	if err != nil {
		return nil, 0, err
	}
	kbUUID, err := uuid.Parse(kbID)
	if err != nil {
		return nil, 0, &ErrKBNotFound{ID: kbID}
	}
	if _, err := s.queries.GetKnowledgeBaseForOwner(ctx, generated.GetKnowledgeBaseForOwnerParams{ID: kbUUID, OwnerUserID: ownerID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, &ErrKBNotFound{ID: kbID}
		}
		return nil, 0, fmt.Errorf("get kb: %w", err)
	}
	rows, err := s.queries.ListConversationsByKBForOwner(ctx, generated.ListConversationsByKBForOwnerParams{
		KbID: kbUUID, OwnerUserID: ownerID, Limit: int32(limit), Offset: int32(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list conversations: %w", err)
	}
	total, err := s.queries.CountConversationsByKBForOwner(ctx, generated.CountConversationsByKBForOwnerParams{
		KbID: kbUUID, OwnerUserID: ownerID,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count conversations: %w", err)
	}

	out := make([]*domain.Conversation, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToConversation(row))
	}
	return out, int(total), nil
}

func (s *Chat) ListMessages(ctx context.Context, userID, conversationID string, limit, offset int) ([]*domain.ChatMessage, int, error) {
	ownerID, err := ownerUUID(userID)
	if err != nil {
		return nil, 0, err
	}
	convID, err := uuid.Parse(conversationID)
	if err != nil {
		return nil, 0, &ErrConversationNotFound{ID: conversationID}
	}
	if _, err := s.queries.GetConversationForOwner(ctx, generated.GetConversationForOwnerParams{ID: convID, OwnerUserID: ownerID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, &ErrConversationNotFound{ID: conversationID}
		}
		return nil, 0, fmt.Errorf("get conversation: %w", err)
	}

	rows, err := s.queries.ListMessagesByConversationForOwner(ctx, generated.ListMessagesByConversationForOwnerParams{
		ConversationID: convID,
		OwnerUserID:    ownerID,
		Limit:          int32(limit),
		Offset:         int32(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list messages: %w", err)
	}
	total, err := s.queries.CountMessagesByConversationForOwner(ctx, generated.CountMessagesByConversationForOwnerParams{ConversationID: convID, OwnerUserID: ownerID})
	if err != nil {
		return nil, 0, fmt.Errorf("count messages: %w", err)
	}

	out := make([]*domain.ChatMessage, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToChatMessage(row))
	}
	return out, int(total), nil
}

func (s *Chat) AskStream(ctx context.Context, userID, conversationID, content string, sink ChatStreamSink) error {
	ownerID, err := ownerUUID(userID)
	if err != nil {
		return err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("content is required")
	}
	convID, err := uuid.Parse(conversationID)
	if err != nil {
		return &ErrConversationNotFound{ID: conversationID}
	}
	conv, err := s.queries.GetConversationForOwner(ctx, generated.GetConversationForOwnerParams{ID: convID, OwnerUserID: ownerID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &ErrConversationNotFound{ID: conversationID}
		}
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv.Mode != domain.ConversationModeRAG && conv.Mode != domain.ConversationModeReAct {
		return fmt.Errorf("unsupported conversation mode: %s", conv.Mode)
	}
	if s.agentResolver == nil {
		return fmt.Errorf("agent resolver is not configured")
	}
	runner, err := s.agentResolver.Resolve(conv.AgentID)
	if err != nil {
		return err
	}
	var prepared *preparedReAct
	if conv.Mode == domain.ConversationModeReAct {
		prepared, err = s.prepareReAct(withOwnerID(ctx, userID), runner, conv, content, sink)
		if err != nil {
			return err
		}
	}
	if _, err := s.createMessage(ctx, ownerID, conv.ID, domain.RoleUser, content, nil, nil, nil); err != nil {
		return err
	}
	if err := s.queries.TouchConversationForOwner(ctx, generated.TouchConversationForOwnerParams{ID: conv.ID, OwnerUserID: ownerID}); err != nil {
		return fmt.Errorf("touch conversation after user message: %w", err)
	}

	if conv.Mode == domain.ConversationModeReAct {
		return s.askReAct(ctx, ownerID, prepared, conv, sink)
	}
	return s.askRAG(ctx, userID, ownerID, conv, content, sink)
}

func (s *Chat) askRAG(ctx context.Context, userID string, ownerID pgtype.UUID, conv generated.Conversation, content string, sink ChatStreamSink) error {
	retrieval, err := s.retrieval.Retrieve(ctx, userID, conv.KbID.String(), content)
	if err != nil {
		return err
	}
	if err := sink.SendRetrieval(ctx, retrieval); err != nil {
		return err
	}

	history, err := s.recentHistory(ctx, ownerID, conv.ID)
	if err != nil {
		return err
	}
	msgs := BuildRAGMessages(retrieval, history, content)
	stream, err := s.llm.ChatStream(ctx, msgs, ports.ChatOptions{Model: s.llmModel, Temperature: 0.2})
	if err != nil {
		_ = sink.SendError(ctx, ChatStreamError{Code: "llm_stream_failed", Message: err.Error()})
		return err
	}

	var answer strings.Builder
	var usage *ports.TokenUsage
	for chunk := range stream {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if chunk.Err != nil {
			_ = sink.SendError(ctx, ChatStreamError{Code: "llm_stream_failed", Message: chunk.Err.Error()})
			return chunk.Err
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		if chunk.Text != "" {
			answer.WriteString(chunk.Text)
			if err := sink.SendToken(ctx, chunk.Text); err != nil {
				return err
			}
		}
	}

	// 客户端断连后 LLM 流会关闭并正常退出循环,
	// 必须在持久化前再查一次取消状态, 避免落半截 assistant 消息 (spec §5.4)。
	if err := ctx.Err(); err != nil {
		return err
	}

	assistantContent := strings.TrimSpace(answer.String())
	if assistantContent == "" {
		err := fmt.Errorf("llm stream completed without content")
		_ = sink.SendError(ctx, ChatStreamError{Code: "llm_stream_failed", Message: err.Error()})
		return err
	}
	assistant, err := s.createMessage(ctx, ownerID, conv.ID, domain.RoleAssistant, assistantContent, retrieval.Citations, nil, usage)
	if err != nil {
		_ = sink.SendError(ctx, ChatStreamError{Code: "assistant_persist_failed", Message: err.Error()})
		return err
	}
	if err := s.queries.TouchConversationForOwner(ctx, generated.TouchConversationForOwnerParams{ID: conv.ID, OwnerUserID: ownerID}); err != nil {
		return fmt.Errorf("touch conversation after assistant message: %w", err)
	}
	return sink.SendDone(ctx, ChatDone{MessageID: assistant.ID, ConversationID: conv.ID.String(), Usage: usage})
}

func (s *Chat) createMessage(ctx context.Context, ownerID pgtype.UUID, convID uuid.UUID, role, content string, citations []domain.Citation, steps []domain.ToolCallStep, usage *ports.TokenUsage) (*domain.ChatMessage, error) {
	citationsJSON := []byte("[]")
	if len(citations) > 0 {
		b, err := json.Marshal(citations)
		if err != nil {
			return nil, fmt.Errorf("marshal citations: %w", err)
		}
		citationsJSON = b
	}

	toolCallsJSON := []byte("[]")
	if len(steps) > 0 {
		b, err := json.Marshal(steps)
		if err != nil {
			return nil, fmt.Errorf("marshal tool calls: %w", err)
		}
		toolCallsJSON = b
	}

	usageJSON := []byte("{}")
	if usage != nil {
		b, err := json.Marshal(usage)
		if err != nil {
			return nil, fmt.Errorf("marshal token usage: %w", err)
		}
		usageJSON = b
	}

	row, err := s.queries.CreateMessageForOwner(ctx, generated.CreateMessageForOwnerParams{
		ConversationID: convID,
		OwnerUserID:    ownerID,
		Role:           role,
		Content:        content,
		Citations:      citationsJSON,
		ToolCalls:      toolCallsJSON,
		TokenUsage:     usageJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}
	return rowToChatMessage(row), nil
}

func (s *Chat) recentHistory(ctx context.Context, ownerID pgtype.UUID, convID uuid.UUID) ([]*domain.ChatMessage, error) {
	if s.historyMessages == 0 {
		return nil, nil
	}
	rows, err := s.queries.ListRecentMessagesByConversationForOwner(ctx, generated.ListRecentMessagesByConversationForOwnerParams{
		ConversationID: convID,
		OwnerUserID:    ownerID,
		Limit:          int32(s.historyMessages),
	})
	if err != nil {
		return nil, fmt.Errorf("list recent messages: %w", err)
	}

	out := make([]*domain.ChatMessage, 0, len(rows))
	for _, row := range rows {
		msg := rowToChatMessage(row)
		if msg.Role == domain.RoleUser || msg.Role == domain.RoleAssistant {
			out = append(out, msg)
		}
	}
	return out, nil
}

func rowToConversation(r generated.Conversation) *domain.Conversation {
	return &domain.Conversation{
		ID:        r.ID.String(),
		KBID:      r.KbID.String(),
		Title:     r.Title,
		Mode:      r.Mode,
		AgentID:   r.AgentID,
		UserID:    r.UserID,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

func rowToChatMessage(r generated.Message) *domain.ChatMessage {
	msg := &domain.ChatMessage{
		ID:             r.ID.String(),
		ConversationID: r.ConversationID.String(),
		Role:           r.Role,
		Content:        r.Content,
		Citations:      []domain.Citation{},
		ToolCalls:      []domain.ToolCallStep{},
		TokenUsage:     map[string]any{},
		CreatedAt:      r.CreatedAt,
	}
	_ = json.Unmarshal(r.Citations, &msg.Citations)
	_ = json.Unmarshal(r.ToolCalls, &msg.ToolCalls)
	_ = json.Unmarshal(r.TokenUsage, &msg.TokenUsage)
	return msg
}
