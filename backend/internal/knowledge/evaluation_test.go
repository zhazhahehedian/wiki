package knowledge

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"sort"
	"strings"
	"testing"
)

// A deterministic bag-of-terms fixture tests the evaluation harness and source
// mappings. It is deliberately not presented as real embedding quality evidence.
func TestBusinessQuestionEvaluationFixture(t *testing.T) {
	terms := []string{"报销", "材料", "发票", "审批单", "时限", "天", "境内", "境外", "请假", "住宿", "限额", "普通员工"}
	vector := func(s string) []float64 {
		out := make([]float64, len(terms))
		for i, term := range terms {
			out[i] = float64(strings.Count(s, term))
		}
		return out
	}
	type candidate struct {
		match Match
		vec   []float64
	}
	candidates := []candidate{}
	for _, name := range []string{"expense.md", "travel.md", "leave.txt"} {
		data, e := EvaluationData.ReadFile("testdata/" + name)
		if e != nil {
			t.Fatal(e)
		}
		p := Parse(context.Background(), name, data)
		blocks, e := Split(p)
		if e != nil {
			t.Fatal(e)
		}
		for _, b := range blocks {
			candidates = append(candidates, candidate{Match{Title: name, Content: b.Content, VersionID: "fixture-k1-1-" + name, ChunkID: "fixture-" + name + "-" + b.Locator.Heading, Locator: b.Locator}, vector(strings.Repeat(b.Locator.Heading+" ", 3) + b.Content)})
		}
	}
	report, e := Evaluate(context.Background(), func(_ context.Context, query string) ([]Match, error) {
		q := vector(query)
		out := []Match{}
		for _, c := range candidates {
			dot, a, b := float64(0), float64(0), float64(0)
			for i, x := range q {
				dot += x * c.vec[i]
				a += x * x
				b += c.vec[i] * c.vec[i]
			}
			if a*b == 0 {
				continue
			}
			score := dot / math.Sqrt(a*b)
			if score >= 0.25 {
				m := c.match
				m.Score = float32(score)
				out = append(out, m)
			}
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
		if len(out) > 5 {
			out = out[:5]
		}
		return out, nil
	})
	report.Mode = "fixture-only"
	report.Model = "deterministic-business-terms-v1"
	report.MinScore = 0.25
	raw, _ := json.MarshalIndent(report, "", "  ")
	t.Log(string(raw))
	if file := os.Getenv("KNOWLEDGE_EVAL_REPORT"); file != "" {
		if e := os.WriteFile(file, raw, 0644); e != nil {
			t.Fatal(e)
		}
	}
	if e != nil || !report.Passed {
		t.Fatal("fixture evaluation failed", e)
	}
}
