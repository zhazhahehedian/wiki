package feishu

import (
	"errors"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

func TestResourceGrammarRecognizesProviderFamiliesAndCanonicalPaths(t *testing.T) {
	hosts := []struct {
		host string
		want string
		ok   bool
	}{
		{host: "docs.acme.feishu.cn", want: "feishu.cn", ok: true},
		{host: "team.larksuite.com", want: "larksuite.com", ok: true},
		{host: "feishu.cn.example.com"},
		{host: "xn--fsqu00a.feishu.cn"},
	}
	for _, tt := range hosts {
		got, ok := providerFamily(tt.host)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("providerFamily(%q) = %q, %v; want %q, %v", tt.host, got, ok, tt.want, tt.ok)
		}
	}

	paths := []struct {
		path          string
		wantType      domain.ResourceType
		wantCanonical string
		wantToken     string
	}{
		{path: "/docx/documentToken/", wantType: domain.ResourceDocx, wantCanonical: "/docx/documentToken", wantToken: "documentToken"},
		{path: "/sheets/workbook", wantType: domain.ResourceSheet, wantCanonical: "/sheets/workbook", wantToken: "workbook"},
		{path: "/base/baseToken", wantType: domain.ResourceBitable, wantCanonical: "/base/baseToken", wantToken: "baseToken"},
		{path: "/wiki/wikiNode", wantType: domain.ResourceWiki, wantCanonical: "/wiki/wikiNode", wantToken: "wikiNode"},
	}
	for _, tt := range paths {
		resourceType, canonical, token, err := parseResourcePath(tt.path)
		if err != nil {
			t.Fatalf("parseResourcePath(%q) error = %v", tt.path, err)
		}
		if resourceType != tt.wantType || canonical != tt.wantCanonical || token != tt.wantToken {
			t.Fatalf("parseResourcePath(%q) = %q, %q, %q; want %q, %q, %q", tt.path, resourceType, canonical, token, tt.wantType, tt.wantCanonical, tt.wantToken)
		}
	}

	_, _, _, err := parseResourcePath("/slides/token")
	var resolveErr *ports.SourceResolveError
	if !errors.As(err, &resolveErr) || resolveErr.Reason != ports.SourceResolveUnsupportedResourceType {
		t.Fatalf("parseResourcePath(unsupported) error = %#v", err)
	}
}
