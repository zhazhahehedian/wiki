package domain

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

type ResourceType string

const (
	ResourceDocx    ResourceType = "docx"
	ResourceSheet   ResourceType = "sheet"
	ResourceBitable ResourceType = "bitable"
	ResourceWiki    ResourceType = "wiki"

	RedactedMetadataValue = "[REDACTED]"
	maxMetadataKeyBytes   = 64
	maxMetadataValueBytes = 2048
)

type ResourceRef struct {
	Type         ResourceType `json:"type"`
	ProviderHost string       `json:"provider_host"`
	Tenant       string       `json:"tenant,omitempty"`
	Token        string       `json:"token"`
	TableID      string       `json:"table_id,omitempty"`
	ViewID       string       `json:"view_id,omitempty"`
	SheetID      string       `json:"sheet_id,omitempty"`
	CanonicalURL string       `json:"canonical_url"`
	Identity     string       `json:"identity"`
}

type SourceMetadata struct {
	values map[string]string
}

type SourceMetadataValidationError struct {
	Reason string
}

func (e *SourceMetadataValidationError) Error() string {
	return "invalid source metadata: " + e.Reason
}

var metadataKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func NewSourceMetadata(input map[string]string) (SourceMetadata, error) {
	values := make(map[string]string, len(input))
	for key, value := range input {
		if !utf8.ValidString(key) || len(key) == 0 || len(key) > maxMetadataKeyBytes || !metadataKeyPattern.MatchString(key) {
			return SourceMetadata{}, &SourceMetadataValidationError{Reason: "invalid key"}
		}
		if isSensitiveMetadataKey(key) {
			continue
		}
		if !utf8.ValidString(value) || len(value) > maxMetadataValueBytes {
			return SourceMetadata{}, &SourceMetadataValidationError{Reason: "invalid value"}
		}
		values[key] = sanitizeMetadataValue(value)
	}
	return SourceMetadata{values: values}, nil
}

func (m SourceMetadata) Values() map[string]string {
	values := make(map[string]string, len(m.values))
	for key, value := range m.values {
		values[key] = value
	}
	return values
}

func (m SourceMetadata) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.Values())
}

func isSensitiveMetadataKey(key string) bool {
	normalized := strings.ToLower(key)
	for _, marker := range []string{"authorization", "cookie", "password", "secret", "signature", "token", "oauth", "code"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func sanitizeMetadataValue(value string) string {
	if parsed, err := url.Parse(value); err == nil && parsed.Host != "" && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")) {
		parsed.Scheme = strings.ToLower(parsed.Scheme)
		parsed.Host = strings.ToLower(parsed.Host)
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		parsed.Fragment = ""
		return parsed.String()
	}
	normalized := strings.ToLower(value)
	for _, marker := range []string{"authorization:", "bearer ", "access_token=", "refresh_token=", "client_secret=", "api_key=", "password=", "cookie=", "signature=", "token=", "secret=", "code="} {
		if strings.Contains(normalized, marker) {
			return RedactedMetadataValue
		}
	}
	return value
}

type CanonicalDocument struct {
	Title          string         `json:"title"`
	Markdown       string         `json:"markdown"`
	RemoteRevision string         `json:"remote_revision"`
	SourceMetadata SourceMetadata `json:"source_metadata"`
	SafeSourceURL  string         `json:"safe_source_url"`
}
