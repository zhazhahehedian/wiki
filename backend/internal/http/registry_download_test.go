package http

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
)

type downloadStubRepo struct {
	registry.Repository
	audits int
}

func (r *downloadStubRepo) Read(context.Context, string, string, bool) (registry.Detail, error) {
	return registry.Detail{Capability: registry.Capability{ID: "cap"}, Versions: []registry.Version{{
		ID: "v1", Version: "1.0.0", BundleKey: "registry/a.zip", BundleSize: 4,
	}}}, nil
}

func (r *downloadStubRepo) RecordDownload(context.Context, string, string, string) error {
	r.audits++
	return nil
}

type scriptedStorage struct {
	data      []byte
	failAfter int
}

func (s *scriptedStorage) Put(context.Context, string, io.Reader, int64, string) error { return nil }
func (s *scriptedStorage) Delete(context.Context, string) error                        { return nil }
func (s *scriptedStorage) Get(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(&failAfterReader{data: s.data, at: s.failAfter}), nil
}

// failAfterReader returns an error once at bytes have been produced.
// A negative at disables the failure.
type failAfterReader struct {
	data []byte
	at   int
	n    int
}

func (f *failAfterReader) Read(p []byte) (int, error) {
	if f.at >= 0 && f.n >= f.at {
		return 0, errors.New("storage interrupted")
	}
	if f.n >= len(f.data) {
		return 0, io.EOF
	}
	if f.at >= 0 && len(p) > f.at-f.n {
		p = p[:f.at-f.n]
	}
	n := copy(p, f.data[f.n:])
	f.n += n
	if f.at >= 0 && f.n >= f.at {
		return n, errors.New("storage interrupted")
	}
	if f.n >= len(f.data) {
		return n, io.EOF
	}
	return n, nil
}

func downloadRequest() *http.Request {
	route := chi.NewRouteContext()
	route.URLParams.Add("slug", "slug")
	route.URLParams.Add("versionID", "v1")
	ctx := WithCurrentUser(context.Background(), domain.User{ID: uuid.NewString()})
	ctx = context.WithValue(ctx, chi.RouteCtxKey, route)
	return httptest.NewRequest(http.MethodGet, "/api/v1/capabilities/slug/versions/v1/bundle", nil).WithContext(ctx)
}

func TestBundleDownloadFailureIsNotSuccess(t *testing.T) {
	repo := &downloadStubRepo{}
	handler := NewRegistryHandler(registry.New(repo, &scriptedStorage{data: []byte("zip!"), failAfter: 0}))
	rec := httptest.NewRecorder()
	handler.Download(rec, downloadRequest())
	if rec.Code == http.StatusOK || repo.audits != 0 || bytes.Contains(rec.Body.Bytes(), []byte("zip")) {
		t.Fatalf("failed copy reported success: status=%d audits=%d body=%q", rec.Code, repo.audits, rec.Body.String())
	}

	repo = &downloadStubRepo{}
	handler = NewRegistryHandler(registry.New(repo, &scriptedStorage{data: []byte("zip!"), failAfter: 2}))
	rec = httptest.NewRecorder()
	func() {
		defer func() {
			if recover() != http.ErrAbortHandler {
				t.Fatal("partial copy finished as a normal response")
			}
		}()
		handler.Download(rec, downloadRequest())
		t.Fatal("partial copy returned without aborting")
	}()
	if repo.audits != 0 {
		t.Fatal("partial copy was audited", repo.audits)
	}

	repo = &downloadStubRepo{}
	handler = NewRegistryHandler(registry.New(repo, &scriptedStorage{data: []byte("zip!"), failAfter: -1}))
	rec = httptest.NewRecorder()
	handler.Download(rec, downloadRequest())
	if rec.Code != http.StatusOK || rec.Body.String() != "zip!" || repo.audits != 1 {
		t.Fatalf("complete download: status=%d audits=%d body=%q", rec.Code, repo.audits, rec.Body.String())
	}
	if rec.Header().Get("Content-Length") != "4" || rec.Header().Get("Content-Type") != "application/zip" {
		t.Fatal("complete download headers", rec.Header())
	}
}
