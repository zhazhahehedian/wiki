package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/publishing"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
)

type PublishingHandler struct{ service *publishing.Service }

func NewPublishingHandler(s *publishing.Service) *PublishingHandler { return &PublishingHandler{s} }
func publishingHTTPError(w http.ResponseWriter, r *http.Request, e error) {
	switch {
	case errors.Is(e, publishing.ErrUnauthenticated):
		WriteError(w, r, NewAPIError(401, "invalid_api_credential", "API 凭证无效、已过期或已撤销。"))
	case errors.Is(e, publishing.ErrForbidden):
		WriteError(w, r, NewAPIError(403, "api_scope_denied", "API 凭证未授予此操作权限。"))
	case errors.Is(e, publishing.ErrNotFound):
		WriteError(w, r, NewAPIError(404, "api_credential_not_found", "凭证不存在。"))
	case errors.Is(e, publishing.ErrInvalid):
		WriteError(w, r, NewAPIError(400, "invalid_api_credential_request", "请提供名称、有效权限和一分钟至 365 天内的到期时间。"))
	default:
		WriteError(w, r, e)
	}
}
func (h *PublishingHandler) Credentials(w http.ResponseWriter, r *http.Request) {
	user, ok := registryUser(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var input publishing.CreateRequest
		if decodeGovernance(w, r, &input) != nil {
			publishingHTTPError(w, r, publishing.ErrInvalid)
			return
		}
		out, e := h.service.Create(r.Context(), user, input)
		if e != nil {
			publishingHTTPError(w, r, e)
			return
		}
		WriteJSON(w, 201, out)
	case http.MethodDelete:
		if e := h.service.Revoke(r.Context(), user, chi.URLParam(r, "id")); e != nil {
			publishingHTTPError(w, r, e)
			return
		}
		w.WriteHeader(204)
	default:
		out, e := h.service.List(r.Context(), user)
		if e != nil {
			publishingHTTPError(w, r, e)
			return
		}
		WriteJSON(w, 200, map[string]any{"items": out})
	}
}
func (h *PublishingHandler) Require(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			parts := strings.Fields(r.Header.Get("Authorization"))
			if len(r.Header.Values("Authorization")) != 1 || len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				publishingHTTPError(w, r, publishing.ErrUnauthenticated)
				return
			}
			p, e := h.service.Authenticate(r.Context(), parts[1], scope)
			if e != nil {
				publishingHTTPError(w, r, e)
				return
			}
			ctx := WithCurrentUser(r.Context(), domain.User{ID: p.UserID})
			ctx = registry.WithCredentialID(ctx, p.CredentialID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
