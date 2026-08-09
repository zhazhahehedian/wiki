package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

type ResourceRef struct {
	Type         ResourceType `json:"type"`
	ProviderHost string       `json:"provider_host"`
	Tenant       string       `json:"tenant,omitempty"`
	Token        string       `json:"token"`
	TableID      string       `json:"table_id,omitempty"`
	ViewID       string       `json:"view_id,omitempty"`
	SheetID      string       `json:"sheet_id,omitempty"`
	CanonicalURL SafeURL      `json:"canonical_url"`
	OriginalURL  SafeURL      `json:"original_url"`
	Identity     string       `json:"identity"`
}

type ResourceRefInput struct {
	Type         ResourceType
	ProviderHost string
	Tenant       string
	Token        string
	TableID      string
	ViewID       string
	SheetID      string
	CanonicalURL SafeURL
	OriginalURL  SafeURL
}

type ResourceRefValidationError struct {
	Reason string
}

func (e *ResourceRefValidationError) Error() string { return "invalid resource reference" }

func NewResourceRef(input ResourceRefInput) (ResourceRef, error) {
	ref := ResourceRef{
		Type:         input.Type,
		ProviderHost: input.ProviderHost,
		Tenant:       input.Tenant,
		Token:        input.Token,
		TableID:      input.TableID,
		ViewID:       input.ViewID,
		SheetID:      input.SheetID,
		CanonicalURL: input.CanonicalURL,
		OriginalURL:  input.OriginalURL,
	}
	ref.Identity = stableResourceIdentity(ref)
	if err := validateResourceRef(ref); err != nil {
		return ResourceRef{}, err
	}
	return ref, nil
}

func (r ResourceRef) MarshalJSON() ([]byte, error) {
	if err := validateResourceRef(r); err != nil {
		return nil, err
	}
	type persistedResourceRef ResourceRef
	return json.Marshal(persistedResourceRef(r))
}

func (r *ResourceRef) UnmarshalJSON(data []byte) error {
	if r == nil {
		return resourceRefError("nil_destination")
	}
	type persistedResourceRef ResourceRef
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var persisted persistedResourceRef
	if err := decoder.Decode(&persisted); err != nil {
		return resourceRefError("invalid_json")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return resourceRefError("invalid_json")
	}
	input := ResourceRefInput{
		Type:         persisted.Type,
		ProviderHost: persisted.ProviderHost,
		Tenant:       persisted.Tenant,
		Token:        persisted.Token,
		TableID:      persisted.TableID,
		ViewID:       persisted.ViewID,
		SheetID:      persisted.SheetID,
		CanonicalURL: persisted.CanonicalURL,
		OriginalURL:  persisted.OriginalURL,
	}
	validated, err := NewResourceRef(input)
	if err != nil {
		return err
	}
	if persisted.Identity != validated.Identity {
		return resourceRefError("invalid_identity")
	}
	*r = validated
	return nil
}

func validateResourceRef(ref ResourceRef) error {
	if !validResourceType(ref.Type) {
		return resourceRefError("invalid_type")
	}
	if ref.ProviderHost != "feishu.cn" && ref.ProviderHost != "larksuite.com" {
		return resourceRefError("invalid_provider_host")
	}
	if !validRequiredResourceIdentifier(ref.Token) {
		return resourceRefError("invalid_token")
	}
	if !validBoundedText(ref.Tenant, maxSourceIdentifierBytes) {
		return resourceRefError("invalid_tenant")
	}
	for _, identifier := range []string{ref.TableID, ref.ViewID, ref.SheetID} {
		if identifier != "" && !validRequiredResourceIdentifier(identifier) {
			return resourceRefError("invalid_selector")
		}
	}
	switch ref.Type {
	case ResourceDocx, ResourceWiki:
		if ref.TableID != "" || ref.ViewID != "" || ref.SheetID != "" {
			return resourceRefError("selector_not_allowed")
		}
	case ResourceSheet:
		if ref.TableID != "" || ref.ViewID != "" {
			return resourceRefError("selector_not_allowed")
		}
	case ResourceBitable:
		if ref.SheetID != "" {
			return resourceRefError("selector_not_allowed")
		}
	}
	if !resourceURLMatches(ref.CanonicalURL, ref) || !resourceURLMatches(ref.OriginalURL, ref) {
		return resourceRefError("invalid_url")
	}
	if ref.Identity != stableResourceIdentity(ref) {
		return resourceRefError("invalid_identity")
	}
	return nil
}

func resourceURLMatches(value SafeURL, ref ResourceRef) bool {
	if value.IsZero() {
		return false
	}
	validated, err := NewSafeURL(value.String())
	if err != nil || validated != value {
		return false
	}
	parsed, err := url.Parse(value.String())
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	providerHost, ok := resourceProviderFamily(parsed.Hostname())
	if !ok || providerHost != ref.ProviderHost {
		return false
	}
	var path string
	switch ref.Type {
	case ResourceDocx:
		path = "/docx/" + ref.Token
	case ResourceSheet:
		path = "/sheets/" + ref.Token
	case ResourceBitable:
		path = "/base/" + ref.Token
	case ResourceWiki:
		path = "/wiki/" + ref.Token
	default:
		return false
	}
	wantQuery := make(url.Values)
	if ref.SheetID != "" {
		wantQuery.Set("sheet", ref.SheetID)
	}
	if ref.TableID != "" {
		wantQuery.Set("table", ref.TableID)
	}
	if ref.ViewID != "" {
		wantQuery.Set("view", ref.ViewID)
	}
	return parsed.Path == path && parsed.RawQuery == wantQuery.Encode()
}

func resourceProviderFamily(host string) (string, bool) {
	host = strings.ToLower(host)
	for _, root := range []string{"feishu.cn", "larksuite.com"} {
		suffix := "." + root
		if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
			return root, true
		}
	}
	return "", false
}

func validRequiredResourceIdentifier(value string) bool {
	return len(value) > 0 && len(value) <= maxSourceIdentifierBytes && sourceIdentifierPattern.MatchString(value)
}

func stableResourceIdentity(ref ResourceRef) string {
	identity := fmt.Sprintf("feishu://%s/%s/%s", ref.ProviderHost, ref.Type, ref.Token)
	switch ref.Type {
	case ResourceSheet:
		if ref.SheetID != "" {
			identity += "/sheet/" + ref.SheetID
		}
	case ResourceBitable:
		if ref.TableID != "" {
			identity += "/table/" + ref.TableID
		}
		if ref.ViewID != "" {
			identity += "/view/" + ref.ViewID
		}
	}
	return identity
}

func resourceRefError(reason string) *ResourceRefValidationError {
	return &ResourceRefValidationError{Reason: reason}
}
