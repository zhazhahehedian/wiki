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

func TestSourceMetadataValidatesAndPersistsTypedSectionLocations(t *testing.T) {
	input := domain.SourceMetadataInput{
		SourceType: domain.ResourceSheet,
		Locations: []domain.SourceLocation{
			{SectionPath: "Workbook / First", SheetName: "First", SheetID: "sh1", RowStart: 1, RowEnd: 3},
			{SectionPath: "Workbook / Empty", SheetName: "Empty", SheetID: "sh2"},
		},
	}
	metadata, err := domain.NewSourceMetadata(input)
	if err != nil {
		t.Fatalf("NewSourceMetadata() error = %v", err)
	}
	input.Locations[0].SheetID = "mutated"
	if got := metadata.Values().Locations[0].SheetID; got != "sh1" {
		t.Fatalf("metadata changed through input alias: %q", got)
	}
	values := metadata.Values()
	values.Locations[0].SheetID = "mutated-again"
	if got := metadata.Values().Locations[0].SheetID; got != "sh1" {
		t.Fatalf("metadata changed through Values alias: %q", got)
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded domain.SourceMetadata
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got := decoded.Values().Locations; len(got) != 2 || got[0].SheetID != "sh1" || got[1].RowStart != 0 {
		t.Fatalf("roundtrip locations = %+v", got)
	}
}

func TestSourceMetadataRejectsInvalidOrSecretBearingSectionLocations(t *testing.T) {
	invalid := []domain.SourceMetadataInput{
		{SourceType: domain.ResourceSheet, Locations: []domain.SourceLocation{{SheetID: "bad/id"}}},
		{SourceType: domain.ResourceSheet, Locations: []domain.SourceLocation{{SheetID: "sh1", RowStart: 2}}},
		{SourceType: domain.ResourceSheet, Locations: []domain.SourceLocation{{SectionPath: strings.Repeat("x", 2049)}}},
	}
	for _, input := range invalid {
		if _, err := domain.NewSourceMetadata(input); err == nil {
			t.Fatalf("NewSourceMetadata(%+v) error = nil", input)
		}
	}
	var metadata domain.SourceMetadata
	err := json.Unmarshal([]byte(`{"source_type":"sheet","locations":[{"sheet_id":"sh1","access_token":"secret"}]}`), &metadata)
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("secret-bearing location error = %v", err)
	}
}

func TestSourceMetadataRejectsTypeIncompatibleFields(t *testing.T) {
	tests := []domain.SourceMetadataInput{
		{SourceType: domain.ResourceDocx, SheetID: "sh1"},
		{SourceType: domain.ResourceWiki, TableID: "tb1", SheetID: "sh1"},
		{SourceType: domain.ResourceSheet, TableID: "tb1"},
		{SourceType: domain.ResourceBitable, SheetID: "sh1"},
		{SourceType: domain.ResourceBitable, ViewID: "vw1"},
		{SourceType: domain.ResourceSheet, Locations: []domain.SourceLocation{{TableID: "tb1"}}},
		{SourceType: domain.ResourceBitable, Locations: []domain.SourceLocation{{SheetID: "sh1"}}},
		{SourceType: domain.ResourceBitable, Locations: []domain.SourceLocation{{ViewID: "vw1"}}},
	}
	for _, input := range tests {
		if _, err := domain.NewSourceMetadata(input); err == nil {
			t.Fatalf("NewSourceMetadata(%+v) error = nil", input)
		}
		encoded, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		var decoded domain.SourceMetadata
		if err := json.Unmarshal(encoded, &decoded); err == nil {
			t.Fatalf("Unmarshal(%s) error = nil", encoded)
		}
	}
}

