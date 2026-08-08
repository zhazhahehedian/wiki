package feishu_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/feishu"
)

func TestURLResolverResolvesCanonicalResources(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want domain.ResourceRef
	}{
		{
			name: "docx",
			raw:  "https://Acme.Feishu.CN/docx/doxcnAb_C-1/?access_token=query-secret#heading",
			want: domain.ResourceRef{Type: domain.ResourceDocx, ProviderHost: "feishu.cn", Token: "doxcnAb_C-1", CanonicalURL: mustSafeURL(t, "https://acme.feishu.cn/docx/doxcnAb_C-1"), Identity: "feishu://feishu.cn/docx/doxcnAb_C-1"},
		},
		{
			name: "sheet",
			raw:  "https://acme.feishu.cn/sheets/workbook?foo=discard&sheet=sheetA#range=A1",
			want: domain.ResourceRef{Type: domain.ResourceSheet, ProviderHost: "feishu.cn", Token: "workbook", SheetID: "sheetA", CanonicalURL: mustSafeURL(t, "https://acme.feishu.cn/sheets/workbook?sheet=sheetA"), Identity: "feishu://feishu.cn/sheet/workbook/sheet/sheetA"},
		},
		{
			name: "bitable",
			raw:  "https://team.larksuite.com/base/baseApp?view=viewB&table=tableA&signature=query-secret",
			want: domain.ResourceRef{Type: domain.ResourceBitable, ProviderHost: "larksuite.com", Token: "baseApp", TableID: "tableA", ViewID: "viewB", CanonicalURL: mustSafeURL(t, "https://team.larksuite.com/base/baseApp?table=tableA&view=viewB"), Identity: "feishu://larksuite.com/bitable/baseApp/table/tableA/view/viewB"},
		},
		{
			name: "wiki",
			raw:  "https://docs.example.feishu.cn/wiki/wikiNode",
			want: domain.ResourceRef{Type: domain.ResourceWiki, ProviderHost: "feishu.cn", Token: "wikiNode", CanonicalURL: mustSafeURL(t, "https://docs.example.feishu.cn/wiki/wikiNode"), Identity: "feishu://feishu.cn/wiki/wikiNode"},
		},
	}

	resolver := feishu.NewURLResolver()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolver.Resolve(tt.raw)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Resolve() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestURLResolverIdentityUsesProviderAndSelectionSemantics(t *testing.T) {
	resolver := feishu.NewURLResolver()
	resolve := func(raw string) domain.ResourceRef {
		t.Helper()
		ref, err := resolver.Resolve(raw)
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v", raw, err)
		}
		return ref
	}

	alpha := resolve("https://alpha.feishu.cn/base/baseToken?table=tableA&view=viewA")
	alias := resolve("https://docs.beta.feishu.cn/base/baseToken?view=viewA&table=tableA")
	otherView := resolve("https://alpha.feishu.cn/base/baseToken?table=tableA&view=viewB")
	otherTable := resolve("https://alpha.feishu.cn/base/baseToken?table=tableB&view=viewA")
	otherProvider := resolve("https://alpha.larksuite.com/base/baseToken?table=tableA&view=viewA")
	sheetA := resolve("https://alpha.feishu.cn/sheets/workbook?sheet=sheetA")
	sheetB := resolve("https://alpha.feishu.cn/sheets/workbook?sheet=sheetB")

	if alpha.Tenant != "" || alias.Tenant != "" {
		t.Fatalf("resolver claimed unverified tenant: %q, %q", alpha.Tenant, alias.Tenant)
	}
	if alpha.Identity != alias.Identity {
		t.Fatalf("provider aliases did not deduplicate: %q != %q", alpha.Identity, alias.Identity)
	}
	for name, ref := range map[string]domain.ResourceRef{"view": otherView, "table": otherTable, "provider": otherProvider} {
		if alpha.Identity == ref.Identity {
			t.Fatalf("%s did not affect identity: %q", name, alpha.Identity)
		}
	}
	if sheetA.Identity == sheetB.Identity {
		t.Fatalf("sheet did not affect identity: %q", sheetA.Identity)
	}
}

