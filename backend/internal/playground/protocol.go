package playground

import (
	"encoding/json"
	"net/http"
	"strings"
)

const ProtocolOpenAI = "openai"
const ProtocolAnthropic = "anthropic"

func protocolName(value string) string {
	if value == "" {
		return ProtocolOpenAI
	}
	return value
}

func NormalizeBaseURL(raw, protocol string) (string, error) {
	protocol = protocolName(protocol)
	if protocol != ProtocolOpenAI && protocol != ProtocolAnthropic {
		return "", ErrInvalid
	}
	base, err := NormalizeURL(raw)
	if err != nil {
		return "", err
	}
	if protocol == ProtocolAnthropic {
		base = strings.TrimSuffix(base, "/v1")
		if strings.HasSuffix(base, "/messages") || strings.HasSuffix(base, "/models") {
			return "", ErrInvalid
		}
	}
	return base, nil
}

func setModelAuthentication(req *http.Request, protocol, key string) {
	if protocolName(protocol) == ProtocolAnthropic {
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+key)
	}
}

func chatBody(protocol string, input ChatRequest) ([]byte, error) {
	if protocolName(protocol) != ProtocolAnthropic {
		return json.Marshal(struct {
			ChatRequest
			Stream bool `json:"stream"`
		}{input, true})
	}
	var system []string
	messages := make([]Message, 0, len(input.Messages))
	for _, message := range input.Messages {
		if message.Role == "system" {
			system = append(system, message.Content)
		} else {
			messages = append(messages, message)
		}
	}
	if len(messages) == 0 {
		return nil, ErrInvalid
	}
	maxTokens := 4096
	if input.MaxTokens != nil {
		maxTokens = *input.MaxTokens
	}
	if (input.Temperature != nil && *input.Temperature > 1) || input.FrequencyPenalty != nil || input.PresencePenalty != nil || (input.TopP != nil && input.Temperature != nil) {
		return nil, ErrInvalid
	}
	return json.Marshal(struct {
		Model       string    `json:"model"`
		Messages    []Message `json:"messages"`
		System      string    `json:"system,omitempty"`
		MaxTokens   int       `json:"max_tokens"`
		Temperature *float64  `json:"temperature,omitempty"`
		TopP        *float64  `json:"top_p,omitempty"`
		Stream      bool      `json:"stream"`
	}{input.Model, messages, strings.Join(system, "\n\n"), maxTokens, input.Temperature, input.TopP, true})
}

func anthropicDelta(data string, emit func(string) error) (bool, error) {
	var event struct {
		Type  string `json:"type"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	}
	if json.Unmarshal([]byte(data), &event) != nil {
		return false, ErrUpstream
	}
	switch event.Type {
	case "error":
		return false, ErrUpstream
	case "message_stop":
		return true, nil
	case "content_block_delta":
		if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
			return false, emit(event.Delta.Text)
		}
	}
	return false, nil
}
