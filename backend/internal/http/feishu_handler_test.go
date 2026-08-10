package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

type fakeHTTPFeishuAccounts struct {
	account domain.OAuthAccount
	err     error
}

func (f fakeHTTPFeishuAccounts) Resolve(context.Context, string) (domain.OAuthAccount, error) {
	return f.account, f.err
}

type fakeHTTPFeishuImport struct {
	input service.FeishuImportInput
	err   error
}

func (f *fakeHTTPFeishuImport) Import(_ context.Context, input service.FeishuImportInput) (*domain.Document, error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return &domain.Document{ID: uuid.NewString(), KBID: input.KBID, SourceType: "feishu-docx"}, nil
}

type fakeHTTPFeishuSync struct {
	userID, accountID, documentID string
	err                           error
}

func (f *fakeHTTPFeishuSync) Sync(_ context.Context, userID, accountID, documentID string) (*domain.Document, error) {
	f.userID, f.accountID, f.documentID = userID, accountID, documentID
	if f.err != nil {
		return nil, f.err
	}
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
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), `"code":"invalid_request"`) {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestFeishuImportHandlerRejectsNonStrictJSONBodies(t *testing.T) {
	for _, body := range []string{
		`{"url":"https://acme.feishu.cn/docx/token","user_id":"attacker"}`,
		`{"url":"https://acme.feishu.cn/docx/token"} {}`,
		`{"url":"https://acme.feishu.cn/docx/token"} trailing`,
		`{"url":`,
	} {
		t.Run(body, func(t *testing.T) {
			rr := serveFeishuImport(t, fakeHTTPFeishuAccounts{account: domain.OAuthAccount{ID: uuid.NewString()}}, &fakeHTTPFeishuImport{}, body)
			if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), `"code":"invalid_request"`) {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
		})
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

func TestFeishuSyncHandlerAcceptsOnlyEmptyOrEmptyObjectBody(t *testing.T) {
	valid := []string{"", " \r\n\t", `{}`}
	for _, body := range valid {
		t.Run("valid_"+body, func(t *testing.T) {
			rr := serveFeishuSync(t, fakeHTTPFeishuAccounts{account: domain.OAuthAccount{ID: uuid.NewString()}}, &fakeHTTPFeishuSync{}, body)
			if rr.Code != http.StatusAccepted {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
		})
	}

	invalid := []string{
		`{"user_id":"attacker"}`,
		`{"oauth_account_id":"attacker"}`,
		`{"account_id":"attacker"}`,
		`{} {}`,
		`{} trailing`,
		`{`,
		`null`,
	}
	for _, body := range invalid {
		t.Run("invalid_"+body, func(t *testing.T) {
			rr := serveFeishuSync(t, fakeHTTPFeishuAccounts{account: domain.OAuthAccount{ID: uuid.NewString()}}, &fakeHTTPFeishuSync{}, body)
			if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), `"code":"invalid_request"`) {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestFeishuHandlersRejectOversizedBodies(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T, string) *httptest.ResponseRecorder
		body string
	}{
		{
			name: "import",
			run: func(t *testing.T, body string) *httptest.ResponseRecorder {
				return serveFeishuImport(t, fakeHTTPFeishuAccounts{account: domain.OAuthAccount{ID: uuid.NewString()}}, &fakeHTTPFeishuImport{}, body)
			},
			body: `{"url":"` + strings.Repeat("a", 1<<20) + `"}`,
		},
		{
			name: "sync",
			run: func(t *testing.T, body string) *httptest.ResponseRecorder {
				return serveFeishuSync(t, fakeHTTPFeishuAccounts{account: domain.OAuthAccount{ID: uuid.NewString()}}, &fakeHTTPFeishuSync{}, body)
			},
			body: `{}` + strings.Repeat(" ", 1<<20),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := tt.run(t, tt.body)
			if rr.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rr.Body.String(), `"code":"request_too_large"`) || !strings.Contains(rr.Body.String(), `"message":"request too large"`) {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestFeishuSyncRejectsOversizedChunkedBody(t *testing.T) {
	userID := uuid.NewString()
	h := NewFeishuHandler(fakeHTTPFeishuAccounts{account: domain.OAuthAccount{ID: uuid.NewString()}}, &fakeHTTPFeishuImport{}, &fakeHTTPFeishuSync{})
	router := chi.NewRouter()
	router.Post("/api/v1/docs/{docID}/sync", h.Sync)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docs/"+uuid.NewString()+"/sync", strings.NewReader(`{}`+strings.Repeat(" ", 1<<20)))
	req.ContentLength = -1
	req = req.WithContext(WithCurrentUser(req.Context(), domain.User{ID: userID}))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rr.Body.String(), `"code":"request_too_large"`) {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestFeishuHandlersHideUnexpectedErrorDetails(t *testing.T) {
	secret := "postgres failed token=secret-token url=https://evil.example/private"
	tests := []struct {
		name string
		run  func(*testing.T) *httptest.ResponseRecorder
	}{
		{name: "account", run: func(t *testing.T) *httptest.ResponseRecorder {
			return serveFeishuImport(t, fakeHTTPFeishuAccounts{err: errors.New(secret)}, &fakeHTTPFeishuImport{}, `{"url":"https://acme.feishu.cn/docx/token"}`)
		}},
		{name: "import", run: func(t *testing.T) *httptest.ResponseRecorder {
			return serveFeishuImport(t, fakeHTTPFeishuAccounts{account: domain.OAuthAccount{ID: uuid.NewString()}}, &fakeHTTPFeishuImport{err: errors.New(secret)}, `{"url":"https://acme.feishu.cn/docx/token"}`)
		}},
		{name: "sync", run: func(t *testing.T) *httptest.ResponseRecorder {
			return serveFeishuSync(t, fakeHTTPFeishuAccounts{account: domain.OAuthAccount{ID: uuid.NewString()}}, &fakeHTTPFeishuSync{err: errors.New(secret)}, `{}`)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := tt.run(t)
			body := rr.Body.String()
			if rr.Code != http.StatusInternalServerError || !strings.Contains(body, `"code":"internal_error"`) || !strings.Contains(body, `"request_id":`) {
				t.Fatalf("status=%d body=%s", rr.Code, body)
			}
			if strings.Contains(body, "secret-token") || strings.Contains(body, "evil.example") || strings.Contains(body, "postgres failed") {
				t.Fatalf("unexpected error leaked: %s", body)
			}
		})
	}
}

func serveFeishuImport(t *testing.T, accounts fakeHTTPFeishuAccounts, imports *fakeHTTPFeishuImport, body string) *httptest.ResponseRecorder {
	t.Helper()
	userID := uuid.NewString()
	h := NewFeishuHandler(accounts, imports, &fakeHTTPFeishuSync{})
	router := chi.NewRouter()
	router.Post("/api/v1/kbs/{kbID}/feishu-imports", h.Import)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kbs/"+uuid.NewString()+"/feishu-imports", strings.NewReader(body))
	req = req.WithContext(WithCurrentUser(req.Context(), domain.User{ID: userID}))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func serveFeishuSync(t *testing.T, accounts fakeHTTPFeishuAccounts, syncer *fakeHTTPFeishuSync, body string) *httptest.ResponseRecorder {
	t.Helper()
	userID := uuid.NewString()
	h := NewFeishuHandler(accounts, &fakeHTTPFeishuImport{}, syncer)
	router := chi.NewRouter()
	router.Post("/api/v1/docs/{docID}/sync", h.Sync)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docs/"+uuid.NewString()+"/sync", strings.NewReader(body))
	req = req.WithContext(WithCurrentUser(req.Context(), domain.User{ID: userID}))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}
