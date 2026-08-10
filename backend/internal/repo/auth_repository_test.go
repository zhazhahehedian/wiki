package repo_test

import (
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/auth"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	httpx "github.com/zenith-wang/it-wiki/backend/internal/http"
	"github.com/zenith-wang/it-wiki/backend/internal/repo"
)

func TestOAuthUserIDIsStableAndProviderScoped(t *testing.T) {
	first := repo.OAuthUserID("feishu", "ou_123")
	second := repo.OAuthUserID("feishu", "ou_123")
	other := repo.OAuthUserID("other", "ou_123")
	if first == "" || first != second || first == other {
		t.Fatalf("OAuthUserID() = %q, %q, other=%q", first, second, other)
	}
}

func TestAuthRepositoryProvidesProductionAuthPorts(t *testing.T) {
	repository := repo.NewAuthRepository(nil)
	var _ ports.AuthRepository = repository
	var _ auth.SessionPersistence = repository
	var _ httpx.UserResolver = repository
}
