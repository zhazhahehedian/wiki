package feishu_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/feishu"
)

func TestURLResolverResolvesCanonicalFeishuResources(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		want     domain.ResourceRef
		identity string
	}{
		{
			name:   "docx normalizes host trailing slash query and fragment",
			rawURL: "https://Acme.Feishu.CN/docx/doxcnAb_C-1/?utm_source=mail&access_token=query-secret#heading",
			want: domain.ResourceRef{
				Type:        domain.ResourceDocx,
				Tenant:      "acme.feishu.cn",
				Token:       "doxcnAb_C-1",
				OriginalURL: "https://acme.feishu.cn/docx/doxcnAb_C-1",
			},
			identity: "feishu://acme.feishu.cn/docx/doxcnAb_C-1",
		},
		{
			name:   "sheet retains the selected sheet",
			rawURL: "https://acme.feishu.cn/sheets/shtcnWorkbook?foo=discard&sheet=shA_1#range=A1",
			want: domain.ResourceRef{
				Type:        domain.ResourceSheet,
				Tenant:      "acme.feishu.cn",
				Token:       "shtcnWorkbook",
				SheetID:     "shA_1",
				OriginalURL: "https://acme.feishu.cn/sheets/shtcnWorkbook?sheet=shA_1",
			},
			identity: "feishu://acme.feishu.cn/sheet/shtcnWorkbook/sheet/shA_1",
		},
		{
			name:   "bitable retains table and view in safe source URL",
			rawURL: "https://team.larksuite.com/base/bascnApp?view=vewCn2&table=tblCn1&signature=query-secret",
			want: domain.ResourceRef{
				Type:        domain.ResourceBitable,
				Tenant:      "team.larksuite.com",
				Token:       "bascnApp",
				TableID:     "tblCn1",
				ViewID:      "vewCn2",
				OriginalURL: "https://team.larksuite.com/base/bascnApp?table=tblCn1&view=vewCn2",
			},
			identity: "feishu://team.larksuite.com/bitable/bascnApp/table/tblCn1",
		},
		{
			name:   "wiki path",
			rawURL: "https://docs.example.feishu.cn/wiki/wikcnNode",
			want: domain.ResourceRef{
				Type:        domain.ResourceWiki,
				Tenant:      "docs.example.feishu.cn",
				Token:       "wikcnNode",
				OriginalURL: "https://docs.example.feishu.cn/wiki/wikcnNode",
			},
			identity: "feishu://docs.example.feishu.cn/wiki/wikcnNode",
		},
	}

	resolver := feishu.NewURLResolver()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolver.Resolve(tt.rawURL)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got.Type != tt.want.Type || got.Tenant != tt.want.Tenant || got.Token != tt.want.Token ||
				got.TableID != tt.want.TableID || got.ViewID != tt.want.ViewID || got.SheetID != tt.want.SheetID ||
				got.OriginalURL != tt.want.OriginalURL {
				t.Fatalf("Resolve() = %+v, want %+v", got, tt.want)
			}
			if got.Identity != tt.identity {
				t.Fatalf("Identity = %q, want %q", got.Identity, tt.identity)
			}
		})
	}
}

func TestURLResolverIdentityUsesUnderlyingResourceSemantics(t *testing.T) {
	resolver := feishu.NewURLResolver()

	base, err := resolver.Resolve("https://acme.feishu.cn/base/baseToken?table=tableA&view=viewA")
	if err != nil {
		t.Fatalf("Resolve(base) error = %v", err)
	}
	otherView, err := resolver.Resolve("https://ACME.FEISHU.CN/base/baseToken/?view=viewB&table=tableA")
	if err != nil {
		t.Fatalf("Resolve(other view) error = %v", err)
	}
	otherTable, err := resolver.Resolve("https://acme.feishu.cn/base/baseToken?table=tableB&view=viewA")
	if err != nil {
		t.Fatalf("Resolve(other table) error = %v", err)
	}
	otherSheet, err := resolver.Resolve("https://acme.feishu.cn/sheets/sheetToken?sheet=sheetB")
	if err != nil {
		t.Fatalf("Resolve(other sheet) error = %v", err)
	}
	sheet, err := resolver.Resolve("https://acme.feishu.cn/sheets/sheetToken?sheet=sheetA")
	if err != nil {
		t.Fatalf("Resolve(sheet) error = %v", err)
	}

	if base.Identity != otherView.Identity {
		t.Fatalf("view changed identity: %q != %q", base.Identity, otherView.Identity)
	}
	if base.Identity == otherTable.Identity {
		t.Fatalf("table did not change identity: %q", base.Identity)
	}
	if sheet.Identity == otherSheet.Identity {
		t.Fatalf("sheet did not change identity: %q", sheet.Identity)
	}
}

