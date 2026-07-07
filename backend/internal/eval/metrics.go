package eval

import "strings"

// Hit 是检索命中的最小评测视图（与 ports.VectorSearchHit 解耦，保持本包纯函数）。
type Hit struct {
	DocumentTitle string
	Content       string
}

type CaseResult struct {
	ID         string  `json:"id"`
	Question   string  `json:"question"`
	DocRank    int     `json:"doc_rank"`    // 期望文档首次命中的名次（1-based）；0 = 未命中
	PassageHit bool    `json:"passage_hit"` // top-5 内存在"期望文档且含任一关键词"的切片
	TopScore   float32 `json:"top_score"`
}

func ScoreCase(c Case, hits []Hit, topScore float32) CaseResult {
	r := CaseResult{ID: c.ID, Question: c.Question, TopScore: topScore}
	for i, h := range hits {
		if h.DocumentTitle != c.ExpectedDocument {
			continue
		}
		if r.DocRank == 0 {
			r.DocRank = i + 1
		}
		if i < 5 && containsAnyKeyword(h.Content, c.ExpectedKeywords) {
			r.PassageHit = true
		}
	}
	return r
}

func containsAnyKeyword(content string, keywords []string) bool {
	for _, kw := range keywords {
		if kw != "" && strings.Contains(content, kw) {
			return true
		}
	}
	return false
}

type Summary struct {
	Cases         int     `json:"cases"`
	DocHitAt1     float64 `json:"doc_hit_at_1"`
	DocHitAt3     float64 `json:"doc_hit_at_3"`
	DocHitAt5     float64 `json:"doc_hit_at_5"`
	PassageHitAt5 float64 `json:"passage_hit_at_5"`
	MRR           float64 `json:"mrr"`
}

func Summarize(results []CaseResult) Summary {
	s := Summary{Cases: len(results)}
	if s.Cases == 0 {
		return s
	}
	var hit1, hit3, hit5, passage int
	var rr float64
	for _, r := range results {
		if r.DocRank >= 1 {
			rr += 1.0 / float64(r.DocRank)
			if r.DocRank <= 1 {
				hit1++
			}
			if r.DocRank <= 3 {
				hit3++
			}
			if r.DocRank <= 5 {
				hit5++
			}
		}
		if r.PassageHit {
			passage++
		}
	}
	n := float64(s.Cases)
	s.DocHitAt1 = float64(hit1) / n
	s.DocHitAt3 = float64(hit3) / n
	s.DocHitAt5 = float64(hit5) / n
	s.PassageHitAt5 = float64(passage) / n
	s.MRR = rr / n
	return s
}
