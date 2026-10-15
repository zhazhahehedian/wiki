package knowledge

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// QueryOptimizer only sees a standalone question and already authorized chunks.
// It cannot introduce documents, alter citations or produce an answer.
// Rerank returns one relevance score in [0,1] per input match, in input order.
type QueryOptimizer interface {
	Rewrite(context.Context, string) (string, error)
	Rerank(context.Context, string, []Match) ([]float32, error)
}
type RetrievalInfo struct {
	Strategy string   `json:"strategy"`
	Rewrite  string   `json:"rewrite"`
	Rerank   string   `json:"rerank"`
	Warnings []string `json:"warnings"`
}

// SearchTerms uses Chinese bigrams and whole Latin identifiers, without a new
// database tokenizer dependency. Terms are bounded; SQL treats them as data.
func SearchTerms(query string) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(s string) {
		if s != "" && !seen[s] && len(out) < 64 {
			seen[s] = true
			out = append(out, s)
		}
	}
	var latin []rune
	flush := func() { add(string(latin)); latin = nil }
	var prev rune
	for _, r := range strings.ToLower(query) {
		if unicode.Is(unicode.Han, r) {
			flush()
			if prev != 0 {
				add(string([]rune{prev, r}))
			}
			prev = r
		} else {
			prev = 0
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
				latin = append(latin, r)
			} else {
				flush()
			}
		}
	}
	flush()
	return out
}
func coverage(terms []string, text string) float32 {
	if len(terms) == 0 {
		return 0
	}
	n := 0
	text = strings.ToLower(text)
	for _, term := range terms {
		if strings.Contains(text, term) {
			n++
		}
	}
	return float32(n) / float32(len(terms))
}
func validRewrite(original, rewritten string) bool {
	if !utf8.ValidString(rewritten) || rewritten == "" || utf8.RuneCountInString(rewritten) > 4000 {
		return false
	}
	// Numbers/identifiers and explicit qualifiers must survive the rewrite.
	atoms := func(text string) map[string]bool {
		out := map[string]bool{}
		for _, token := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
			return !(unicode.IsDigit(r) || unicode.Is(unicode.Latin, r) || strings.ContainsRune("-_.%/", r))
		}) {
			out[token] = true
		}
		return out
	}
	a, b := atoms(original), atoms(rewritten)
	if len(a) != len(b) {
		return false
	}
	for token := range a {
		if !b[token] {
			return false
		}
	}
	for _, term := range []string{"不", "未", "无", "禁止", "仅", "境内", "境外", "国内", "国外", "之前", "之后", "以上", "以下", "元", "美元", "天", "月", "年", "%"} {
		if strings.Contains(original, term) != strings.Contains(rewritten, term) {
			return false
		}
	}
	return true
}
func (s *Service) retrieve(ctx context.Context, p Principal, in SearchRequest) ([]Match, RetrievalInfo, error) {
	info := RetrievalInfo{Strategy: "hybrid_rrf", Rewrite: "not_configured", Rerank: "local", Warnings: []string{}}
	queries := []string{in.Query}
	if s.Optimizer != nil {
		c, cancel := context.WithTimeout(ctx, 3*time.Second)
		q, err := s.Optimizer.Rewrite(c, in.Query)
		cancel()
		q = strings.TrimSpace(q)
		if err == nil && validRewrite(in.Query, q) {
			info.Rewrite = "applied"
			if q != in.Query {
				queries = append(queries, q)
			} else {
				info.Rewrite = "unchanged"
			}
		} else {
			info.Rewrite = "fallback"
			info.Warnings = append(info.Warnings, "rewrite_unavailable_or_invalid")
		}
	}
	if ctx.Err() != nil {
		return nil, info, ctx.Err()
	}
	unavailable := func() ([]Match, RetrievalInfo, error) {
		if ctx.Err() != nil {
			return nil, info, ctx.Err()
		}
		return nil, info, ErrUnavailable
	}
	// Original query embedding remains an independent mandatory path. Optional
	// rewritten-query failure must not discard a usable original query result.
	lists := [][]Match{}
	relevant := map[string]bool{} // chunks that passed the vector minScore threshold
	candidateK := max(20, in.TopK*3)
	for i, q := range queries {
		c, cancel := context.WithTimeout(ctx, 8*time.Second)
		vectors, err := s.embed.Embed(c, []string{q})
		cancel()
		if ctx.Err() != nil {
			return nil, info, ctx.Err()
		}
		if err != nil || len(vectors) != 1 || !validVector(vectors[0], s.embed.Dim()) {
			if i == 0 {
				return nil, info, ErrUnavailable
			}
			info.Warnings = append(info.Warnings, "rewrite_embedding_unavailable")
			continue
		}
		matches, err := s.Repo.Search(ctx, in.KBIDs, vectors[0], candidateK, s.minScore)
		if err != nil {
			return unavailable()
		}
		for _, m := range matches {
			relevant[m.ChunkID] = true
		}
		lists = append(lists, matches)
	}
	// Original-question lexical recall avoids speculative rewrite keywords
	// opening a second low-confidence path for no-answer questions.
	lexical, err := s.Repo.LexicalSearch(ctx, in.KBIDs, SearchTerms(in.Query), candidateK)
	if err != nil {
		return unavailable()
	}
	lists = append(lists, lexical)
	matches := fuse(in.Query, lists)
	judged := false // a model relevance score replaced the vector threshold
	if s.Optimizer != nil && len(matches) > 0 {
		if len(matches) > candidateK {
			matches = matches[:candidateK]
		}
		// Revoke/delete/version checks before sending content to a ranking provider.
		if matches, err = s.Repo.Recheck(ctx, p, in.KBIDs, matches); err != nil {
			return nil, info, err
		}
		if len(matches) > 0 {
			c, cancel := context.WithTimeout(ctx, 5*time.Second)
			scores, e := s.Optimizer.Rerank(c, in.Query, matches)
			cancel()
			if e == nil && len(scores) == len(matches) {
				judged = true
				info.Rerank = "model"
				out := []Match{}
				for i, m := range matches {
					if scores[i] >= 0.5 {
						m.Score = scores[i]
						out = append(out, m)
					}
				}
				matches = out
				sort.SliceStable(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })
			} else {
				info.Rerank = "fallback"
				info.Warnings = append(info.Warnings, "rerank_unavailable_or_invalid")
			}
		}
	}
	// Keyword overlap alone is not evidence of an answer: without a model
	// judgment, only chunks that met the vector threshold may be returned.
	// Lexical matches still contribute to their ranking through fusion.
	if !judged {
		matches = slices.DeleteFunc(matches, func(m Match) bool { return !relevant[m.ChunkID] })
	}
	if ctx.Err() != nil {
		return nil, info, ctx.Err()
	}
	if len(matches) > in.TopK {
		matches = matches[:in.TopK]
	}
	matches, err = s.Repo.Recheck(ctx, p, in.KBIDs, matches)
	return matches, info, err
}
func fuse(query string, lists [][]Match) []Match {
	type entry struct {
		match Match
		rrf   float32
	}
	entries := map[string]*entry{}
	for _, list := range lists {
		seen := map[string]bool{}
		for i, m := range list {
			if seen[m.ChunkID] {
				continue
			}
			seen[m.ChunkID] = true
			e := entries[m.ChunkID]
			if e == nil {
				e = &entry{match: m}
				entries[m.ChunkID] = e
			}
			e.rrf += 1 / float32(60+i+1)
		}
	}
	out := []Match{}
	terms := SearchTerms(query)
	for _, e := range entries {
		m := e.match
		// Local feature reranking boosts literal constraints and section matches.
		// RRF and coverage are rank signals, not cosine or answer probabilities.
		m.Score = 0.65*(e.rrf*61/float32(max(1, len(lists)))) + 0.25*coverage(terms, m.Content) + 0.10*coverage(terms, m.Locator.Heading+" "+m.Title)
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ChunkID < out[j].ChunkID
		}
		return out[i].Score > out[j].Score
	})
	return out
}
