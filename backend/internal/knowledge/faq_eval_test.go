package knowledge

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"testing"
)

func TestFAQChunkingImprovesCitation(t *testing.T) {
	data, err := EvaluationData.ReadFile("testdata/faq.md")
	if err != nil {
		t.Fatal(err)
	}
	parsed := Parse(context.Background(), "faq.md", data)
	if !parsed.Valid() {
		t.Fatal(parsed.Diagnostics)
	}
	after, err := Split(parsed)
	if err != nil {
		t.Fatal(err)
	}
	flat := parsed
	for i := range flat.Blocks {
		flat.Blocks[i].Locator.Heading = ""
	}
	before := mergeWithoutQuestionBoundaries(flat.Blocks)
	beforeReport := scoreFAQ(t, before)
	afterReport := scoreFAQ(t, after)
	rawBefore, _ := json.Marshal(beforeReport)
	rawAfter, _ := json.Marshal(afterReport)
	t.Logf("faq before recall_at_5=%.2f citation_accuracy=%.2f no_answer_false_positive_rate=%.2f", beforeReport.Recall, beforeReport.CitationAccuracy, beforeReport.NoAnswerFalsePositive)
	t.Logf("faq after recall_at_5=%.2f citation_accuracy=%.2f no_answer_false_positive_rate=%.2f", afterReport.Recall, afterReport.CitationAccuracy, afterReport.NoAnswerFalsePositive)
	t.Logf("before %s", rawBefore)
	t.Logf("after %s", rawAfter)
	if afterReport.Recall < beforeReport.Recall || afterReport.CitationAccuracy <= beforeReport.CitationAccuracy || !afterReport.Passed {
		t.Fatalf("expected better citation after chunking: before=%+v after=%+v", beforeReport, afterReport)
	}
	for _, b := range after {
		if strings.Contains(b.Content, "卡控") && strings.Contains(b.Content, "发票和审批单") {
			t.Fatalf("Q2.3 still shares a chunk with Q2.2: %s", b.Content)
		}
		if strings.Contains(b.Content, "卡控") && strings.Contains(b.Content, "视觉模型识别") {
			t.Fatalf("answer chunk still contains image OCR: %s", b.Content)
		}
	}
}

func TestDOCXQuestionParagraphsDoNotMerge(t *testing.T) {
	body := `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>【Q2.2】报销材料怎么准备？</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>提交发票和审批单。</w:t></w:r></w:p>` +
		`<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>【Q2.3】物料隔离怎么处理？</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>先做先进先出卡控。</w:t></w:r></w:p>`
	p := Parse(context.Background(), "faq.docx", docxFixture(t, body))
	blocks, err := Split(p)
	if err != nil || !p.Valid() {
		t.Fatal(err, p.Diagnostics)
	}
	var materials, isolation string
	for _, b := range blocks {
		if strings.Contains(b.Content, "发票和审批单") {
			materials = b.Content
		}
		if strings.Contains(b.Content, "卡控") {
			isolation = b.Content
		}
	}
	if materials == "" || isolation == "" || strings.Contains(materials, "卡控") || strings.Contains(isolation, "发票") {
		t.Fatalf("materials=%q isolation=%q", materials, isolation)
	}
}

func mergeWithoutQuestionBoundaries(blocks []Block) []Block {
	groups := []Block{}
	for _, b := range blocks {
		n := len(groups)
		if n > 0 && b.Locator.Page == 0 && groups[n-1].Locator.Page == 0 && len([]rune(groups[n-1].Content))+len([]rune(b.Content))+2 <= chunkRunes {
			groups[n-1].Content += "\n\n" + b.Content
			groups[n-1].Locator.LineEnd = b.Locator.LineEnd
			continue
		}
		groups = append(groups, b)
	}
	return groups
}

func scoreFAQ(t *testing.T, blocks []Block) EvaluationReport {
	t.Helper()
	terms := []string{"报销", "发票", "审批", "物料", "超期", "卡控", "截图", "界面", "董事会"}
	vec := func(s string) []float64 {
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
	for i, b := range blocks {
		candidates = append(candidates, candidate{Match{Title: "faq.md", Content: b.Content, VersionID: "faq-v1", ChunkID: "faq-" + strings.TrimSpace(b.Locator.Heading), Locator: b.Locator}, vec(b.Content + " " + b.Locator.Heading)})
		_ = i
	}
	report, err := EvaluateSuite(context.Background(), func(_ context.Context, query string) ([]Match, error) {
		q := vec(query)
		out := []Match{}
		for _, c := range candidates {
			dot, a, b := 0.0, 0.0, 0.0
			for i, x := range q {
				dot += x * c.vec[i]
				a += x * x
				b += c.vec[i] * c.vec[i]
			}
			if a*b == 0 {
				continue
			}
			score := dot / math.Sqrt(a*b)
			if score >= 0.2 {
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
	}, "faq")
	if err != nil {
		t.Fatal(err)
	}
	report.Mode = "faq-fixture"
	report.Model = "deterministic-faq-terms-v1"
	report.MinScore = 0.2
	return report
}
