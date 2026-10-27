package knowledge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOptimizerWireAndStrictResponses(t *testing.T) {
	content := `{"query":"报销材料"}`
	finish := "stop"
	status := 200
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer fixture" {
			t.Error("wrong endpoint/auth")
		}
		var input map[string]any
		if json.NewDecoder(r.Body).Decode(&input) != nil || input["model"] != "fixture-model" || input["tools"] != nil {
			t.Error("invalid request")
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": map[string]string{"content": content}}}})
	}))
	defer server.Close()
	o, e := NewModelOptimizer(server.URL+"/v1", "fixture", "fixture-model")
	if e != nil {
		t.Fatal(e)
	}
	if q, e := o.Rewrite(context.Background(), "需要啥报销材料"); e != nil || q != "报销材料" {
		t.Fatal(q, e)
	}
	for _, bad := range []string{`{"results":[{"index":0}]}`, `{"results":[{"score":1}]}`, `{"results":[{"index":1,"score":1}]}`, `{"results":[{"index":0,"score":2}]}`, `{"results":[]} trailing`, strings.Repeat("x", 70<<10)} {
		content = bad
		if _, e = o.Rerank(context.Background(), "q", []Match{{ChunkID: "a"}}); e == nil {
			t.Fatal("invalid model output accepted")
		}
	}
	content = `{"results":[{"index":1,"score":0.9},{"index":0,"score":0.1}]}`
	scores, e := o.Rerank(context.Background(), "q", []Match{{ChunkID: "a"}, {ChunkID: "b"}})
	if e != nil || scores[0] != 0.1 || scores[1] != 0.9 {
		t.Fatal(scores, e)
	}
	content = `{"results":[{"index":0,"score":1},{"index":0,"score":1}]}`
	if _, e = o.Rerank(context.Background(), "q", []Match{{}, {}}); e == nil {
		t.Fatal("duplicate index accepted")
	}
	content = `{"query":"q"}`
	finish = "length"
	if _, e = o.Rewrite(context.Background(), "q"); e == nil {
		t.Fatal("truncated completion accepted")
	}
	finish = "stop"
	status = 503
	if _, e = o.Rewrite(context.Background(), "q"); e != ErrUnavailable {
		t.Fatal("upstream body escaped", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = o.Rewrite(ctx, "q"); e == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestDeepSeekFlashUsesNonThinkingJSONForOptimization(t *testing.T) {
	for _, tc := range []struct {
		base, model string
		disabled    bool
	}{
		{"https://api.deepseek.com/v1", "deepseek-flash", true},
		{"https://example.com/v1", "fixture-model", false},
	} {
		t.Run(tc.model, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					t.Fatal("invalid request")
				}
				if tc.disabled {
					thinking, ok := body["thinking"].(map[string]any)
					format, formatOK := body["response_format"].(map[string]any)
					if !ok || thinking["type"] != "disabled" || !formatOK || format["type"] != "json_object" {
						t.Error("missing non-thinking JSON settings")
					}
				} else if body["thinking"] != nil || body["response_format"] != nil {
					t.Error("provider-specific fields leaked")
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": `{"query":"业务问题"}`}}}})
			}))
			defer server.Close()
			o, err := NewModelOptimizer(tc.base, "fixture", tc.model)
			if err != nil {
				t.Fatal(err)
			}
			o.endpoint = server.URL
			if _, err := o.Rewrite(context.Background(), "业务问题"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOptimizerPreservesExplicitExclusionsWithoutSendingRewrite(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(500)
	}))
	defer server.Close()
	o, err := NewModelOptimizer(server.URL, "fixture", "fixture-model")
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"不是境内，是境外住宿，限额用什么币种？", "仅限普通员工的标准是什么？", "未审批的费用怎么处理？", "无发票是否可以报销？", "禁止报销的项目有哪些？"} {
		got, err := o.Rewrite(context.Background(), q)
		if err != nil || got != q || calls != 0 {
			t.Fatal("explicit qualifier was rewritten", got, err, calls)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := o.Rewrite(ctx, "不是境内"); err == nil {
		t.Fatal("cancelled request accepted")
	}
}
