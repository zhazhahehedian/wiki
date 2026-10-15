package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	k "github.com/zenith-wang/it-wiki/backend/internal/knowledge"
)

type KnowledgeHandler struct{ service *k.Service }

func NewKnowledgeHandler(s *k.Service) *KnowledgeHandler { return &KnowledgeHandler{s} }
func knowledgeError(w http.ResponseWriter, r *http.Request, e error) {
	status := 503
	code := "knowledge_unavailable"
	message := "知识服务依赖暂不可用，请稍后重试"
	switch {
	case errors.Is(e, k.ErrInvalid):
		status = 400
		code = "invalid_request"
		message = "请检查输入、文档格式或请求字段"
	case errors.Is(e, k.ErrUnauthenticated):
		status = 401
		code = "invalid_credential"
		message = "应用凭证无效或已失效"
	case errors.Is(e, k.ErrForbidden):
		status = 403
		code = "knowledge_forbidden"
		message = "无权访问所请求的知识资源"
	case errors.Is(e, k.ErrNotFound):
		status = 404
		code = "knowledge_not_found"
		message = "知识资源不存在"
	case errors.Is(e, k.ErrConflict):
		status = 409
		code = "knowledge_conflict"
		message = "版本或共享条件不满足，请刷新检查"
	case errors.Is(e, k.ErrNotReady):
		status = 409
		code = "knowledge_not_ready"
		message = "知识库尚无可检索版本，请检查文档处理状态与索引配置"
	case errors.Is(e, k.ErrLimited):
		status = 429
		code = "knowledge_rate_limited"
		message = "请求过于频繁，请稍后重试"
		w.Header().Set("Retry-After", "60")
	case errors.Is(e, context.DeadlineExceeded):
		status = 504
		code = "knowledge_timeout"
		message = "检索超时，请稍后重试"
	case errors.Is(e, context.Canceled):
		return
	}
	WriteError(w, r, NewAPIError(status, code, message))
}
func knowledgeJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		knowledgeError(w, r, k.ErrInvalid)
		return false
	}
	if e := d.Decode(&struct{}{}); e != io.EOF {
		knowledgeError(w, r, k.ErrInvalid)
		return false
	}
	return true
}
func knowledgeUser(w http.ResponseWriter, r *http.Request) (k.Principal, bool) {
	id, ok := registryUser(w, r)
	return k.Principal{UserID: id}, ok
}
func knowledgeResult(w http.ResponseWriter, r *http.Request, value any, e error) {
	if e != nil {
		knowledgeError(w, r, e)
		return
	}
	WriteJSON(w, 200, value)
}
func (h *KnowledgeHandler) Bases(w http.ResponseWriter, r *http.Request) {
	p, ok := knowledgeUser(w, r)
	if !ok {
		return
	}
	if r.Method == "GET" {
		out, e := h.service.Repo.ListKB(r.Context(), p)
		knowledgeResult(w, r, out, e)
		return
	}
	var in k.KB
	if !knowledgeJSON(w, r, &in) {
		return
	}
	out, e := h.service.SaveKB(r.Context(), p, chi.URLParam(r, "id"), in)
	knowledgeResult(w, r, out, e)
}
func (h *KnowledgeHandler) DeleteBase(w http.ResponseWriter, r *http.Request) {
	p, ok := knowledgeUser(w, r)
	if !ok {
		return
	}
	knowledgeResult(w, r, map[string]bool{"deleted": true}, h.service.Repo.DeleteKB(r.Context(), p, chi.URLParam(r, "id")))
}
func (h *KnowledgeHandler) Documents(w http.ResponseWriter, r *http.Request) {
	p, ok := knowledgeUser(w, r)
	if !ok {
		return
	}
	out, e := h.service.Repo.Documents(r.Context(), p, chi.URLParam(r, "id"))
	knowledgeResult(w, r, out, e)
}
func (h *KnowledgeHandler) Upload(w http.ResponseWriter, r *http.Request) {
	p, ok := knowledgeUser(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, k.MaxUpload+64<<10)
	if e := r.ParseMultipartForm(1 << 20); e != nil {
		knowledgeError(w, r, k.ErrInvalid)
		return
	}
	defer r.MultipartForm.RemoveAll()
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["file"]) != 1 {
		knowledgeError(w, r, k.ErrInvalid)
		return
	}
	f, header, e := r.FormFile("file")
	if e != nil {
		knowledgeError(w, r, k.ErrInvalid)
		return
	}
	defer f.Close()
	data, e := io.ReadAll(io.LimitReader(f, k.MaxUpload+1))
	if e != nil {
		knowledgeError(w, r, k.ErrInvalid)
		return
	}
	out, e := h.service.Upload(r.Context(), p, chi.URLParam(r, "id"), chi.URLParam(r, "documentID"), header.Filename, data)
	knowledgeResult(w, r, out, e)
}
func (h *KnowledgeHandler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	p, ok := knowledgeUser(w, r)
	if !ok {
		return
	}
	knowledgeResult(w, r, map[string]bool{"deleted": true}, h.service.Repo.DeleteDocument(r.Context(), p, chi.URLParam(r, "id")))
}
func (h *KnowledgeHandler) Retry(w http.ResponseWriter, r *http.Request) {
	p, ok := knowledgeUser(w, r)
	if !ok {
		return
	}
	knowledgeResult(w, r, map[string]bool{"queued": true}, h.service.Repo.Retry(r.Context(), p, chi.URLParam(r, "id")))
}
func (h *KnowledgeHandler) Preview(w http.ResponseWriter, r *http.Request) {
	p, ok := knowledgeUser(w, r)
	if !ok {
		return
	}
	out, e := h.service.Repo.Preview(r.Context(), p, chi.URLParam(r, "id"))
	knowledgeResult(w, r, out, e)
}
func (h *KnowledgeHandler) Search(w http.ResponseWriter, r *http.Request) {
	p, ok := knowledgeUser(w, r)
	if !ok {
		return
	}
	h.search(w, r, p)
}
func (h *KnowledgeHandler) MachineSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	auth := strings.Fields(r.Header.Get("Authorization"))
	if len(auth) != 2 || !strings.EqualFold(auth[0], "Bearer") {
		knowledgeError(w, r, k.ErrUnauthenticated)
		return
	}
	p, e := h.service.Authenticate(r.Context(), auth[1])
	if e != nil {
		knowledgeError(w, r, e)
		return
	}
	h.search(w, r, p)
}
func (h *KnowledgeHandler) search(w http.ResponseWriter, r *http.Request, p k.Principal) {
	start := time.Now()
	var in k.SearchRequest
	if !knowledgeJSON(w, r, &in) {
		return
	}
	info := k.RetrievalInfo{}
	out, e := h.service.SearchDetailed(r.Context(), p, in, &info)
	status := "ok"
	if e != nil {
		status = "knowledge_unavailable"
		for _, known := range []error{k.ErrInvalid, k.ErrForbidden, k.ErrUnauthenticated, k.ErrNotFound, k.ErrConflict, k.ErrNotReady, k.ErrLimited, context.DeadlineExceeded, context.Canceled} {
			if errors.Is(e, known) {
				status = known.Error()
				break
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if auditErr := h.service.Repo.Audit(ctx, p, in.KBIDs, middleware.GetReqID(r.Context()), status, len(out), time.Since(start)); auditErr != nil && e == nil {
		e = k.ErrUnavailable
	}
	if e == nil {
		out, e = h.service.Repo.Recheck(r.Context(), p, in.KBIDs, out)
	}
	knowledgeResult(w, r, map[string]any{"request_id": middleware.GetReqID(r.Context()), "matches": out, "retrieval": info}, e)
}
func (h *KnowledgeHandler) Audit(w http.ResponseWriter, r *http.Request) {
	p, ok := knowledgeUser(w, r)
	if !ok {
		return
	}
	out, e := h.service.Repo.AuditEntries(r.Context(), p)
	knowledgeResult(w, r, out, e)
}
func (h *KnowledgeHandler) Applications(w http.ResponseWriter, r *http.Request) {
	p, ok := knowledgeUser(w, r)
	if !ok {
		return
	}
	if r.Method == "GET" {
		out, e := h.service.Repo.Applications(r.Context(), p)
		knowledgeResult(w, r, out, e)
		return
	}
	id := chi.URLParam(r, "id")
	if id != "" {
		if _, e := uuid.Parse(id); e != nil {
			knowledgeError(w, r, k.ErrInvalid)
			return
		}
	}
	var in k.ApplicationInput
	if !knowledgeJSON(w, r, &in) {
		return
	}
	out, e := h.service.SaveApplication(r.Context(), p, id, in)
	knowledgeResult(w, r, out, e)
}
func (h *KnowledgeHandler) Rotate(w http.ResponseWriter, r *http.Request) {
	p, ok := knowledgeUser(w, r)
	if !ok {
		return
	}
	out, e := h.service.Rotate(r.Context(), p, chi.URLParam(r, "id"))
	knowledgeResult(w, r, out, e)
}
func (h *KnowledgeHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	p, ok := knowledgeUser(w, r)
	if !ok {
		return
	}
	knowledgeResult(w, r, map[string]bool{"revoked": true}, h.service.Repo.Revoke(r.Context(), p, chi.URLParam(r, "id")))
}
