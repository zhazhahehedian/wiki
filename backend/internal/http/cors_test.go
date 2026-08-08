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
	}))

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
