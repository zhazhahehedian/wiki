package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type fakeEmbedder struct {
	dim     int
	texts   []string
	vectors [][]float32
	err     error
}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	f.texts = texts
	return f.vectors, f.err
}

func (f *fakeEmbedder) Dim() int { return f.dim }

func (f *fakeEmbedder) Model() string { return "fake-embed" }

type fakeVectorStore struct {
	kbID  string
	query []float32
	opts  ports.VectorSearchOptions
	hits  []ports.VectorSearchHit
	err   error
}

func (f *fakeVectorStore) InsertChunks(context.Context, []domain.ChunkWithEmbedding) error {
	return nil
}

func (f *fakeVectorStore) Search(_ context.Context, kbID string, query []float32, opts ports.VectorSearchOptions) ([]ports.VectorSearchHit, error) {
	f.kbID = kbID
	f.query = query
	f.opts = opts
	return f.hits, f.err
}

func TestRetrievalFiltersCrossKBHits(t *testing.T) {
	embed := &fakeEmbedder{dim: 3, vectors: [][]float32{{1, 0, 0}}}
	vstore := &fakeVectorStore{hits: []ports.VectorSearchHit{
		{KBID: "kb-1", ChunkID: "chunk-1", DocumentID: "doc-1", DocumentTitle: "Runbook.md", Seq: 1, Content: "rotate password", Score: 0.9},
		{KBID: "kb-2", ChunkID: "chunk-2", DocumentID: "doc-2", DocumentTitle: "Other.md", Seq: 2, Content: "leak", Score: 0.95},
	}}
	svc := NewRetrieval(embed, vstore, 6, 0.5)

	got, err := svc.Retrieve(context.Background(), " kb-1 ", " rotate? ")
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}

	if len(embed.texts) != 1 || embed.texts[0] != "rotate?" {
		t.Fatalf("embedded texts = %#v, want trimmed question", embed.texts)
	}
	if vstore.kbID != "kb-1" {
		t.Fatalf("vector kbID = %q, want kb-1", vstore.kbID)
	}
	if vstore.opts.TopK != 6 || vstore.opts.MinScore != 0.5 {
		t.Fatalf("vector opts = %+v, want topK=6 minScore=0.5", vstore.opts)
	}
	if len(got.Hits) != 1 || got.Hits[0].ChunkID != "chunk-1" {
		t.Fatalf("hits = %#v", got.Hits)
	}
	if got.EvidenceLevel != domain.EvidenceSufficient {
		t.Fatalf("evidence = %s", got.EvidenceLevel)
	}
	if got.Question != "rotate?" {
		t.Fatalf("question = %q, want rotate?", got.Question)
	}
	if len(got.Citations) != 1 || got.Citations[0].ID != "c1" || got.Citations[0].Snippet != "rotate password" {
		t.Fatalf("citations = %#v", got.Citations)
	}
}

func TestRetrievalEvidenceNoneWhenNoHits(t *testing.T) {
	svc := NewRetrieval(&fakeEmbedder{dim: 1, vectors: [][]float32{{1}}}, &fakeVectorStore{}, 8, 0)

	got, err := svc.Retrieve(context.Background(), "kb-1", "question")
	if err != nil {
		t.Fatal(err)
	}
	if got.EvidenceLevel != domain.EvidenceNone {
		t.Fatalf("evidence = %s, want none", got.EvidenceLevel)
	}
}

func TestRetrievalEvidenceWeakWhenTopScoreBelowMinScore(t *testing.T) {
	svc := NewRetrieval(
		&fakeEmbedder{dim: 1, vectors: [][]float32{{1}}},
		&fakeVectorStore{hits: []ports.VectorSearchHit{{KBID: "kb-1", ChunkID: "c1", Score: 0.1, Content: "weak"}}},
		8,
		0.5,
	)

	got, err := svc.Retrieve(context.Background(), "kb-1", "question")
	if err != nil {
		t.Fatal(err)
	}
	if got.EvidenceLevel != domain.EvidenceWeak {
		t.Fatalf("evidence = %s, want weak", got.EvidenceLevel)
	}
}

func TestRetrievalRejectsEmbeddingDimMismatch(t *testing.T) {
	svc := NewRetrieval(&fakeEmbedder{dim: 2, vectors: [][]float32{{1}}}, &fakeVectorStore{}, 8, 0)

	if _, err := svc.Retrieve(context.Background(), "kb-1", "question"); err == nil {
		t.Fatal("expected dim mismatch error")
	}
}

func TestRetrievalPropagatesVectorSearchError(t *testing.T) {
	want := errors.New("search failed")
	svc := NewRetrieval(&fakeEmbedder{dim: 1, vectors: [][]float32{{1}}}, &fakeVectorStore{err: want}, 8, 0)

	if _, err := svc.Retrieve(context.Background(), "kb-1", "question"); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestRetrievalRejectsEmptyInputs(t *testing.T) {
	svc := NewRetrieval(&fakeEmbedder{dim: 1, vectors: [][]float32{{1}}}, &fakeVectorStore{}, 8, 0)

	if _, err := svc.Retrieve(context.Background(), "", "question"); err == nil {
		t.Fatal("expected empty kb id error")
	}
	if _, err := svc.Retrieve(context.Background(), "kb-1", " "); err == nil {
		t.Fatal("expected empty question error")
	}
}

func TestTrimSnippetCollapsesWhitespaceAndTruncates(t *testing.T) {
	got := trimSnippet(" first\n\nsecond   third ", 12)
	if strings.Contains(got, "\n") {
		t.Fatalf("snippet contains newline: %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("snippet = %q, want ellipsis", got)
	}
	if len([]rune(got)) > 12 {
		t.Fatalf("snippet length = %d, want <= 12", len([]rune(got)))
	}
}
