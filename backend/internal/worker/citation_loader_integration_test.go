package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/feishu"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/parser"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/splitter"
)

func TestLoaderSectionPathsDriveCitationMetadata(t *testing.T) {
	t.Run("multi sheet", func(t *testing.T) {
		server := newCitationSheetServer(t, false)
		defer server.Close()
		client := feishu.NewClient(feishu.ClientConfig{BaseURL: server.URL}, server.Client())
		document, err := feishu.NewSheetLoader(client).Load(context.Background(), citationRef(t, "https://acme.feishu.cn/sheets/book"), "token")
		if err != nil {
			t.Fatal(err)
		}
		assertLoaderCitations(t, document, map[string]map[string]any{
			"First":  {"sheet_id": "sh1", "sheet_name": "First", "row_start": 1, "row_end": 2},
			"Second": {"sheet_id": "sh2", "sheet_name": "Second", "row_start": 1, "row_end": 2},
		})
	})

	t.Run("duplicate punctuation sheet names", func(t *testing.T) {
		server := newCitationNamedSheetServer(t, "Status [Q1] - #_", "Status [Q1] - #_")
		defer server.Close()
		client := feishu.NewClient(feishu.ClientConfig{BaseURL: server.URL}, server.Client())
		document, err := feishu.NewSheetLoader(client).Load(context.Background(), citationRef(t, "https://acme.feishu.cn/sheets/book"), "token")
		if err != nil {
			t.Fatal(err)
		}
		assertLoaderCitations(t, document, map[string]map[string]any{
			`Status \[Q1\] \- \#\_ \[sh1\]`: {"sheet_id": "sh1", "sheet_name": "Status [Q1] - #_", "row_start": 1, "row_end": 1},
			`Status \[Q1\] \- \#\_ \[sh2\]`: {"sheet_id": "sh2", "sheet_name": "Status [Q1] - #_", "row_start": 1, "row_end": 1},
		})
	})

	t.Run("whitespace equivalent sheet names", func(t *testing.T) {
		server := newCitationNamedSheetServer(t, "Status", " Status ")
		defer server.Close()
		client := feishu.NewClient(feishu.ClientConfig{BaseURL: server.URL}, server.Client())
		document, err := feishu.NewSheetLoader(client).Load(context.Background(), citationRef(t, "https://acme.feishu.cn/sheets/book"), "token")
		if err != nil {
			t.Fatal(err)
		}
		assertLoaderCitations(t, document, map[string]map[string]any{
			`Status \[sh1\]`: {"sheet_id": "sh1", "sheet_name": "Status", "row_start": 1, "row_end": 1},
			`Status \[sh2\]`: {"sheet_id": "sh2", "sheet_name": " Status ", "row_start": 1, "row_end": 1},
		})
	})

	t.Run("contiguous paginated sheet aggregates citation bounds", func(t *testing.T) {
		server := newCitationPaginatedSheetServer(t)
		defer server.Close()
		client := feishu.NewClient(feishu.ClientConfig{BaseURL: server.URL}, server.Client())
		document, err := feishu.NewSheetLoader(client).Load(context.Background(), citationRef(t, "https://acme.feishu.cn/sheets/book"), "token")
		if err != nil {
			t.Fatal(err)
		}
		locations := document.SourceMetadata.Values().Locations
		if len(locations) != 2 || locations[0].RowStart != 1 || locations[0].RowEnd != 500 || locations[1].RowStart != 501 || locations[1].RowEnd != 501 {
			t.Fatalf("locations = %+v, want preserved physical ranges", locations)
		}
		assertLoaderCitations(t, document, map[string]map[string]any{
			"Paged": {"sheet_id": "sh1", "sheet_name": "Paged", "row_start": 1, "row_end": 501},
		})
	})

	t.Run("sparse paginated sheet retains detailed ranges", func(t *testing.T) {
		server := newCitationSparseSheetServer(t)
		defer server.Close()
		client := feishu.NewClient(feishu.ClientConfig{BaseURL: server.URL}, server.Client())
		document, err := feishu.NewSheetLoader(client).Load(context.Background(), citationRef(t, "https://acme.feishu.cn/sheets/book"), "token")
		if err != nil {
			t.Fatal(err)
		}
		locations := document.SourceMetadata.Values().Locations
		if len(locations) != 2 || locations[0].RowStart != 1 || locations[0].RowEnd != 1 || locations[1].RowStart != 750 || locations[1].RowEnd != 751 {
			t.Fatalf("locations = %+v, want preserved sparse ranges", locations)
		}
		assertLoaderCitations(t, document, map[string]map[string]any{
			"Sparse": {
				"sheet_id": "sh1", "sheet_name": "Sparse", "row_start": nil, "row_end": nil,
				"locations": locations,
			},
		})
	})

	t.Run("multi table", func(t *testing.T) {
		server := newCitationBitableServer(t, false, false)
		defer server.Close()
		client := feishu.NewClient(feishu.ClientConfig{BaseURL: server.URL}, server.Client())
		document, err := feishu.NewBitableLoader(client, feishu.BitableConfig{}).Load(context.Background(), citationRef(t, "https://acme.feishu.cn/base/baseA"), "token")
		if err != nil {
			t.Fatal(err)
		}
		assertLoaderCitations(t, document, map[string]map[string]any{
			"Alpha": {"table_id": "tb1", "row_start": 1, "row_end": 1},
			"Beta":  {"table_id": "tb2", "row_start": 1, "row_end": 1},
		})
	})

	t.Run("duplicate punctuation table names", func(t *testing.T) {
		server := newCitationNamedBitableServer(t, "Status [Q1] - #_", "Status [Q1] - #_")
		defer server.Close()
		client := feishu.NewClient(feishu.ClientConfig{BaseURL: server.URL}, server.Client())
		document, err := feishu.NewBitableLoader(client, feishu.BitableConfig{}).Load(context.Background(), citationRef(t, "https://acme.feishu.cn/base/baseA"), "token")
		if err != nil {
			t.Fatal(err)
		}
		assertLoaderCitations(t, document, map[string]map[string]any{
			`Status \[Q1\] \- \#\_ \[tb1\]`: {"table_id": "tb1", "row_start": 1, "row_end": 1},
			`Status \[Q1\] \- \#\_ \[tb2\]`: {"table_id": "tb2", "row_start": 1, "row_end": 1},
		})
	})

	t.Run("whitespace equivalent table names", func(t *testing.T) {
		server := newCitationNamedBitableServer(t, "Status", " Status ")
		defer server.Close()
		client := feishu.NewClient(feishu.ClientConfig{BaseURL: server.URL}, server.Client())
		document, err := feishu.NewBitableLoader(client, feishu.BitableConfig{}).Load(context.Background(), citationRef(t, "https://acme.feishu.cn/base/baseA"), "token")
		if err != nil {
			t.Fatal(err)
		}
		assertLoaderCitations(t, document, map[string]map[string]any{
			`Status \[tb1\]`: {"table_id": "tb1", "row_start": 1, "row_end": 1},
			`Status \[tb2\]`: {"table_id": "tb2", "row_start": 1, "row_end": 1},
		})
	})

	t.Run("selected view", func(t *testing.T) {
		server := newCitationBitableServer(t, true, false)
		defer server.Close()
		client := feishu.NewClient(feishu.ClientConfig{BaseURL: server.URL}, server.Client())
		document, err := feishu.NewBitableLoader(client, feishu.BitableConfig{}).Load(context.Background(), citationRef(t, "https://acme.feishu.cn/base/baseA?table=tb1&view=vw2"), "token")
		if err != nil {
			t.Fatal(err)
		}
		assertLoaderCitations(t, document, map[string]map[string]any{
			"Alpha / Current": {"table_id": "tb1", "view_id": "vw2", "row_start": 1, "row_end": 1},
		})
	})

	t.Run("wiki delegated bitable", func(t *testing.T) {
		server := newCitationBitableServer(t, false, true)
		defer server.Close()
		wikiClient := feishu.NewClient(feishu.ClientConfig{BaseURL: server.URL}, server.Client())
		bitableClient := feishu.NewClient(feishu.ClientConfig{BaseURL: server.URL}, server.Client())
		document, err := feishu.NewWikiLoader(wikiClient, nil, nil, feishu.NewBitableLoader(bitableClient, feishu.BitableConfig{})).Load(context.Background(), citationRef(t, "https://acme.feishu.cn/wiki/wikiA"), "token")
		if err != nil {
			t.Fatal(err)
		}
		if document.SourceMetadata.Values().SourceType != domain.ResourceWiki {
			t.Fatalf("wiki source type = %q", document.SourceMetadata.Values().SourceType)
		}
		assertLoaderCitations(t, document, map[string]map[string]any{
			"Alpha": {"table_id": "tb1", "row_start": 1, "row_end": 1},
			"Beta":  {"table_id": "tb2", "row_start": 1, "row_end": 1},
		})
	})

	t.Run("wiki delegated sheet", func(t *testing.T) {
		server := newCitationSheetServer(t, true)
		defer server.Close()
		wikiClient := feishu.NewClient(feishu.ClientConfig{BaseURL: server.URL}, server.Client())
		sheetClient := feishu.NewClient(feishu.ClientConfig{BaseURL: server.URL}, server.Client())
		document, err := feishu.NewWikiLoader(wikiClient, nil, feishu.NewSheetLoader(sheetClient), nil).Load(context.Background(), citationRef(t, "https://acme.feishu.cn/wiki/wikiA"), "token")
		if err != nil {
			t.Fatal(err)
		}
		if document.SourceMetadata.Values().SourceType != domain.ResourceWiki {
			t.Fatalf("wiki source type = %q", document.SourceMetadata.Values().SourceType)
		}
		assertLoaderCitations(t, document, map[string]map[string]any{
			"First":  {"sheet_id": "sh1", "sheet_name": "First", "row_start": 1, "row_end": 2},
			"Second": {"sheet_id": "sh2", "sheet_name": "Second", "row_start": 1, "row_end": 2},
		})
	})
}

