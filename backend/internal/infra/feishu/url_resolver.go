package feishu

import (
	"net/url"
	"strings"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

const maxSourceURLBytes = 4096

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

	refInput := domain.ResourceRefInput{Type: resourceType, ProviderHost: providerHost, Token: token}
	safeQuery := make(url.Values)
	switch resourceType {
	case domain.ResourceSheet:
		refInput.SheetID, err = selector(query, "sheet")
		if err != nil {
			return domain.ResourceRef{}, err
		}
		if refInput.SheetID != "" {
			safeQuery.Set("sheet", refInput.SheetID)
		}
	case domain.ResourceBitable:
		refInput.TableID, err = selector(query, "table")
		if err != nil {
			return domain.ResourceRef{}, err
		}
		refInput.ViewID, err = selector(query, "view")
		if err != nil {
			return domain.ResourceRef{}, err
		}
		if refInput.TableID != "" {
			safeQuery.Set("table", refInput.TableID)
		}
		if refInput.ViewID != "" {
			safeQuery.Set("view", refInput.ViewID)
		}
	}

	safeURL := url.URL{Scheme: "https", Host: host, Path: canonicalPath, RawQuery: safeQuery.Encode()}
	refInput.CanonicalURL, err = domain.NewSafeURL(safeURL.String())
	if err != nil {
		return domain.ResourceRef{}, resolveError(ports.SourceResolveMalformedURL)
	}
	refInput.OriginalURL = refInput.CanonicalURL
	ref, err := domain.NewResourceRef(refInput)
	if err != nil {
		return domain.ResourceRef{}, resolveError(ports.SourceResolveMalformedURL)
	}
	return ref, nil
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
