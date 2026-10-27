package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/zenith-wang/it-wiki/backend/internal/directory"
)

type DirectoryHandler struct{ service *directory.Service }

func NewDirectoryHandler(s *directory.Service) *DirectoryHandler { return &DirectoryHandler{s} }
func directoryHTTPError(w http.ResponseWriter, r *http.Request, e error) bool {
	switch {
	case errors.Is(e, directory.ErrAuthorization):
		WriteError(w, r, NewAPIError(403, "directory_authorization_required", "请联系管理员开通飞书通讯录用户权限，并重新授权登录；当前目录或部门不可访问。"))
	case errors.Is(e, directory.ErrInvalid):
		WriteError(w, r, NewAPIError(400, "invalid_directory_selection", "请选择当前账号可见的飞书部门和用户。"))
	case errors.Is(e, directory.ErrUnavailable):
		WriteError(w, r, NewAPIError(502, "directory_unavailable", "飞书通讯录暂时不可用，请稍后重试。"))
	default:
		return false
	}
	return true
}
func (h *DirectoryHandler) Departments(w http.ResponseWriter, r *http.Request) {
	user, ok := registryUser(w, r)
	if !ok {
		return
	}
	parent := r.URL.Query().Get("parent_id")
	if parent == "" {
		parent = "0"
	}
	page, e := h.service.Departments(r.Context(), user, parent, r.URL.Query().Get("page_token"))
	if e != nil {
		if !directoryHTTPError(w, r, e) {
			WriteError(w, r, e)
		}
		return
	}
	WriteJSON(w, 200, page)
}
func (h *DirectoryHandler) Users(w http.ResponseWriter, r *http.Request) {
	user, ok := registryUser(w, r)
	if !ok {
		return
	}
	department := r.URL.Query().Get("department_id")
	if department == "" {
		department = "0"
	}
	page, e := h.service.Users(r.Context(), user, department, r.URL.Query().Get("page_token"))
	if e != nil {
		if !directoryHTTPError(w, r, e) {
			WriteError(w, r, e)
		}
		return
	}
	WriteJSON(w, 200, page)
}
func (h *DirectoryHandler) Membership(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Directory browsing never grants membership. Refresh only where registry
		// permissions are consumed, and let the repository reject expired data.
		path := r.URL.Path
		if strings.HasPrefix(path, "/api/v1/catalog") || strings.HasPrefix(path, "/api/v1/knowledge/") || strings.HasPrefix(path, "/api/v1/capabilities") || path == "/api/v1/governance/me" || strings.HasPrefix(path, "/api/v1/open/capabilities") {
			_ = h.service.EnsureMembership(r.Context(), UserIDFromContext(r.Context()))
		}
		next.ServeHTTP(w, r)
	})
}
