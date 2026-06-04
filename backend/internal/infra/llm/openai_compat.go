package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

type OpenAICompat struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func New(cfg Config) *OpenAICompat {
	return &OpenAICompat{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

type chatRequest struct {
	Model       string          `json:"model"`
	Messages    []ports.Message `json:"messages"`
	Temperature float32         `json:"temperature,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Stream      bool            `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message ports.Message `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type streamChoice struct {
	Delta struct {
		Content string `json:"content"`
	} `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

type streamResponse struct {
	Choices []streamChoice    `json:"choices"`
	Usage   *ports.TokenUsage `json:"usage,omitempty"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *OpenAICompat) Chat(ctx context.Context, msgs []ports.Message, opts ports.ChatOptions) (*ports.Message, error) {
	reqBody := chatRequest{
		Model:       modelOrDefault(opts.Model, c.model),
		Messages:    msgs,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
		Stream:      false,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := c.newRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("llm api %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed chatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal llm response: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("llm api error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("llm response has no choices")
	}
	return &parsed.Choices[0].Message, nil
}

func (c *OpenAICompat) ChatStream(ctx context.Context, msgs []ports.Message, opts ports.ChatOptions) (<-chan ports.StreamChunk, error) {
	reqBody := chatRequest{
		Model:       modelOrDefault(opts.Model, c.model),
		Messages:    msgs,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
		Stream:      true,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := c.newRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm stream http: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("llm stream api %d: %s", resp.StatusCode, string(respBody))
	}

	out := make(chan ports.StreamChunk)
	go func() {
		defer close(out)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "[DONE]" {
				select {
				case out <- ports.StreamChunk{Done: true}:
				case <-ctx.Done():
				}
				return
			}

			chunk, err := parseStreamPayload(payload)
			if err != nil {
				select {
				case out <- ports.StreamChunk{Err: err}:
				case <-ctx.Done():
				}
				return
			}
			if chunk.Text == "" && !chunk.Done && chunk.Usage == nil {
				continue
			}
			select {
			case out <- chunk:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case out <- ports.StreamChunk{Err: err}:
			case <-ctx.Done():
			}
		}
	}()
	return out, nil
}

func parseStreamPayload(payload string) (ports.StreamChunk, error) {
	var parsed streamResponse
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return ports.StreamChunk{}, err
	}
	if parsed.Error != nil {
		return ports.StreamChunk{}, fmt.Errorf("llm stream error: %s", parsed.Error.Message)
	}

	var text string
	var done bool
	if len(parsed.Choices) > 0 {
		text = parsed.Choices[0].Delta.Content
		done = parsed.Choices[0].FinishReason != nil
	}
	return ports.StreamChunk{Text: text, Done: done, Usage: parsed.Usage}, nil
}

func modelOrDefault(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func (c *OpenAICompat) newRequest(ctx context.Context, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	return req, nil
}
