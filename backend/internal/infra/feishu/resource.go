package feishu

import (
	"fmt"
	"regexp"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

var resourceIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func stableResourceIdentity(ref domain.ResourceRef) string {
	identity := fmt.Sprintf("feishu://%s/%s/%s", ref.ProviderHost, ref.Type, ref.Token)
	switch ref.Type {
	case domain.ResourceSheet:
		if ref.SheetID != "" {
			identity += "/sheet/" + ref.SheetID
		}
	case domain.ResourceBitable:
		if ref.TableID != "" {
			identity += "/table/" + ref.TableID
		}
		if ref.ViewID != "" {
			identity += "/view/" + ref.ViewID
		}
	}
	return identity
}