func assertLoaderCitations(t *testing.T, document domain.CanonicalDocument, expected map[string]map[string]any) {
	t.Helper()
	parsed, err := (parser.Markdown{}).Parse(context.Background(), strings.NewReader(document.Markdown), "text/markdown")
	if err != nil {
		t.Fatal(err)
	}
	pieces, err := buildChunkPieces(context.Background(), splitter.New(), parsed, 400, 60)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := canonicalMetadata(document)
	if err != nil {
		t.Fatal(err)
	}
	locationPaths := map[string]bool{}
	for _, location := range document.SourceMetadata.Values().Locations {
		locationPaths[location.SectionPath] = true
	}
	seen := map[string]bool{}
	for _, piece := range pieces {
		sectionPath, _ := piece.Metadata["section_path"].(string)
		want, ok := expected[sectionPath]
		if !ok {
			continue
		}
		seen[sectionPath] = true
		if !locationPaths[sectionPath] {
			t.Errorf("loader location paths %v do not contain emitted section %q", locationPaths, sectionPath)
		}
		assertCitationFields(t, remoteCitationMetadata(raw, sectionPath), want)
		patched := citationMetadataPatcher(raw)(map[string]any{"section_path": sectionPath, "content_hash": "keep"})
		assertCitationFields(t, patched, want)
		if patched["content_hash"] != "keep" {
			t.Errorf("same-checksum patch dropped unrelated metadata: %+v", patched)
		}
	}
	for sectionPath := range expected {
		if !seen[sectionPath] {
			t.Errorf("no chunk emitted for section %q; Markdown:\n%s", sectionPath, document.Markdown)
		}
	}
}

