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
