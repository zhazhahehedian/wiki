package feishu

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

func TestSheetLoaderPaginatesSheetsAndRowsIntoCanonicalSections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture := ""
		switch {
		case strings.HasSuffix(r.URL.Path, "/spreadsheets/sheetbook"):
			fixture = "testdata/sheet_workbook.json"
		case strings.HasSuffix(r.URL.Path, "/sheets/query") && r.URL.Query().Get("page_token") == "s2":
			fixture = "testdata/sheet_list_page2.json"
		case strings.HasSuffix(r.URL.Path, "/sheets/query"):
			fixture = "testdata/sheet_list_page1.json"
		case strings.HasSuffix(r.URL.Path, "/values/sh1!A1:ZZZ3"):
			fixture = "testdata/sheet_values_page1.json"
		case strings.HasSuffix(r.URL.Path, "/values/sh2"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"value_range":{"values":[]},"has_more":false}}`))
			return
		default:
			t.Fatalf("unexpected request %s", r.URL)
		}
		body, err := os.ReadFile(fixture)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	document, err := NewSheetLoader(NewClient(ClientConfig{BaseURL: server.URL}, server.Client())).Load(context.Background(), mustResourceRef(t, domain.ResourceSheet, "sheetbook", ""), "token")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := []string{
		"## 成员", `<!-- feishu-sheet sheet_id="sh1" rows="1-3" -->`,
		"| 姓名 | 说明 |", "| 张三 | a\\|b |", "| 李四 | 第二页 |",
		"## 空表", `<!-- feishu-sheet sheet_id="sh2" rows="0-0" -->`,
	}
	for _, fragment := range want {
		if !strings.Contains(document.Markdown, fragment) {
			t.Fatalf("Markdown missing %q:\n%s", fragment, document.Markdown)
		}
	}
	if document.Title != "团队数据" || document.RemoteRevision != "9" {
		t.Fatalf("document = %+v", document)
	}
}

func TestSheetLoaderRetainsTypedLocationsForEverySheet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/spreadsheets/sheetbook"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"spreadsheet":{"spreadsheet_token":"sheetbook","title":"Book","revision":1}}}`))
		case strings.HasSuffix(r.URL.Path, "/sheets/query"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"sheets":[{"sheet_id":"sh1","title":"First"},{"sheet_id":"sh2","title":"Empty"}],"has_more":false}}`))
		case strings.HasSuffix(r.URL.Path, "/values/sh1"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"valueRange":{"revision":1,"values":[["H"],["V"]]}}}`))
		case strings.HasSuffix(r.URL.Path, "/values/sh2"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"valueRange":{"revision":1,"values":[]}}}`))
		default:
			t.Fatalf("unexpected request: %s", r.URL)
		}
	}))
	defer server.Close()

	document, err := NewSheetLoader(NewClient(ClientConfig{BaseURL: server.URL}, server.Client())).Load(context.Background(), mustResourceRef(t, domain.ResourceSheet, "sheetbook", ""), "token")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	locations := document.SourceMetadata.Values().Locations
	if len(locations) != 2 {
		t.Fatalf("locations = %+v, want 2 entries", locations)
	}
	if locations[0].SheetID != "sh1" || locations[0].SheetName != "First" || locations[0].RowStart != 1 || locations[0].RowEnd != 2 {
		t.Fatalf("first location = %+v", locations[0])
	}
	if locations[1].SheetID != "sh2" || locations[1].SheetName != "Empty" || locations[1].RowStart != 0 || locations[1].RowEnd != 0 {
		t.Fatalf("second location = %+v", locations[1])
	}
}

func TestSheetLoaderRetainsSelectedSheetMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/spreadsheets/sheetbook"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"spreadsheet":{"spreadsheet_token":"sheetbook","title":"Book","revision":1}}}`))
		case strings.HasSuffix(r.URL.Path, "/sheets/query"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"sheets":[{"sheet_id":"sh1","title":"Only"}],"has_more":false}}`))
		default:
			_, _ = w.Write([]byte(`{"code":0,"data":{"value_range":{"values":[["H"],["V"]]},"has_more":false}}`))
		}
	}))
	defer server.Close()

	document, err := NewSheetLoader(NewClient(ClientConfig{BaseURL: server.URL}, server.Client())).Load(context.Background(), mustResourceRef(t, domain.ResourceSheet, "sheetbook", "sh1"), "token")
	if err != nil {
		t.Fatal(err)
	}
	metadata := document.SourceMetadata.Values()
	if metadata.SheetID != "sh1" || metadata.SheetName != "Only" || metadata.RowStart != 1 || metadata.RowEnd != 2 {
		t.Fatalf("metadata = %+v", metadata)
	}
}

func TestSheetLoaderAcceptsCamelCaseValueRangeRevision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/spreadsheets/sheetbook"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"spreadsheet":{"spreadsheet_token":"sheetbook","title":"Book"}}}`))
		case strings.HasSuffix(r.URL.Path, "/sheets/query"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"sheets":[{"sheet_id":"sh1","title":"Only"}],"has_more":false}}`))
		default:
			_, _ = w.Write([]byte(`{"code":0,"data":{"valueRange":{"revision":12,"values":[["H"],["V"]]},"has_more":false}}`))
		}
	}))
	defer server.Close()

	document, err := NewSheetLoader(NewClient(ClientConfig{BaseURL: server.URL}, server.Client())).Load(context.Background(), mustResourceRef(t, domain.ResourceSheet, "sheetbook", "sh1"), "token")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if document.RemoteRevision != "12" || !strings.Contains(document.Markdown, "| H |") {
		t.Fatalf("document = %+v", document)
	}
}
