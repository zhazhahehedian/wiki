package service

import (
	"strconv"
	"strings"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

const citationSnippetRunes = 360

func BuildCitations(hits []ports.VectorSearchHit) []domain.Citation {
	citations := make([]domain.Citation, 0, len(hits))
	for i, hit := range hits {
		citations = append(citations, domain.Citation{
			ID:            "c" + strconv.Itoa(i+1),
			ChunkID:       hit.ChunkID,
			DocumentID:    hit.DocumentID,
			DocumentTitle: hit.DocumentTitle,
			Seq:           hit.Seq,
			Score:         hit.Score,
			Snippet:       trimSnippet(hit.Content, citationSnippetRunes),
		})
	}
	return citations
}

func trimSnippet(text string, maxRunes int) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if maxRunes < 4 {
		maxRunes = 4
	}
	runes := []rune(normalized)
	if len(runes) <= maxRunes {
		return normalized
	}
	return string(runes[:maxRunes-3]) + "..."
}
