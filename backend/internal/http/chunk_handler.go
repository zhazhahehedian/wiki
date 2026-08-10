package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
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
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	docID := chi.URLParam(r, "id")

	if _, err := h.docSvc.Get(r.Context(), userID, docID); err != nil {
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
	chunks, total, err := h.vstore.ListByDocument(r.Context(), userID, docID, p.Limit, p.Offset)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteListResponse(w, total, chunks)
}

func (h *ChunkHandler) Neighbors(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	kbID := chi.URLParam(r, "kbID")
	chunkID := chi.URLParam(r, "chunkID")

	window := 2
	if s := r.URL.Query().Get("window"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 0 || v > 10 {
			WriteError(w, r, NewAPIError(http.StatusBadRequest, CodeValidationFailed, "window must be int in [0,10]"))
			return
		}
		window = v
	}

	primary, err := h.vstore.GetChunk(r.Context(), userID, kbID, chunkID)
	if err != nil {
		WriteError(w, r, NewAPIError(http.StatusNotFound, CodeChunkNotFound, "chunk not found"))
		return
	}
	chunks, err := h.vstore.ListNeighbors(r.Context(), userID, kbID, primary.DocumentID, primary.Seq, window)
	if err != nil {
		WriteError(w, r, err)
		return
	}

	out := domain.ChunkNeighbors{PrimaryChunkID: primary.ID, Window: window}
	for _, c := range chunks {
		out.Chunks = append(out.Chunks, domain.NeighborChunk{
			ID:         c.ID,
			DocumentID: c.DocumentID,
			Seq:        c.Seq,
			Content:    c.Content,
			IsPrimary:  c.ID == primary.ID,
		})
	}
	WriteJSON(w, http.StatusOK, out)
}