func assertCitationFields(t *testing.T, got, want map[string]any) {
	t.Helper()
	for key, value := range want {
		if !reflect.DeepEqual(got[key], value) {
			t.Errorf("citation[%s] = %#v, want %#v; all=%+v", key, got[key], value, got)
		}
	}
	_, wantLocations := want["locations"]
	if _, ok := got["locations"]; ok && !wantLocations {
		t.Errorf("raw locations leaked into citation metadata: %+v", got)
	}
}

func citationRef(t *testing.T, rawURL string) domain.ResourceRef {
	t.Helper()
	ref, err := feishu.NewURLResolver().Resolve(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func newCitationSheetServer(t *testing.T, wiki bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case wiki && strings.HasSuffix(r.URL.Path, "/spaces/get_node"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"node":{"node_token":"wikiA","obj_token":"book","obj_type":"sheet","title":"Wiki Sheets"}}}`))
		case strings.HasSuffix(r.URL.Path, "/spreadsheets/book"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"spreadsheet":{"token":"book","title":"Workbook"}}}`))
		case strings.HasSuffix(r.URL.Path, "/sheets/query"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"sheets":[{"sheet_id":"sh1","title":"First"},{"sheet_id":"sh2","title":"Second"}]}}`))
		case strings.HasSuffix(r.URL.Path, "/values/sh1"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"valueRange":{"revision":2,"range":"First!A1:B2","values":[["Name","Value"],["A","1"]]}}}`))
		case strings.HasSuffix(r.URL.Path, "/values/sh2"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"valueRange":{"revision":2,"range":"Second!A1:B2","values":[["Name","Value"],["B","2"]]}}}`))
		default:
			t.Fatalf("unexpected request: %s", r.URL)
		}
	}))
}

func newCitationNamedSheetServer(t *testing.T, firstName, secondName string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/spreadsheets/book"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"spreadsheet":{"token":"book","title":"Workbook"}}}`))
		case strings.HasSuffix(r.URL.Path, "/sheets/query"):
			if err := json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"sheets": []map[string]any{
					{"sheet_id": "sh1", "title": firstName},
					{"sheet_id": "sh2", "title": secondName},
				}},
			}); err != nil {
				t.Fatal(err)
			}
		case strings.HasSuffix(r.URL.Path, "/values/sh1"), strings.HasSuffix(r.URL.Path, "/values/sh2"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"valueRange":{"revision":2,"values":[["Value"]]}}}`))
		default:
			t.Fatalf("unexpected request: %s", r.URL)
		}
	}))
}

func newCitationPaginatedSheetServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/spreadsheets/book"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"spreadsheet":{"token":"book","title":"Workbook"}}}`))
		case strings.HasSuffix(r.URL.Path, "/sheets/query"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"sheets":[{"sheet_id":"sh1","title":"Paged","row_count":501}]}}`))
		case strings.HasSuffix(r.URL.Path, "/values/sh1!A1:ZZZ500"):
			writeCitationSheetPage(t, w, "Paged!A1:A500", 500)
		case strings.HasSuffix(r.URL.Path, "/values/sh1!A501:ZZZ501"):
			writeCitationSheetPage(t, w, "Paged!A501:A501", 1)
		default:
			t.Fatalf("unexpected request: %s", r.URL)
		}
	}))
}

func newCitationSparseSheetServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/spreadsheets/book"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"spreadsheet":{"token":"book","title":"Workbook"}}}`))
		case strings.HasSuffix(r.URL.Path, "/sheets/query"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"sheets":[{"sheet_id":"sh1","title":"Sparse","row_count":1000}]}}`))
		case strings.HasSuffix(r.URL.Path, "/values/sh1!A1:ZZZ500"):
			writeCitationSheetPage(t, w, "Sparse!A1:A1", 1)
		case strings.HasSuffix(r.URL.Path, "/values/sh1!A501:ZZZ1000"):
			writeCitationSheetPage(t, w, "Sparse!A750:A751", 2)
		default:
			t.Fatalf("unexpected request: %s", r.URL)
		}
	}))
}

func writeCitationSheetPage(t *testing.T, w http.ResponseWriter, cellRange string, count int) {
	t.Helper()
	values := make([][]string, count)
	for i := range values {
		values[i] = []string{"Value"}
	}
	if err := json.NewEncoder(w).Encode(map[string]any{
		"code": 0,
		"data": map[string]any{
			"valueRange": map[string]any{"revision": 2, "range": cellRange, "values": values},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func newCitationBitableServer(t *testing.T, withView, wiki bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case wiki && strings.HasSuffix(r.URL.Path, "/spaces/get_node"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"node":{"node_token":"wikiA","obj_token":"baseA","obj_type":"bitable","title":"Wiki Base"}}}`))
		case strings.HasSuffix(r.URL.Path, "/apps/baseA"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"app":{"app_token":"baseA","name":"Operations","revision":3}}}`))
		case strings.HasSuffix(r.URL.Path, "/tables"):
			if withView {
				_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"table_id":"tb1","name":"Alpha"}],"has_more":false}}`))
			} else {
				_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"table_id":"tb1","name":"Alpha"},{"table_id":"tb2","name":"Beta"}],"has_more":false}}`))
			}
		case strings.HasSuffix(r.URL.Path, "/views"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"view_id":"vw1","view_name":"All"},{"view_id":"vw2","view_name":"Current"}],"has_more":false}}`))
		case strings.HasSuffix(r.URL.Path, "/records"):
			name := "A"
			if strings.Contains(r.URL.Path, "/tb2/") {
				name = "B"
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"record_id":"rec1","fields":{"Name":"` + name + `"}}],"has_more":false}}`))
		default:
			t.Fatalf("unexpected request: %s", r.URL)
		}
	}))
}

func newCitationNamedBitableServer(t *testing.T, firstName, secondName string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/apps/baseA"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"app":{"app_token":"baseA","name":"Operations","revision":3}}}`))
		case strings.HasSuffix(r.URL.Path, "/tables"):
			if err := json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"items": []map[string]any{
						{"table_id": "tb1", "name": firstName},
						{"table_id": "tb2", "name": secondName},
					},
					"has_more": false,
				},
			}); err != nil {
				t.Fatal(err)
			}
		case strings.HasSuffix(r.URL.Path, "/records"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"record_id":"rec1","fields":{"Name":"A"}}],"has_more":false}}`))
		default:
			t.Fatalf("unexpected request: %s", r.URL)
		}
	}))
}
