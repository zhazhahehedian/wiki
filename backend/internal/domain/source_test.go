package domain_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

func TestNewSourceMetadataValidatesAndRedactsBeforePersistence(t *testing.T) {
	input := map[string]string{
		"section_path": "Overview",
		"source_link":  "https://acme.feishu.cn/docx/token?access_token=query-secret#heading",
		"access_token": "oauth-secret",
		"note":         "Authorization: Bearer bearer-secret",
		"result":       "code=callback-secret",
	}
	metadata, err := domain.NewSourceMetadata(input)
	if err != nil {
		t.Fatalf("NewSourceMetadata() error = %v", err)
	}
	input["section_path"] = "mutated"

	want := map[string]string{
		"section_path": "Overview",
		"source_link":  "https://acme.feishu.cn/docx/token",
		"note":         domain.RedactedMetadataValue,
		"result":       domain.RedactedMetadataValue,
	}
	if got := metadata.Values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Values() = %#v, want %#v", got, want)
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	for _, secret := range []string{"query-secret", "oauth-secret", "bearer-secret", "callback-secret", "access_token"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("metadata JSON leaked %q: %s", secret, encoded)
		}
	}
}

func TestNewSourceMetadataRejectsInvalidOrOversizedValuesWithoutLeakingThem(t *testing.T) {
	secret := "sensitive-metadata-value"
	tests := []map[string]string{
		{"bad key": secret},
		{strings.Repeat("k", 65): secret},
		{"valid_key": strings.Repeat(secret, 200)},
		{"valid_key": string([]byte{0xff, 0xfe})},
	}
	for _, input := range tests {
		_, err := domain.NewSourceMetadata(input)
		var validationErr *domain.SourceMetadataValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("NewSourceMetadata() error = %#v, want validation error", err)
		}
		if strings.Contains(err.Error(), secret) || len(err.Error()) > 128 {
			t.Fatalf("validation error leaked metadata: %v", err)
		}
	}
}

func TestCanonicalDocumentUsesSafeSourceTypes(t *testing.T) {
	metadata, err := domain.NewSourceMetadata(map[string]string{"section_path": "Overview"})
	if err != nil {
		t.Fatalf("NewSourceMetadata() error = %v", err)
	}
	document := domain.CanonicalDocument{
		Title:          "Title",
		Markdown:       "# Title",
		RemoteRevision: "revision-1",
		SourceMetadata: metadata,
		SafeSourceURL:  "https://acme.feishu.cn/docx/token",
	}
	if document.SafeSourceURL == "" || document.SourceMetadata.Values()["section_path"] != "Overview" {
		t.Fatalf("CanonicalDocument = %+v", document)
	}
}
