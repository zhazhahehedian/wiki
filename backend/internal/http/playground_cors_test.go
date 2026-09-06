package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCORSAllowsModelConfigurationPut(t *testing.T) {
	called := false
	handler := CORS(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }), "http://localhost:3000")
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/playground/connection", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Access-Control-Request-Method", "PUT")
	request.Header.Set("Access-Control-Request-Headers", "content-type,x-csrf-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 204 || !strings.Contains(response.Header().Get("Access-Control-Allow-Methods"), "PUT") || response.Header().Get("Access-Control-Allow-Credentials") != "true" || called {
		t.Fatal("configuration preflight failed")
	}
}
