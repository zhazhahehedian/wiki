package embedder

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestOpenAICompatBatchesByTenAndKeepsOrder(t *testing.T) {
	var mu sync.Mutex
	sizes, dims := []int{}, []int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in embedRequest
		if r.URL.Path != "/v1/embeddings" || json.NewDecoder(r.Body).Decode(&in) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		sizes, dims = append(sizes, len(in.Input)), append(dims, in.Dimensions)
		mu.Unlock()
		if len(in.Input) > 10 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"batch size is invalid, it should not be larger than 10: ` + in.Input[0] + `"}}`))
			return
		}
		var b strings.Builder
		b.WriteString(`{"data":[`)
		for i := len(in.Input) - 1; i >= 0; i-- {
			var n int
			fmt.Sscanf(in.Input[i], "t%d", &n)
			fmt.Fprintf(&b, `{"index":%d,"embedding":[%d,1]}`, i, n)
			if i > 0 {
				b.WriteString(",")
			}
		}
		b.WriteString(`]}`)
		_, _ = w.Write([]byte(b.String()))
	}))
	defer srv.Close()

	texts := []string{}
	for i := 0; i < 23; i++ {
		texts = append(texts, fmt.Sprintf("t%d", i))
	}
	got, err := New(Config{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "text-embedding-v4", Dim: 2}).Embed(context.Background(), texts)
	if err != nil || len(got) != 23 {
		t.Fatal(err, len(got))
	}
	for i, v := range got {
		if int(v[0]) != i {
			t.Fatal("order lost", i, v)
		}
	}
	if fmt.Sprint(sizes) != "[10 10 3]" || fmt.Sprint(dims) != "[0 0 0]" {
		t.Fatal(sizes, dims)
	}

	bailian := New(Config{BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Model: "text-embedding-v4", Dim: 1024})
	if !bailian.sendDimension {
		t.Fatal("Bailian dimensions not sent")
	}
}

func TestOpenAICompatRejectsMalformedAndRedactsErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
	}{
		"status":    {http.StatusBadRequest, `{"error":{"message":"secret text"}}`},
		"count":     {http.StatusOK, `{"data":[]}`},
		"duplicate": {http.StatusOK, `{"data":[{"index":0,"embedding":[1,1]},{"index":0,"embedding":[1,1]}]}`},
		"dimension": {http.StatusOK, `{"data":[{"index":0,"embedding":[1]},{"index":1,"embedding":[1]}]}`},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		_, err := New(Config{BaseURL: srv.URL, Model: "m", Dim: 2}).Embed(context.Background(), []string{"secret text", "b"})
		srv.Close()
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal(name, err)
		}
	}
}
