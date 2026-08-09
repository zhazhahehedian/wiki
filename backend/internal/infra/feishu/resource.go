package feishu

import (
	"regexp"
	"strings"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

const (
	maxHostnameBytes           = 253
	maxResourceIdentifierBytes = 256
)

var resourceIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func resolveError(reason ports.SourceResolveErrorReason) *ports.SourceResolveError {
	return ports.NewSourceResolveError(reason)
}

func providerFamily(host string) (string, bool) {
	if !isDNSName(host) {
		return "", false
	}
	for _, root := range []string{"feishu.cn", "larksuite.com"} {
		suffix := "." + root
		if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
			return root, true
		}
	}
	return "", false
}

func isDNSName(host string) bool {
	if len(host) == 0 || len(host) > maxHostnameBytes {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' || strings.HasPrefix(label, "xn--") {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}

func parseResourcePath(path string) (domain.ResourceType, string, string, error) {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] != "" {
		return "", "", "", resolveError(ports.SourceResolveInvalidPath)
	}
	var resourceType domain.ResourceType
	switch parts[1] {
	case "docx":
		resourceType = domain.ResourceDocx
	case "sheets":
		resourceType = domain.ResourceSheet
	case "base":
		resourceType = domain.ResourceBitable
	case "wiki":
		resourceType = domain.ResourceWiki
	default:
		return "", "", "", resolveError(ports.SourceResolveUnsupportedResourceType)
	}
	if len(parts) != 3 {
		return "", "", "", resolveError(ports.SourceResolveInvalidPath)
	}
	if len(parts[2]) > maxResourceIdentifierBytes {
		return "", "", "", resolveError(ports.SourceResolveIdentifierTooLong)
	}
	if !resourceIdentifierPattern.MatchString(parts[2]) {
		return "", "", "", resolveError(ports.SourceResolveInvalidPath)
	}
	return resourceType, "/" + parts[1] + "/" + parts[2], parts[2], nil
}
