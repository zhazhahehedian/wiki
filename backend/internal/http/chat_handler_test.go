package http

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

func TestMapChatErrorMapsPortUnknownAgent(t *testing.T) {
	mapped := mapChatError(fmt.Errorf("resolver failed: %w", ports.ErrUnknownAgent))
	apiErr, ok := mapped.(*APIError)
	if !ok {
		t.Fatalf("mapChatError() = %T, want *APIError", mapped)
	}
	if apiErr.HTTPStatus != http.StatusBadRequest || apiErr.Code != CodeUnknownAgent {
		t.Fatalf("mapChatError() = status %d code %q, want status %d code %q",
			apiErr.HTTPStatus, apiErr.Code, http.StatusBadRequest, CodeUnknownAgent)
	}
}
