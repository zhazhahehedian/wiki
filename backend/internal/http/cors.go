package http

import (
	"net/http"
	"os"
	"strings"
)

func CORS(next http.Handler, configuredOrigins ...string) http.Handler {
	allowedOrigin := ""
	if len(configuredOrigins) > 0 {
		allowedOrigin = strings.TrimRight(configuredOrigins[0], "/")
	}
	if allowedOrigin == "" {
		allowedOrigin = strings.TrimRight(os.Getenv("FRONTEND_ORIGIN"), "/")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		w.Header().Add("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token")
		w.Header().Set("Access-Control-Expose-Headers", "X-Total-Count, X-Request-Id")

		if origin != "" {
			if allowedOrigin == "" || origin != allowedOrigin {
				WriteError(w, r, NewAPIError(http.StatusForbidden, CodeCSRFRejected, "origin is not allowed"))
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
