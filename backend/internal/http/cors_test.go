package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 前端切换会话模式走 PATCH /api/v1/conversations/{id}，浏览器跨域预检要求
// Access-Control-Allow-Methods 包含 PATCH，否则 fetch 直接报 Failed to fetch。
func TestCORSPreflightAllowsPatch(t *testing.T) {
	handler := CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("preflight 请求不应到达下游 handler")
	}), "http://localhost:3000")

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/conversations/abc", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", http.MethodPatch)
	req.Header.Set("Access-Control-Request-Headers", "content-type")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("预检状态码 = %d，期望 %d", rec.Code, http.StatusNoContent)
	}
	allowed := rec.Header().Get("Access-Control-Allow-Methods")
	if !strings.Contains(allowed, http.MethodPatch) {
		t.Fatalf("Access-Control-Allow-Methods = %q，缺少 PATCH", allowed)
	}
}

func TestCORSCredentialsAndExactOrigin(t *testing.T) {
	handler := CORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), "https://app.example.test")

	allowed := httptest.NewRequest(http.MethodOptions, "/api/v1/kbs", nil)
	allowed.Header.Set("Origin", "https://app.example.test")
	allowed.Header.Set("Access-Control-Request-Method", http.MethodPost)
	allowedRec := httptest.NewRecorder()
	handler.ServeHTTP(allowedRec, allowed)
	if allowedRec.Code != http.StatusNoContent || allowedRec.Header().Get("Access-Control-Allow-Origin") != "https://app.example.test" || allowedRec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("allowed preflight = %d %#v", allowedRec.Code, allowedRec.Header())
	}
	if !strings.Contains(allowedRec.Header().Get("Access-Control-Allow-Headers"), CSRFHeaderName) {
		t.Fatalf("allowed headers = %q", allowedRec.Header().Get("Access-Control-Allow-Headers"))
	}

	rejected := httptest.NewRequest(http.MethodOptions, "/api/v1/kbs", nil)
	rejected.Header.Set("Origin", "https://evil.test")
	rejected.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rejectedRec := httptest.NewRecorder()
	handler.ServeHTTP(rejectedRec, rejected)
	if rejectedRec.Code != http.StatusForbidden || rejectedRec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("rejected preflight = %d %#v", rejectedRec.Code, rejectedRec.Header())
	}
}
