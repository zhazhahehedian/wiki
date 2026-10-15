package knowledge

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

type hybridRepo struct {
	Repository
	deny, errorOnRecheck              error
	vector, lexical                   []Match
	checks, vectorCalls, lexicalCalls int
}

func (r *hybridRepo) Authorize(context.Context, Principal, []string) ([]KB, error) {
	return []KB{{HasActive: true, Model: "test", Dim: 2}}, r.deny
}
func (r *hybridRepo) Search(_ context.Context, ids []string, _ []float32, top int, _ float32) ([]Match, error) {
	r.vectorCalls++
	return r.vector, nil
}
func (r *hybridRepo) LexicalSearch(_ context.Context, ids, terms []string, top int) ([]Match, error) {
	r.lexicalCalls++
	return r.lexical, nil
}
func (r *hybridRepo) Recheck(_ context.Context, _ Principal, _ []string, m []Match) ([]Match, error) {
	r.checks++
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
	s := New(r, nil, &testEmbed{}, 0.5)
	s.Optimizer = o
	info := RetrievalInfo{}
	got, e := s.SearchDetailed(context.Background(), Principal{}, question(), &info)
	if e != nil || len(got) != 1 || got[0].ChunkID != "domestic" || got[0].VersionID != "v2" || got[0].Locator.Line != 9 || r.vectorCalls != 2 || r.lexicalCalls != 1 || r.checks != 2 || info.Rerank != "model" {
		t.Fatalf("%+v %+v %v", got, info, e)
	}
}
func TestOptimizationAuthorizationAndInFlightRevoke(t *testing.T) {
	r := &hybridRepo{deny: ErrForbidden, vector: []Match{{ChunkID: "a"}}}
	o := &fixtureOptimizer{rewrite: question().Query, scores: []float32{1}}
	s := New(r, nil, &testEmbed{}, 0.5)
	s.Optimizer = o
	if _, e := s.Search(context.Background(), Principal{}, question()); !errors.Is(e, ErrForbidden) || o.calls != 0 {
		t.Fatal("query leaked before authorization")
	}
	r.deny = nil
	r.errorOnRecheck = ErrForbidden
	if _, e := s.Search(context.Background(), Principal{}, question()); !errors.Is(e, ErrForbidden) || o.rankCalls != 0 {
		t.Fatal("revoked content sent to reranker")
	}
	r.errorOnRecheck = nil
	o.beforeRank = func() { r.errorOnRecheck = ErrUnauthenticated }
	if _, e := s.Search(context.Background(), Principal{}, question()); !errors.Is(e, ErrUnauthenticated) {
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
		{"invalid scores", []float32{float32(math.NaN())}, nil, 1, "fallback"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &hybridRepo{vector: []Match{{ChunkID: "a", Content: "境内住宿费用"}}}
			o := &fixtureOptimizer{rewrite: question().Query, scores: tc.scores, err: tc.err}
			s := New(r, nil, &testEmbed{}, 0.5)
			s.Optimizer = o
			info := RetrievalInfo{}
			got, e := s.SearchDetailed(context.Background(), Principal{}, question(), &info)
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
	r := &hybridRepo{}
	o := &fixtureOptimizer{rewrite: "境外住宿费用"}
	s := New(r, nil, &testEmbed{}, 0.5)
	s.Optimizer = o
	info := RetrievalInfo{}
	_, e := s.SearchDetailed(context.Background(), Principal{}, question(), &info)
	if e != nil || info.Rewrite != "fallback" || r.vectorCalls != 1 {
		t.Fatal(info, e)
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
