package skillbuilder

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authstore "github.com/zenith-wang/it-wiki/backend/internal/auth"
	"github.com/zenith-wang/it-wiki/backend/internal/playground"
)

type connectionFixture struct{ value playground.Connection }

func (f *connectionFixture) Get(_ context.Context, user string) (playground.Connection, error) {
	if f.value.UserID != user {
		return playground.Connection{}, playground.ErrNotConfigured
	}
	return f.value, nil
}
func (f *connectionFixture) Save(_ context.Context, value playground.Connection, _ int64) (playground.Connection, error) {
	f.value = value
	return value, nil
}
func (f *connectionFixture) Delete(context.Context, string) error { return nil }

// These are local protocol fixtures, not acceptance calls to the named models.
func TestBuilderUsesCurrentProviderParametersAndSkipsThinkingDeltas(t *testing.T) {
	for _, tc := range []struct {
		model, protocol string
		budget          int
	}{
		{"gpt-5.6", playground.ProtocolOpenAI, 0},
		{"kimi-k3", playground.ProtocolOpenAI, 262144},
		{"claude-opus-5", playground.ProtocolAnthropic, 0},
	} {
		t.Run(tc.model, func(t *testing.T) {
			requestData := request()
			requestData.Model = tc.model
			requestData.Material = strings.Repeat("参考说明：保留 \"字段\" 与路径 references/template.md。\\n\n", 512)
			requestData.Messages = nil
			for range 4 {
				requestData.Messages = append(requestData.Messages,
					playground.Message{Role: "user", Content: strings.Repeat("按参考说明定制审核流程，保留示例与检查项。", 64)},
					playground.Message{Role: "assistant", Content: "已记录当前要求，请补充验收条件。"})
			}
			requestData.Messages = append(requestData.Messages, playground.Message{Role: "user", Content: "根据上述要求更新草案，保留附件引用。"})
			previousDraft := validDraft()
			requestData.Draft = &previousDraft
			if tc.budget != 0 {
				requestData.MaxTokens = &tc.budget
			}
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var input map[string]any
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				field, absent := "max_completion_tokens", "max_tokens"
				if tc.protocol == playground.ProtocolAnthropic {
					field, absent = absent, field
				}
				budget := tc.budget
				if budget == 0 {
					budget = DefaultOutputTokens
				}
				if input["model"] != tc.model || input[field] != float64(budget) {
					t.Error("model or budget lost before the upstream request")
				}
				for _, name := range []string{absent, "temperature", "top_p", "frequency_penalty", "presence_penalty"} {
					if _, ok := input[name]; ok {
						t.Errorf("unexpected parameter %s", name)
					}
				}
				var messages struct {
					Messages []playground.Message `json:"messages"`
				}
				rawMessages, _ := json.Marshal(input)
				if json.Unmarshal(rawMessages, &messages) != nil {
					t.Error("invalid upstream messages")
					w.WriteHeader(500)
					return
				}
				start := 0
				if tc.protocol == playground.ProtocolOpenAI {
					start = 1
				}
				if len(messages.Messages) != start+2+len(requestData.Messages) {
					t.Error("conversation turns lost")
					w.WriteHeader(500)
					return
				}
				var reference struct {
					Material string `json:"reference_material"`
					Draft    *Draft `json:"current_draft"`
				}
				_, data, _ := strings.Cut(messages.Messages[start].Content, "\n")
				if json.Unmarshal([]byte(data), &reference) != nil || reference.Material != requestData.Material || reference.Draft == nil || reference.Draft.Instructions != previousDraft.Instructions {
					t.Error("reference material or edited draft was truncated or altered")
					w.WriteHeader(500)
					return
				}
				for i, message := range requestData.Messages {
					if messages.Messages[start+2+i] != message {
						t.Error("conversation history altered")
						w.WriteHeader(500)
						return
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				generated := validDraft()
				generated.Instructions += "\n\n" + strings.Repeat("逐项核对资料，列出缺失内容并交付审核清单。\n", 256)
				encodedReply, _ := json.Marshal(Reply{Message: "已根据参考说明更新草案", Draft: &generated})
				reply := string(encodedReply)
				var chunk []byte
				if tc.protocol == playground.ProtocolAnthropic {
					fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"private reasoning\"}}\n\n")
					chunk, _ = json.Marshal(map[string]any{"type": "content_block_delta", "delta": map[string]string{"type": "text_delta", "text": reply}})
					fmt.Fprintf(w, "data: %s\n\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n", chunk)
				} else {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"private reasoning\"}}]}\n\n")
					chunk, _ = json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": reply}, "finish_reason": "stop"}}})
					fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
				}
			}))
			defer upstream.Close()
			protector, err := authstore.NewAESGCMProtector([]byte("12345678901234567890123456789012"))
			if err != nil {
				t.Fatal(err)
			}
			model := playground.New(&connectionFixture{}, protector, upstream.Client())
			if _, err := model.Save(context.Background(), "alice", playground.SaveRequest{Protocol: tc.protocol, BaseURL: upstream.URL, APIKey: "fixture-key", Models: []string{tc.model}, DefaultModel: tc.model}); err != nil {
				t.Fatal(err)
			}
			reply, err := New(model, nil).Turn(context.Background(), "alice", requestData)
			if err != nil || reply.Message != "已根据参考说明更新草案" || reply.Draft == nil || len(reply.Draft.Instructions) < 12000 || calls != 1 {
				t.Fatalf("builder protocol roundtrip failed: %v %+v calls=%d", err, reply, calls)
			}
		})
	}
}
