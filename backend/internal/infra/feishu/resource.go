package feishu

import (
	"fmt"
	"regexp"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

const unsupportedFeishuURLCode = "unsupported_feishu_url"

type ResolveErrorReason string

const (
	ResolveMalformedURL            ResolveErrorReason = "malformed_url"
	ResolveUnsupportedScheme       ResolveErrorReason = "unsupported_scheme"
	ResolveUserInfoNotAllowed      ResolveErrorReason = "userinfo_not_allowed"
	ResolvePortNotAllowed          ResolveErrorReason = "port_not_allowed"
	ResolveUnsupportedHost         ResolveErrorReason = "unsupported_host"
	ResolveUnsupportedResourceType ResolveErrorReason = "unsupported_resource_type"
	ResolveInvalidPath             ResolveErrorReason = "invalid_path"
	ResolveEncodedPathNotAllowed   ResolveErrorReason = "encoded_path_not_allowed"
	ResolveAmbiguousSelector       ResolveErrorReason = "ambiguous_selector"
	ResolveInvalidSelector         ResolveErrorReason = "invalid_selector"
)

// ResolveError is safe to map to an API error and intentionally omits the
// input URL, whose query string may contain secrets.
type ResolveError struct {
	Code   string
	Reason ResolveErrorReason
}

func (e *ResolveError) Error() string {
	return fmt.Sprintf("resolve Feishu URL: %s", e.Reason)
}

func newResolveError(reason ResolveErrorReason) *ResolveError {
	return &ResolveError{Code: unsupportedFeishuURLCode, Reason: reason}
}

var resourceIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func stableResourceIdentity(ref domain.ResourceRef) string {
	identity := fmt.Sprintf("feishu://%s/%s/%s", ref.Tenant, ref.Type, ref.Token)
	switch ref.Type {
	case domain.ResourceSheet:
		if ref.SheetID != "" {
			identity += "/sheet/" + ref.SheetID
		}
	case domain.ResourceBitable:
		if ref.TableID != "" {
			identity += "/table/" + ref.TableID
		}
	}
	return identity
}
