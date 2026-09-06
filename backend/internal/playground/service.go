package playground

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

var (
	ErrNotConfigured = errors.New("model connection is not configured")
	ErrInvalid       = errors.New("invalid model configuration or request")
	ErrConflict      = errors.New("configuration changed; reload before saving")
	ErrUpstream      = errors.New("model service failed or returned an invalid stream")
)

type Connection struct {
	Protocol     string   `json:"protocol"`
	UserID       string   `json:"-"`
	BaseURL      string   `json:"baseUrl"`
	Ciphertext   []byte   `json:"-"`
	Models       []string `json:"models"`
	DefaultModel string   `json:"defaultModel"`
	Version      int64    `json:"version"`
	HasKey       bool     `json:"hasKey"`
}
type SaveRequest struct {
	Protocol     string   `json:"protocol"`
	BaseURL      string   `json:"baseUrl"`
	APIKey       string   `json:"apiKey"`
	Models       []string `json:"models"`
	DefaultModel string   `json:"defaultModel"`
	Version      int64    `json:"version"`
}
type Store interface {
	Get(context.Context, string) (Connection, error)
	Save(context.Context, Connection, int64) (Connection, error)
	Delete(context.Context, string) error
}
type Service struct {
	store     Store
	protector ports.TokenProtector
	client    *http.Client
}

func New(store Store, protector ports.TokenProtector, client *http.Client) *Service {
	return &Service{store, protector, client}
}
func (s *Service) Get(ctx context.Context, user string) (Connection, error) {
	c, e := s.store.Get(ctx, user)
	c.HasKey = len(c.Ciphertext) > 0
	return c, e
}
func (s *Service) Delete(ctx context.Context, user string) error { return s.store.Delete(ctx, user) }

type secretEnvelope struct {
	Protocol string
	User     string
	BaseURL  string
	Key      string
}

func (s *Service) key(c Connection) (string, error) {
	plain, err := s.protector.Decrypt(c.Ciphertext)
	if err != nil {
		return "", ErrUpstream
	}
	var secret secretEnvelope
	if json.Unmarshal([]byte(plain), &secret) != nil || secret.User != c.UserID || secret.BaseURL != c.BaseURL || protocolName(secret.Protocol) != protocolName(c.Protocol) {
		return "", ErrUpstream
	}
	return secret.Key, nil
}
func (s *Service) Save(ctx context.Context, user string, r SaveRequest) (Connection, error) {
	r.Protocol = protocolName(r.Protocol)
	base, err := NormalizeBaseURL(r.BaseURL, r.Protocol)
	if err != nil {
		return Connection{}, ErrInvalid
	}
	if len(r.Models) == 0 || len(r.Models) > 100 || r.Version < 0 {
		return Connection{}, ErrInvalid
	}
	models := make([]string, 0, len(r.Models))
	seen := map[string]bool{}
	found := false
	for _, model := range r.Models {
		model = strings.TrimSpace(model)
		if model == "" || len(model) > 200 || strings.IndexFunc(model, unicode.IsControl) >= 0 {
			return Connection{}, ErrInvalid
		}
		if !seen[model] {
			models = append(models, model)
			seen[model] = true
		}
		if model == r.DefaultModel {
			found = true
		}
	}
	if !found {
		return Connection{}, ErrInvalid
	}
	key := strings.TrimSpace(r.APIKey)
	if key == "" {
		old, e := s.store.Get(ctx, user)
		if e != nil || old.BaseURL != base || protocolName(old.Protocol) != r.Protocol {
			return Connection{}, ErrInvalid
		}
		if old.Version != r.Version {
			return Connection{}, ErrConflict
		}
		key, e = s.key(old)
		if e != nil {
			return Connection{}, e
		}
	}
	if len(key) > 8192 || strings.IndexFunc(key, unicode.IsSpace) >= 0 || strings.IndexFunc(key, unicode.IsControl) >= 0 {
		return Connection{}, ErrInvalid
	}
	plain, _ := json.Marshal(secretEnvelope{Protocol: r.Protocol, User: user, BaseURL: base, Key: key})
	cipher, err := s.protector.Encrypt(string(plain))
	if err != nil {
		return Connection{}, err
	}
	c, err := s.store.Save(ctx, Connection{Protocol: r.Protocol, UserID: user, BaseURL: base, Ciphertext: cipher, Models: models, DefaultModel: r.DefaultModel}, r.Version)
	c.HasKey = err == nil
	return c, err
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type ChatRequest struct {
	Model            string    `json:"model"`
	Messages         []Message `json:"messages"`
	Temperature      *float64  `json:"temperature,omitempty"`
	MaxTokens        *int      `json:"max_tokens,omitempty"`
	TopP             *float64  `json:"top_p,omitempty"`
	FrequencyPenalty *float64  `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64  `json:"presence_penalty,omitempty"`
}

// Stream emits only validated text deltas. Provider bodies/headers and keys are
// never forwarded as error messages. Conversations are not persisted.
func (s *Service) Stream(ctx context.Context, user string, r ChatRequest, emit func(string) error) error {
	c, err := s.store.Get(ctx, user)
	if err != nil {
		return err
	}
	allowed := false
	for _, m := range c.Models {
		if m == r.Model {
			allowed = true
		}
	}
	if !allowed || len(r.Messages) == 0 || len(r.Messages) > 100 {
		return ErrInvalid
	}
	size := 0
	for _, m := range r.Messages {
		if m.Role != "user" && m.Role != "assistant" && m.Role != "system" {
			return ErrInvalid
		}
		size += len(m.Content)
	}
	if size > 256*1024 || size == 0 ||
		(r.Temperature != nil && (*r.Temperature < 0 || *r.Temperature > 2)) ||
		(r.MaxTokens != nil && (*r.MaxTokens < 1 || *r.MaxTokens > 32768)) ||
		(r.TopP != nil && (*r.TopP < 0 || *r.TopP > 1)) ||
		(r.FrequencyPenalty != nil && (*r.FrequencyPenalty < -2 || *r.FrequencyPenalty > 2)) ||
		(r.PresencePenalty != nil && (*r.PresencePenalty < -2 || *r.PresencePenalty > 2)) {
		return ErrInvalid
	}
	key, err := s.key(c)
	if err != nil {
		return err
	}
	body, err := chatBody(c.Protocol, r)
	if err != nil {
		return ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	path := "/chat/completions"
	if protocolName(c.Protocol) == ProtocolAnthropic {
		path = "/v1/messages"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return ErrInvalid
	}
	setModelAuthentication(req, c.Protocol, key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	response, err := s.client.Do(req)
	if err != nil {
		return ErrUpstream
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || !strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		return ErrUpstream
	}
	scanner := bufio.NewScanner(io.LimitReader(response.Body, 8*1024*1024))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	var event []string
	finished := false
	process := func() error {
		data := strings.Join(event, "\n")
		event = nil
		if data == "" {
			return nil
		}
		if protocolName(c.Protocol) == ProtocolAnthropic {
			var err error
			finished, err = anthropicDelta(data, emit)
			return err
		}
		if data == "[DONE]" {
			finished = true
			return nil
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Error json.RawMessage `json:"error"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil || len(chunk.Error) > 0 {
			return ErrUpstream
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			return emit(chunk.Choices[0].Delta.Content)
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := process(); err != nil {
				return err
			}
			if finished {
				return nil
			}
		} else if strings.HasPrefix(line, "data:") {
			event = append(event, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if len(event) > 0 {
		if err := process(); err != nil {
			return err
		}
	}
	if scanner.Err() != nil || !finished {
		return ErrUpstream
	}
	return nil
}
