package http

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func SanitizedAccessLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		startedAt := time.Now()
		defer func() {
			log.Printf(
				"method=%s path=%s status=%d bytes=%d duration=%s request_id=%s",
				r.Method,
				r.URL.Path,
				wrapped.Status(),
				wrapped.BytesWritten(),
				time.Since(startedAt),
				middleware.GetReqID(r.Context()),
			)
		}()
		next.ServeHTTP(wrapped, r)
	})
}
