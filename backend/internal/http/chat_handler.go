package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

type ChatHandler struct {
	svc *service.Chat
}

func NewChatHandler(svc *service.Chat) *ChatHandler {
	return &ChatHandler{svc: svc}
}

type createConversationRequest struct {
	Mode string `json:"mode"`
}

func (h *ChatHandler) CreateConversation(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var req createConversationRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			WriteError(w, r, NewAPIError(http.StatusBadRequest, CodeValidationFailed, "invalid JSON body"))
			return
		}
	}
	kbID := chi.URLParam(r, "kbID")
	conv, err := h.svc.CreateConversation(r.Context(), userID, kbID, req.Mode)
	if err != nil {
		WriteError(w, r, mapChatError(err))
		return
	}
	WriteJSON(w, http.StatusCreated, conv)
}

type updateConversationRequest struct {
	Mode string `json:"mode"`
}

func (h *ChatHandler) UpdateConversation(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var req updateConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, r, NewAPIError(http.StatusBadRequest, CodeValidationFailed, "invalid JSON body"))
		return
	}
	conv, err := h.svc.UpdateMode(r.Context(), userID, chi.URLParam(r, "conversationID"), req.Mode)
	if err != nil {
		WriteError(w, r, mapChatError(err))
		return
	}
	WriteJSON(w, http.StatusOK, conv)
}

func (h *ChatHandler) ListConversations(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	p, err := ParsePagination(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	kbID := chi.URLParam(r, "kbID")
	items, total, err := h.svc.ListConversations(r.Context(), userID, kbID, p.Limit, p.Offset)
	if err != nil {
		WriteError(w, r, mapChatError(err))
		return
	}
	WriteListResponse(w, total, items)
}

func (h *ChatHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	p, err := ParsePagination(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	conversationID := chi.URLParam(r, "conversationID")
	items, total, err := h.svc.ListMessages(r.Context(), userID, conversationID, p.Limit, p.Offset)
	if err != nil {
		WriteError(w, r, mapChatError(err))
		return
	}
	WriteListResponse(w, total, items)
}

type streamMessageRequest struct {
	Content string `json:"content"`
}

func (h *ChatHandler) StreamMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var req streamMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, r, NewAPIError(http.StatusBadRequest, CodeValidationFailed, "invalid JSON body"))
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		WriteError(w, r, NewAPIError(http.StatusBadRequest, CodeValidationFailed, "content is required"))
		return
	}

	sink := &httpChatSink{w: w}
	err := h.svc.AskStream(r.Context(), userID, chi.URLParam(r, "conversationID"), req.Content, sink)
	if err != nil && !sink.Started() {
		WriteError(w, r, mapChatError(err))
	}
}

func mapChatError(err error) error {
	if ports.IsUnknownAgent(err) {
		return NewAPIError(http.StatusBadRequest, CodeUnknownAgent, err.Error())
	}
	if errors.Is(err, service.ErrInvalidMode) {
		return NewAPIError(http.StatusBadRequest, CodeValidationFailed, err.Error())
	}
	var convNotFound *service.ErrConversationNotFound
	if errors.As(err, &convNotFound) {
		return NewAPIError(http.StatusNotFound, CodeConversationNotFound, err.Error())
	}
	var kbNotFound *service.ErrKBNotFound
	if errors.As(err, &kbNotFound) {
		return NewAPIError(http.StatusNotFound, CodeKBNotFound, err.Error())
	}
	return err
}

type httpChatSink struct {
	w       http.ResponseWriter
	started bool
}

func (s *httpChatSink) ensureStarted() {
	if !s.started {
		WriteSSEHeaders(s.w)
		s.started = true
	}
}

func (s *httpChatSink) Started() bool { return s.started }

func (s *httpChatSink) SendRetrieval(_ context.Context, result *service.RetrievalResult) error {
	s.ensureStarted()
	return WriteSSEEvent(s.w, "retrieval", map[string]any{
		"evidence_level": result.EvidenceLevel,
		"citations":      result.Citations,
	})
}

func (s *httpChatSink) SendToken(_ context.Context, text string) error {
	s.ensureStarted()
	return WriteSSEEvent(s.w, "token", map[string]string{"text": text})
}

func (s *httpChatSink) SendToolCall(_ context.Context, ev domain.ToolCallEvent) error {
	s.ensureStarted()
	return WriteSSEEvent(s.w, "tool_call", ev)
}

func (s *httpChatSink) SendToolResult(ctx context.Context, ev domain.ToolResultEvent) error {
	s.ensureStarted()
	if ev.Error != "" {
		log.Printf("request failed request_id=%s error_category=tool_execution_error", middleware.GetReqID(ctx))
		ev.Error = domain.ToolExecutionFailed
		ev.Result = ""
	}
	return WriteSSEEvent(s.w, "tool_result", ev)
}

func (s *httpChatSink) SendDone(_ context.Context, done service.ChatDone) error {
	s.ensureStarted()
	return WriteSSEEvent(s.w, "done", done)
}

func (s *httpChatSink) SendError(ctx context.Context, _ service.ChatStreamError) error {
	s.ensureStarted()
	log.Printf("request failed request_id=%s error_category=chat_stream_error", middleware.GetReqID(ctx))
	return WriteSSEEvent(s.w, "error", service.InternalChatStreamError())
}
