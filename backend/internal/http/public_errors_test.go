package http

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

const publicErrorSecret = "token=secret-token provider_body=private-content"

type secretPublicError struct{}

func (secretPublicError) Error() string { return publicErrorSecret }

func TestWriteErrorRedactsUnmappedErrorsAndLogsSafeContext(t *testing.T) {
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previous) })

	recorder := httptest.NewRecorder()
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, secretPublicError{})
	}))
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/failure", nil))

	body := recorder.Body.String()
	if recorder.Code != http.StatusInternalServerError || !strings.Contains(body, `"code":"internal_error"`) || !strings.Contains(body, `"message":"internal server error"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, body)
	}
	requestID := recorder.Header().Get("X-Request-Id")
	if requestID == "" || !strings.Contains(logs.String(), "request_id="+requestID) || !strings.Contains(logs.String(), "error_type=http.secretPublicError") {
		t.Fatalf("request_id=%q logs=%q", requestID, logs.String())
	}
	if strings.Contains(body, publicErrorSecret) || strings.Contains(logs.String(), publicErrorSecret) {
		t.Fatalf("secret leaked: body=%q logs=%q", body, logs.String())
	}
}

func TestHTTPChatSinkRedactsAndSafelyLogsStreamErrors(t *testing.T) {
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previous) })

	recorder := httptest.NewRecorder()
	sink := &httpChatSink{w: recorder}
	ctx := context.WithValue(context.Background(), middleware.RequestIDKey, "request-123")
	if err := sink.SendError(ctx, service.ChatStreamError{Code: "provider_failure", Message: publicErrorSecret}); err != nil {
		t.Fatal(err)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, `"code":"internal_error"`) || !strings.Contains(body, `"message":"internal server error"`) {
		t.Fatalf("body=%s", body)
	}
	if !strings.Contains(logs.String(), "request_id=request-123") || !strings.Contains(logs.String(), "error_category=chat_stream_error") {
		t.Fatalf("logs=%q", logs.String())
	}
	if strings.Contains(body, publicErrorSecret) || strings.Contains(logs.String(), publicErrorSecret) {
		t.Fatalf("secret leaked: body=%q logs=%q", body, logs.String())
	}
}

func TestHTTPChatSinkRedactsAndSafelyLogsToolErrors(t *testing.T) {
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previous) })

	recorder := httptest.NewRecorder()
	sink := &httpChatSink{w: recorder}
	ctx := context.WithValue(context.Background(), middleware.RequestIDKey, "request-tool-123")
	if err := sink.SendToolResult(ctx, domain.ToolResultEvent{ID: "call-1", Name: "kb_retrieval", Error: publicErrorSecret}); err != nil {
		t.Fatal(err)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, `"error":"tool execution failed"`) {
		t.Fatalf("body=%s", body)
	}
	if !strings.Contains(logs.String(), "request_id=request-tool-123") || !strings.Contains(logs.String(), "error_category=tool_execution_error") {
		t.Fatalf("logs=%q", logs.String())
	}
	if strings.Contains(body, publicErrorSecret) || strings.Contains(logs.String(), publicErrorSecret) {
		t.Fatalf("secret leaked: body=%q logs=%q", body, logs.String())
	}
}

func TestKnowledgeBaseDocumentAndChatJSONErrorsAreRedacted(t *testing.T) {
	userID := uuid.NewString()
	tests := []struct {
		name    string
		handler http.Handler
		target  string
	}{
		{
			name:    "knowledge_base",
			handler: http.HandlerFunc(NewKBHandler(service.NewKB(errorKBQueries{}, "embed", 3)).List),
			target:  "/kbs",
		},
		{
			name:    "document",
			handler: http.HandlerFunc(NewDocumentHandler(service.NewDocument(errorDocumentQueries{}, nil), nil, 1024).Get),
			target:  "/docs/" + uuid.NewString(),
		},
		{
			name:    "chat",
			handler: http.HandlerFunc(NewChatHandler(service.NewChat(errorChatQueries{}, nil, nil, "", 0, nil, nil)).ListConversations),
			target:  "/kbs/" + uuid.NewString() + "/conversations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newTestRoute(tt.name, tt.handler)
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			req = req.WithContext(WithCurrentUser(req.Context(), domainUser(userID)))
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			body := recorder.Body.String()
			if recorder.Code != http.StatusInternalServerError || !strings.Contains(body, `"code":"internal_error"`) || !strings.Contains(body, `"message":"internal server error"`) {
				t.Fatalf("status=%d body=%s", recorder.Code, body)
			}
			if strings.Contains(body, publicErrorSecret) {
				t.Fatalf("secret leaked: %s", body)
			}
		})
	}
}

func newTestRoute(kind string, handler http.Handler) http.Handler {
	router := chi.NewRouter()
	switch kind {
	case "knowledge_base":
		router.Get("/kbs", handler.ServeHTTP)
	case "document":
		router.Get("/docs/{id}", handler.ServeHTTP)
	case "chat":
		router.Get("/kbs/{kbID}/conversations", handler.ServeHTTP)
	}
	return router
}

func domainUser(id string) domain.User { return domain.User{ID: id} }

type errorKBQueries struct{}

func (errorKBQueries) CreateKnowledgeBaseForOwner(context.Context, generated.CreateKnowledgeBaseForOwnerParams) (generated.KnowledgeBase, error) {
	return generated.KnowledgeBase{}, errors.New(publicErrorSecret)
}
func (errorKBQueries) GetKnowledgeBaseForOwner(context.Context, generated.GetKnowledgeBaseForOwnerParams) (generated.KnowledgeBase, error) {
	return generated.KnowledgeBase{}, errors.New(publicErrorSecret)
}
func (errorKBQueries) ListKnowledgeBasesForOwner(context.Context, generated.ListKnowledgeBasesForOwnerParams) ([]generated.KnowledgeBase, error) {
	return nil, errors.New(publicErrorSecret)
}
func (errorKBQueries) CountKnowledgeBasesForOwner(context.Context, pgtype.UUID) (int64, error) {
	return 0, errors.New(publicErrorSecret)
}
func (errorKBQueries) DeleteKnowledgeBaseForOwner(context.Context, generated.DeleteKnowledgeBaseForOwnerParams) error {
	return errors.New(publicErrorSecret)
}

type errorDocumentQueries struct{}

func (errorDocumentQueries) GetKnowledgeBaseForOwner(context.Context, generated.GetKnowledgeBaseForOwnerParams) (generated.KnowledgeBase, error) {
	return generated.KnowledgeBase{}, errors.New(publicErrorSecret)
}
func (errorDocumentQueries) GetDocumentForOwner(context.Context, generated.GetDocumentForOwnerParams) (generated.Document, error) {
	return generated.Document{}, errors.New(publicErrorSecret)
}
func (errorDocumentQueries) ListDocumentsByKBForOwner(context.Context, generated.ListDocumentsByKBForOwnerParams) ([]generated.Document, error) {
	return nil, errors.New(publicErrorSecret)
}
func (errorDocumentQueries) CountDocumentsByKBForOwner(context.Context, generated.CountDocumentsByKBForOwnerParams) (int64, error) {
	return 0, errors.New(publicErrorSecret)
}
func (errorDocumentQueries) DeleteDocumentForOwner(context.Context, generated.DeleteDocumentForOwnerParams) (generated.DeleteDocumentForOwnerRow, error) {
	return generated.DeleteDocumentForOwnerRow{}, errors.New(publicErrorSecret)
}

type errorChatQueries struct{}

func (errorChatQueries) GetKnowledgeBaseForOwner(context.Context, generated.GetKnowledgeBaseForOwnerParams) (generated.KnowledgeBase, error) {
	return generated.KnowledgeBase{}, errors.New(publicErrorSecret)
}
func (errorChatQueries) CreateConversationForOwner(context.Context, generated.CreateConversationForOwnerParams) (generated.Conversation, error) {
	return generated.Conversation{}, errors.New(publicErrorSecret)
}
func (errorChatQueries) GetConversationForOwner(context.Context, generated.GetConversationForOwnerParams) (generated.Conversation, error) {
	return generated.Conversation{}, errors.New(publicErrorSecret)
}
func (errorChatQueries) ListConversationsByKBForOwner(context.Context, generated.ListConversationsByKBForOwnerParams) ([]generated.Conversation, error) {
	return nil, errors.New(publicErrorSecret)
}
func (errorChatQueries) CountConversationsByKBForOwner(context.Context, generated.CountConversationsByKBForOwnerParams) (int64, error) {
	return 0, errors.New(publicErrorSecret)
}
func (errorChatQueries) CreateMessageForOwner(context.Context, generated.CreateMessageForOwnerParams) (generated.Message, error) {
	return generated.Message{}, errors.New(publicErrorSecret)
}
func (errorChatQueries) ListMessagesByConversationForOwner(context.Context, generated.ListMessagesByConversationForOwnerParams) ([]generated.Message, error) {
	return nil, errors.New(publicErrorSecret)
}
func (errorChatQueries) CountMessagesByConversationForOwner(context.Context, generated.CountMessagesByConversationForOwnerParams) (int64, error) {
	return 0, errors.New(publicErrorSecret)
}
func (errorChatQueries) ListRecentMessagesByConversationForOwner(context.Context, generated.ListRecentMessagesByConversationForOwnerParams) ([]generated.Message, error) {
	return nil, errors.New(publicErrorSecret)
}
func (errorChatQueries) TouchConversationForOwner(context.Context, generated.TouchConversationForOwnerParams) error {
	return errors.New(publicErrorSecret)
}
func (errorChatQueries) UpdateConversationModeForOwner(context.Context, generated.UpdateConversationModeForOwnerParams) (generated.Conversation, error) {
	return generated.Conversation{}, errors.New(publicErrorSecret)
}