func TestURLResolverRejectsUnsafeOrUnsupportedURLsWithTypedErrors(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		reason feishu.ResolveErrorReason
	}{
		{name: "malformed", rawURL: "://not-a-url", reason: feishu.ResolveMalformedURL},
		{name: "http scheme", rawURL: "http://acme.feishu.cn/docx/token", reason: feishu.ResolveUnsupportedScheme},
		{name: "javascript scheme", rawURL: "javascript:alert(1)", reason: feishu.ResolveUnsupportedScheme},
		{name: "userinfo", rawURL: "https://user:password@acme.feishu.cn/docx/token", reason: feishu.ResolveUserInfoNotAllowed},
		{name: "explicit port", rawURL: "https://acme.feishu.cn:443/docx/token", reason: feishu.ResolvePortNotAllowed},
		{name: "unrelated host", rawURL: "https://feishu.cn.example.com/docx/token", reason: feishu.ResolveUnsupportedHost},
		{name: "suffix confusion", rawURL: "https://evilfeishu.cn/docx/token", reason: feishu.ResolveUnsupportedHost},
		{name: "bare service domain", rawURL: "https://feishu.cn/docx/token", reason: feishu.ResolveUnsupportedHost},
		{name: "malformed tenant labels", rawURL: "https://foo..feishu.cn/docx/token", reason: feishu.ResolveUnsupportedHost},
		{name: "unsupported resource type", rawURL: "https://acme.feishu.cn/docs/token", reason: feishu.ResolveUnsupportedResourceType},
		{name: "empty token", rawURL: "https://acme.feishu.cn/docx/", reason: feishu.ResolveInvalidPath},
		{name: "extra path segment", rawURL: "https://acme.feishu.cn/docx/token/child", reason: feishu.ResolveInvalidPath},
		{name: "encoded slash", rawURL: "https://acme.feishu.cn/docx/token%2Fchild", reason: feishu.ResolveEncodedPathNotAllowed},
		{name: "encoded dot segment", rawURL: "https://acme.feishu.cn/docx/%2e%2e", reason: feishu.ResolveEncodedPathNotAllowed},
		{name: "encoded token", rawURL: "https://acme.feishu.cn/docx/%74oken", reason: feishu.ResolveEncodedPathNotAllowed},
		{name: "ambiguous table", rawURL: "https://acme.feishu.cn/base/token?table=one&table=two", reason: feishu.ResolveAmbiguousSelector},
		{name: "invalid sheet selector", rawURL: "https://acme.feishu.cn/sheets/token?sheet=not%2Fa%2Fsheet", reason: feishu.ResolveInvalidSelector},
	}

	resolver := feishu.NewURLResolver()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolver.Resolve(tt.rawURL)
			var resolveErr *feishu.ResolveError
			if !errors.As(err, &resolveErr) {
				t.Fatalf("Resolve() error = %#v, want *ResolveError", err)
			}
			if resolveErr.Code != "unsupported_feishu_url" || resolveErr.Reason != tt.reason {
				t.Fatalf("ResolveError = %+v, want code unsupported_feishu_url reason %q", resolveErr, tt.reason)
			}
			if strings.Contains(err.Error(), tt.rawURL) || strings.Contains(err.Error(), "password") || strings.Contains(err.Error(), "query-secret") {
				t.Fatalf("Resolve() leaked URL data: %v", err)
			}
		})
	}
}

func TestURLResolverRejectsSecretBearingMalformedURLWithoutLeakingIt(t *testing.T) {
	const secret = "query-secret-value"
	_, err := feishu.NewURLResolver().Resolve("https://acme.feishu.cn:bad/docx/token?access_token=" + secret)
	if err == nil {
		t.Fatal("Resolve() error = nil")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "access_token") {
		t.Fatalf("Resolve() error leaked query data: %v", err)
	}
}
