package domain

import (
	"bytes"
	"encoding/json"
	"io"
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

	maxSectionPathBytes      = 2048
	maxSheetNameBytes        = 512
	maxSourceIdentifierBytes = 256
	maxRemoteRevisionBytes   = 256
	maxCanonicalTitleBytes   = 512
	maxSourceRow             = 10_000_000
	maxSourceLocations       = 1_000
)

type SourceLocation struct {
	SectionPath string `json:"section_path,omitempty"`
	SheetName   string `json:"sheet_name,omitempty"`
	TableID     string `json:"table_id,omitempty"`
	ViewID      string `json:"view_id,omitempty"`
	SheetID     string `json:"sheet_id,omitempty"`
	RowStart    int    `json:"row_start,omitempty"`
	RowEnd      int    `json:"row_end,omitempty"`
}

type SourceMetadataInput struct {
	SourceType     ResourceType     `json:"source_type"`
	SectionPath    string           `json:"section_path,omitempty"`
	SheetName      string           `json:"sheet_name,omitempty"`
	TableID        string           `json:"table_id,omitempty"`
	ViewID         string           `json:"view_id,omitempty"`
	SheetID        string           `json:"sheet_id,omitempty"`
	RowStart       int              `json:"row_start,omitempty"`
	RowEnd         int              `json:"row_end,omitempty"`
	RemoteRevision string           `json:"remote_revision,omitempty"`
	ImageURL       SafeURL          `json:"image_url"`
	SourceLocator  SafeURL          `json:"source_locator"`
	Locations      []SourceLocation `json:"locations,omitempty"`
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
	if err := validateSourceCompatibility(input.SourceType, input.SheetName, input.TableID, input.ViewID, input.SheetID); err != nil {
		return SourceMetadata{}, err
	}
	if len(input.Locations) > maxSourceLocations {
		return SourceMetadata{}, sourceMetadataError("too_many_locations")
	}
	for _, location := range input.Locations {
		if err := validateSourceLocation(location); err != nil {
			return SourceMetadata{}, err
		}
		if err := validateSourceCompatibility(input.SourceType, location.SheetName, location.TableID, location.ViewID, location.SheetID); err != nil {
			return SourceMetadata{}, err
		}
	}
	if input.SourceType == ResourceWiki {
		hasSheet := input.SheetName != "" || input.SheetID != ""
		hasTable := input.TableID != "" || input.ViewID != ""
		for _, location := range input.Locations {
			hasSheet = hasSheet || location.SheetName != "" || location.SheetID != ""
			hasTable = hasTable || location.TableID != "" || location.ViewID != ""
		}
		if hasSheet && hasTable {
			return SourceMetadata{}, sourceMetadataError("mixed_wiki_source_fields")
		}
	}
	input.Locations = cloneSourceLocations(input.Locations)
	return SourceMetadata{values: input}, nil
}

func validateSourceCompatibility(sourceType ResourceType, sheetName, tableID, viewID, sheetID string) error {
	if viewID != "" && tableID == "" {
		return sourceMetadataError("view_without_table")
	}
	switch sourceType {
	case ResourceSheet:
		if tableID != "" || viewID != "" {
			return sourceMetadataError("incompatible_source_fields")
		}
	case ResourceBitable:
		if sheetName != "" || sheetID != "" {
			return sourceMetadataError("incompatible_source_fields")
		}
	case ResourceDocx:
		if sheetName != "" || sheetID != "" || tableID != "" || viewID != "" {
			return sourceMetadataError("incompatible_source_fields")
		}
	case ResourceWiki:
		if (sheetName != "" || sheetID != "") && (tableID != "" || viewID != "") {
			return sourceMetadataError("incompatible_source_fields")
		}
	}
	return nil
}

func (m SourceMetadata) Values() SourceMetadataInput {
	values := m.values
	values.Locations = cloneSourceLocations(values.Locations)
	return values
}

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

