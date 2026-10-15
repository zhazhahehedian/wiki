package skillbuilder

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

// Local protocol fixtures: the builder round-trips through the real playground
// service, sends the budget in the protocol's field and ignores reasoning deltas.
func TestBuilderProtocolRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		protocol, field, absent string
		budget                  int
	}{
		{playground.ProtocolOpenAI, "max_completion_tokens", "max_tokens", 0},
		{playground.ProtocolAnthropic, "max_tokens", "max_completion_tokens", 16000},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			want := tc.budget
			if want == 0 {
				want = playground.DefaultOutputTokens
			}
			draft := validDraft()
			encoded, _ := json.Marshal(Reply{Message: "已更新草案", Draft: &draft})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input map[string]any
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input[tc.field] != float64(want) {
					t.Errorf("budget field %s = %v; want %d", tc.field, input[tc.field], want)
				}
				if _, ok := input[tc.absent]; ok {
					t.Errorf("unexpected parameter %s", tc.absent)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if tc.protocol == playground.ProtocolAnthropic {
					chunk, _ := json.Marshal(map[string]any{"type": "content_block_delta", "delta": map[string]string{"type": "text_delta", "text": string(encoded)}})
					fmt.Fprintf(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"private reasoning\"}}\n\ndata: %s\n\ndata: {\"type\":\"message_stop\"}\n\n", chunk)
				} else {
					chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": string(encoded)}, "finish_reason": "stop"}}})
					fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"private reasoning\"}}]}\n\ndata: %s\n\ndata: [DONE]\n\n", chunk)
				}
			}))
			defer upstream.Close()
			protector, err := authstore.NewAESGCMProtector([]byte("12345678901234567890123456789012"))
			if err != nil {
				t.Fatal(err)
			}
			model := playground.New(&connectionFixture{}, protector, upstream.Client())
			if _, err := model.Save(context.Background(), "alice", playground.SaveRequest{Protocol: tc.protocol, BaseURL: upstream.URL, APIKey: "fixture-key", Models: []string{"model"}, DefaultModel: "model"}); err != nil {
				t.Fatal(err)
			}
			r := request()
			r.Model = "model"
			if tc.budget != 0 {
				r.MaxTokens = &tc.budget
			}
			reply, err := New(model, nil).Turn(context.Background(), "alice", r)
			if err != nil || reply.Message != "已更新草案" || reply.Draft == nil || reply.Draft.Instructions != draft.Instructions {
				t.Fatalf("builder protocol roundtrip failed: %v %+v", err, reply)
			}
		})
	}
}
