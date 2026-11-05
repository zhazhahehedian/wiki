package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRequestTimeout = 15 * time.Second
	defaultMaxRetries     = 3
	defaultMaxRetryDelay  = 30 * time.Second
	maxAPIResponseBytes   = 8 << 20
)

type WaitFunc func(context.Context, time.Duration) error

type ClientConfig struct {
	BaseURL       string
	Timeout       time.Duration
	MaxRetries    int
	Backoff       time.Duration
	MaxRetryDelay time.Duration
	Wait          WaitFunc
	Now           func() time.Time
}

type Client struct {
	baseURL       string
	timeout       time.Duration
	maxRetries    int
	backoff       time.Duration
	maxRetryDelay time.Duration
	wait          WaitFunc
	now           func() time.Time
	http          *http.Client
}

type APIErrorCode string

const (
	APIForbidden APIErrorCode = "resource_forbidden"
	APINotFound  APIErrorCode = "resource_not_found"
	APIRateLimit APIErrorCode = "rate_limited"
	APIFailed    APIErrorCode = "api_error"
	APITooLarge  APIErrorCode = "too_large"
	APIMalformed APIErrorCode = "malformed"
)

// APIError deliberately keeps only cancellation causes so provider bodies and
// transport details never reach responses or logs.
type APIError struct {
	Code  APIErrorCode `json:"code"`
	cause error
}

func newAPIError(code APIErrorCode, cause error) *APIError {
	var safe error
	switch {
	case errors.Is(cause, context.Canceled):
		safe = context.Canceled
	case errors.Is(cause, context.DeadlineExceeded):
		safe = context.DeadlineExceeded
	}
	return &APIError{Code: code, cause: safe}
}

func (e *APIError) Error() string { return "Feishu API request failed" }
func (e *APIError) Unwrap() error { return e.cause }

type apiEnvelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func NewClient(config ClientConfig, httpClient *http.Client) *Client {
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultRequestTimeout
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = defaultMaxRetries
	}
	if config.MaxRetries < 0 {
		config.MaxRetries = 0
	}
	if config.Backoff <= 0 {
		config.Backoff = 100 * time.Millisecond
	}
	if config.MaxRetryDelay <= 0 {
		config.MaxRetryDelay = defaultMaxRetryDelay
	}
	if config.Wait == nil {
		config.Wait = waitForContext
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL: strings.TrimRight(config.BaseURL, "/"), timeout: config.Timeout,
		maxRetries: config.MaxRetries, backoff: config.Backoff, maxRetryDelay: config.MaxRetryDelay, wait: config.Wait,
		now: config.Now, http: httpClient,
	}
}

func (c *Client) Get(ctx context.Context, accessToken, path string, query url.Values, out any) error {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	for attempt := 0; ; attempt++ {
		requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
		if err != nil {
			cancel()
			return newAPIError(APIMalformed, nil)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Accept", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			cancel()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if attempt < c.maxRetries {
				if err := c.wait(ctx, c.retryDelay(attempt, "")); err != nil {
					return err
				}
				continue
			}
			return newAPIError(APIFailed, err)
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponseBytes+1))
		_ = resp.Body.Close()
		cancel()
		if readErr != nil {
			if attempt < c.maxRetries {
				if err := c.wait(ctx, c.retryDelay(attempt, resp.Header.Get("Retry-After"))); err != nil {
					return err
				}
				continue
			}
			return newAPIError(APIFailed, readErr)
		}
		if retryableStatus(resp.StatusCode) && attempt < c.maxRetries {
			if err := c.wait(ctx, c.retryDelay(attempt, resp.Header.Get("Retry-After"))); err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			var providerError apiEnvelope
			if len(body) <= maxAPIResponseBytes && decodeExactJSON(body, &providerError) == nil && providerError.Code != 0 {
				if code := feishuErrorCode(providerError.Code); code != APIFailed {
					return newAPIError(code, nil)
				}
			}
			return newAPIError(httpErrorCode(resp.StatusCode), nil)
		}
		if len(body) > maxAPIResponseBytes {
			return newAPIError(APITooLarge, nil)
		}

		var envelope apiEnvelope
		if err := decodeExactJSON(body, &envelope); err != nil {
			return newAPIError(APIMalformed, nil)
		}
		if envelope.Code != 0 {
			return newAPIError(feishuErrorCode(envelope.Code), nil)
		}
		if len(envelope.Data) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Data), []byte("null")) {
			return newAPIError(APIMalformed, nil)
		}
		if err := decodeExactJSON(envelope.Data, out); err != nil {
			return newAPIError(APIMalformed, nil)
		}
		return nil
	}
}

func decodeExactJSON(data []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON data")
	}
	return nil
}

func retryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func httpErrorCode(status int) APIErrorCode {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return APIForbidden
	case http.StatusNotFound:
		return APINotFound
	case http.StatusTooManyRequests:
		return APIRateLimit
	default:
		return APIFailed
	}
}

func feishuErrorCode(code int) APIErrorCode {
	switch code {
	case 99991663, 99991668, 99991661, 99991664, 99991672, 41050, 40004:
		return APIForbidden
	case 1770002, 1254045:
		return APINotFound
	case 99991400:
		return APIRateLimit
	default:
		return APIFailed
	}
}

func (c *Client) retryDelay(attempt int, retryAfter string) time.Duration {
	if seconds, err := strconv.ParseInt(strings.TrimSpace(retryAfter), 10, 64); err == nil && seconds >= 0 {
		if seconds > int64(c.maxRetryDelay/time.Second) {
			return c.maxRetryDelay
		}
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(retryAfter); err == nil {
		if delay := at.Sub(c.now()); delay > 0 {
			if delay > c.maxRetryDelay {
				return c.maxRetryDelay
			}
			return delay
		}
		return 0
	}
	delay := c.backoff << attempt
	if delay > c.maxRetryDelay {
		return c.maxRetryDelay
	}
	return delay
}

func waitForContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
