package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

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

func (h *FeishuHandler) Import(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var req feishuImportRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || req.URL == "" {
		WriteError(w, r, NewAPIError(http.StatusBadRequest, CodeValidationFailed, "invalid Feishu import request"))
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
	return err
}
