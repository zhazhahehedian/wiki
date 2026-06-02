package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

type KBHandler struct {
	svc *service.KB
}

func NewKBHandler(svc *service.KB) *KBHandler {
	return &KBHandler{svc: svc}
}

func (h *KBHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in service.CreateKBInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		WriteError(w, r, NewAPIError(http.StatusBadRequest, CodeValidationFailed, "invalid json body"))
		return
	}
	if in.Name == "" {
		WriteError(w, r, NewAPIError(http.StatusBadRequest, CodeValidationFailed, "name is required"))
		return
	}
	kb, err := h.svc.Create(r.Context(), in)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusCreated, kb)
}

func (h *KBHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	kb, err := h.svc.Get(r.Context(), id)
	if err != nil {
		var notFound *service.ErrKBNotFound
		if errors.As(err, &notFound) {
			WriteError(w, r, NewAPIError(http.StatusNotFound, CodeKBNotFound, err.Error()))
			return
		}
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, kb)
}

func (h *KBHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := ParsePagination(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	kbs, total, err := h.svc.List(r.Context(), p.Limit, p.Offset)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteListResponse(w, total, kbs)
}

func (h *KBHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.svc.Delete(r.Context(), id); err != nil {
		var notFound *service.ErrKBNotFound
		if errors.As(err, &notFound) {
			WriteError(w, r, NewAPIError(http.StatusNotFound, CodeKBNotFound, err.Error()))
			return
		}
		WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
