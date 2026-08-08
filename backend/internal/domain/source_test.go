package domain_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

func TestSafeURLCanonicalizesAndRoundTripsJSON(t *testing.T) {
	safeURL, err := domain.NewSafeURL("https://Acme.Feishu.CN/base/baseToken?view=viewB&table=tableA")
	if err != nil {
		t.Fatalf("NewSafeURL() error = %v", err)
	}
	const want = "https://acme.feishu.cn/base/baseToken?table=tableA&view=viewB"
	if safeURL.String() != want || safeURL.IsZero() {
		t.Fatalf("SafeURL = %q, want %q", safeURL.String(), want)
	}
	encoded, err := json.Marshal(safeURL)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded domain.SafeURL
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded.String() != want {
		t.Fatalf("roundtrip = %q, want %q", decoded.String(), want)
	}
	var zero domain.SafeURL
	if encoded, err := json.Marshal(zero); err != nil || string(encoded) != "null" {
		t.Fatalf("zero Marshal() = %s, %v", encoded, err)
	}
	if err := json.Unmarshal([]byte("null"), &decoded); err != nil || !decoded.IsZero() {
		t.Fatalf("null Unmarshal() = %q, %v", decoded.String(), err)
	}
}

func TestSafeURLRejectsUnsafeConstructionAndJSON(t *testing.T) {
	tests := []string{
		"ftp://acme.feishu.cn/docx/token",
		"https://user:password@acme.feishu.cn/docx/token",
		"https://acme.feishu.cn:443/docx/token",
		"https://acme.feishu.cn:/docx/token",
		"https://acme.feishu.cn/docx/token#fragment",
		"https://acme.feishu.cn/docx/token?access_token=secret",
		"https://cdn.example/image.png?X-Amz-Signature=secret",
		"https://acme.feishu.cn/base/token?table=one&table=two",
		"https://acme.feishu.cn/base/token?view=bad%2Fselector",
		"https://acme.feishu.cn\\@evil.example/docx/token",
	}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			_, err := domain.NewSafeURL(raw)
			var validationErr *domain.SafeURLValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("NewSafeURL() error = %#v, want validation error", err)
			}
			if strings.Contains(err.Error(), raw) || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "password") {
				t.Fatalf("NewSafeURL() leaked unsafe value: %v", err)
			}
			encoded, marshalErr := json.Marshal(raw)
			if marshalErr != nil {
				t.Fatalf("marshal test input: %v", marshalErr)
			}
			var decoded domain.SafeURL
			if err := json.Unmarshal(encoded, &decoded); !errors.As(err, &validationErr) {
				t.Fatalf("SafeURL.UnmarshalJSON() error = %#v, want validation error", err)
			}
		})
	}
}

func TestSourceMetadataUsesOnlyTypedAllowlistedFieldsAndRoundTrips(t *testing.T) {
	locator := mustSafeURL(t, "https://acme.feishu.cn/base/baseToken?table=tableA&view=viewA")
	imageURL := mustSafeURL(t, "https://cdn.example/images/diagram.png")
	input := domain.SourceMetadataInput{
		SourceType:     domain.ResourceBitable,
		SectionPath:    "概要 / token_count 指标",
		TableID:        "tableA",
		ViewID:         "viewA",
		RowStart:       2,
		RowEnd:         20,
		RemoteRevision: "revision_2026-08-09",
		ImageURL:       imageURL,
		SourceLocator:  locator,
	}
	metadata, err := domain.NewSourceMetadata(input)
	if err != nil {
		t.Fatalf("NewSourceMetadata() error = %v", err)
	}
	if got := metadata.Values(); !reflect.DeepEqual(got, input) {
		t.Fatalf("Values() = %+v, want %+v", got, input)
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded domain.SourceMetadata
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got := decoded.Values(); !reflect.DeepEqual(got, input) {
		t.Fatalf("roundtrip = %+v, want %+v", got, input)
	}
}

func TestSourceMetadataRejectsUnknownSecretBearingJSON(t *testing.T) {
	tests := []string{
		`{"source_type":"docx","raw_response":{"access_token":"secret"}}`,
		`{"source_type":"docx","cookie":"session=secret"}`,
		`{"source_type":"docx","image_url":"https://cdn.example/image.png?X-Amz-Signature=secret"}`,
	}
	for _, raw := range tests {
		var metadata domain.SourceMetadata
		if err := json.Unmarshal([]byte(raw), &metadata); err == nil {
			t.Fatalf("Unmarshal(%s) error = nil", raw)
		} else if strings.Contains(err.Error(), "access_token\":\"secret") || strings.Contains(err.Error(), "session=secret") || strings.Contains(err.Error(), "X-Amz-Signature=secret") {
			t.Fatalf("Unmarshal() leaked secret value: %v", err)
		}
	}
}

func TestSourceMetadataValidatesEnumsBoundsAndRows(t *testing.T) {
	tests := []domain.SourceMetadataInput{
		{SourceType: "unknown"},
		{SourceType: domain.ResourceDocx, SectionPath: strings.Repeat("a", 2049)},
		{SourceType: domain.ResourceBitable, TableID: strings.Repeat("a", 257)},
		{SourceType: domain.ResourceSheet, SheetID: "bad/id"},
		{SourceType: domain.ResourceSheet, RowStart: 2},
		{SourceType: domain.ResourceSheet, RowStart: 20, RowEnd: 2},
		{SourceType: domain.ResourceSheet, RowStart: -1, RowEnd: 2},
		{SourceType: domain.ResourceDocx, RemoteRevision: strings.Repeat("r", 257)},
		{SourceType: domain.ResourceDocx, SectionPath: string([]byte{0xff})},
	}
	for _, input := range tests {
		_, err := domain.NewSourceMetadata(input)
		var validationErr *domain.SourceMetadataValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("NewSourceMetadata(%+v) error = %#v, want validation error", input, err)
		}
		if len(err.Error()) > 128 {
			t.Fatalf("validation error too detailed: %v", err)
		}
	}
}

func TestCanonicalDocumentUsesOpaqueSafeSourceURL(t *testing.T) {
	metadata, err := domain.NewSourceMetadata(domain.SourceMetadataInput{SourceType: domain.ResourceDocx, SectionPath: "Overview"})
	if err != nil {
		t.Fatalf("NewSourceMetadata() error = %v", err)
	}
	document := domain.CanonicalDocument{
		Title:          "Title",
		Markdown:       "# Title",
		RemoteRevision: "revision-1",
		SourceMetadata: metadata,
		SafeSourceURL:  mustSafeURL(t, "https://acme.feishu.cn/docx/token"),
	}
	if document.SafeSourceURL.String() == "" || document.SourceMetadata.Values().SectionPath != "Overview" {
		t.Fatalf("CanonicalDocument = %+v", document)
	}
}

func mustSafeURL(t *testing.T, raw string) domain.SafeURL {
	t.Helper()
	value, err := domain.NewSafeURL(raw)
	if err != nil {
		t.Fatalf("NewSafeURL(%q) error = %v", raw, err)
	}
	return value
}
