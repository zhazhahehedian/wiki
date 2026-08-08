package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

const defaultBaseURL = "https://open.feishu.cn"

type OAuthConfig struct {
	BaseURL     string
	AppID       string
	AppSecret   string
	RedirectURL string
	Now         func() time.Time
}

type OAuthClient struct {
	config OAuthConfig
	http   *http.Client
}

func NewOAuthClient(config OAuthConfig, httpClient *http.Client) *OAuthClient {
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}
	config.BaseURL = strings.TrimRight(config.BaseURL, "/")
	if config.Now == nil {
		config.Now = time.Now
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &OAuthClient{config: config, http: httpClient}
}

func (c *OAuthClient) ExchangeCode(ctx context.Context, code string) (domain.OAuthToken, error) {
	return c.requestToken(ctx, map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     c.config.AppID,
		"client_secret": c.config.AppSecret,
		"code":          code,
		"redirect_uri":  c.config.RedirectURL,
	})
}

func (c *OAuthClient) RefreshToken(ctx context.Context, refreshToken string) (domain.OAuthToken, error) {
	return c.requestToken(ctx, map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     c.config.AppID,
		"client_secret": c.config.AppSecret,
		"refresh_token": refreshToken,
	})
}

func (c *OAuthClient) UserInfo(ctx context.Context, accessToken string) (domain.FeishuIdentity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.config.BaseURL+"/open-apis/authen/v1/user_info", nil)
	if err != nil {
		return domain.FeishuIdentity{}, fmt.Errorf("create user info request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := c.http.Do(req)
	if err != nil {
		return domain.FeishuIdentity{}, &ports.OAuthError{Code: "request_failed", Message: "provider request failed", Cause: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, resp.Body)
		return domain.FeishuIdentity{}, &ports.OAuthError{Code: "http_error", Message: "provider returned a non-success status"}
	}
	var response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OpenID    string `json:"open_id"`
			UnionID   string `json:"union_id"`
			TenantKey string `json:"tenant_key"`
			Name      string `json:"name"`
			AvatarURL string `json:"avatar_url"`
			Email     string `json:"email"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return domain.FeishuIdentity{}, &ports.OAuthError{Code: "malformed_response", Message: "provider returned malformed JSON"}
	}
	if response.Code != 0 {
		return domain.FeishuIdentity{}, &ports.OAuthError{Code: fmt.Sprintf("feishu_%d", response.Code), Message: "provider rejected user info request"}
	}
	if response.Data.OpenID == "" || response.Data.TenantKey == "" {
		return domain.FeishuIdentity{}, &ports.OAuthError{Code: "malformed_response", Message: "provider response omitted identity fields"}
	}
	return domain.FeishuIdentity{
		OpenID:      response.Data.OpenID,
		UnionID:     response.Data.UnionID,
		TenantKey:   response.Data.TenantKey,
		DisplayName: response.Data.Name,
		AvatarURL:   response.Data.AvatarURL,
		Email:       response.Data.Email,
	}, nil
}

func (c *OAuthClient) requestToken(ctx context.Context, payload map[string]string) (domain.OAuthToken, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return domain.OAuthToken{}, fmt.Errorf("encode oauth token request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.BaseURL+"/open-apis/authen/v2/oauth/token", bytes.NewReader(body))
	if err != nil {
		return domain.OAuthToken{}, fmt.Errorf("create oauth token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return domain.OAuthToken{}, &ports.OAuthError{Code: "request_failed", Message: "provider request failed", Cause: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, resp.Body)
		return domain.OAuthToken{}, &ports.OAuthError{Code: "http_error", Message: "provider returned a non-success status"}
	}
	var response struct {
		Code             int    `json:"code"`
		Msg              string `json:"msg"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresIn        int64  `json:"expires_in"`
		RefreshExpiresIn int64  `json:"refresh_expires_in"`
		Scope            string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return domain.OAuthToken{}, &ports.OAuthError{Code: "malformed_response", Message: "provider returned malformed JSON"}
	}
	if response.Error != "" || response.Code != 0 {
		code := response.Error
		if code == "" {
			code = fmt.Sprintf("feishu_%d", response.Code)
		}
		return domain.OAuthToken{}, &ports.OAuthError{Code: code, Message: "provider rejected token request"}
	}
	if response.AccessToken == "" || response.ExpiresIn <= 0 {
		return domain.OAuthToken{}, &ports.OAuthError{Code: "malformed_response", Message: "provider response omitted token fields"}
	}
	now := c.config.Now()
	token := domain.OAuthToken{
		AccessToken:          response.AccessToken,
		RefreshToken:         response.RefreshToken,
		AccessTokenExpiresAt: now.Add(time.Duration(response.ExpiresIn) * time.Second),
		Scopes:               strings.Fields(response.Scope),
	}
	if response.RefreshExpiresIn > 0 {
		expiresAt := now.Add(time.Duration(response.RefreshExpiresIn) * time.Second)
		token.RefreshTokenExpiresAt = &expiresAt
	}
	return token, nil
}

var _ ports.OAuthClient = (*OAuthClient)(nil)
