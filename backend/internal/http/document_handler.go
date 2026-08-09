package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

type DocumentHandler struct {
	docSvc       *service.Document
	ingestionSvc *service.Ingestion
	maxUpload    int64
}

func NewDocumentHandler(docSvc *service.Document, ingestionSvc *service.Ingestion, maxUploadBytes int64) *DocumentHandler {
	return &DocumentHandler{
		docSvc: docSvc, ingestionSvc: ingestionSvc, maxUpload: maxUploadBytes,
	}
}

func (h *DocumentHandler) Upload(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	kbID := chi.URLParam(r, "id")

	r.Body = http.MaxBytesReader(w, r.Body, h.maxUpload)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			WriteError(w, r, NewAPIError(http.StatusRequestEntityTooLarge, CodePayloadTooLarge,
				"upload exceeds size limit"))
			return
		}
		WriteError(w, r, NewAPIError(http.StatusBadRequest, CodeValidationFailed, err.Error()))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		WriteError(w, r, NewAPIError(http.StatusBadRequest, CodeValidationFailed, "missing form field 'file'"))
		return
	}
	defer file.Close()

	mime := header.Header.Get("Content-Type")
	if mime == "" {
		mime = "application/octet-stream"
	}

	doc, err := h.ingestionSvc.Upload(r.Context(), userID, service.UploadInput{
		KBID:     kbID,
		Title:    header.Filename,
		MimeType: mime,
		Body:     file,
		Size:     header.Size,
	})
	if err != nil {
		var dup *service.ErrDuplicateChecksum
		if errors.As(err, &dup) {
			WriteError(w, r, WithDetails(
				NewAPIError(http.StatusConflict, CodeDuplicateChecksum,
					"document with same content already exists in this KB"),
				map[string]any{"existing_doc_id": dup.ExistingDocID},
			))
			return
		}
		var kbnotfound *service.ErrKBNotFound
		if errors.As(err, &kbnotfound) {
			WriteError(w, r, NewAPIError(http.StatusNotFound, CodeKBNotFound, err.Error()))
			return
		}
		WriteError(w, r, err)
		return
	}

	WriteJSON(w, http.StatusCreated, doc)
}

func (h *DocumentHandler) ListByKB(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	kbID := chi.URLParam(r, "id")
	p, err := ParsePagination(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	var statusFilter *string
	if s := r.URL.Query().Get("status"); s != "" {
		statusFilter = &s
	}
	docs, total, err := h.docSvc.ListByKB(r.Context(), userID, kbID, statusFilter, p.Limit, p.Offset)
	if err != nil {
		var kbNF *service.ErrKBNotFound
		if errors.As(err, &kbNF) {
			WriteError(w, r, NewAPIError(http.StatusNotFound, CodeKBNotFound, err.Error()))
			return
		}
		WriteError(w, r, err)
		return
	}
	WriteListResponse(w, total, docs)
}

func (h *DocumentHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	doc, err := h.docSvc.Get(r.Context(), userID, id)
	if err != nil {
		var notFound *service.ErrDocNotFound
		if errors.As(err, &notFound) {
			WriteError(w, r, NewAPIError(http.StatusNotFound, CodeDocNotFound, err.Error()))
			return
		}
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, doc)
}

func (h *DocumentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.docSvc.Delete(r.Context(), userID, id); err != nil {
		var notFound *service.ErrDocNotFound
		if errors.As(err, &notFound) {
			WriteError(w, r, NewAPIError(http.StatusNotFound, CodeDocNotFound, err.Error()))
			return
		}
		WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *DocumentHandler) Reingest(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	doc, err := h.ingestionSvc.Reingest(r.Context(), userID, id)
	if err != nil {
		var notFound *service.ErrDocNotFound
		if errors.As(err, &notFound) {
			WriteError(w, r, NewAPIError(http.StatusNotFound, CodeDocNotFound, err.Error()))
			return
		}
		var busy *service.ErrDocProcessing
		if errors.As(err, &busy) {
			WriteError(w, r, NewAPIError(http.StatusConflict, CodeValidationFailed, err.Error()))
			return
		}
		var unsupported *service.ErrRemoteReingestUnsupported
		if errors.As(err, &unsupported) {
			WriteError(w, r, NewAPIError(http.StatusUnprocessableEntity, CodeUnsupportedOperation, err.Error()))
			return
		}
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusAccepted, doc)
}

func (h *DocumentHandler) ReingestKB(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	kbID := chi.URLParam(r, "id")
	n, err := h.ingestionSvc.ReingestKB(r.Context(), userID, kbID)
	if err != nil {
		var kbNF *service.ErrKBNotFound
		if errors.As(err, &kbNF) {
			WriteError(w, r, NewAPIError(http.StatusNotFound, CodeKBNotFound, err.Error()))
			return
		}
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusAccepted, map[string]any{"enqueued": n})
}
