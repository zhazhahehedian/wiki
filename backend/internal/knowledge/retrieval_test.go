package knowledge

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// hybridRepo returns fixed vector (already above minScore) and keyword lists.
type hybridRepo struct {
	Repository
	deny, errorOnRecheck error
	vector, lexical      []Match
}

func (r *hybridRepo) Authorize(context.Context, Principal, []string) ([]KB, error) {
	return []KB{{HasActive: true, Model: "test", Dim: 2}}, r.deny
}
func (r *hybridRepo) Search(context.Context, []string, []float32, int, float32) ([]Match, error) {
	return r.vector, nil
}
func (r *hybridRepo) LexicalSearch(context.Context, []string, []string, int) ([]Match, error) {
	return r.lexical, nil
}
func (r *hybridRepo) Recheck(_ context.Context, _ Principal, _ []string, m []Match) ([]Match, error) {
	return m, r.errorOnRecheck
}

type fixtureOptimizer struct {
	rewrite          string
	err              error
	scores           []float32
	calls, rankCalls int
	beforeRank       func()
	received         []Match
}

func (o *fixtureOptimizer) Rewrite(context.Context, string) (string, error) {
	o.calls++
	return o.rewrite, o.err
}
func (o *fixtureOptimizer) Rerank(_ context.Context, _ string, m []Match) ([]float32, error) {
	o.rankCalls++
	o.received = m
	if o.beforeRank != nil {
		o.beforeRank()
	}
	return o.scores, o.err
}
func question() SearchRequest {
	return SearchRequest{Query: "境内住宿费用", KBIDs: []string{"00000000-0000-0000-0000-000000000001"}, TopK: 1}
}
func chunkIDs(m []Match) []string {
	out := []string{}
	for _, x := range m {
		out = append(out, x.ChunkID)
	}
	return out
}
func TestHybridRecallAndRerankPreserveCitations(t *testing.T) {
	wrong := Match{ChunkID: "foreign", Content: "境外住宿费用", VersionID: "v1"}
	right := Match{ChunkID: "domestic", Content: "境内住宿费用", VersionID: "v2", Locator: Locator{Line: 9}}
	r := &hybridRepo{vector: []Match{wrong}, lexical: []Match{right}}
	o := &fixtureOptimizer{rewrite: "境内住宿费用限额"}
	o.beforeRank = func() {
		o.scores = make([]float32, len(o.received))
		for i, m := range o.received {
			if m.ChunkID == "domestic" {
				o.scores[i] = 0.95
			}
		}
	}
	e := &testEmbed{}
	s := New(r, nil, e, 0.5)
	s.Optimizer = o
	got, info, err := s.Search(context.Background(), Principal{}, question())
	if err != nil || len(got) != 1 || got[0].ChunkID != "domestic" || got[0].VersionID != "v2" || got[0].Locator.Line != 9 || info.Rerank != "model" || info.Strategy != "hybrid_rrf" {
		t.Fatalf("%+v %+v %v", got, info, err)
	}
	if len(o.received) != 2 || !reflect.DeepEqual(e.texts, []string{"境内住宿费用", "境内住宿费用限额"}) {
		t.Fatal("reranker must judge both recall paths for original and rewritten query", chunkIDs(o.received), e.texts)
	}
}

