package http

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
)

// MaxConcurrentUploads bounds memory: each multipart save buffers its bundle.
const MaxConcurrentUploads = 4

type RegistryHandler struct {
	service *registry.Service
	uploads chan struct{}
}

func NewRegistryHandler(s *registry.Service) *RegistryHandler {
	return &RegistryHandler{service: s, uploads: make(chan struct{}, MaxConcurrentUploads)}
}
func registryHTTPError(w http.ResponseWriter, r *http.Request, e error) {
	if directoryHTTPError(w, r, e) {
		return
	}
	if r.Context().Err() != nil {
		return
	}
	switch {
	case errors.Is(e, registry.ErrMCPPackage):
		WriteError(w, r, NewAPIError(400, "invalid_mcp_package", "请上传一个有效的 MCP ZIP 程序包（不超过 100 MiB、2000 条目、解压后 500 MiB）；不支持加密包、链接或不安全路径。未填写 Endpoint 时必须有程序包。"))
	case errors.Is(e, registry.ErrSkillPublication):
		WriteError(w, r, NewAPIError(400, "invalid_skill_publication", "Skill 发布格式不完整：SKILL.md 需包含与标识一致的 name（最长 64）和非空 description（最长 1024）的 YAML 文件头、正文，且包内文件引用必须存在。请编辑完整包后重新提交。"))
	case errors.Is(e, registry.ErrInvalid):
		WriteError(w, r, NewAPIError(400, "invalid_capability", "请检查名称、版本、可见范围、MCP 接入信息/程序包或 Skill 附件格式"))
	case errors.Is(e, registry.ErrNotFound):
		WriteError(w, r, NewAPIError(404, "capability_not_found", "能力或版本不存在"))
	case errors.Is(e, registry.ErrConflict):
		WriteError(w, r, NewAPIError(409, "capability_conflict", "状态、版本或内容已变化；请重新加载后再操作"))
	case errors.Is(e, registry.ErrForbidden):
		WriteError(w, r, NewAPIError(403, "registry_forbidden", "当前账号无权执行此治理操作"))
	case errors.Is(e, registry.ErrIdentity):
		WriteError(w, r, NewAPIError(403, "feishu_identity_required", "需要有效的飞书账号身份"))
	default:
		WriteError(w, r, e)
	}
}
func registryUser(w http.ResponseWriter, r *http.Request) (string, bool) {
	*r = *r.WithContext(registry.WithRequestID(r.Context(), middleware.GetReqID(r.Context())))
	w.Header().Set("Cache-Control", "no-store")
	return requireUserID(w, r)
}
func (h *RegistryHandler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := registryUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit, offset := int64(24), int64(0)
	for name, dst := range map[string]*int64{"limit": &limit, "offset": &offset} {
		if q.Has(name) {
			n, e := strconv.ParseInt(q.Get(name), 10, 32)
			if e != nil {
				registryHTTPError(w, r, registry.ErrInvalid)
				return
			}
			*dst = n
		}
	}
	page, e := h.service.List(r.Context(), user, registry.Filter{Type: q.Get("type"), Status: q.Get("status"), Department: q.Get("department"), Search: q.Get("q"), Limit: int32(limit), Offset: int32(offset)})
	if e != nil {
		registryHTTPError(w, r, e)
		return
	}
	WriteJSON(w, 200, page)
}
func (h *RegistryHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := registryUser(w, r)
	if !ok {
		return
	}
	d, e := h.service.Get(r.Context(), user, chi.URLParam(r, "slug"))
	if e != nil {
		registryHTTPError(w, r, e)
		return
	}
	WriteJSON(w, 200, d)
}
func (h *RegistryHandler) Save(w http.ResponseWriter, r *http.Request) {
	user, ok := registryUser(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, registry.MaxMCPBundleBytes+512*1024)
	if media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err == nil && media == "application/json" {
		r.Body = http.MaxBytesReader(w, r.Body, 300*1024)
		var input registry.SaveRequest
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		var extra any
		if d.Decode(&input) != nil || d.Decode(&extra) != io.EOF {
			registryHTTPError(w, r, registry.ErrInvalid)
			return
		}
		h.saveResult(w, r, user, input, nil)
		return
	}
	reader, e := r.MultipartReader()
	if e != nil {
		registryHTTPError(w, r, registry.ErrInvalid)
		return
	}
	select {
	case h.uploads <- struct{}{}:
		defer func() { <-h.uploads }()
	case <-r.Context().Done():
		return
	}
	var input registry.SaveRequest
	hasMetadata := false
	files := []registry.File{}
	total := 0
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			registryHTTPError(w, r, registry.ErrInvalid)
			return
		}
		if part.FormName() == "metadata" && !hasMetadata {
			raw, err := io.ReadAll(io.LimitReader(part, 300*1024+1))
			if err != nil || len(raw) > 300*1024 {
				registryHTTPError(w, r, registry.ErrInvalid)
				return
			}
			d := json.NewDecoder(strings.NewReader(string(raw)))
			d.DisallowUnknownFields()
			var extra any
			if d.Decode(&input) != nil || d.Decode(&extra) != io.EOF {
				registryHTTPError(w, r, registry.ErrInvalid)
				return
			}
			hasMetadata = true
		} else if part.FormName() == "files" && len(files) < registry.MaxBundleFiles {
			// FileName() applies filepath.Base; inspect the original filename so a
			// traversal or conflicting nested path cannot be silently accepted.
			_, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
			if err != nil {
				registryHTTPError(w, r, registry.ErrInvalid)
				return
			}
			limit := registry.MaxMCPBundleBytes
			if hasMetadata && input.Type == "skill" {
				limit = registry.MaxBundleBytes
			}
			raw, err := io.ReadAll(io.LimitReader(part, int64(limit-total+1)))
			total += len(raw)
			if err != nil || total > limit {
				registryHTTPError(w, r, registry.ErrInvalid)
				return
			}
			files = append(files, registry.File{Name: params["filename"], Data: raw})
		} else {
			registryHTTPError(w, r, registry.ErrInvalid)
			return
		}
		_ = part.Close()
	}
	if !hasMetadata {
		registryHTTPError(w, r, registry.ErrInvalid)
		return
	}
	h.saveResult(w, r, user, input, files)
}
func (h *RegistryHandler) saveResult(w http.ResponseWriter, r *http.Request, user string, input registry.SaveRequest, files []registry.File) {
	slug := chi.URLParam(r, "slug")
	detail, e := h.service.Save(r.Context(), user, slug, input, files)
	if e != nil {
		registryHTTPError(w, r, e)
		return
	}
	status := 200
	if slug == "" {
		status = 201
	}
	WriteJSON(w, status, detail)
}
func (h *RegistryHandler) Download(w http.ResponseWriter, r *http.Request) {
	user, ok := registryUser(w, r)
	if !ok {
		return
	}
	slug := chi.URLParam(r, "slug")
	version, body, e := h.service.Download(r.Context(), user, slug, chi.URLParam(r, "versionID"))
	if e != nil {
		registryHTTPError(w, r, e)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": slug + "-" + version.Version + ".zip"}))
	if version.BundleSize > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(version.BundleSize, 10))
	}
	if version.BundleSHA256 != "" {
		w.Header().Set("X-Bundle-SHA256", version.BundleSHA256)
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, body)
}
