package domain

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"unicode/utf8"
)

type ResourceType string

const (
	ResourceDocx    ResourceType = "docx"
	ResourceSheet   ResourceType = "sheet"
	ResourceBitable ResourceType = "bitable"
	ResourceWiki    ResourceType = "wiki"

	maxSectionPathBytes      = 2048
	maxSheetNameBytes        = 512
	maxSourceIdentifierBytes = 256
	maxRemoteRevisionBytes   = 256
	maxSourceRow             = 10_000_000
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
	Identity     string       `json:"identity"`
}

type SourceMetadataInput struct {
	SourceType     ResourceType `json:"source_type"`
	SectionPath    string       `json:"section_path,omitempty"`
	SheetName      string       `json:"sheet_name,omitempty"`
	TableID        string       `json:"table_id,omitempty"`
	ViewID         string       `json:"view_id,omitempty"`
	SheetID        string       `json:"sheet_id,omitempty"`
	RowStart       int          `json:"row_start,omitempty"`
	RowEnd         int          `json:"row_end,omitempty"`
	RemoteRevision string       `json:"remote_revision,omitempty"`
	ImageURL       SafeURL      `json:"image_url"`
	SourceLocator  SafeURL      `json:"source_locator"`
}

type SourceMetadata struct {
	values SourceMetadataInput
}

type SourceMetadataValidationError struct {
	Reason string
}

func (e *SourceMetadataValidationError) Error() string { return "invalid source metadata" }

var sourceIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func NewSourceMetadata(input SourceMetadataInput) (SourceMetadata, error) {
	if !validResourceType(input.SourceType) {
		return SourceMetadata{}, sourceMetadataError("invalid_source_type")
	}
	if !validBoundedText(input.SectionPath, maxSectionPathBytes) || !validBoundedText(input.SheetName, maxSheetNameBytes) || !validBoundedText(input.RemoteRevision, maxRemoteRevisionBytes) {
		return SourceMetadata{}, sourceMetadataError("invalid_text")
	}
	for _, identifier := range []string{input.TableID, input.ViewID, input.SheetID} {
		if identifier != "" && (len(identifier) > maxSourceIdentifierBytes || !sourceIdentifierPattern.MatchString(identifier)) {
			return SourceMetadata{}, sourceMetadataError("invalid_identifier")
		}
	}
	if (input.RowStart == 0) != (input.RowEnd == 0) || input.RowStart < 0 || input.RowEnd < 0 || input.RowStart > input.RowEnd || input.RowEnd > maxSourceRow {
		return SourceMetadata{}, sourceMetadataError("invalid_row_range")
	}
	return SourceMetadata{values: input}, nil
}

func (m SourceMetadata) Values() SourceMetadataInput { return m.values }

func (m SourceMetadata) MarshalJSON() ([]byte, error) {
	validated, err := NewSourceMetadata(m.values)
	if err != nil {
		return nil, err
	}
	return json.Marshal(validated.values)
}

func (m *SourceMetadata) UnmarshalJSON(data []byte) error {
	if m == nil {
		return sourceMetadataError("nil_destination")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var input SourceMetadataInput
	if err := decoder.Decode(&input); err != nil {
		return sourceMetadataError("invalid_json")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return sourceMetadataError("invalid_json")
	}
	validated, err := NewSourceMetadata(input)
	if err != nil {
		return err
	}
	*m = validated
	return nil
}

func sourceMetadataError(reason string) *SourceMetadataValidationError {
	return &SourceMetadataValidationError{Reason: reason}
}

func validResourceType(resourceType ResourceType) bool {
	switch resourceType {
	case ResourceDocx, ResourceSheet, ResourceBitable, ResourceWiki:
		return true
	default:
		return false
	}
}

func validBoundedText(value string, maxBytes int) bool {
	return utf8.ValidString(value) && len(value) <= maxBytes
}

type CanonicalDocument struct {
	Title          string         `json:"title"`
	Markdown       string         `json:"markdown"`
	RemoteRevision string         `json:"remote_revision"`
	SourceMetadata SourceMetadata `json:"source_metadata"`
	SafeSourceURL  SafeURL        `json:"safe_source_url"`
}
