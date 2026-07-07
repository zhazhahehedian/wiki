package eval

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Case 是一条检索评测用例（docs/eval/retrieval-eval.yaml 中的一项）。
type Case struct {
	ID               string   `yaml:"id"`
	Question         string   `yaml:"question"`
	ExpectedDocument string   `yaml:"expected_document"`
	ExpectedKeywords []string `yaml:"expected_keywords"`
}

func LoadCases(path string) ([]Case, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read eval set: %w", err)
	}
	var cases []Case
	if err := yaml.Unmarshal(buf, &cases); err != nil {
		return nil, fmt.Errorf("parse eval set: %w", err)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("eval set is empty: %s", path)
	}
	for i := range cases {
		if cases[i].ID == "" {
			cases[i].ID = fmt.Sprintf("q%03d", i+1)
		}
		if strings.TrimSpace(cases[i].Question) == "" {
			return nil, fmt.Errorf("case %s: question is required", cases[i].ID)
		}
		if strings.TrimSpace(cases[i].ExpectedDocument) == "" {
			return nil, fmt.Errorf("case %s: expected_document is required", cases[i].ID)
		}
	}
	return cases, nil
}
