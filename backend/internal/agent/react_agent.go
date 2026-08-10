package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

// Compatibility aliases keep the original public surface while making
// ReactAgent satisfy ports.AgentRunner directly.
type EventSink = ports.AgentEventSink
type Result = ports.AgentResult

var _ ports.AgentRunner = (*ReactAgent)(nil)

type ReactAgent struct {
	llm           ports.LLMClient
	model         string
	maxIterations int
}

const (
	maxIterationsNotice = "Tool call limit reached. Answer now using the information you already have; do not request any more tool calls."
	resultMaxRunes      = 2000 // SSE 与持久化统一截断长度（spec §6.1/§6.2）
)

func New(llm ports.LLMClient, model string, maxIterations int) *ReactAgent {
	if maxIterations < 1 {
		maxIterations = 5
	}
	return &ReactAgent{llm: llm, model: model, maxIterations: maxIterations}
}

func (a *ReactAgent) Run(ctx context.Context, msgs []ports.Message, tools []ports.Tool, sink EventSink) (*Result, error) {
	res := &Result{}
	var usage ports.TokenUsage
	var usageSeen bool

	for iter := 1; iter <= a.maxIterations; iter++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		opts := ports.ChatOptions{Model: a.model, Temperature: 0.2}
		if iter < a.maxIterations {
			opts.Tools = definitions(tools)
		} else {
			// 最终轮不给工具，强制文字收尾（spec §4.1）
			msgs = append(msgs, ports.Message{Role: "system", Content: maxIterationsNotice})
		}

		stream, err := a.llm.ChatStream(ctx, msgs, opts)
		if err != nil {
			return nil, err
		}
		var text strings.Builder
		var calls []ports.ToolCall
		for chunk := range stream {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if chunk.Err != nil {
				return nil, chunk.Err
			}
			if chunk.Usage != nil {
				usageSeen = true
				usage.PromptTokens += chunk.Usage.PromptTokens
				usage.CompletionTokens += chunk.Usage.CompletionTokens
				usage.TotalTokens += chunk.Usage.TotalTokens
			}
			if chunk.Text != "" {
				text.WriteString(chunk.Text)
				if err := sink.SendToken(ctx, chunk.Text); err != nil {
					return nil, err
				}
			}
			calls = append(calls, chunk.ToolCalls...)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if len(calls) == 0 {
			res.Content = strings.TrimSpace(text.String())
			if res.Content == "" {
				return nil, fmt.Errorf("llm stream completed without content")
			}
			if usageSeen {
				res.Usage = &usage
			}
			return res, nil
		}

		thought := strings.TrimSpace(text.String())
		msgs = append(msgs, ports.Message{Role: domain.RoleAssistant, Content: thought, ToolCalls: calls})
		for i, call := range calls {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if err := sink.SendToolCall(ctx, domain.ToolCallEvent{ID: call.ID, Name: call.Name, Arguments: call.Arguments}); err != nil {
				return nil, err
			}

			start := time.Now()
			resultText, invokeErr := invokeTool(ctx, tools, call)
			elapsed := time.Since(start).Milliseconds()
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			step := domain.ToolCallStep{
				Step:       len(res.Steps) + 1,
				ID:         call.ID,
				Name:       call.Name,
				Arguments:  normalizeArguments(call.Arguments),
				DurationMs: elapsed,
			}
			if i == 0 {
				step.Thought = thought
			}
			ev := domain.ToolResultEvent{ID: call.ID, Name: call.Name, DurationMs: elapsed}
			if invokeErr != nil {
				// 工具失败不中断循环：错误作为 result 回喂 LLM（spec §4.1）
				step.Error = domain.ToolExecutionFailed
				ev.Error = domain.ToolExecutionFailed
				resultText = fmt.Sprintf(`{"error":%q}`, domain.ToolExecutionFailed)
			} else {
				step.Result = truncateRunes(resultText, resultMaxRunes)
				ev.Result = step.Result
			}
			if err := sink.SendToolResult(ctx, ev); err != nil {
				return nil, err
			}
			res.Steps = append(res.Steps, step)
			msgs = append(msgs, ports.Message{Role: domain.RoleTool, Content: resultText, ToolCallID: call.ID})
		}
	}
	// 最终轮不提供 tools，正常情况下不可达
	return nil, fmt.Errorf("react loop exceeded %d iterations", a.maxIterations)
}

func definitions(tools []ports.Tool) []ports.ToolDefinition {
	defs := make([]ports.ToolDefinition, 0, len(tools))
	for _, t := range tools {
		defs = append(defs, ports.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.ParametersSchema(),
		})
	}
	return defs
}

func invokeTool(ctx context.Context, tools []ports.Tool, call ports.ToolCall) (string, error) {
	for _, t := range tools {
		if t.Name() == call.Name {
			return t.Invoke(ctx, call.Arguments)
		}
	}
	return "", fmt.Errorf("unknown tool: %s", call.Name)
}

// normalizeArguments 保证写入 JSONB 的 arguments 一定是合法 JSON：
// LLM 偶发输出坏 JSON 时降级为带引号的字符串。
func normalizeArguments(raw string) json.RawMessage {
	if raw == "" {
		return json.RawMessage(`{}`)
	}
	if json.Valid([]byte(raw)) {
		return json.RawMessage(raw)
	}
	quoted, _ := json.Marshal(raw)
	return quoted
}

func truncateRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes-3]) + "..."
}