func validateSourceLocation(location SourceLocation) error {
	if !validBoundedText(location.SectionPath, maxSectionPathBytes) || !validBoundedText(location.SheetName, maxSheetNameBytes) {
		return sourceMetadataError("invalid_location_text")
	}
	for _, identifier := range []string{location.TableID, location.ViewID, location.SheetID} {
		if identifier != "" && (len(identifier) > maxSourceIdentifierBytes || !sourceIdentifierPattern.MatchString(identifier)) {
			return sourceMetadataError("invalid_location_identifier")
		}
	}
	if (location.RowStart == 0) != (location.RowEnd == 0) || location.RowStart < 0 || location.RowEnd < 0 || location.RowStart > location.RowEnd || location.RowEnd > maxSourceRow {
		return sourceMetadataError("invalid_location_row_range")
	}
	return nil
}

func cloneSourceLocations(locations []SourceLocation) []SourceLocation {
	if locations == nil {
		return nil
	}
	return append([]SourceLocation(nil), locations...)
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

type CanonicalDocumentInput struct {
	Title          string         `json:"title"`
	Markdown       string         `json:"markdown"`
	RemoteRevision string         `json:"remote_revision"`
	SourceMetadata SourceMetadata `json:"source_metadata"`
	SafeSourceURL  SafeURL        `json:"safe_source_url"`
}

type CanonicalDocumentValidationError struct {
	Reason string
}

func (e *CanonicalDocumentValidationError) Error() string { return "invalid canonical document" }

func NewCanonicalDocument(input CanonicalDocumentInput) (CanonicalDocument, error) {
	document := CanonicalDocument{
		Title:          input.Title,
		Markdown:       input.Markdown,
		RemoteRevision: input.RemoteRevision,
		SourceMetadata: input.SourceMetadata,
		SafeSourceURL:  input.SafeSourceURL,
	}
	if err := validateCanonicalDocument(document); err != nil {
		return CanonicalDocument{}, err
	}
	return document, nil
}

func (d CanonicalDocument) MarshalJSON() ([]byte, error) {
	if err := validateCanonicalDocument(d); err != nil {
		return nil, err
	}
	return json.Marshal(CanonicalDocumentInput{
		Title:          d.Title,
		Markdown:       d.Markdown,
		RemoteRevision: d.RemoteRevision,
		SourceMetadata: d.SourceMetadata,
		SafeSourceURL:  d.SafeSourceURL,
	})
}

func (d *CanonicalDocument) UnmarshalJSON(data []byte) error {
	if d == nil {
		return canonicalDocumentError("nil_destination")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var input CanonicalDocumentInput
	if err := decoder.Decode(&input); err != nil {
		return canonicalDocumentError("invalid_json")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return canonicalDocumentError("invalid_json")
	}
	validated, err := NewCanonicalDocument(input)
	if err != nil {
		return err
	}
	*d = validated
	return nil
}

func validateCanonicalDocument(document CanonicalDocument) error {
	if !utf8.ValidString(document.Title) || strings.TrimSpace(document.Title) == "" || len(document.Title) > maxCanonicalTitleBytes {
		return canonicalDocumentError("invalid_title")
	}
	if !utf8.ValidString(document.Markdown) {
		return canonicalDocumentError("invalid_markdown")
	}
	if !validBoundedText(document.RemoteRevision, maxRemoteRevisionBytes) || strings.TrimSpace(document.RemoteRevision) == "" {
		return canonicalDocumentError("invalid_remote_revision")
	}
	if _, err := NewSourceMetadata(document.SourceMetadata.Values()); err != nil {
		return canonicalDocumentError("invalid_source_metadata")
	}
	if document.SafeSourceURL.IsZero() {
		return canonicalDocumentError("missing_source_url")
	}
	validatedURL, err := NewSafeURL(document.SafeSourceURL.String())
	if err != nil || validatedURL != document.SafeSourceURL {
		return canonicalDocumentError("invalid_source_url")
	}
	return nil
}

func canonicalDocumentError(reason string) *CanonicalDocumentValidationError {
	return &CanonicalDocumentValidationError{Reason: reason}
}
