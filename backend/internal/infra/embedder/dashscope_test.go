package embedder

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func dashScopeFixture(t *testing.T, handler http.HandlerFunc) *DashScope {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	e, err := NewDashScope(Config{BaseURL: server.URL + "/api/v1", APIKey: "fixture", Model: "tongyi-embedding-vision-plus-2026-03-06", Dim: 64})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestDashScopeBatchesAndPreservesInputOrder(t *testing.T) {
	calls := 0
	e := dashScopeFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/services/embeddings/multimodal-embedding/multimodal-embedding" || r.Header.Get("Authorization") != "Bearer fixture" {
			t.Error("wrong protocol")
		}
		var body struct {
			Model      string
			Input      struct{ Contents []struct{ Text string } }
			Parameters struct{ Dimension int }
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Input.Contents) > 20 || body.Parameters.Dimension != 64 || body.Model != "tongyi-embedding-vision-plus-2026-03-06" {
			t.Fatal("wrong request")
		}
		items := []any{}
		for i := len(body.Input.Contents) - 1; i >= 0; i-- {
			var n int
			fmt.Sscanf(body.Input.Contents[i].Text, "text-%d", &n)
			vec := make([]float32, 64)
			vec[0] = float32(n + 1)
			items = append(items, map[string]any{"index": i, "type": "text", "embedding": vec})
		}
		calls++
		json.NewEncoder(w).Encode(map[string]any{"output": map[string]any{"embeddings": items}})
	})
	texts := make([]string, 45)
	for i := range texts {
		texts[i] = fmt.Sprintf("text-%d", i)
	}
	got, err := e.Embed(context.Background(), texts)
	if err != nil || len(got) != 45 || calls != 3 {
		t.Fatal(len(got), calls, err)
	}
	for i, v := range got {
		if v[0] != float32(i+1) {
			t.Fatal("input reordered", i, v[0])
		}
	}
}

func TestDashScopeRejectsMalformedResultsAndRedactsErrors(t *testing.T) {
	vector := make([]float32, 64)
	vector[0] = 1
	entry := func(index any, kind string, vec []float32) map[string]any {
		return map[string]any{"index": index, "type": kind, "embedding": vec}
	}
	for _, items := range [][]any{
		{}, {entry(nil, "text", vector)}, {entry(1, "text", vector)}, {entry(-1, "text", vector)},
		{entry(0, "image", vector)}, {entry(0, "text", []float32{1})}, {entry(0, "text", make([]float32, 64))},
	} {
		e := dashScopeFixture(t, func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"output": map[string]any{"embeddings": items}})
		})
		if _, err := e.Embed(context.Background(), []string{"one"}); err == nil {
			t.Fatal("malformed result accepted")
		}
	}
	e := dashScopeFixture(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"output": map[string]any{"embeddings": []any{entry(0, "text", vector), entry(0, "text", vector)}}})
	})
	if _, err := e.Embed(context.Background(), []string{"one", "two"}); err == nil {
		t.Fatal("duplicate index accepted")
	}
	e = dashScopeFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprint(w, "private text and fixture-secret")
	})
	if _, err := e.Embed(context.Background(), []string{"one"}); err == nil || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "fixture-secret") {
		t.Fatal("provider error body escaped", err)
	}
}

func TestDashScopeLimitsCancellationAndRedirect(t *testing.T) {
	calls := 0
	e := dashScopeFixture(t, func(w http.ResponseWriter, r *http.Request) { calls++; http.Redirect(w, r, "/elsewhere", 307) })
	if _, err := e.Embed(context.Background(), []string{"small", strings.Repeat("中", 257)}); err == nil || calls != 0 {
		t.Fatal("invalid batch was sent")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Embed(ctx, []string{"text"}); err == nil {
		t.Fatal("cancel ignored")
	}
	if _, err := e.Embed(context.Background(), []string{"text"}); err == nil || calls != 1 {
		t.Fatal("redirect followed", calls, err)
	}
}
