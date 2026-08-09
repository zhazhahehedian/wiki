package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

type documentResponseQueries struct {
	document generated.Document
}

func (q documentResponseQueries) GetKnowledgeBaseForOwner(context.Context, generated.GetKnowledgeBaseForOwnerParams) (generated.KnowledgeBase, error) {
	return generated.KnowledgeBase{}, nil
}

func (q documentResponseQueries) GetDocumentForOwner(context.Context, generated.GetDocumentForOwnerParams) (generated.Document, error) {
	return q.document, nil
}

func (q documentResponseQueries) ListDocumentsByKBForOwner(context.Context, generated.ListDocumentsByKBForOwnerParams) ([]generated.Document, error) {
	return nil, nil
}

func (q documentResponseQueries) CountDocumentsByKBForOwner(context.Context, generated.CountDocumentsByKBForOwnerParams) (int64, error) {
	return 0, nil
}

func (q documentResponseQueries) DeleteDocumentForOwner(context.Context, generated.DeleteDocumentForOwnerParams) (generated.DeleteDocumentForOwnerRow, error) {
	return generated.DeleteDocumentForOwnerRow{ContentRef: q.document.ContentRef, PendingContentRef: q.document.PendingContentRef}, nil
}

func (q documentResponseQueries) FindDocumentByChecksumForOwner(context.Context, generated.FindDocumentByChecksumForOwnerParams) (generated.Document, error) {
	return generated.Document{}, nil
}

func (q documentResponseQueries) CreateDocumentForOwner(context.Context, generated.CreateDocumentForOwnerParams) (generated.Document, error) {
	return q.document, nil
}

func (q documentResponseQueries) UpdateDocumentStatusForOwner(context.Context, generated.UpdateDocumentStatusForOwnerParams) error {
	return nil
}

type recordingDocumentIngestionQueue struct{ calls int }

func (q *recordingDocumentIngestionQueue) EnqueueIngestion(context.Context, string) error {
	q.calls++
	return nil
}

func TestDocumentGetExposesFeishuSyncFieldsWithoutPrivateState(t *testing.T) {
	userID, kbID, docID, accountID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	sourceURL := "https://acme.feishu.cn/docx/DocToken_123"
	remoteRevision := "rev-42"
	lastSyncError := "remote rate limit"
	privateValue := "must-not-leak"
	syncedAt := time.Date(2026, time.August, 9, 10, 11, 12, 0, time.UTC)
	createdAt := syncedAt.Add(-2 * time.Hour)
	updatedAt := syncedAt.Add(-time.Hour)

	queries := documentResponseQueries{document: generated.Document{
		ID:                    docID,
		KbID:                  kbID,
		SourceType:            "feishu-docx",
		SourceRef:             "DocToken_123",
		Title:                 "Runbook",
		MimeType:              "text/markdown",
		Bytes:                 128,
		Checksum:              "checksum",
		Status:                string(domain.StatusReady),
		Metadata:              json.RawMessage(`{"section_path":"Operations"}`),
		CreatedAt:             createdAt,
		UpdatedAt:             updatedAt,
		ContentRef:            &privateValue,
		SourceUrl:             &sourceURL,
		RemoteRevision:        &remoteRevision,
		OauthAccountID:        pgtype.UUID{Bytes: accountID, Valid: true},
		PendingContentRef:     &privateValue,
		PendingChecksum:       &privateValue,
		PendingRemoteRevision: &privateValue,
		SyncStatus:            "failed",
		LastSyncError:         &lastSyncError,
		LastSyncedAt:          pgtype.Timestamptz{Time: syncedAt, Valid: true},
		PendingTitle:          &privateValue,
		PendingBytes:          new(int64),
		PendingMetadata:       []byte(`{"secret":"must-not-leak"}`),
	}}
	handler := NewDocumentHandler(service.NewDocument(queries, nil), nil, 0)
	router := chi.NewRouter()
	router.Get("/api/v1/docs/{id}", handler.Get)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs/"+docID.String(), nil)
	req = req.WithContext(WithCurrentUser(req.Context(), domain.User{ID: userID.String()}))
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	publicValues := map[string]any{
		"source_url":      sourceURL,
		"remote_revision": remoteRevision,
		"sync_status":     "failed",
		"last_sync_error": lastSyncError,
		"last_synced_at":  syncedAt.Format(time.RFC3339),
	}
	for key, want := range publicValues {
		if got := body[key]; got != want {
			t.Errorf("response[%q]=%v, want %v; body=%s", key, got, want, recorder.Body.String())
		}
	}

	gotKeys := make([]string, 0, len(body))
	for key := range body {
		gotKeys = append(gotKeys, key)
	}
	sort.Strings(gotKeys)
	wantKeys := []string{
		"bytes", "checksum", "created_at", "id", "kb_id", "last_sync_error",
		"last_synced_at", "metadata", "mime_type", "remote_revision", "source_ref",
		"source_type", "source_url", "status", "sync_status", "title", "updated_at",
	}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("response keys=%v, want %v; body=%s", gotKeys, wantKeys, recorder.Body.String())
	}
}

func TestDocumentReingestRejectsRemoteDocumentWithPublicSyncGuidance(t *testing.T) {
	userID, kbID, docID := uuid.New(), uuid.New(), uuid.New()
	privateSourceRef := "provider-token=private-content"
	queries := documentResponseQueries{document: generated.Document{
		ID: docID, KbID: kbID, SourceType: "feishu-docx", SourceRef: privateSourceRef,
		Status: string(domain.StatusReady), Metadata: json.RawMessage(`{}`),
	}}
	queue := &recordingDocumentIngestionQueue{}
	handler := NewDocumentHandler(nil, service.NewIngestion(queries, nil, queue), 0)
	router := chi.NewRouter()
	router.Post("/api/v1/docs/{id}/reingest", handler.Reingest)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docs/"+docID.String()+"/reingest", nil)
	req = req.WithContext(WithCurrentUser(req.Context(), domain.User{ID: userID.String()}))
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body errorEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != CodeUnsupportedOperation || body.Error.Message != "remote documents cannot be reingested; use /sync" {
		t.Fatalf("error=%+v", body.Error)
	}
	if queue.calls != 0 {
		t.Fatalf("queue calls=%d, want 0", queue.calls)
	}
	if bodyText := recorder.Body.String(); len(bodyText) == 0 || strings.Contains(bodyText, privateSourceRef) {
		t.Fatalf("response leaked private source reference: %s", bodyText)
	}
}
