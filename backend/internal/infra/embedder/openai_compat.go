package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type OpenAICompat struct {
	baseURL string
	apiKey  string
	model   string
	dim     int
	client  *http.Client
}

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
	Dim     int
}

func New(cfg Config) *OpenAICompat {
	return &OpenAICompat{
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		dim:     cfg.Dim,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
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
	body, err := json.Marshal(embedRequest{Model: e.model, Input: texts})
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
		return nil, fmt.Errorf("embedding http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding api %d: %s", resp.StatusCode, string(respBody))
	}

	var out embedResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("embedding api error: %s", out.Error.Message)
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("embedding count mismatch: got %d, want %d", len(out.Data), len(texts))
	}

	result := make([][]float32, len(texts))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(texts) {
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
