package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Feishu codes that retrying cannot fix: the user is outside the app's
// availability scope, or the bot capability is unavailable to them.
var permanentMessageCodes = map[int]bool{230013: true, 230006: true, 99991672: true}

// BotError is a redacted bot API failure. Code is Feishu's numeric code, which
// identifies the cause in logs without echoing provider messages.
type BotError struct {
	Status int
	Code   int
}

func (e *BotError) Error() string {
	return fmt.Sprintf("Feishu bot request failed (status %d, code %d)", e.Status, e.Code)
}

// BotClient sends text messages as the app using a cached tenant token.
// Requires the app's bot capability and the im:message:send_as_bot scope.
type BotClient struct {
	baseURL, appID, appSecret string
	http                      *http.Client
	now                       func() time.Time

	mu      sync.Mutex
	token   string
	expires time.Time
}

func NewBotClient(baseURL, appID, appSecret string, httpClient *http.Client) *BotClient {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &BotClient{baseURL: strings.TrimRight(baseURL, "/"), appID: appID, appSecret: appSecret, http: httpClient, now: time.Now}
}

func (c *BotClient) Permanent(err error) bool {
	var be *BotError
	return errors.As(err, &be) && permanentMessageCodes[be.Code]
}

func (c *BotClient) Send(ctx context.Context, openID, text, key string) error {
	token, err := c.tenantToken(ctx)
	if err != nil {
		return err
	}
	content, _ := json.Marshal(map[string]string{"text": text})
	body := map[string]string{"receive_id": openID, "msg_type": "text", "content": string(content), "uuid": key}
	var out struct {
		Code int `json:"code"`
	}
	err = c.post(ctx, "/open-apis/im/v1/messages?receive_id_type=open_id", token, body, &out)
	var be *BotError
	if errors.As(err, &be) && (be.Code == 99991663 || be.Code == 99991661 || be.Status == http.StatusUnauthorized) {
		c.mu.Lock()
		c.token = ""
		c.mu.Unlock()
	}
	return err
}

func (c *BotClient) tenantToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && c.now().Before(c.expires) {
		return c.token, nil
	}
	var out struct {
		Code   int    `json:"code"`
		Token  string `json:"tenant_access_token"`
		Expire int64  `json:"expire"`
	}
	if err := c.post(ctx, "/open-apis/auth/v3/tenant_access_token/internal", "", map[string]string{"app_id": c.appID, "app_secret": c.appSecret}, &out); err != nil {
		return "", err
	}
	if out.Token == "" || out.Expire <= 0 {
		return "", &BotError{Status: http.StatusOK, Code: -1}
	}
	// Refresh five minutes early so a token never expires mid-request.
	c.token, c.expires = out.Token, c.now().Add(time.Duration(out.Expire)*time.Second-5*time.Minute)
	return c.token, nil
}

func (c *BotClient) post(ctx context.Context, path, token string, payload any, out any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &BotError{Code: -1}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponseBytes+1))
	if err != nil || len(data) > maxAPIResponseBytes {
		return &BotError{Status: resp.StatusCode, Code: -1}
	}
	var envelope struct {
		Code int `json:"code"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return &BotError{Status: resp.StatusCode, Code: -1}
	}
	if resp.StatusCode != http.StatusOK || envelope.Code != 0 {
		return &BotError{Status: resp.StatusCode, Code: envelope.Code}
	}
	if json.Unmarshal(data, out) != nil {
		return &BotError{Status: resp.StatusCode, Code: -1}
	}
	return nil
}
