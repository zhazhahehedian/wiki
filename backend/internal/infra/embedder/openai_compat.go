package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OpenAICompat calls an OpenAI compatible /embeddings endpoint. Requests are
// split into batches of at most 10 texts, the limit of Bailian
// text-embedding-v4; Bailian hosts also receive an explicit dimensions value.
type OpenAICompat struct {
	baseURL       string
	apiKey        string
	model         string
	dim           int
	sendDimension bool
	client        *http.Client
}

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
	Dim     int
}

const openAIBatch = 10

func New(cfg Config) *OpenAICompat {
	host := ""
	if u, err := url.Parse(cfg.BaseURL); err == nil {
		host = strings.ToLower(u.Hostname())
	}
	return &OpenAICompat{
		baseURL:       cfg.BaseURL,
		apiKey:        cfg.APIKey,
		model:         cfg.Model,
		dim:           cfg.Dim,
		sendDimension: host == "aliyuncs.com" || strings.HasSuffix(host, ".aliyuncs.com"),
		client:        &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

type embedRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (e *OpenAICompat) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	result := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += openAIBatch {
		batch, err := e.batch(ctx, texts[start:min(start+openAIBatch, len(texts))])
		if err != nil {
			return nil, err
		}
		result = append(result, batch...)
	}
	return result, nil
}

func (e *OpenAICompat) batch(ctx context.Context, texts []string) ([][]float32, error) {
	in := embedRequest{Model: e.model, Input: texts}
	if e.sendDimension {
		in.Dimensions = e.dim
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("embedding transport failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// The upstream body may echo request text; it is not included.
		return nil, fmt.Errorf("embedding API status %d", resp.StatusCode)
	}

	const maxResponse = 8 << 20
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil || len(respBody) > maxResponse {
		return nil, fmt.Errorf("invalid embedding response size")
	}
	var out embedResponse
	if err := json.Unmarshal(respBody, &out); err != nil || out.Error != nil {
		return nil, fmt.Errorf("invalid embedding response")
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("embedding count mismatch: got %d, want %d", len(out.Data), len(texts))
	}

	result := make([][]float32, len(texts))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(texts) || result[d.Index] != nil {
			return nil, fmt.Errorf("invalid index %d", d.Index)
		}
		if len(d.Embedding) != e.dim {
			return nil, fmt.Errorf("embedding dim mismatch: got %d, want %d", len(d.Embedding), e.dim)
		}
		result[d.Index] = d.Embedding
	}
	return result, nil
}

func (e *OpenAICompat) Dim() int      { return e.dim }
func (e *OpenAICompat) Model() string { return e.model }
