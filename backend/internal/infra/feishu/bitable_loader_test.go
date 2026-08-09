package feishu

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

func TestBitableLoaderPaginatesAndDeterministicallyFlattensRecords(t *testing.T) {
	server := newBitableFixtureServer(t)
	defer server.Close()

	loader := NewBitableLoader(NewClient(ClientConfig{BaseURL: server.URL}, server.Client()), BitableConfig{MaxRows: 10, MaxOutputBytes: 10000})
	document, err := loader.Load(context.Background(), mustResourceRef(t, domain.ResourceBitable, "baseA", "tb1"), "token")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := []string{
		"## 主表", `<!-- feishu-bitable table_id="tb1" rows="1-2" -->`,
		"| Record ID | Active | Complex | Name |", "| rec1 | true | a=2; 1; z=尾 | 张三 |",
		"| rec2 | false |  | 李四 |",
	}
	for _, fragment := range want {
		if !strings.Contains(document.Markdown, fragment) {
			t.Fatalf("Markdown missing %q:\n%s", fragment, document.Markdown)
		}
	}
	if document.RemoteRevision != "7" || document.SourceMetadata.Values().TableID != "tb1" {
		t.Fatalf("document metadata = %+v", document)
	}
}

func TestBitableLoaderReturnsExplicitErrorsForRowAndOutputLimits(t *testing.T) {
	for _, test := range []struct {
		name   string
		config BitableConfig
	}{
		{name: "rows", config: BitableConfig{MaxRows: 1, MaxOutputBytes: 10000}},
		{name: "bytes", config: BitableConfig{MaxRows: 10, MaxOutputBytes: 20}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := newBitableFixtureServer(t)
			defer server.Close()
			_, err := NewBitableLoader(NewClient(ClientConfig{BaseURL: server.URL}, server.Client()), test.config).Load(context.Background(), mustResourceRef(t, domain.ResourceBitable, "baseA", "tb1"), "token")
			var loadErr *ports.SourceLoadError
			if !errors.As(err, &loadErr) || loadErr.Code != ports.SourceLoadTooLarge {
				t.Fatalf("Load() error = %#v", err)
			}
		})
	}
}

func newBitableFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture := ""
		if strings.HasSuffix(r.URL.Path, "/tables") || strings.HasSuffix(r.URL.Path, "/views") {
			if got := r.URL.Query().Get("page_size"); got != "100" {
				t.Fatalf("page_size = %q, want 100 for %s", got, r.URL.Path)
			}
		}
		if strings.HasSuffix(r.URL.Path, "/records") && r.URL.Query().Get("page_size") != "500" {
			t.Fatalf("records page_size = %q", r.URL.Query().Get("page_size"))
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/apps/baseA"):
			fixture = "testdata/bitable_app.json"
		case strings.HasSuffix(r.URL.Path, "/tables") && r.URL.Query().Get("page_token") == "t2":
			fixture = "testdata/bitable_tables_page2.json"
		case strings.HasSuffix(r.URL.Path, "/tables"):
			fixture = "testdata/bitable_tables_page1.json"
		case strings.HasSuffix(r.URL.Path, "/views") && r.URL.Query().Get("page_token") == "v2":
			fixture = "testdata/bitable_views_page2.json"
		case strings.HasSuffix(r.URL.Path, "/views"):
			fixture = "testdata/bitable_views_page1.json"
		case strings.HasSuffix(r.URL.Path, "/records") && r.URL.Query().Get("view_id") == "vw2":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[],"has_more":false}}`))
			return
		case strings.HasSuffix(r.URL.Path, "/records") && r.URL.Query().Get("page_token") == "p2":
			fixture = "testdata/bitable_records_page2.json"
		case strings.HasSuffix(r.URL.Path, "/records"):
			fixture = "testdata/bitable_records_page1.json"
		default:
			t.Fatalf("unexpected request %s", r.URL)
		}
		body, err := os.ReadFile(fixture)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(body)
	}))
}
