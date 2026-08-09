package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

const (
	CodeValidationFailed        = "validation_failed"
	CodeKBNotFound              = "kb_not_found"
	CodeDocNotFound             = "doc_not_found"
	CodeConversationNotFound    = "conversation_not_found"
	CodeUnknownAgent            = "unknown_agent"
	CodeChunkNotFound           = "chunk_not_found"
	CodeDuplicateChecksum       = "duplicate_checksum"
	CodeLLMStreamFailed         = "llm_stream_failed"
	CodePayloadTooLarge         = "payload_too_large"
	CodeUnsupportedMediaType    = "unsupported_media_type"
	CodeInternalError           = "internal_error"
	CodeResourceAlreadyImported = "resource_already_imported"
	CodeSyncInProgress          = "sync_in_progress"
	CodeUnsupportedFeishuURL    = "unsupported_feishu_url"
	CodeUnsupportedResource     = "unsupported_resource"
	CodeResourceNotFound        = "resource_not_found"
	CodeResourceForbidden       = "resource_forbidden"
	CodeReauthRequired          = "reauth_required"
	CodeInvalidRequest          = "invalid_request"
)

type APIError struct {
	HTTPStatus int            `json:"-"`
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	Details    map[string]any `json:"details,omitempty"`
}

func (e *APIError) Error() string { return e.Message }

func NewAPIError(status int, code, message string) *APIError {
	return &APIError{HTTPStatus: status, Code: code, Message: message}
}

func WithDetails(e *APIError, details map[string]any) *APIError {
	e.Details = details
	return e
}

type errorEnvelope struct {
	Error envelopeBody `json:"error"`
}

type envelopeBody struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	RequestID string         `json:"request_id"`
	Details   map[string]any `json:"details,omitempty"`
}

func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	apiErr, ok := err.(*APIError)
	if !ok {
		apiErr = NewAPIError(http.StatusInternalServerError, CodeInternalError, err.Error())
	}
	reqID := middleware.GetReqID(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-Id", reqID)
	w.WriteHeader(apiErr.HTTPStatus)
	_ = json.NewEncoder(w).Encode(errorEnvelope{
		Error: envelopeBody{
			Code:      apiErr.Code,
			Message:   apiErr.Message,
			RequestID: reqID,
			Details:   apiErr.Details,
		},
	})
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
