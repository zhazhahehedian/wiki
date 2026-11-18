package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

const CodeInternalError = "internal_error"

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
	reqID := middleware.GetReqID(r.Context())
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		slog.Error("request failed", "request_id", reqID, "error_type", fmt.Sprintf("%T", err))
		apiErr = internalServerAPIError()
	} else if apiErr.HTTPStatus >= http.StatusInternalServerError {
		slog.Error("request failed", "request_id", reqID, "error_category", apiErr.Code)
	}

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

func internalServerAPIError() *APIError {
	return NewAPIError(http.StatusInternalServerError, CodeInternalError, "internal server error")
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
