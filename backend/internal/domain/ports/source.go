package ports

import "github.com/zenith-wang/it-wiki/backend/internal/domain"

// SourceResolver turns a user-provided source URL into a stable resource
// reference without accessing the remote provider.
type SourceResolver interface {
	Resolve(rawURL string) (domain.ResourceRef, error)
}
