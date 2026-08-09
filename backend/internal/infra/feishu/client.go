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

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

const (
	defaultRequestTimeout = 15 * time.Second
	defaultMaxRetries     = 3
	defaultMaxRetryDelay  = 30 * time.Second
	maxAPIResponseBytes   = 8 << 20
	defaultMaxPages       = 1_000
	defaultMaxBlocks      = 100_000
	defaultMaxRows        = 10_000
	defaultMaxOutputBytes = 10 << 20
	defaultMaxDepth       = 100
)

type WaitFunc func(context.Context, time.Duration) error

type ClientConfig struct {
	BaseURL        string
	Timeout        time.Duration
	MaxRetries     int
	Backoff        time.Duration
	MaxRetryDelay  time.Duration
	Wait           WaitFunc
	Now            func() time.Time
	ResourceLimits ResourceLimits
}

type ResourceLimits struct {
	MaxPages       int
	MaxBlocks      int
	MaxRows        int
	MaxOutputBytes int
	MaxDepth       int
}

type Client struct {
	baseURL        string
	timeout        time.Duration
	maxRetries     int
	backoff        time.Duration
	maxRetryDelay  time.Duration
	wait           WaitFunc
	now            func() time.Time
	http           *http.Client
	resourceLimits ResourceLimits
}

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
	config.ResourceLimits = withDefaultResourceLimits(config.ResourceLimits)
	return &Client{
		baseURL: strings.TrimRight(config.BaseURL, "/"), timeout: config.Timeout,
		maxRetries: config.MaxRetries, backoff: config.Backoff, maxRetryDelay: config.MaxRetryDelay, wait: config.Wait,
		now: config.Now, http: httpClient, resourceLimits: config.ResourceLimits,
	}
}

func withDefaultResourceLimits(limits ResourceLimits) ResourceLimits {
	if limits.MaxPages <= 0 {
		limits.MaxPages = defaultMaxPages
	}
	if limits.MaxBlocks <= 0 {
		limits.MaxBlocks = defaultMaxBlocks
	}
	if limits.MaxRows <= 0 {
		limits.MaxRows = defaultMaxRows
	}
	if limits.MaxOutputBytes <= 0 {
		limits.MaxOutputBytes = defaultMaxOutputBytes
	}
	if limits.MaxDepth <= 0 {
		limits.MaxDepth = defaultMaxDepth
	}
	return limits
}

type resourceBudget struct {
	limits ResourceLimits
	pages  int
	blocks int
	rows   int
	bytes  int
}

func newResourceBudget(limits ResourceLimits) *resourceBudget {
	return &resourceBudget{limits: withDefaultResourceLimits(limits)}
}

type resourceBudgetContextKey struct{}

func resourceBudgetFromContext(ctx context.Context, limits ResourceLimits) (*resourceBudget, error) {
	if budget, ok := ctx.Value(resourceBudgetContextKey{}).(*resourceBudget); ok && budget != nil {
		if err := budget.tighten(limits); err != nil {
			return nil, err
		}
		return budget, nil
	}
	return newResourceBudget(limits), nil
}

func contextWithResourceBudget(ctx context.Context, budget *resourceBudget) context.Context {
	return context.WithValue(ctx, resourceBudgetContextKey{}, budget)
}

func (b *resourceBudget) consume(current *int, amount, maximum int) error {
	if amount < 0 || *current > maximum-amount {
		return ports.NewSourceLoadError(ports.SourceLoadTooLarge, nil)
	}
	*current += amount
	return nil
}

func (b *resourceBudget) tighten(limits ResourceLimits) error {
	limits = withDefaultResourceLimits(limits)
	b.limits.MaxPages = min(b.limits.MaxPages, limits.MaxPages)
	b.limits.MaxBlocks = min(b.limits.MaxBlocks, limits.MaxBlocks)
	b.limits.MaxRows = min(b.limits.MaxRows, limits.MaxRows)
	b.limits.MaxOutputBytes = min(b.limits.MaxOutputBytes, limits.MaxOutputBytes)
	b.limits.MaxDepth = min(b.limits.MaxDepth, limits.MaxDepth)
	if b.pages > b.limits.MaxPages || b.blocks > b.limits.MaxBlocks || b.rows > b.limits.MaxRows || b.bytes > b.limits.MaxOutputBytes {
		return ports.NewSourceLoadError(ports.SourceLoadTooLarge, nil)
	}
	return nil
}

func (b *resourceBudget) Page() error { return b.consume(&b.pages, 1, b.limits.MaxPages) }
func (b *resourceBudget) Blocks(count int) error {
	return b.consume(&b.blocks, count, b.limits.MaxBlocks)
}
func (b *resourceBudget) Rows(count int) error { return b.consume(&b.rows, count, b.limits.MaxRows) }
func (b *resourceBudget) Bytes(count int) error {
	return b.consume(&b.bytes, count, b.limits.MaxOutputBytes)
}
func (b *resourceBudget) CheckAdditionalOutputBytes(total int) error {
	if total < 0 || b.bytes > b.limits.MaxOutputBytes-total {
		return ports.NewSourceLoadError(ports.SourceLoadTooLarge, nil)
	}
	return nil
}
func (b *resourceBudget) Depth(depth int) error {
	if depth > b.limits.MaxDepth {
		return ports.NewSourceLoadError(ports.SourceLoadTooLarge, nil)
	}
	return nil
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
			return ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
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
			return ports.NewSourceLoadError(ports.SourceLoadAPIError, err)
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
			return ports.NewSourceLoadError(ports.SourceLoadAPIError, readErr)
		}
		if retryableStatus(resp.StatusCode) && attempt < c.maxRetries {
			if err := c.wait(ctx, c.retryDelay(attempt, resp.Header.Get("Retry-After"))); err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return ports.NewSourceLoadError(httpErrorCode(resp.StatusCode), nil)
		}
		if len(body) > maxAPIResponseBytes {
			return ports.NewSourceLoadError(ports.SourceLoadTooLarge, nil)
		}

		var envelope apiEnvelope
		if err := decodeExactJSON(body, &envelope); err != nil {
			return ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
		}
		if envelope.Code != 0 {
			return ports.NewSourceLoadError(feishuErrorCode(envelope.Code), nil)
		}
		if len(envelope.Data) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Data), []byte("null")) {
			return ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
		}
		if err := decodeExactJSON(envelope.Data, out); err != nil {
			return ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
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

func httpErrorCode(status int) ports.SourceLoadErrorCode {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ports.SourceLoadForbidden
	case http.StatusNotFound:
		return ports.SourceLoadNotFound
	case http.StatusTooManyRequests:
		return ports.SourceLoadRateLimit
	default:
		return ports.SourceLoadAPIError
	}
}

func feishuErrorCode(code int) ports.SourceLoadErrorCode {
	switch code {
	case 99991663, 99991668:
		return ports.SourceLoadForbidden
	case 1770002, 1254045:
		return ports.SourceLoadNotFound
	case 99991400:
		return ports.SourceLoadRateLimit
	default:
		return ports.SourceLoadAPIError
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

type pageTokenTracker struct{ seen map[string]struct{} }

func validAPIIdentifier(value string) bool {
	return len(value) > 0 && len(value) <= maxResourceIdentifierBytes && resourceIdentifierPattern.MatchString(value)
}

func newPageTokenTracker() *pageTokenTracker {
	return &pageTokenTracker{seen: make(map[string]struct{})}
}

func (t *pageTokenTracker) Advance(token string) error {
	if token == "" {
		return nil
	}
	if _, exists := t.seen[token]; exists {
		return ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	t.seen[token] = struct{}{}
	return nil
}
