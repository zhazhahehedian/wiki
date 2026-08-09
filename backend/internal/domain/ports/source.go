package ports

import (
	"context"
	"errors"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

const ErrorCodeUnsupportedFeishuURL = "unsupported_feishu_url"

var ErrUnsupportedFeishuURL = errors.New("unsupported Feishu URL")

type SourceResolveErrorReason string

const (
	SourceResolveMalformedURL            SourceResolveErrorReason = "malformed_url"
	SourceResolveUnsupportedScheme       SourceResolveErrorReason = "unsupported_scheme"
	SourceResolveUserInfoNotAllowed      SourceResolveErrorReason = "userinfo_not_allowed"
	SourceResolvePortNotAllowed          SourceResolveErrorReason = "port_not_allowed"
	SourceResolveUnsupportedHost         SourceResolveErrorReason = "unsupported_host"
	SourceResolveAuthorityConfusion      SourceResolveErrorReason = "authority_confusion"
	SourceResolveUnsupportedResourceType SourceResolveErrorReason = "unsupported_resource_type"
	SourceResolveInvalidPath             SourceResolveErrorReason = "invalid_path"
	SourceResolveEncodedPathNotAllowed   SourceResolveErrorReason = "encoded_path_not_allowed"
	SourceResolveAmbiguousSelector       SourceResolveErrorReason = "ambiguous_selector"
	SourceResolveInvalidSelector         SourceResolveErrorReason = "invalid_selector"
	SourceResolveInputTooLong            SourceResolveErrorReason = "input_too_long"
	SourceResolveIdentifierTooLong       SourceResolveErrorReason = "identifier_too_long"
)

type SourceResolveError struct {
	Code   string
	Reason SourceResolveErrorReason
}

func NewSourceResolveError(reason SourceResolveErrorReason) *SourceResolveError {
	return &SourceResolveError{Code: ErrorCodeUnsupportedFeishuURL, Reason: reason}
}

func (e *SourceResolveError) Error() string {
	return ErrUnsupportedFeishuURL.Error()
}

func (e *SourceResolveError) Unwrap() error {
	return ErrUnsupportedFeishuURL
}

type SourceResolver interface {
	Resolve(rawURL string) (domain.ResourceRef, error)
}

type SourceLoadErrorCode string

const (
	SourceLoadForbidden   SourceLoadErrorCode = "resource_forbidden"
	SourceLoadNotFound    SourceLoadErrorCode = "resource_not_found"
	SourceLoadRateLimit   SourceLoadErrorCode = "rate_limited"
	SourceLoadAPIError    SourceLoadErrorCode = "api_error"
	SourceLoadTooLarge    SourceLoadErrorCode = "too_large"
	SourceLoadMalformed   SourceLoadErrorCode = "malformed"
	SourceLoadUnsupported SourceLoadErrorCode = "unsupported_resource"
)

type SourceLoadError struct {
	Code  SourceLoadErrorCode `json:"code"`
	cause error
}

func NewSourceLoadError(code SourceLoadErrorCode, cause error) *SourceLoadError {
	var safeCause error
	switch {
	case errors.Is(cause, context.Canceled):
		safeCause = context.Canceled
	case errors.Is(cause, context.DeadlineExceeded):
		safeCause = context.DeadlineExceeded
	}
	return &SourceLoadError{Code: code, cause: safeCause}
}

func (e *SourceLoadError) Error() string { return "Feishu source load failed" }

func (e *SourceLoadError) Unwrap() error { return e.cause }

type SourceLoader interface {
	Load(ctx context.Context, ref domain.ResourceRef, accessToken string) (domain.CanonicalDocument, error)
}