// Keyword overlap is not evidence of an answer: a chunk found only lexically
// is returned only when a relevance model judged it, never on keywords alone.
func TestKeywordOnlyMatchesNeedRelevanceJudgment(t *testing.T) {
	vector := Match{ChunkID: "vector", Content: "差旅标准", VersionID: "v1"}
	keyword := Match{ChunkID: "keyword", Content: "境内住宿费用", VersionID: "v2"}
	for _, tc := range []struct {
		name            string
		vector, lexical []Match
		optimizer       *fixtureOptimizer
		want            []string
	}{
		{"local keyword only", nil, []Match{keyword}, nil, []string{}},
		{"local mixed", []Match{vector}, []Match{keyword}, nil, []string{"vector"}},
		{"local both paths", []Match{keyword}, []Match{keyword}, nil, []string{"keyword"}},
		{"rerank fallback", nil, []Match{keyword}, &fixtureOptimizer{rewrite: question().Query, err: ErrUnavailable}, []string{}},
		{"rerank judged", nil, []Match{keyword}, &fixtureOptimizer{rewrite: question().Query, scores: []float32{0.9}}, []string{"keyword"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(&hybridRepo{vector: tc.vector, lexical: tc.lexical}, nil, &testEmbed{}, 0.5)
			if tc.optimizer != nil {
				s.Optimizer = tc.optimizer
			}
			req := question()
			req.TopK = 5
			got, _, err := s.Search(context.Background(), Principal{}, req)
			if err != nil || !reflect.DeepEqual(chunkIDs(got), tc.want) {
				t.Fatal(chunkIDs(got), err)
			}
		})
	}
}
func TestOptimizationAuthorizationAndInFlightRevoke(t *testing.T) {
	r := &hybridRepo{deny: ErrForbidden, vector: []Match{{ChunkID: "a"}}}
	o := &fixtureOptimizer{rewrite: question().Query, scores: []float32{1}}
	s := New(r, nil, &testEmbed{}, 0.5)
	s.Optimizer = o
	if _, _, e := s.Search(context.Background(), Principal{}, question()); !errors.Is(e, ErrForbidden) || o.calls != 0 {
		t.Fatal("query leaked before authorization")
	}
	r.deny = nil
	r.errorOnRecheck = ErrForbidden
	if _, _, e := s.Search(context.Background(), Principal{}, question()); !errors.Is(e, ErrForbidden) || o.rankCalls != 0 {
		t.Fatal("revoked content sent to reranker")
	}
	r.errorOnRecheck = nil
	o.beforeRank = func() { r.errorOnRecheck = ErrUnauthenticated }
	if _, _, e := s.Search(context.Background(), Principal{}, question()); !errors.Is(e, ErrUnauthenticated) {
		t.Fatal("in-flight revoke ignored", e)
	}
}
func TestOptimizationFallbackAndNoAnswer(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scores []float32
		err    error
		count  int
		mode   string
	}{
		{"no answer", []float32{0.1}, nil, 0, "model"},
		{"provider failure", nil, ErrUnavailable, 1, "fallback"},
		{"missing scores", nil, nil, 1, "fallback"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &hybridRepo{vector: []Match{{ChunkID: "a", Content: "境内住宿费用"}}}
			o := &fixtureOptimizer{rewrite: question().Query, scores: tc.scores, err: tc.err}
			s := New(r, nil, &testEmbed{}, 0.5)
			s.Optimizer = o
			got, info, e := s.Search(context.Background(), Principal{}, question())
			if e != nil || len(got) != tc.count || info.Rerank != tc.mode {
				t.Fatal(got, info, e)
			}
		})
	}
}
func TestRewriteCannotDropConstraints(t *testing.T) {
	if validRewrite("500 元", "5000 元") || validRewrite("可报销", "不可报销") || validRewrite(strings.Repeat("费用", 100)+" 500 元", "费用 5000 元") {
		t.Fatal("rewritten constraints changed")
	}
	for _, q := range []string{"不允许报销 500 元", "境内政策 FIN-2026", "仅限正式员工", "2026年之前"} {
		if validRewrite(q, "报销政策") {
			t.Fatal("unsafe rewrite accepted", q)
		}
		if !validRewrite(q, q) {
			t.Fatal("original query rejected", q)
		}
	}
	e := &testEmbed{}
	s := New(&hybridRepo{}, nil, e, 0.5)
	s.Optimizer = &fixtureOptimizer{rewrite: "境外住宿费用"}
	_, info, err := s.Search(context.Background(), Principal{}, question())
	if err != nil || info.Rewrite != "fallback" || !reflect.DeepEqual(e.texts, []string{question().Query}) {
		t.Fatal("rejected rewrite was searched", info, e.texts, err)
	}
}
func TestFusionDeduplicatesAndTermsBounded(t *testing.T) {
	a := Match{ChunkID: "a", Content: "报销材料"}
	b := Match{ChunkID: "b", Content: "其他"}
	got := fuse("报销材料", [][]Match{{b, a, a}, {a}})
	if len(got) != 2 || got[0].ChunkID != "a" {
		t.Fatal(got)
	}
	if terms := SearchTerms("报销 报销 FIN-2026 500"); !reflect.DeepEqual(terms, []string{"报销", "fin-2026", "500"}) {
		t.Fatal(terms)
	}
	if len(SearchTerms(strings.Repeat("a b c ", 2000))) > 64 {
		t.Fatal("unbounded terms")
	}
}
