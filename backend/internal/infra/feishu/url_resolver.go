package feishu

import (
	"net/url"
	"strings"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

const (
	maxSourceURLBytes          = 4096
	maxHostnameBytes           = 253
	maxResourceIdentifierBytes = 256
)

type URLResolver struct{}

func NewURLResolver() *URLResolver { return &URLResolver{} }

func (r *URLResolver) Resolve(rawURL string) (domain.ResourceRef, error) {
	if len(rawURL) > maxSourceURLBytes {
		return domain.ResourceRef{}, resolveError(ports.SourceResolveInputTooLong)
	}
	rawURL = strings.TrimSpace(rawURL)
	if strings.Contains(rawURL, `\`) {
		return domain.ResourceRef{}, resolveError(ports.SourceResolveAuthorityConfusion)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" {
		return domain.ResourceRef{}, resolveError(ports.SourceResolveMalformedURL)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return domain.ResourceRef{}, resolveError(ports.SourceResolveUnsupportedScheme)
	}
	if parsed.Opaque != "" || parsed.Host == "" {
		return domain.ResourceRef{}, resolveError(ports.SourceResolveMalformedURL)
	}
	if parsed.User != nil {
		return domain.ResourceRef{}, resolveError(ports.SourceResolveUserInfoNotAllowed)
	}
	if strings.Contains(parsed.Host, ":") {
		return domain.ResourceRef{}, resolveError(ports.SourceResolvePortNotAllowed)
	}

	host := strings.ToLower(parsed.Hostname())
	providerHost, ok := providerFamily(host)
	if !ok {
		return domain.ResourceRef{}, resolveError(ports.SourceResolveUnsupportedHost)
	}
	if parsed.RawPath != "" || strings.Contains(parsed.EscapedPath(), "%") {
		return domain.ResourceRef{}, resolveError(ports.SourceResolveEncodedPathNotAllowed)
	}

	resourceType, canonicalPath, token, err := parseResourcePath(parsed.Path)
	if err != nil {
		return domain.ResourceRef{}, err
	}
	query, queryErr := url.ParseQuery(parsed.RawQuery)
	if queryErr != nil {
		return domain.ResourceRef{}, resolveError(ports.SourceResolveInvalidSelector)
	}

	ref := domain.ResourceRef{Type: resourceType, ProviderHost: providerHost, Token: token}
	safeQuery := make(url.Values)
	switch resourceType {
	case domain.ResourceSheet:
		ref.SheetID, err = selector(query, "sheet")
		if err != nil {
			return domain.ResourceRef{}, err
		}
		if ref.SheetID != "" {
			safeQuery.Set("sheet", ref.SheetID)
		}
	case domain.ResourceBitable:
		ref.TableID, err = selector(query, "table")
		if err != nil {
			return domain.ResourceRef{}, err
		}
		ref.ViewID, err = selector(query, "view")
		if err != nil {
			return domain.ResourceRef{}, err
		}
		if ref.TableID != "" {
			safeQuery.Set("table", ref.TableID)
		}
		if ref.ViewID != "" {
			safeQuery.Set("view", ref.ViewID)
		}
	}

	safeURL := url.URL{Scheme: "https", Host: host, Path: canonicalPath, RawQuery: safeQuery.Encode()}
	ref.CanonicalURL = safeURL.String()
	ref.Identity = stableResourceIdentity(ref)
	return ref, nil
}

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

func selector(query url.Values, key string) (string, error) {
	values, ok := query[key]
	if !ok {
		return "", nil
	}
	if len(values) != 1 {
		return "", resolveError(ports.SourceResolveAmbiguousSelector)
	}
	if len(values[0]) > maxResourceIdentifierBytes {
		return "", resolveError(ports.SourceResolveIdentifierTooLong)
	}
	if !resourceIdentifierPattern.MatchString(values[0]) {
		return "", resolveError(ports.SourceResolveInvalidSelector)
	}
	return values[0], nil
}

var _ ports.SourceResolver = (*URLResolver)(nil)
