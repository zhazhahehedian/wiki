package eval

import (
	"math"
	"testing"
)

func TestScoreCaseComputesDocRankAndPassageHit(t *testing.T) {
	c := Case{
		ID:               "q001",
		ExpectedDocument: "部署指南.md",
		ExpectedKeywords: []string{"DATABASE_URL"},
	}
	hits := []Hit{
		{DocumentTitle: "其他文档.md", Content: "无关内容"},
		{DocumentTitle: "部署指南.md", Content: "这里没有关键词"},
		{DocumentTitle: "部署指南.md", Content: "配置 DATABASE_URL 即可"},
	}
	r := ScoreCase(c, hits, 0.85)
	if r.DocRank != 2 {
		t.Errorf("DocRank = %d, want 2", r.DocRank)
	}
	if !r.PassageHit {
		t.Error("PassageHit = false, want true (rank 3 hit contains keyword)")
	}
	if r.TopScore != 0.85 {
		t.Errorf("TopScore = %f, want 0.85", r.TopScore)
	}
}

func TestScoreCaseMissAndKeywordOutsideTop5(t *testing.T) {
	c := Case{ID: "q002", ExpectedDocument: "部署指南.md", ExpectedKeywords: []string{"goose"}}

	r := ScoreCase(c, []Hit{{DocumentTitle: "别的.md", Content: "goose"}}, 0.3)
	if r.DocRank != 0 || r.PassageHit {
		t.Errorf("miss case: DocRank=%d PassageHit=%v, want 0/false", r.DocRank, r.PassageHit)
	}

	hits := make([]Hit, 6)
	for i := range hits {
		hits[i] = Hit{DocumentTitle: "别的.md", Content: "x"}
	}
	hits[5] = Hit{DocumentTitle: "部署指南.md", Content: "goose up"} // 第 6 名
	r = ScoreCase(c, hits, 0.9)
	if r.DocRank != 6 {
		t.Errorf("DocRank = %d, want 6", r.DocRank)
	}
	if r.PassageHit {
		t.Error("PassageHit = true, want false (keyword hit is outside top-5)")
	}
}

func TestSummarizeAggregatesMetrics(t *testing.T) {
	results := []CaseResult{
		{ID: "a", DocRank: 1, PassageHit: true},
		{ID: "b", DocRank: 4, PassageHit: false},
		{ID: "c", DocRank: 0, PassageHit: false},
		{ID: "d", DocRank: 2, PassageHit: true},
	}
	s := Summarize(results)
	if s.Cases != 4 {
		t.Fatalf("Cases = %d, want 4", s.Cases)
	}
	if s.DocHitAt1 != 0.25 {
		t.Errorf("DocHitAt1 = %f, want 0.25", s.DocHitAt1)
	}
	if s.DocHitAt3 != 0.5 {
		t.Errorf("DocHitAt3 = %f, want 0.5", s.DocHitAt3)
	}
	if s.DocHitAt5 != 0.75 {
		t.Errorf("DocHitAt5 = %f, want 0.75", s.DocHitAt5)
	}
	if s.PassageHitAt5 != 0.5 {
		t.Errorf("PassageHitAt5 = %f, want 0.5", s.PassageHitAt5)
	}
	wantMRR := (1.0 + 0.25 + 0.0 + 0.5) / 4.0
	if math.Abs(s.MRR-wantMRR) > 1e-9 {
		t.Errorf("MRR = %f, want %f", s.MRR, wantMRR)
	}
}
