package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

type FeishuAccountResolver interface {
	Resolve(context.Context, string) (domain.OAuthAccount, error)
}

type FeishuImporter interface {
	Import(context.Context, service.FeishuImportInput) (*domain.Document, error)
}

type FeishuSyncer interface {
	Sync(context.Context, string, string, string) (*domain.Document, error)
}

type FeishuHandler struct {
	accounts FeishuAccountResolver
	imports  FeishuImporter
	syncer   FeishuSyncer
}

func NewFeishuHandler(accounts FeishuAccountResolver, imports FeishuImporter, syncer FeishuSyncer) *FeishuHandler {
	return &FeishuHandler{accounts: accounts, imports: imports, syncer: syncer}
}

type feishuImportRequest struct {
	URL string `json:"url"`
}

type feishuSyncRequest struct{}

var errInvalidFeishuRequest = errors.New("invalid Feishu request")

func (h *FeishuHandler) Import(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var req feishuImportRequest
	if err := decodeFeishuImportRequest(r.Body, &req); err != nil {
		WriteError(w, r, invalidFeishuRequestError())
		return
	}
	account, err := h.accounts.Resolve(r.Context(), userID)
	if err != nil {
		WriteError(w, r, mapFeishuHTTPError(err))
		return
	}
	doc, err := h.imports.Import(r.Context(), service.FeishuImportInput{UserID: userID, KBID: chi.URLParam(r, "kbID"), OAuthAccountID: account.ID, URL: req.URL})
	if err != nil {
		WriteError(w, r, mapFeishuHTTPError(err))
		return
	}
	WriteJSON(w, http.StatusAccepted, doc)
}

func (h *FeishuHandler) Sync(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	if err := decodeFeishuSyncRequest(r.Body); err != nil {
		WriteError(w, r, invalidFeishuRequestError())
		return
	}
	account, err := h.accounts.Resolve(r.Context(), userID)
	if err != nil {
		WriteError(w, r, mapFeishuHTTPError(err))
		return
	}
	doc, err := h.syncer.Sync(r.Context(), userID, account.ID, chi.URLParam(r, "docID"))
	if err != nil {
		WriteError(w, r, mapFeishuHTTPError(err))
		return
	}
	WriteJSON(w, http.StatusAccepted, doc)
}

func decodeFeishuImportRequest(body io.Reader, req *feishuImportRequest) error {
	if body == nil {
		return errInvalidFeishuRequest
	}
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(req); err != nil || strings.TrimSpace(req.URL) == "" {
		return errInvalidFeishuRequest
	}
	if err := requireJSONEOF(decoder); err != nil {
		return errInvalidFeishuRequest
	}
	return nil
}

// Sync accepts either no JSON value or exactly one empty object.
func decodeFeishuSyncRequest(body io.Reader) error {
	if body == nil {
		return nil
	}
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var req *feishuSyncRequest
	err := decoder.Decode(&req)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil || req == nil {
		return errInvalidFeishuRequest
	}
	if err := requireJSONEOF(decoder); err != nil {
		return errInvalidFeishuRequest
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errInvalidFeishuRequest
	}
	return nil
}

func invalidFeishuRequestError() *APIError {
	return NewAPIError(http.StatusBadRequest, CodeInvalidRequest, "invalid request")
}

func mapFeishuHTTPError(err error) error {
	var already *service.ErrFeishuAlreadyImported
	if errors.As(err, &already) {
		return NewAPIError(http.StatusConflict, CodeResourceAlreadyImported, "Feishu resource already imported")
	}
	var busy *service.ErrFeishuSyncInProgress
	if errors.As(err, &busy) {
		return NewAPIError(http.StatusConflict, CodeSyncInProgress, "Feishu document sync is already in progress")
	}
	var unsupportedDoc *service.ErrUnsupportedFeishuDocument
	if errors.As(err, &unsupportedDoc) {
		return NewAPIError(http.StatusUnprocessableEntity, CodeUnsupportedResource, "unsupported resource")
	}
	if errors.Is(err, ports.ErrUnsupportedFeishuURL) {
		return NewAPIError(http.StatusUnprocessableEntity, CodeUnsupportedFeishuURL, "unsupported Feishu URL")
	}
	var loadErr *ports.SourceLoadError
	if errors.As(err, &loadErr) {
		switch loadErr.Code {
		case ports.SourceLoadForbidden:
			return NewAPIError(http.StatusForbidden, CodeResourceForbidden, "resource access forbidden")
		case ports.SourceLoadNotFound:
			return NewAPIError(http.StatusNotFound, CodeResourceNotFound, "resource not found")
		case ports.SourceLoadUnsupported:
			return NewAPIError(http.StatusUnprocessableEntity, CodeUnsupportedResource, "unsupported resource")
		}
	}
	var reauth *service.ErrFeishuReauthRequired
	if errors.As(err, &reauth) {
		return NewAPIError(http.StatusUnauthorized, CodeReauthRequired, "Feishu reauthentication required")
	}
	var noAccount *service.ErrFeishuAccountNotFound
	if errors.As(err, &noAccount) {
		return NewAPIError(http.StatusNotFound, CodeResourceNotFound, "resource not found")
	}
	var noDoc *service.ErrDocNotFound
	var ownership *service.ErrFeishuImportOwnership
	if errors.As(err, &noDoc) || errors.As(err, &ownership) {
		return NewAPIError(http.StatusNotFound, CodeResourceNotFound, "resource not found")
	}
	return NewAPIError(http.StatusInternalServerError, CodeInternalError, "internal server error")
}
