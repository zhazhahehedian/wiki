package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DashScope embeds text through the selected vision model's native API.
// This does not enable image ingestion in the knowledge service.
type DashScope struct {
	endpoint, key, model string
	dim                  int
	client               *http.Client
}

func NewDashScope(cfg Config) (*DashScope, error) {
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || cfg.APIKey == "" {
		return nil, fmt.Errorf("invalid DashScope embedding configuration")
	}
	if cfg.Model != "tongyi-embedding-vision-plus-2026-03-06" {
		return nil, fmt.Errorf("unsupported DashScope embedding model")
	}
	switch cfg.Dim {
	case 64, 128, 256, 512, 1024, 1152:
	default:
		return nil, fmt.Errorf("unsupported DashScope embedding dimension")
	}
	return &DashScope{endpoint: strings.TrimRight(cfg.BaseURL, "/") + "/services/embeddings/multimodal-embedding/multimodal-embedding", key: cfg.APIKey, model: cfg.Model, dim: cfg.Dim,
		client: &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (e *DashScope) Dim() int      { return e.dim }
func (e *DashScope) Model() string { return e.model }

// A conservative input budget for the model's 1024-token ceiling. It is not a
// tokenizer: bytes bound byte-level text tokens and leave room for templates.
func (e *DashScope) MaxInputBytes() int { return 768 }

func (e *DashScope) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	for _, text := range texts {
		if strings.TrimSpace(text) == "" || len(text) > e.MaxInputBytes() {
			return nil, fmt.Errorf("embedding text exceeds input budget or is empty")
		}
	}
	var result [][]float32
	for start := 0; start < len(texts); start += 20 {
		batch, err := e.batch(ctx, texts[start:min(start+20, len(texts))])
		if err != nil {
			return nil, err
		}
		result = append(result, batch...)
	}
	return result, nil
}

func (e *DashScope) batch(ctx context.Context, texts []string) ([][]float32, error) {
	contents := make([]map[string]string, len(texts))
	for i, text := range texts {
		contents[i] = map[string]string{"text": text}
	}
	body, err := json.Marshal(map[string]any{"model": e.model, "input": map[string]any{"contents": contents}, "parameters": map[string]int{"dimension": e.dim}})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("invalid embedding request")
	}
	req.Header.Set("Authorization", "Bearer "+e.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("embedding transport failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API status %d", resp.StatusCode)
	}
	const maxResponse = 4 << 20
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil || len(raw) > maxResponse {
		return nil, fmt.Errorf("invalid embedding response size")
	}
	var out struct {
		Code   string `json:"code"`
		Output struct {
			Embeddings []struct {
				Index     *int      `json:"index"`
				Embedding []float32 `json:"embedding"`
				Type      string    `json:"type"`
			} `json:"embeddings"`
		} `json:"output"`
	}
	if json.Unmarshal(raw, &out) != nil || out.Code != "" || len(out.Output.Embeddings) != len(texts) {
		return nil, fmt.Errorf("invalid embedding response")
	}
	result := make([][]float32, len(texts))
	for _, item := range out.Output.Embeddings {
		if item.Index == nil || *item.Index < 0 || *item.Index >= len(texts) || result[*item.Index] != nil || item.Type != "text" || len(item.Embedding) != e.dim {
			return nil, fmt.Errorf("invalid embedding index, type or dimension")
		}
		norm := float64(0)
		for _, value := range item.Embedding {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return nil, fmt.Errorf("invalid embedding vector")
			}
			norm += float64(value) * float64(value)
		}
		if norm == 0 {
			return nil, fmt.Errorf("invalid embedding vector")
		}
		result[*item.Index] = item.Embedding
	}
	return result, nil
}
