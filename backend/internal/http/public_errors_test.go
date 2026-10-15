package http

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
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
