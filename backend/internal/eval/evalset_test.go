package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "eval.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp yaml: %v", err)
	}
	return path
}

func TestLoadCasesParsesYAMLAndFillsDefaultIDs(t *testing.T) {
	path := writeTempYAML(t, `
- question: "生产环境如何配置数据库连接？"
  expected_document: "部署指南.md"
  expected_keywords: ["DATABASE_URL"]
- id: custom-id
  question: "用什么命令跑迁移？"
  expected_document: "部署指南.md"
  expected_keywords: ["goose"]
`)
	cases, err := LoadCases(path)
	if err != nil {
		t.Fatalf("LoadCases() error = %v", err)
	}
	if len(cases) != 2 {
		t.Fatalf("len(cases) = %d, want 2", len(cases))
	}
	if cases[0].ID != "q001" {
		t.Errorf("cases[0].ID = %q, want auto-filled q001", cases[0].ID)
	}
	if cases[1].ID != "custom-id" {
		t.Errorf("cases[1].ID = %q, want custom-id", cases[1].ID)
	}
	if cases[0].ExpectedKeywords[0] != "DATABASE_URL" {
		t.Errorf("keywords = %#v", cases[0].ExpectedKeywords)
	}
}

func TestLoadCasesRejectsMissingQuestionOrDocument(t *testing.T) {
	path := writeTempYAML(t, `
- question: ""
  expected_document: "部署指南.md"
`)
	if _, err := LoadCases(path); err == nil || !strings.Contains(err.Error(), "question is required") {
		t.Fatalf("LoadCases() error = %v, want question is required", err)
	}

	path = writeTempYAML(t, `
- question: "有问题没文档"
`)
	if _, err := LoadCases(path); err == nil || !strings.Contains(err.Error(), "expected_document is required") {
		t.Fatalf("LoadCases() error = %v, want expected_document is required", err)
	}
}

func TestLoadCasesRejectsEmptySet(t *testing.T) {
	path := writeTempYAML(t, "")
	if _, err := LoadCases(path); err == nil {
		t.Fatal("LoadCases() on empty set should error")
	}
}
