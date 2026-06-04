package service

import (
	"fmt"
	"strings"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

func BuildRAGMessages(retrieval *RetrievalResult, history []*domain.ChatMessage, question string) []ports.Message {
	system := strings.Join([]string{
		"You are a deterministic knowledge base assistant.",
		"Answer using the provided context when possible.",
		"Do not invent facts that are absent from the context.",
		"If evidence_level is none, say the KB does not contain enough information.",
		"Prefer concise answers.",
		"When using a cited chunk, include inline markers like [1] where natural.",
	}, "\n")

	msgs := []ports.Message{
		{Role: "system", Content: system},
		{Role: "system", Content: buildContextBlock(retrieval)},
	}
	for _, m := range history {
		if m.Role == domain.RoleUser || m.Role == domain.RoleAssistant {
			msgs = append(msgs, ports.Message{Role: m.Role, Content: m.Content})
		}
	}
	msgs = append(msgs, ports.Message{Role: domain.RoleUser, Content: question})
	return msgs
}

func buildContextBlock(retrieval *RetrievalResult) string {
	if retrieval == nil || len(retrieval.Hits) == 0 {
		return "evidence_level: none\ncontext:\nNo retrieved chunks were found."
	}

	var b strings.Builder
	fmt.Fprintf(&b, "evidence_level: %s\ncontext:\n", retrieval.EvidenceLevel)
	for i, hit := range retrieval.Hits {
		fmt.Fprintf(&b, "[%d]\nchunk_id: %s\ndocument_id: %s\ndocument_title: %s\nseq: %d\nscore: %.4f\ncontent:\n%s\n\n",
			i+1, hit.ChunkID, hit.DocumentID, hit.DocumentTitle, hit.Seq, hit.Score, hit.Content)
	}
	return b.String()
}