func TestWikiSourceMetadataAllowsOneDelegatedLocationFamily(t *testing.T) {
	for _, input := range []domain.SourceMetadataInput{
		{SourceType: domain.ResourceWiki, SheetName: "Sheet", SheetID: "sh1", Locations: []domain.SourceLocation{{SheetName: "Sheet", SheetID: "sh1"}}},
		{SourceType: domain.ResourceWiki, TableID: "tb1", ViewID: "vw1", Locations: []domain.SourceLocation{{TableID: "tb1", ViewID: "vw1"}}},
	} {
		if _, err := domain.NewSourceMetadata(input); err != nil {
			t.Fatalf("NewSourceMetadata(%+v) error = %v", input, err)
		}
	}
	if _, err := domain.NewSourceMetadata(domain.SourceMetadataInput{SourceType: domain.ResourceWiki, SheetID: "sh1", TableID: "tb1"}); err == nil {
		t.Fatal("mixed Wiki selector families error = nil")
	}
}

func TestCanonicalDocumentUsesOpaqueSafeSourceURL(t *testing.T) {
	metadata, err := domain.NewSourceMetadata(domain.SourceMetadataInput{SourceType: domain.ResourceDocx, SectionPath: "Overview"})
	if err != nil {
		t.Fatalf("NewSourceMetadata() error = %v", err)
	}
	document, err := domain.NewCanonicalDocument(domain.CanonicalDocumentInput{
		Title:          "Title",
		Markdown:       "# Title",
		RemoteRevision: "revision-1",
		SourceMetadata: metadata,
		SafeSourceURL:  mustSafeURL(t, "https://acme.feishu.cn/docx/token"),
	})
	if err != nil {
		t.Fatalf("NewCanonicalDocument() error = %v", err)
	}
	if document.SafeSourceURL.String() == "" || document.SourceMetadata.Values().SectionPath != "Overview" {
		t.Fatalf("CanonicalDocument = %+v", document)
	}
}

func TestResourceRefJSONRejectsInvalidPersistedState(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "empty token",
			raw:  `{"type":"docx","provider_host":"feishu.cn","token":"","canonical_url":"https://acme.feishu.cn/docx/token","original_url":"https://acme.feishu.cn/docx/token","identity":"feishu://feishu.cn/docx/"}`,
		},
		{
			name: "invalid type",
			raw:  `{"type":"slides","provider_host":"feishu.cn","token":"token","canonical_url":"https://acme.feishu.cn/docx/token","original_url":"https://acme.feishu.cn/docx/token","identity":"feishu://feishu.cn/slides/token"}`,
		},
		{
			name: "null required URL",
			raw:  `{"type":"docx","provider_host":"feishu.cn","token":"token","canonical_url":null,"original_url":"https://acme.feishu.cn/docx/token","identity":"feishu://feishu.cn/docx/token"}`,
		},
		{
			name: "null required original URL",
			raw:  `{"type":"docx","provider_host":"feishu.cn","token":"token","canonical_url":"https://acme.feishu.cn/docx/token","original_url":null,"identity":"feishu://feishu.cn/docx/token"}`,
		},
		{
			name: "inconsistent identity",
			raw:  `{"type":"docx","provider_host":"feishu.cn","token":"token","canonical_url":"https://acme.feishu.cn/docx/token","original_url":"https://acme.feishu.cn/docx/token","identity":"feishu://feishu.cn/docx/other"}`,
		},
		{
			name: "type-incompatible selector",
			raw:  `{"type":"docx","provider_host":"feishu.cn","token":"token","table_id":"tableA","canonical_url":"https://acme.feishu.cn/docx/token","original_url":"https://acme.feishu.cn/docx/token","identity":"feishu://feishu.cn/docx/token"}`,
		},
		{
			name: "unknown field",
			raw:  `{"type":"docx","provider_host":"feishu.cn","token":"token","canonical_url":"https://acme.feishu.cn/docx/token","original_url":"https://acme.feishu.cn/docx/token","identity":"feishu://feishu.cn/docx/token","access_token":"secret"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ref domain.ResourceRef
			if err := json.Unmarshal([]byte(tt.raw), &ref); err == nil {
				t.Fatalf("Unmarshal(%s) error = nil", tt.name)
			} else if strings.Contains(err.Error(), "secret") {
				t.Fatalf("Unmarshal() leaked secret-bearing input: %v", err)
			}
		})
	}
}

func TestCanonicalDocumentJSONRejectsInvalidPersistedState(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "missing source metadata",
			raw:  `{"title":"Title","markdown":"# Title","remote_revision":"revision-1","safe_source_url":"https://acme.feishu.cn/docx/token"}`,
		},
		{
			name: "null required source URL",
			raw:  `{"title":"Title","markdown":"# Title","remote_revision":"revision-1","source_metadata":{"source_type":"docx"},"safe_source_url":null}`,
		},
		{
			name: "missing title",
			raw:  `{"markdown":"# Title","remote_revision":"revision-1","source_metadata":{"source_type":"docx"},"safe_source_url":"https://acme.feishu.cn/docx/token"}`,
		},
		{
			name: "missing remote revision",
			raw:  `{"title":"Title","markdown":"# Title","source_metadata":{"source_type":"docx"},"safe_source_url":"https://acme.feishu.cn/docx/token"}`,
		},
		{
			name: "unknown field",
			raw:  `{"title":"Title","markdown":"# Title","remote_revision":"revision-1","source_metadata":{"source_type":"docx"},"safe_source_url":"https://acme.feishu.cn/docx/token","authorization":"Bearer secret"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var document domain.CanonicalDocument
			if err := json.Unmarshal([]byte(tt.raw), &document); err == nil {
				t.Fatalf("Unmarshal(%s) error = nil", tt.name)
			} else if strings.Contains(err.Error(), "secret") {
				t.Fatalf("Unmarshal() leaked secret-bearing input: %v", err)
			}
		})
	}
}

