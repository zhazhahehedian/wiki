package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

func newTestKBRetrieval(t *testing.T, hits []ports.VectorSearchHit, onRetrieval func(context.Context, *service.RetrievalResult) error) *KBRetrieval {
	t.Helper()
	retrieval := service.NewRetrieval(
		fakeEmbedder{dim: 1},
		fakeVectorStore{hits: hits},
		8, 0,
	)
	return NewKBRetrieval(retrieval, "kb-1", onRetrieval)
}

type fakeEmbedder struct{ dim int }

func (f fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1}
	}
	return out, nil
}
func (f fakeEmbedder) Dim() int      { return f.dim }
func (f fakeEmbedder) Model() string { return "fake-embedder" }

type fakeVectorStore struct{ hits []ports.VectorSearchHit }

func (f fakeVectorStore) ReplaceChunks(context.Context, string, []domain.ChunkWithEmbedding) error {
	return nil
}
func (f fakeVectorStore) DeleteByDocument(context.Context, string) error { return nil }
func (f fakeVectorStore) Search(context.Context, string, []float32, ports.VectorSearchOptions) ([]ports.VectorSearchHit, error) {
	return f.hits, nil
}

func TestKBRetrievalInvokeFormatsHitsAndFiresCallback(t *testing.T) {
	hits := []ports.VectorSearchHit{{
		KBID: "kb-1", ChunkID: "ch-1", DocumentID: "doc-1",
		DocumentTitle: "Runbook.md", Seq: 3, Score: 0.91,
		Content: "重启服务的步骤",
	}}
	var got *service.RetrievalResult
	tool := newTestKBRetrieval(t, hits, func(_ context.Context, r *service.RetrievalResult) error {
		got = r
		return nil
	})

	if tool.Name() != "kb_retrieval" {
		t.Errorf("Name() = %q", tool.Name())
	}
	result, err := tool.Invoke(context.Background(), `{"query":"怎么重启"}`)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	for _, want := range []string{"[1]", "Runbook.md", "重启服务的步骤"} {
		if !strings.Contains(result, want) {
			t.Errorf("result missing %q\nresult: %s", want, result)
		}
	}
	if got == nil || len(got.Hits) != 1 {
		t.Fatalf("onRetrieval not fired or wrong hits: %+v", got)
	}
}

func TestKBRetrievalInvokeRejectsBadArguments(t *testing.T) {
	tool := newTestKBRetrieval(t, nil, nil)
	for _, args := range []string{``, `not-json`, `{}`, `{"query":"  "}`} {
		if _, err := tool.Invoke(context.Background(), args); err == nil {
			t.Errorf("Invoke(%q) expected error", args)
		}
	}
}

func TestKBRetrievalInvokeNoHitsReturnsExplicitText(t *testing.T) {
	tool := newTestKBRetrieval(t, nil, func(context.Context, *service.RetrievalResult) error { return nil })
	result, err := tool.Invoke(context.Background(), `{"query":"不存在的内容"}`)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !strings.Contains(result, "no relevant content") {
		t.Errorf("result = %q, want no-hit notice", result)
	}
}
