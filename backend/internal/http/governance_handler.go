package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
)

func registryFilter(r *http.Request) (registry.Filter, error) {
	q := r.URL.Query()
	f := registry.Filter{Type: q.Get("type"), Status: q.Get("status"), Department: q.Get("department"), Search: q.Get("q"), Limit: 24}
	for name, dst := range map[string]*int32{"limit": &f.Limit, "offset": &f.Offset} {
		if q.Has(name) {
			n, e := strconv.ParseInt(q.Get(name), 10, 32)
			if e != nil {
				return f, registry.ErrInvalid
			}
			*dst = int32(n)
		}
	}
	return f, nil
}
func decodeGovernance(w http.ResponseWriter, r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	var extra any
	if d.Decode(out) != nil || d.Decode(&extra) != io.EOF {
		return registry.ErrInvalid
	}
	return nil
}
func (h *RegistryHandler) Identity(w http.ResponseWriter, r *http.Request) {
	user, ok := registryUser(w, r)
	if !ok {
		return
	}
	d, e := h.service.Identity(r.Context(), user)
	if e != nil {
		registryHTTPError(w, r, e)
		return
	}
	WriteJSON(w, 200, d)
}
func (h *RegistryHandler) Catalog(w http.ResponseWriter, r *http.Request) {
	h.governanceList(w, r, false)
}
func (h *RegistryHandler) Reviews(w http.ResponseWriter, r *http.Request) {
	h.governanceList(w, r, true)
}
func (h *RegistryHandler) governanceList(w http.ResponseWriter, r *http.Request, reviews bool) {
	user, ok := registryUser(w, r)
	if !ok {
		return
	}
	f, e := registryFilter(r)
	if e != nil {
		registryHTTPError(w, r, e)
		return
	}
	var page registry.Page
	if reviews {
		page, e = h.service.Reviews(r.Context(), user, f)
	} else {
		page, e = h.service.Catalog(r.Context(), user, f)
	}
	if e != nil {
		registryHTTPError(w, r, e)
		return
	}
	WriteJSON(w, 200, page)
}
func (h *RegistryHandler) Published(w http.ResponseWriter, r *http.Request) {
	user, ok := registryUser(w, r)
	if !ok {
		return
	}
	d, e := h.service.Published(r.Context(), user, chi.URLParam(r, "slug"))
	if e != nil {
		registryHTTPError(w, r, e)
		return
	}
	WriteJSON(w, 200, d)
}
func (h *RegistryHandler) Action(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := registryUser(w, r)
		if !ok {
			return
		}
		var input registry.ActionRequest
		if e := decodeGovernance(w, r, &input); e != nil {
			registryHTTPError(w, r, e)
			return
		}
		d, e := h.service.Act(r.Context(), user, chi.URLParam(r, "slug"), action, input)
		if e != nil {
			registryHTTPError(w, r, e)
			return
		}
		WriteJSON(w, 200, d)
	}
}
func (h *RegistryHandler) Files(w http.ResponseWriter, r *http.Request) {
	user, ok := registryUser(w, r)
	if !ok {
		return
	}
	files, e := h.service.Files(r.Context(), user, chi.URLParam(r, "slug"), chi.URLParam(r, "versionID"))
	if e != nil {
		registryHTTPError(w, r, e)
		return
	}
	WriteJSON(w, 200, map[string]any{"files": files})
}
func (h *RegistryHandler) Audit(w http.ResponseWriter, r *http.Request) {
	user, ok := registryUser(w, r)
	if !ok {
		return
	}
	f, e := registryFilter(r)
	if e != nil {
		registryHTTPError(w, r, e)
		return
	}
	q := r.URL.Query()
	page, e := h.service.Audit(r.Context(), user, registry.AuditFilter{Limit: f.Limit, Offset: f.Offset, Action: q.Get("action"), Actor: q.Get("actor"), Target: q.Get("target")})
	if e != nil {
		registryHTTPError(w, r, e)
		return
	}
	WriteJSON(w, 200, page)
}
func (h *RegistryHandler) Profiles(w http.ResponseWriter, r *http.Request) {
	user, ok := registryUser(w, r)
	if !ok {
		return
	}
	f, e := registryFilter(r)
	if e != nil {
		registryHTTPError(w, r, e)
		return
	}
	page, e := h.service.Profiles(r.Context(), user, f)
	if e != nil {
		registryHTTPError(w, r, e)
		return
	}
	WriteJSON(w, 200, page)
}
func (h *RegistryHandler) SetDepartment(w http.ResponseWriter, r *http.Request) {
	user, ok := registryUser(w, r)
	if !ok {
		return
	}
	var input registry.DepartmentRequest
	if e := decodeGovernance(w, r, &input); e != nil {
		registryHTTPError(w, r, e)
		return
	}
	d, e := h.service.SetDepartment(r.Context(), user, chi.URLParam(r, "openID"), input)
	if e != nil {
		registryHTTPError(w, r, e)
		return
	}
	WriteJSON(w, 200, d)
}
