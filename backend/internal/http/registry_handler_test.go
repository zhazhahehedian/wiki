package http

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestRegistryCancellationDoesNotWriteServerError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("GET", "/api/v1/capabilities", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	registryHTTPError(w, req, ctx.Err())
	if w.Body.Len() != 0 || w.Code >= 400 {
		t.Fatal("cancelled navigation produced error response")
	}
}
