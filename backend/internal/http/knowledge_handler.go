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
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
)

type KnowledgeHandler struct{ service *k.Service }

func NewKnowledgeHandler(s *k.Service) *KnowledgeHandler                { return &KnowledgeHandler{s} }
func (h *KnowledgeHandler) WithDirectory(d registry.SelectionValidator) { h.service.Directory = d }

// knowledgeFailure is the public form of a knowledge error; its code is also
// the search audit status.
type knowledgeFailure struct {
	status        int
	code, message string
}

func classifyKnowledge(e error) knowledgeFailure {
	switch {
	case errors.Is(e, k.ErrInvalid):
		return knowledgeFailure{400, "invalid_request", "请检查输入、文档格式或请求字段"}
	case errors.Is(e, k.ErrUnauthenticated):
		return knowledgeFailure{401, "invalid_credential", "应用凭证无效或已失效"}
	case errors.Is(e, k.ErrForbidden):
		return knowledgeFailure{403, "knowledge_forbidden", "无权访问所请求的知识资源"}
	case errors.Is(e, k.ErrNotFound):
		return knowledgeFailure{404, "knowledge_not_found", "知识资源不存在"}
	case errors.Is(e, k.ErrConflict):
		return knowledgeFailure{409, "knowledge_conflict", "版本或共享条件不满足，请刷新检查"}
	case errors.Is(e, k.ErrNotReady):
		return knowledgeFailure{409, "knowledge_not_ready", "知识库尚无可检索版本，请检查文档处理状态与索引配置"}
	case errors.Is(e, k.ErrLimited):
		return knowledgeFailure{429, "knowledge_rate_limited", "请求过于频繁，请稍后重试"}
	case errors.Is(e, context.DeadlineExceeded):
		return knowledgeFailure{504, "knowledge_timeout", "检索超时，请稍后重试"}
	case errors.Is(e, context.Canceled):
		return knowledgeFailure{0, "client_canceled", ""}
	}
	return knowledgeFailure{503, "knowledge_unavailable", "知识服务依赖暂不可用，请稍后重试"}
}
func knowledgeError(w http.ResponseWriter, r *http.Request, e error) {
	if directoryHTTPError(w, r, e) {
		return
	}
	f := classifyKnowledge(e)
	if f.status == 0 {
		return // client went away; nothing to write
	}
	if f.status == 429 {
		w.Header().Set("Retry-After", "60")
	}
	WriteError(w, r, NewAPIError(f.status, f.code, f.message))
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
	// Normalized once here so the audit stores canonical, deduplicated IDs.
	var out []k.Match
	var info k.RetrievalInfo
	e := k.ValidateSearch(&in)
	if e != nil {
		in.KBIDs = nil
	} else {
		out, info, e = h.service.Search(r.Context(), p, in)
	}
	status := "ok"
	if e != nil {
		status = classifyKnowledge(e).code
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// The audit records exactly the outcome returned below; a result that
	// cannot be audited is withheld.
	if auditErr := h.service.Repo.Audit(ctx, p, in.KBIDs, middleware.GetReqID(r.Context()), status, len(out), time.Since(start)); auditErr != nil && e == nil {
		e = k.ErrUnavailable
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