func TestNewResourceRefValidatesConstructionAndComputesIdentity(t *testing.T) {
	canonicalURL := mustSafeURL(t, "https://docs.acme.feishu.cn/base/baseToken?table=tableA&view=viewA")
	input := domain.ResourceRefInput{
		Type:         domain.ResourceBitable,
		ProviderHost: "feishu.cn",
		Token:        "baseToken",
		TableID:      "tableA",
		ViewID:       "viewA",
		CanonicalURL: canonicalURL,
		OriginalURL:  canonicalURL,
	}
	ref, err := domain.NewResourceRef(input)
	if err != nil {
		t.Fatalf("NewResourceRef() error = %v", err)
	}
	if ref.Identity != "feishu://feishu.cn/bitable/baseToken/table/tableA/view/viewA" {
		t.Fatalf("Identity = %q", ref.Identity)
	}
	if ref.OriginalURL.String() != canonicalURL.String() {
		t.Fatalf("OriginalURL = %q, want %q", ref.OriginalURL.String(), canonicalURL.String())
	}

	invalid := []domain.ResourceRefInput{
		{Type: domain.ResourceDocx, ProviderHost: "feishu.cn", CanonicalURL: canonicalURL, OriginalURL: canonicalURL},
		{Type: "slides", ProviderHost: "feishu.cn", Token: "token", CanonicalURL: canonicalURL, OriginalURL: canonicalURL},
		{Type: domain.ResourceDocx, ProviderHost: "acme.feishu.cn", Token: "token", CanonicalURL: canonicalURL, OriginalURL: canonicalURL},
		{Type: domain.ResourceDocx, ProviderHost: "feishu.cn", Token: "token", TableID: "tableA", CanonicalURL: canonicalURL, OriginalURL: canonicalURL},
		{Type: domain.ResourceDocx, ProviderHost: "feishu.cn", Token: "token"},
	}
	for _, invalidInput := range invalid {
		_, err := domain.NewResourceRef(invalidInput)
		var validationErr *domain.ResourceRefValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("NewResourceRef(%+v) error = %#v, want validation error", invalidInput, err)
		}
	}
}

