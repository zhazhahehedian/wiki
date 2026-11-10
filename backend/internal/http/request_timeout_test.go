package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestTimeoutExtendsOnlyTransfers(t *testing.T) {
	for _, tc := range []struct {
		method, path, contentType string
		want                      time.Duration
	}{
		{"GET", "/api/v1/capabilities/demo/versions/v1/bundle", "", DownloadTimeout},
		{"GET", "/api/v1/open/capabilities/demo/versions/v1/bundle", "", DownloadTimeout},
		{"PUT", "/api/v1/open/capabilities/demo", "multipart/form-data; boundary=x", TransferTimeout},
		{"POST", "/api/v1/bundle-uploads/u1/complete", "", TransferTimeout},
		{"PUT", "/api/v1/bundle-uploads/u1/parts/1", "application/octet-stream", time.Minute},
		{"POST", "/api/v1/bundle-uploads", "application/json", time.Minute},
		{"POST", "/api/v1/open/capabilities", "multipart/form-data; boundary=x", TransferTimeout},
		{"PUT", "/api/v1/capabilities/demo", "application/json", time.Minute},
		{"GET", "/api/v1/capabilities/demo", "", time.Minute},
	} {
		var remaining time.Duration
		handler := requestTimeout(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			deadline, _ := r.Context().Deadline()
			remaining = time.Until(deadline)
		}))
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("Content-Type", tc.contentType)
		handler.ServeHTTP(httptest.NewRecorder(), req)
		if remaining > tc.want || remaining < tc.want-5*time.Second {
			t.Fatalf("%s %s timeout = %v, want %v", tc.method, tc.path, remaining, tc.want)
		}
	}
}
