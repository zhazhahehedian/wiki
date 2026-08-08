package feishu_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/feishu"
)

func TestOAuthClientExchangesAuthorizationCode(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/authen/v2/oauth/token" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		for key, want := range map[string]string{"grant_type": "authorization_code", "client_id": "app-id", "client_secret": "app-secret", "code": "auth-code", "redirect_uri": "https://wiki.example/callback"} {
			if body[key] != want {
				t.Fatalf("request[%s] = %q, want %q", key, body[key], want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"access_token":"access","refresh_token":"refresh","expires_in":7200,"refresh_expires_in":2592000,"scope":"docs:document:readonly offline_access"}`))
	}))
	defer server.Close()

	client := feishu.NewOAuthClient(feishu.OAuthConfig{BaseURL: server.URL, AppID: "app-id", AppSecret: "app-secret", RedirectURL: "https://wiki.example/callback", Now: func() time.Time { return now }}, server.Client())
	token, err := client.ExchangeCode(context.Background(), "auth-code")
	if err != nil {
		t.Fatalf("ExchangeCode() error = %v", err)
	}
	if token.AccessToken != "access" || token.RefreshToken != "refresh" || !token.AccessTokenExpiresAt.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("ExchangeCode() = %+v", token)
	}
	if len(token.Scopes) != 2 || token.Scopes[0] != "docs:document:readonly" {
		t.Fatalf("scopes = %#v", token.Scopes)
	}
}

func TestOAuthClientReadsUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/authen/v1/user_info" || r.Header.Get("Authorization") != "Bearer access" {
			t.Fatalf("request = %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"open_id":"ou_1","union_id":"on_1","tenant_key":"tenant-a","name":"Wiki User","avatar_url":"https://avatar","email":"user@example.com"}}`))
	}))
	defer server.Close()

	client := feishu.NewOAuthClient(feishu.OAuthConfig{BaseURL: server.URL}, server.Client())
	identity, err := client.UserInfo(context.Background(), "access")
	if err != nil {
		t.Fatalf("UserInfo() error = %v", err)
	}
	if identity.OpenID != "ou_1" || identity.TenantKey != "tenant-a" || identity.DisplayName != "Wiki User" || identity.Email != "user@example.com" {
		t.Fatalf("UserInfo() = %+v", identity)
	}
}

func TestOAuthClientRejectsNon2xxMalformedAndAPIErrorResponses(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantCode string
	}{
		{name: "non-2xx", status: http.StatusBadGateway, body: `upstream failed`, wantCode: "http_error"},
		{name: "malformed", status: http.StatusOK, body: `{`, wantCode: "malformed_response"},
		{name: "api error", status: http.StatusOK, body: `{"code":20029,"error":"invalid_grant","error_description":"authorization code expired"}`, wantCode: "invalid_grant"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			client := feishu.NewOAuthClient(feishu.OAuthConfig{BaseURL: server.URL}, server.Client())
			_, err := client.ExchangeCode(context.Background(), "code")
			var oauthErr *ports.OAuthError
			if !errors.As(err, &oauthErr) || oauthErr.Code != tt.wantCode {
				t.Fatalf("ExchangeCode() error = %#v, want code %q", err, tt.wantCode)
			}
			if strings.Contains(err.Error(), "code\"") {
				t.Fatal("error leaked request authorization code")
			}
		})
	}
}

func TestOAuthClientDoesNotLeakCredentialsInErrors(t *testing.T) {
	const code = "sensitive-authorization-code"
	const secret = "sensitive-app-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":20029,"error":"invalid_grant","error_description":"authorization code sensitive-authorization-code was rejected for sensitive-app-secret"}`))
	}))
	defer server.Close()

	client := feishu.NewOAuthClient(feishu.OAuthConfig{BaseURL: server.URL, AppSecret: secret}, server.Client())
	_, err := client.ExchangeCode(context.Background(), code)
	if err == nil {
		t.Fatal("ExchangeCode() error = nil")
	}
	if strings.Contains(err.Error(), code) || strings.Contains(err.Error(), secret) {
		t.Fatalf("ExchangeCode() error leaked credentials: %v", err)
	}
}

func TestOAuthClientRefreshesToken(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["grant_type"] != "refresh_token" || body["refresh_token"] != "refresh-v1" {
			t.Fatalf("refresh request = %#v", body)
		}
		_, _ = w.Write([]byte(`{"code":0,"access_token":"access-v2","refresh_token":"refresh-v2","expires_in":7200}`))
	}))
	defer server.Close()

	client := feishu.NewOAuthClient(feishu.OAuthConfig{BaseURL: server.URL, Now: func() time.Time { return now }}, server.Client())
	token, err := client.RefreshToken(context.Background(), "refresh-v1")
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	if token.AccessToken != "access-v2" || token.RefreshToken != "refresh-v2" || !token.AccessTokenExpiresAt.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("RefreshToken() = %+v", token)
	}
}

func TestOAuthClientRejectsBadUserInfoResponses(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantCode string
	}{
		{name: "non-2xx", status: http.StatusBadGateway, body: `upstream failed`, wantCode: "http_error"},
		{name: "malformed", status: http.StatusOK, body: `{`, wantCode: "malformed_response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client := feishu.NewOAuthClient(feishu.OAuthConfig{BaseURL: server.URL}, server.Client())
			_, err := client.UserInfo(context.Background(), "sensitive-access-token")
			var oauthErr *ports.OAuthError
			if !errors.As(err, &oauthErr) || oauthErr.Code != tt.wantCode {
				t.Fatalf("UserInfo() error = %#v, want code %q", err, tt.wantCode)
			}
			if strings.Contains(err.Error(), "sensitive-access-token") {
				t.Fatalf("UserInfo() error leaked token: %v", err)
			}
		})
	}
}

func TestOAuthClientDoesNotLeakCredentialsFromTransportErrors(t *testing.T) {
	const code = "sensitive-code-from-request"
	const secret = "sensitive-client-secret"
	transportErr := errors.New("transport copied sensitive-code-from-request and sensitive-client-secret")
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportErr
	})}
	client := feishu.NewOAuthClient(feishu.OAuthConfig{BaseURL: "https://feishu.invalid", AppSecret: secret}, httpClient)

	_, err := client.ExchangeCode(context.Background(), code)
	if err == nil {
		t.Fatal("ExchangeCode() error = nil")
	}
	if strings.Contains(err.Error(), code) || strings.Contains(err.Error(), secret) {
		t.Fatalf("ExchangeCode() error leaked credentials: %v", err)
	}
	if !errors.Is(err, transportErr) {
		t.Fatalf("ExchangeCode() error does not retain transport cause: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