func TestResourceRefJSONRoundTripsThroughValidation(t *testing.T) {
	safeURL := mustSafeURL(t, "https://acme.feishu.cn/sheets/workbook?sheet=sheetA")
	want, err := domain.NewResourceRef(domain.ResourceRefInput{
		Type:         domain.ResourceSheet,
		ProviderHost: "feishu.cn",
		Token:        "workbook",
		SheetID:      "sheetA",
		CanonicalURL: safeURL,
		OriginalURL:  safeURL,
	})
	if err != nil {
		t.Fatalf("NewResourceRef() error = %v", err)
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var got domain.ResourceRef
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("roundtrip = %+v, want %+v", got, want)
	}
	if err := json.Unmarshal(append(encoded, []byte(` {}`)...), &got); err == nil {
		t.Fatal("Unmarshal() trailing JSON error = nil")
	}
}

func TestResourceRefJSONMarshalRejectsMutatedInvalidState(t *testing.T) {
	safeURL := mustSafeURL(t, "https://acme.feishu.cn/docx/token")
	ref, err := domain.NewResourceRef(domain.ResourceRefInput{
		Type:         domain.ResourceDocx,
		ProviderHost: "feishu.cn",
		Token:        "token",
		CanonicalURL: safeURL,
		OriginalURL:  safeURL,
	})
	if err != nil {
		t.Fatalf("NewResourceRef() error = %v", err)
	}
	ref.Identity = "feishu://feishu.cn/docx/other"
	if _, err := json.Marshal(ref); err == nil {
		t.Fatal("Marshal() inconsistent identity error = nil")
	}
}

func TestResourceRefIdentityIgnoresVerifiedTenantAlias(t *testing.T) {
	safeURL := mustSafeURL(t, "https://acme.feishu.cn/docx/token")
	newRef := func(tenant string) domain.ResourceRef {
		t.Helper()
		ref, err := domain.NewResourceRef(domain.ResourceRefInput{
			Type:         domain.ResourceDocx,
			ProviderHost: "feishu.cn",
			Tenant:       tenant,
			Token:        "token",
			CanonicalURL: safeURL,
			OriginalURL:  safeURL,
		})
		if err != nil {
			t.Fatalf("NewResourceRef(tenant=%q) error = %v", tenant, err)
		}
		return ref
	}
	alpha := newRef("tenant.alpha.example")
	beta := newRef("tenant.beta.example")
	if alpha.Identity != beta.Identity {
		t.Fatalf("tenant alias affected identity: %q != %q", alpha.Identity, beta.Identity)
	}
}

func TestCanonicalDocumentConstructionAndJSONRoundTrip(t *testing.T) {
	metadata, err := domain.NewSourceMetadata(domain.SourceMetadataInput{SourceType: domain.ResourceDocx, SectionPath: "Overview"})
	if err != nil {
		t.Fatalf("NewSourceMetadata() error = %v", err)
	}
	input := domain.CanonicalDocumentInput{
		Title:          "Title",
		Markdown:       "# Title",
		RemoteRevision: "revision-1",
		SourceMetadata: metadata,
		SafeSourceURL:  mustSafeURL(t, "https://acme.feishu.cn/docx/token"),
	}
	want, err := domain.NewCanonicalDocument(input)
	if err != nil {
		t.Fatalf("NewCanonicalDocument() error = %v", err)
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var got domain.CanonicalDocument
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("roundtrip = %+v, want %+v", got, want)
	}
	if err := json.Unmarshal(append(encoded, []byte(` {}`)...), &got); err == nil {
		t.Fatal("Unmarshal() trailing JSON error = nil")
	}
	want.SafeSourceURL = domain.SafeURL{}
	if _, err := json.Marshal(want); err == nil {
		t.Fatal("Marshal() missing required source URL error = nil")
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
