package feishu

import (
	"net/url"
	"strings"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type URLResolver struct{}

func NewURLResolver() *URLResolver {
	return &URLResolver{}
}

func (r *URLResolver) Resolve(rawURL string) (domain.ResourceRef, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" {
		return domain.ResourceRef{}, newResolveError(ResolveMalformedURL)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return domain.ResourceRef{}, newResolveError(ResolveUnsupportedScheme)
	}
	if parsed.Opaque != "" || parsed.Host == "" {
		return domain.ResourceRef{}, newResolveError(ResolveMalformedURL)
	}
	if parsed.User != nil {
		return domain.ResourceRef{}, newResolveError(ResolveUserInfoNotAllowed)
	}
	if parsed.Port() != "" {
		return domain.ResourceRef{}, newResolveError(ResolvePortNotAllowed)
	}

	host := strings.ToLower(parsed.Hostname())
	if !isEnterpriseHost(host) {
		return domain.ResourceRef{}, newResolveError(ResolveUnsupportedHost)
	}
	if parsed.RawPath != "" || strings.Contains(parsed.EscapedPath(), "%") {
		return domain.ResourceRef{}, newResolveError(ResolveEncodedPathNotAllowed)
	}

	resourceType, canonicalPath, token, err := parseResourcePath(parsed.Path)
	if err != nil {
		return domain.ResourceRef{}, err
	}
	query, queryErr := url.ParseQuery(parsed.RawQuery)
	if queryErr != nil {
		return domain.ResourceRef{}, newResolveError(ResolveInvalidSelector)
	}

	ref := domain.ResourceRef{
		Type:   resourceType,
		Tenant: host,
		Token:  token,
	}
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
	ref.OriginalURL = safeURL.String()
	ref.Identity = stableResourceIdentity(ref)
	return ref, nil
}

func isEnterpriseHost(host string) bool {
	if !isDNSName(host) {
		return false
	}
	for _, root := range []string{"feishu.cn", "larksuite.com"} {
		suffix := "." + root
		if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
			return true
		}
	}
	return false
}

func isDNSName(host string) bool {
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
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
	if strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] != "" {
		return "", "", "", newResolveError(ResolveInvalidPath)
	}

	var resourceType domain.ResourceType
	if len(parts) >= 2 {
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
			return "", "", "", newResolveError(ResolveUnsupportedResourceType)
		}
	}
	if len(parts) != 3 || !resourceIdentifierPattern.MatchString(parts[2]) {
		return "", "", "", newResolveError(ResolveInvalidPath)
	}
	return resourceType, "/" + parts[1] + "/" + parts[2], parts[2], nil
}

func selector(query url.Values, key string) (string, error) {
	values, ok := query[key]
	if !ok {
		return "", nil
	}
	if len(values) != 1 {
		return "", newResolveError(ResolveAmbiguousSelector)
	}
	if !resourceIdentifierPattern.MatchString(values[0]) {
		return "", newResolveError(ResolveInvalidSelector)
	}
	return values[0], nil
}

var _ ports.SourceResolver = (*URLResolver)(nil)
