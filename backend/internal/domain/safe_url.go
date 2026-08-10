package domain

import (
	"bytes"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

const (
	maxSafeURLBytes      = 4096
	maxSafeSelectorBytes = 256
	maxSafeHostnameBytes = 253
)

var safeSelectorPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type SafeURL struct {
	value string
}

type SafeURLValidationError struct {
	Reason string
}

func (e *SafeURLValidationError) Error() string { return "invalid safe URL" }

func NewSafeURL(raw string) (SafeURL, error) {
	if len(raw) == 0 || len(raw) > maxSafeURLBytes || strings.TrimSpace(raw) != raw || strings.Contains(raw, `\`) {
		return SafeURL{}, safeURLError("invalid_input")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || parsed.Scheme == "" || parsed.Host == "" {
		return SafeURL{}, safeURLError("malformed_url")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return SafeURL{}, safeURLError("unsupported_scheme")
	}
	if parsed.User != nil {
		return SafeURL{}, safeURLError("userinfo_not_allowed")
	}
	if strings.Contains(parsed.Host, ":") {
		return SafeURL{}, safeURLError("port_not_allowed")
	}
	if parsed.Fragment != "" {
		return SafeURL{}, safeURLError("fragment_not_allowed")
	}
	host := strings.ToLower(parsed.Hostname())
	if !validSafeHostname(host) {
		return SafeURL{}, safeURLError("invalid_host")
	}
	lowerPath := strings.ToLower(parsed.EscapedPath())
	for _, encodedSeparator := range []string{"%2f", "%5c", "%00"} {
		if strings.Contains(lowerPath, encodedSeparator) {
			return SafeURL{}, safeURLError("invalid_path")
		}
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return SafeURL{}, safeURLError("invalid_query")
	}
	canonicalQuery := make(url.Values)
	for key, values := range query {
		if key != "table" && key != "view" && key != "sheet" {
			return SafeURL{}, safeURLError("query_key_not_allowed")
		}
		if len(values) != 1 || len(values[0]) == 0 || len(values[0]) > maxSafeSelectorBytes || !safeSelectorPattern.MatchString(values[0]) {
			return SafeURL{}, safeURLError("invalid_query_value")
		}
		canonicalQuery.Set(key, values[0])
	}
	parsed.Host = host
	parsed.RawQuery = canonicalQuery.Encode()
	parsed.ForceQuery = false
	parsed.RawFragment = ""
	return SafeURL{value: parsed.String()}, nil
}

func (u SafeURL) String() string { return u.value }

func (u SafeURL) IsZero() bool { return u.value == "" }

func (u SafeURL) MarshalJSON() ([]byte, error) {
	if u.IsZero() {
		return []byte("null"), nil
	}
	validated, err := NewSafeURL(u.value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(validated.value)
}

func (u *SafeURL) UnmarshalJSON(data []byte) error {
	if u == nil {
		return safeURLError("nil_destination")
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		*u = SafeURL{}
		return nil
	}
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return safeURLError("invalid_json")
	}
	validated, err := NewSafeURL(raw)
	if err != nil {
		return err
	}
	*u = validated
	return nil
}

func safeURLError(reason string) *SafeURLValidationError {
	return &SafeURLValidationError{Reason: reason}
}

func validSafeHostname(host string) bool {
	if len(host) == 0 || len(host) > maxSafeHostnameBytes {
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
