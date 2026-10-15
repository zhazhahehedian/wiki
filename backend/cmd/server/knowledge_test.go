package main

import "testing"

func TestKnowledgeRejectsPartialOptimizerConfiguration(t *testing.T) {
	for _, key := range []string{"EMBEDDING_MODEL", "EMBEDDING_BASE_URL", "EMBEDDING_API_KEY", "KNOWLEDGE_MIN_SCORE", "KNOWLEDGE_OPTIMIZER_BASE_URL", "KNOWLEDGE_OPTIMIZER_API_KEY", "KNOWLEDGE_OPTIMIZER_MODEL"} {
		t.Setenv(key, "")
	}
	t.Setenv("KNOWLEDGE_OPTIMIZER_MODEL", "fixture")
	if _, _, err := buildKnowledge(nil, nil); err == nil {
		t.Fatal("partial optimizer config accepted")
	}
}

func TestKnowledgeRejectsInvalidScoreConfiguration(t *testing.T) {
	for _, key := range []string{"EMBEDDING_MODEL", "EMBEDDING_BASE_URL", "EMBEDDING_API_KEY"} {
		t.Setenv(key, "")
	}
	for _, value := range []string{"NaN", "Inf", "-0.1", "1.1", "invalid"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("KNOWLEDGE_MIN_SCORE", value)
			if _, _, e := buildKnowledge(nil, nil); e == nil {
				t.Fatal("invalid threshold accepted")
			}
		})
	}
}
