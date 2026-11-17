package http

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/zenith-wang/it-wiki/backend/internal/logx"
)

// RequestID keeps a caller id only when it is safe to write into a log line.
// Otherwise it generates one. The value is stored where chi's GetReqID reads it.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := logx.SanitizeRequestID(r.Header.Get("X-Request-Id"))
		if !ok {
			id = logx.NewRequestID()
		}
		ctx := context.WithValue(r.Context(), middleware.RequestIDKey, id)
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
