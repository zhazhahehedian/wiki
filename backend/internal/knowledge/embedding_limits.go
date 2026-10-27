package knowledge

import "github.com/zenith-wang/it-wiki/backend/internal/domain/ports"

func (s *Service) embeddingInputLimit() int {
	if limited, ok := s.embed.(ports.EmbeddingInputLimiter); ok {
		return limited.MaxInputBytes()
	}
	return 0
}
