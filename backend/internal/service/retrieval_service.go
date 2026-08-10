package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type Retrieval struct {
	embedder ports.Embedder
	vstore   ports.OwnedVectorSearch
	topK     int
	minScore float32
}

type RetrievalResult = ports.RetrievalResult

func NewRetrieval(embedder ports.Embedder, vstore ports.OwnedVectorSearch, topK int, minScore float32) *Retrieval {
	if topK < 1 {
		topK = 8
	}
	return &Retrieval{embedder: embedder, vstore: vstore, topK: topK, minScore: minScore}
}

func (s *Retrieval) Retrieve(ctx context.Context, userID, kbID, question string) (*RetrievalResult, error) {
	ownerID, err := requiredOwnerUUID(userID)
	if err != nil {
		return nil, err
	}
	kbID = strings.TrimSpace(kbID)
	question = strings.TrimSpace(question)
	if kbID == "" {
		return nil, fmt.Errorf("kb id is required")
	}
	if question == "" {
		return nil, fmt.Errorf("question is required")
	}

	embeddings, err := s.embedder.Embed(ctx, []string{question})
	if err != nil {
		return nil, fmt.Errorf("embed question: %w", err)
	}
	if len(embeddings) != 1 {
		return nil, fmt.Errorf("embed question: got %d vectors, want 1", len(embeddings))
	}
	if len(embeddings[0]) != s.embedder.Dim() {
		return nil, fmt.Errorf("query embedding dim mismatch: got %d, want %d", len(embeddings[0]), s.embedder.Dim())
	}

	rawHits, err := s.vstore.SearchForOwner(ctx, ownerID.String(), kbID, embeddings[0], ports.VectorSearchOptions{
		TopK:     s.topK,
		MinScore: s.minScore,
	})
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	hits := make([]ports.VectorSearchHit, 0, len(rawHits))
	for _, hit := range rawHits {
		if hit.KBID != kbID {
			continue
		}
		hits = append(hits, hit)
	}

	return &RetrievalResult{
		Question:      question,
		EvidenceLevel: evidenceLevel(hits, s.minScore),
		Hits:          hits,
		Citations:     BuildCitations(hits),
	}, nil
}

func evidenceLevel(hits []ports.VectorSearchHit, minScore float32) string {
	if len(hits) == 0 {
		return domain.EvidenceNone
	}
	if hits[0].Score < minScore {
		return domain.EvidenceWeak
	}
	return domain.EvidenceSufficient
}
