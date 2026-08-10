package http

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"

	"github.com/google/uuid"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

const (
	SessionCookieName = "it_wiki_session"
	CSRFCookieName    = "it_wiki_csrf"
	CSRFHeaderName    = "X-CSRF-Token"

	CodeUnauthenticated = "unauthenticated"
	CodeCSRFRejected    = "csrf_rejected"
)

type authContextKey uint8

const (
	currentUserContextKey authContextKey = iota
	userIDContextKey
)

func WithCurrentUser(ctx context.Context, user domain.User) context.Context {
	ctx = context.WithValue(ctx, currentUserContextKey, user)
	return context.WithValue(ctx, userIDContextKey, user.ID)
}

func CurrentUserFromContext(ctx context.Context) (domain.User, bool) {
	user, ok := ctx.Value(currentUserContextKey).(domain.User)
	return user, ok
}

func UserIDFromContext(ctx context.Context) string {
	userID, _ := ctx.Value(userIDContextKey).(string)
	return userID
}

func requireUserID(w http.ResponseWriter, r *http.Request) (string, bool) {
	user, ok := CurrentUserFromContext(r.Context())
	if !ok || user.ID == "" || UserIDFromContext(r.Context()) != user.ID {
		writeUnauthenticated(w, r)
		return "", false
	}
	if _, err := uuid.Parse(user.ID); err != nil {
		writeUnauthenticated(w, r)
		return "", false
	}
	return user.ID, true
}

func (h *AuthHandler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.disabled {
			writeUnauthenticated(w, r)
			return
		}
		cookie, err := r.Cookie(SessionCookieName)
		if err != nil || cookie.Value == "" {
			writeUnauthenticated(w, r)
			return
		}
		session, err := h.sessions.Get(r.Context(), cookie.Value)
		if err != nil {
			writeUnauthenticated(w, r)
			return
		}
		user, err := h.users.User(r.Context(), session.UserID)
		if err != nil || user.ID == "" || user.ID != session.UserID {
			writeUnauthenticated(w, r)
			return
		}
		if !isSafeMethod(r.Method) {
			if r.Header.Get("Origin") != h.frontendOrigin || !validCSRF(r.Header.Get(CSRFHeaderName), session.CSRFTokenHash) {
				WriteError(w, r, NewAPIError(http.StatusForbidden, CodeCSRFRejected, "request origin or csrf token is invalid"))
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(WithCurrentUser(r.Context(), user)))
	})
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func validCSRF(raw string, wantHash []byte) bool {
	if raw == "" || len(wantHash) != sha256.Size {
		return false
	}
	got := sha256.Sum256([]byte(raw))
	return subtle.ConstantTimeCompare(got[:], wantHash) == 1
}

func writeUnauthenticated(w http.ResponseWriter, r *http.Request) {
	WriteError(w, r, NewAPIError(http.StatusUnauthorized, CodeUnauthenticated, "authentication required"))
}
