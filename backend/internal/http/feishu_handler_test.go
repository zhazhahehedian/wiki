package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

type fakeHTTPFeishuAccounts struct{ account domain.OAuthAccount }

func (f fakeHTTPFeishuAccounts) Resolve(context.Context, string) (domain.OAuthAccount, error) {
	return f.account, nil
}

type fakeHTTPFeishuImport struct{ input service.FeishuImportInput }

func (f *fakeHTTPFeishuImport) Import(_ context.Context, input service.FeishuImportInput) (*domain.Document, error) {
	f.input = input
	return &domain.Document{ID: uuid.NewString(), KBID: input.KBID, SourceType: "feishu-docx"}, nil
}

type fakeHTTPFeishuSync struct{ userID, accountID, documentID string }

func (f *fakeHTTPFeishuSync) Sync(_ context.Context, userID, accountID, documentID string) (*domain.Document, error) {
	f.userID, f.accountID, f.documentID = userID, accountID, documentID
	return &domain.Document{ID: documentID, SourceType: "feishu-docx"}, nil
}

func TestFeishuImportHandlerDerivesOwnerAndAccountServerSide(t *testing.T) {
	userID, accountID, kbID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	imports := &fakeHTTPFeishuImport{}
	h := NewFeishuHandler(fakeHTTPFeishuAccounts{account: domain.OAuthAccount{ID: accountID, UserID: userID}}, imports, &fakeHTTPFeishuSync{})
	router := chi.NewRouter()
	router.Post("/api/v1/kbs/{kbID}/feishu-imports", h.Import)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kbs/"+kbID+"/feishu-imports", strings.NewReader(`{"url":"https://acme.feishu.cn/docx/token"}`))
	req = req.WithContext(WithCurrentUser(req.Context(), domain.User{ID: userID}))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if imports.input.UserID != userID || imports.input.OAuthAccountID != accountID || imports.input.KBID != kbID {
		t.Fatalf("import input=%#v", imports.input)
	}
}

func TestFeishuImportHandlerRejectsBrowserSuppliedAccountID(t *testing.T) {
	userID := uuid.NewString()
	h := NewFeishuHandler(fakeHTTPFeishuAccounts{account: domain.OAuthAccount{ID: uuid.NewString(), UserID: userID}}, &fakeHTTPFeishuImport{}, &fakeHTTPFeishuSync{})
	router := chi.NewRouter()
	router.Post("/api/v1/kbs/{kbID}/feishu-imports", h.Import)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kbs/"+uuid.NewString()+"/feishu-imports", strings.NewReader(`{"url":"https://acme.feishu.cn/docx/token","oauth_account_id":"attacker"}`))
	req = req.WithContext(WithCurrentUser(req.Context(), domain.User{ID: userID}))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestFeishuSyncHandlerDerivesOwnerAndAccountServerSide(t *testing.T) {
	userID, accountID, docID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	syncer := &fakeHTTPFeishuSync{}
	h := NewFeishuHandler(fakeHTTPFeishuAccounts{account: domain.OAuthAccount{ID: accountID, UserID: userID}}, &fakeHTTPFeishuImport{}, syncer)
	router := chi.NewRouter()
	router.Post("/api/v1/docs/{docID}/sync", h.Sync)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docs/"+docID+"/sync", nil)
	req = req.WithContext(WithCurrentUser(req.Context(), domain.User{ID: userID}))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted || syncer.userID != userID || syncer.accountID != accountID || syncer.documentID != docID {
		t.Fatalf("status=%d sync=%#v body=%s", rr.Code, syncer, rr.Body.String())
	}
}
