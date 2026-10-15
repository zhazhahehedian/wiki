package knowledge

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed testdata/*.json testdata/*.md testdata/*.txt
var EvaluationData embed.FS

type EvaluationCase struct {
	ID       string   `json:"id"`
	Query    string   `json:"query"`
	Title    string   `json:"title"`
	Evidence []string `json:"evidence"`
	Line     int      `json:"line"`
	First    bool     `json:"first"`
	NoAnswer bool     `json:"no_answer"`
}
type EvaluationResult struct {
	ID       string   `json:"id"`
	Passed   bool     `json:"passed"`
	Matches  int      `json:"matches"`
	Versions []string `json:"versions"`
}
type EvaluationReport struct {
	Suite                 string             `json:"suite"`
	OptimizerModel        string             `json:"optimizer_model,omitempty"`
	Retrieval             []RetrievalInfo    `json:"retrieval,omitempty"`
	Mode                  string             `json:"mode"`
	Model                 string             `json:"model"`
	TopK                  int                `json:"top_k"`
	MinScore              float32            `json:"min_score"`
	Recall                float64            `json:"recall_at_5"`
	CitationAccuracy      float64            `json:"citation_accuracy"`
	NoAnswerFalsePositive float64            `json:"no_answer_false_positive_rate"`
	Passed                bool               `json:"passed"`
	Cases                 []EvaluationResult `json:"cases"`
}

func Evaluate(ctx context.Context, search func(context.Context, string) ([]Match, error)) (EvaluationReport, error) {
	return EvaluateSuite(ctx, search, "baseline")
}
func EvaluateSuite(ctx context.Context, search func(context.Context, string) ([]Match, error), suite string) (EvaluationReport, error) {
	file := "testdata/evaluation.json"
	if suite == "optimization" {
		file = "testdata/optimization-evaluation.json"
	} else if suite != "baseline" {
		return EvaluationReport{}, fmt.Errorf("unknown evaluation suite")
	}
	raw, err := EvaluationData.ReadFile(file)
	if err != nil {
		return EvaluationReport{}, err
	}
	var cases []EvaluationCase
	if e := json.Unmarshal(raw, &cases); e != nil {
		return EvaluationReport{}, e
	}
	out := EvaluationReport{Suite: suite, TopK: 5, Cases: []EvaluationResult{}, Passed: true}
	positive, hits, citations, negative, falsePositive := 0, 0, 0, 0, 0
	for _, c := range cases {
		matches, e := search(ctx, c.Query)
		if e != nil {
			return out, fmt.Errorf("evaluation case %s failed", c.ID)
		}
		result := EvaluationResult{ID: c.ID, Matches: len(matches), Versions: []string{}}
		for _, m := range matches {
			result.Versions = append(result.Versions, m.VersionID)
		}
		if c.NoAnswer {
			negative++
			result.Passed = len(matches) == 0
			if !result.Passed {
				falsePositive++
			}
		} else {
			positive++
			for i, m := range matches {
				if i >= 5 || (c.First && i > 0) {
					break
				}
				found := m.Title == c.Title
				for _, evidence := range c.Evidence {
					found = found && strings.Contains(m.Content, evidence)
				}
				if found {
					hits++
					if m.Locator.Line == c.Line && m.VersionID != "" && m.ChunkID != "" {
						citations++
						result.Passed = true
					}
					break
				}
			}
		}
		out.Passed = out.Passed && result.Passed
		out.Cases = append(out.Cases, result)
	}
	if positive > 0 {
		out.Recall = float64(hits) / float64(positive)
	}
	if hits > 0 {
		out.CitationAccuracy = float64(citations) / float64(hits)
	}
	if negative > 0 {
		out.NoAnswerFalsePositive = float64(falsePositive) / float64(negative)
	}
	out.Passed = out.Passed && out.Recall >= 0.9 && out.CitationAccuracy == 1 && out.NoAnswerFalsePositive == 0
	return out, nil
}
