package http

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/zenith-wang/it-wiki/backend/internal/infra/vectorstore"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

type ChunkHandler struct {
	vstore *vectorstore.Pgvector
	docSvc *service.Document
}

func NewChunkHandler(vstore *vectorstore.Pgvector, docSvc *service.Document) *ChunkHandler {
	return &ChunkHandler{vstore: vstore, docSvc: docSvc}
}

func (h *ChunkHandler) ListByDoc(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "id")

	if _, err := h.docSvc.Get(r.Context(), docID); err != nil {
		var notFound *service.ErrDocNotFound
		if errors.As(err, &notFound) {
			WriteError(w, r, NewAPIError(http.StatusNotFound, CodeDocNotFound, err.Error()))
			return
		}
		WriteError(w, r, err)
		return
	}

	p, err := ParsePagination(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	chunks, total, err := h.vstore.ListByDocument(r.Context(), docID, p.Limit, p.Offset)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteListResponse(w, total, chunks)
}
