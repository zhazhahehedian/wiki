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

type wireFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type wireToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireFunctionCall `json:"function"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type wireTool struct {
	Type     string           `json:"type"`
	Function wireToolFunction `json:"function"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []wireMessage `json:"messages"`
	Temperature float32       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream"`
	Tools       []wireTool    `json:"tools,omitempty"`
}

func toWireMessages(msgs []ports.Message) []wireMessage {
	out := make([]wireMessage, 0, len(msgs))
	for _, m := range msgs {
		wm := wireMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, c := range m.ToolCalls {
			wm.ToolCalls = append(wm.ToolCalls, wireToolCall{
				ID: c.ID, Type: "function",
				Function: wireFunctionCall{Name: c.Name, Arguments: c.Arguments},
			})
		}
		out = append(out, wm)
	}
	return out
}

func toWireTools(defs []ports.ToolDefinition) []wireTool {
	if len(defs) == 0 {
		return nil
	}
	out := make([]wireTool, 0, len(defs))
	for _, d := range defs {
		out = append(out, wireTool{Type: "function", Function: wireToolFunction{
			Name: d.Name, Description: d.Description, Parameters: d.Parameters,
		}})
	}
	return out
}

type chatResponse struct {
	Choices []struct {
		Message wireMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func fromWireMessage(m wireMessage) *ports.Message {
	out := &ports.Message{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
	for _, c := range m.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, ports.ToolCall{ID: c.ID, Name: c.Function.Name, Arguments: c.Function.Arguments})
	}
	return out
}

type streamToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type streamChoice struct {
	Delta struct {
		Content   string                `json:"content"`
		ToolCalls []streamToolCallDelta `json:"tool_calls"`
	} `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

type toolCallAggregator struct {
	order []int
	calls map[int]*ports.ToolCall
}

func newToolCallAggregator() *toolCallAggregator {
	return &toolCallAggregator{calls: map[int]*ports.ToolCall{}}
}

func (a *toolCallAggregator) ingest(deltas []streamToolCallDelta) {
	for _, d := range deltas {
		c, ok := a.calls[d.Index]
		if !ok {
			c = &ports.ToolCall{}
			a.calls[d.Index] = c
			a.order = append(a.order, d.Index)
		}
		if d.ID != "" {
			c.ID = d.ID
		}
		if d.Function.Name != "" {
			c.Name = d.Function.Name
		}
		c.Arguments += d.Function.Arguments
	}
}

// flush 返回聚合完成的调用并清空聚合器。
func (a *toolCallAggregator) flush() []ports.ToolCall {
	if len(a.order) == 0 {
		return nil
	}
	out := make([]ports.ToolCall, 0, len(a.order))
	for _, idx := range a.order {
		out = append(out, *a.calls[idx])
	}
	a.order = nil
	a.calls = map[int]*ports.ToolCall{}
	return out
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
		Messages:    toWireMessages(msgs),
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
		Stream:      false,
		Tools:       toWireTools(opts.Tools),
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
	return fromWireMessage(parsed.Choices[0].Message), nil
}

func (c *OpenAICompat) ChatStream(ctx context.Context, msgs []ports.Message, opts ports.ChatOptions) (<-chan ports.StreamChunk, error) {
	reqBody := chatRequest{
		Model:       modelOrDefault(opts.Model, c.model),
		Messages:    toWireMessages(msgs),
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
		Stream:      true,
		Tools:       toWireTools(opts.Tools),
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

		emit := func(chunk ports.StreamChunk) bool {
			select {
			case out <- chunk:
				return true
			case <-ctx.Done():
				return false
			}
		}
		agg := newToolCallAggregator()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "[DONE]" {
				// 有的兼容实现不发 finish_reason，兜底 flush
				if calls := agg.flush(); len(calls) > 0 {
					if !emit(ports.StreamChunk{ToolCalls: calls}) {
						return
					}
				}
				emit(ports.StreamChunk{Done: true})
				return
			}

			var parsed streamResponse
			if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
				emit(ports.StreamChunk{Err: err})
				return
			}
			if parsed.Error != nil {
				emit(ports.StreamChunk{Err: fmt.Errorf("llm stream error: %s", parsed.Error.Message)})
				return
			}

			chunk := ports.StreamChunk{Usage: parsed.Usage}
			if len(parsed.Choices) > 0 {
				chunk.Text = parsed.Choices[0].Delta.Content
				agg.ingest(parsed.Choices[0].Delta.ToolCalls)
				if parsed.Choices[0].FinishReason != nil {
					chunk.ToolCalls = agg.flush()
					chunk.Done = true
				}
			}
			if chunk.Text == "" && !chunk.Done && chunk.Usage == nil && len(chunk.ToolCalls) == 0 {
				continue
			}
			if !emit(chunk) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			emit(ports.StreamChunk{Err: err})
		}
	}()
	return out, nil
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