func TestURLResolverRejectsUnsafeURLsWithPortableTypedErrors(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		reason ports.SourceResolveErrorReason
	}{
		{name: "malformed", raw: "://bad", reason: ports.SourceResolveMalformedURL},
		{name: "scheme", raw: "http://acme.feishu.cn/docx/token", reason: ports.SourceResolveUnsupportedScheme},
		{name: "userinfo", raw: "https://user:password@acme.feishu.cn/docx/token", reason: ports.SourceResolveUserInfoNotAllowed},
		{name: "port", raw: "https://acme.feishu.cn:443/docx/token", reason: ports.SourceResolvePortNotAllowed},
		{name: "empty port", raw: "https://acme.feishu.cn:/docx/token", reason: ports.SourceResolvePortNotAllowed},
		{name: "unrelated host", raw: "https://feishu.cn.example.com/docx/token", reason: ports.SourceResolveUnsupportedHost},
		{name: "suffix confusion", raw: "https://evilfeishu.cn/docx/token", reason: ports.SourceResolveUnsupportedHost},
		{name: "bare provider", raw: "https://feishu.cn/docx/token", reason: ports.SourceResolveUnsupportedHost},
		{name: "empty label", raw: "https://foo..feishu.cn/docx/token", reason: ports.SourceResolveUnsupportedHost},
		{name: "trailing dot", raw: "https://acme.feishu.cn./docx/token", reason: ports.SourceResolveUnsupportedHost},
		{name: "unicode host", raw: "https://例子.feishu.cn/docx/token", reason: ports.SourceResolveUnsupportedHost},
		{name: "punycode host", raw: "https://xn--fsqu00a.feishu.cn/docx/token", reason: ports.SourceResolveUnsupportedHost},
		{name: "backslash authority", raw: "https://acme.feishu.cn\\@evil.example/docx/token", reason: ports.SourceResolveAuthorityConfusion},
		{name: "encoded authority", raw: "https://acme.feishu.cn/%5c@evil.example/docx/token", reason: ports.SourceResolveEncodedPathNotAllowed},
		{name: "unsupported type", raw: "https://acme.feishu.cn/docs/token", reason: ports.SourceResolveUnsupportedResourceType},
		{name: "empty token", raw: "https://acme.feishu.cn/docx/", reason: ports.SourceResolveInvalidPath},
		{name: "extra path", raw: "https://acme.feishu.cn/docx/token/child", reason: ports.SourceResolveInvalidPath},
		{name: "encoded slash", raw: "https://acme.feishu.cn/docx/token%2Fchild", reason: ports.SourceResolveEncodedPathNotAllowed},
		{name: "encoded dot", raw: "https://acme.feishu.cn/docx/%2e%2e", reason: ports.SourceResolveEncodedPathNotAllowed},
		{name: "encoded token", raw: "https://acme.feishu.cn/docx/%74oken", reason: ports.SourceResolveEncodedPathNotAllowed},
		{name: "duplicate table", raw: "https://acme.feishu.cn/base/token?table=one&table=two", reason: ports.SourceResolveAmbiguousSelector},
		{name: "invalid selector", raw: "https://acme.feishu.cn/sheets/token?sheet=not%2Fa%2Fsheet", reason: ports.SourceResolveInvalidSelector},
	}

	resolver := feishu.NewURLResolver()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolver.Resolve(tt.raw)
			var resolveErr *ports.SourceResolveError
			if !errors.As(err, &resolveErr) || !errors.Is(err, ports.ErrUnsupportedFeishuURL) {
				t.Fatalf("Resolve() error = %#v, want portable typed error", err)
			}
			if resolveErr.Code != ports.ErrorCodeUnsupportedFeishuURL || resolveErr.Reason != tt.reason {
				t.Fatalf("SourceResolveError = %+v, want reason %q", resolveErr, tt.reason)
			}
			for _, sensitive := range []string{tt.raw, "password", "access_token", "token%2Fchild"} {
				if strings.Contains(err.Error(), sensitive) {
					t.Fatalf("Resolve() leaked input %q: %v", sensitive, err)
				}
			}
		})
	}
}

func TestURLResolverBoundsUntrustedInput(t *testing.T) {
	longHost := strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 52) + ".feishu.cn"
	tests := []struct {
		name   string
		raw    string
		reason ports.SourceResolveErrorReason
	}{
		{name: "URL", raw: "https://acme.feishu.cn/docx/token?padding=" + strings.Repeat("x", 4096), reason: ports.SourceResolveInputTooLong},
		{name: "whitespace padded URL", raw: strings.Repeat(" ", 4096) + "https://acme.feishu.cn/docx/token", reason: ports.SourceResolveInputTooLong},
		{name: "host", raw: "https://" + longHost + "/docx/token", reason: ports.SourceResolveUnsupportedHost},
		{name: "host label", raw: "https://" + strings.Repeat("a", 64) + ".feishu.cn/docx/token", reason: ports.SourceResolveUnsupportedHost},
		{name: "resource token", raw: "https://acme.feishu.cn/docx/" + strings.Repeat("a", 257), reason: ports.SourceResolveIdentifierTooLong},
		{name: "selector", raw: "https://acme.feishu.cn/base/token?table=" + strings.Repeat("a", 257), reason: ports.SourceResolveIdentifierTooLong},
	}

	resolver := feishu.NewURLResolver()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolver.Resolve(tt.raw)
			var resolveErr *ports.SourceResolveError
			if !errors.As(err, &resolveErr) || !errors.Is(err, ports.ErrUnsupportedFeishuURL) || resolveErr.Reason != tt.reason {
				t.Fatalf("Resolve() error = %#v, want reason %q", err, tt.reason)
			}
			if len(err.Error()) > 128 || strings.Contains(err.Error(), strings.Repeat("a", 257)) {
				t.Fatalf("Resolve() leaked oversized input: %v", err)
			}
		})
	}
}

func TestSourceResolveErrorFormattingCannotLeakAReasonValue(t *testing.T) {
	const secret = "secret-from-untrusted-cause"
	err := ports.NewSourceResolveError(ports.SourceResolveErrorReason(secret))
	if strings.Contains(err.Error(), secret) || !errors.Is(err, ports.ErrUnsupportedFeishuURL) {
		t.Fatalf("SourceResolveError leaked reason: %v", err)
	}
}
func TestURLResolverNeverLeaksSecretBearingMalformedURL(t *testing.T) {
	const secret = "query-secret-value"
	_, err := feishu.NewURLResolver().Resolve("https://acme.feishu.cn:bad/docx/privateToken?access_token=" + secret)
	if err == nil {
		t.Fatal("Resolve() error = nil")
	}
	for _, value := range []string{secret, "privateToken", "access_token"} {
		if strings.Contains(err.Error(), value) {
			t.Fatalf("Resolve() leaked %q: %v", value, err)
		}
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
