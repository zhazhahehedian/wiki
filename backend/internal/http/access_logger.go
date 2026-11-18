package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func SanitizedAccessLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		startedAt := time.Now()
		defer func() {
			slog.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.Status(),
				"bytes", wrapped.BytesWritten(),
				"duration", time.Since(startedAt).String(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		}()
		next.ServeHTTP(wrapped, r)
	})
}
