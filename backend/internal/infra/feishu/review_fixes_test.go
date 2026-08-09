package feishu

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

func TestDocxLoaderEnforcesCumulativePageLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/documents/docA") {
			_, _ = w.Write([]byte(`{"code":0,"data":{"document":{"document_id":"docA","revision_id":1,"title":"Title"}}}`))
			return
		}
		next := r.URL.Query().Get("page_token") + "x"
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[],"has_more":true,"page_token":"` + next + `"}}`))
	}))
	defer server.Close()
	client := NewClient(ClientConfig{BaseURL: server.URL, ResourceLimits: ResourceLimits{MaxPages: 2}}, server.Client())
	_, err := NewDocxLoader(client).Load(context.Background(), mustResourceRef(t, domain.ResourceDocx, "docA", ""), "token")
	assertLoadCode(t, err, ports.SourceLoadTooLarge)
}

func TestLoadersEnforceSharedCumulativeBudgets(t *testing.T) {
	t.Run("sheet rows", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/spreadsheets/book"):
				_, _ = w.Write([]byte(`{"code":0,"data":{"spreadsheet":{"token":"book","title":"Book"}}}`))
			case strings.HasSuffix(r.URL.Path, "/sheets/query"):
				_, _ = w.Write([]byte(`{"code":0,"data":{"sheets":[{"sheet_id":"sh1","title":"Data"}]}}`))
			default:
				_, _ = w.Write([]byte(`{"code":0,"data":{"valueRange":{"revision":1,"values":[["a"],["b"]]}}}`))
			}
		}))
		defer server.Close()
		client := NewClient(ClientConfig{BaseURL: server.URL, ResourceLimits: ResourceLimits{MaxRows: 1}}, server.Client())
		_, err := NewSheetLoader(client).Load(context.Background(), mustResourceRef(t, domain.ResourceSheet, "book", "sh1"), "token")
		assertLoadCode(t, err, ports.SourceLoadTooLarge)
	})

	t.Run("bitable pages", func(t *testing.T) {
		server := newBitableFixtureServer(t)
		defer server.Close()
		client := NewClient(ClientConfig{BaseURL: server.URL, ResourceLimits: ResourceLimits{MaxPages: 1}}, server.Client())
		_, err := NewBitableLoader(client, BitableConfig{}).Load(context.Background(), mustResourceRef(t, domain.ResourceBitable, "baseA", "tb1"), "token")
		assertLoadCode(t, err, ports.SourceLoadTooLarge)
	})

	t.Run("docx blocks", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/documents/docA") {
				_, _ = w.Write([]byte(`{"code":0,"data":{"document":{"document_id":"docA","revision_id":1,"title":"Title"}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"block_id":"one","block_type":1},{"block_id":"two","block_type":1}],"has_more":false}}`))
		}))
		defer server.Close()
		client := NewClient(ClientConfig{BaseURL: server.URL, ResourceLimits: ResourceLimits{MaxBlocks: 1}}, server.Client())
		_, err := NewDocxLoader(client).Load(context.Background(), mustResourceRef(t, domain.ResourceDocx, "docA", ""), "token")
		assertLoadCode(t, err, ports.SourceLoadTooLarge)
	})
}

func TestDocxRendererHonorsDepthAndCancellation(t *testing.T) {
	blocks := map[string]docxBlock{
		"a": {BlockID: "a", BlockType: 1, Children: []string{"b"}},
		"b": {BlockID: "b", BlockType: 1, Children: []string{"c"}},
		"c": {BlockID: "c", BlockType: 1},
	}
	renderer := docxRenderer{ctx: context.Background(), budget: newResourceBudget(ResourceLimits{MaxDepth: 1}), blocks: blocks, normalizer: NewMarkdownNormalizer()}
	_, err := renderer.renderBlock("a", 0, map[string]bool{})
	assertLoadCode(t, err, ports.SourceLoadTooLarge)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	renderer.ctx, renderer.budget = canceled, newResourceBudget(ResourceLimits{})
	_, err = renderer.renderBlock("a", 0, map[string]bool{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("renderBlock() error = %v, want context canceled", err)
	}
}

func TestSheetLoaderUsesOfficialQueryShapeAndPhysicalSegments(t *testing.T) {
	var valuePaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/spreadsheets/book"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"spreadsheet":{"token":"book","title":"Book"}}}`))
		case strings.HasSuffix(r.URL.Path, "/sheets/query"):
			if r.URL.RawQuery != "" {
				t.Fatalf("query endpoint received invented pagination: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"sheets":[{"sheet_id":"sh1","title":"Data","grid_properties":{"row_count":501}}]}}`))
		case strings.Contains(r.URL.Path, "/values/"):
			valuePaths = append(valuePaths, r.URL.Path)
			if strings.HasSuffix(r.URL.Path, "A1:ZZZ500") {
				_, _ = w.Write([]byte(`{"code":0,"data":{"valueRange":{"revision":9,"values":[["first"]]}}}`))
			} else {
				_, _ = w.Write([]byte(`{"code":0,"data":{"valueRange":{"revision":9,"values":[["last"]]}}}`))
			}
		default:
			t.Fatalf("unexpected request %s", r.URL)
		}
	}))
	defer server.Close()
	doc, err := NewSheetLoader(NewClient(ClientConfig{BaseURL: server.URL}, server.Client())).Load(context.Background(), mustResourceRef(t, domain.ResourceSheet, "book", "sh1"), "token")
	if err != nil {
		t.Fatal(err)
	}
	if len(valuePaths) != 2 || !strings.HasSuffix(valuePaths[0], "A1:ZZZ500") || !strings.HasSuffix(valuePaths[1], "A501:ZZZ501") {
		t.Fatalf("value paths = %v", valuePaths)
	}
	locations := doc.SourceMetadata.Values().Locations
	if len(locations) != 2 || locations[0].RowStart != 1 || locations[0].RowEnd != 1 || locations[1].RowStart != 501 || locations[1].RowEnd != 501 {
		t.Fatalf("locations = %+v", locations)
	}
}

func TestBitableOmitsEmptyViewAndAllowListsStableValues(t *testing.T) {
	if got := flattenBitableValue(map[string]any{
		"text": "Visible", "name": "Display", "file_token": "secret-token", "record_id": "rec-secret",
		"url": "https://example.com/permanent", "download_url": "https://example.com/tmp?token=secret",
	}); strings.Contains(got, "secret") || strings.Contains(got, "rec-") || strings.Contains(got, "tmp") || !strings.Contains(got, "Visible") || !strings.Contains(got, "https://example.com/permanent") {
		t.Fatalf("flattenBitableValue() = %q", got)
	}
	server := newBitableFixtureServer(t)
	defer server.Close()
	client := NewClient(ClientConfig{BaseURL: server.URL}, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/records") && r.URL.Query().Has("view_id") {
			t.Fatalf("empty view_id query was sent: %s", r.URL.RawQuery)
		}
		return server.Client().Transport.RoundTrip(r)
	})})
	_, err := NewBitableLoader(client, BitableConfig{}).Load(context.Background(), mustResourceRef(t, domain.ResourceBitable, "baseA", "tb1"), "token")
	if err != nil {
		t.Fatal(err)
	}
}

func TestBitableMetadataUsesPerSectionLocationsOnly(t *testing.T) {
	server := newBitableFixtureServer(t)
	defer server.Close()
	doc, err := NewBitableLoader(NewClient(ClientConfig{BaseURL: server.URL}, server.Client()), BitableConfig{}).Load(context.Background(), mustResourceRef(t, domain.ResourceBitable, "baseA", ""), "token")
	if err != nil {
		t.Fatal(err)
	}
	metadata := doc.SourceMetadata.Values()
	if metadata.RowStart != 0 || metadata.RowEnd != 0 || len(metadata.Locations) < 2 {
		t.Fatalf("metadata = %+v", metadata)
	}
	for _, location := range metadata.Locations {
		if location.TableID == "" || location.SheetID != "" {
			t.Fatalf("location = %+v", location)
		}
	}
}

func TestDocxRendererAcceptsDocumentedContainersAndLeaves(t *testing.T) {
	var page docxBlocksData
	payload := `{"items":[
		{"block_id":"root","block_type":34,"quote_container":{},"children":["todo","equation","file","grid","sheet","chat"]},
		{"block_id":"todo","block_type":17,"todo":{"elements":[{"text_run":{"content":"Do it"}}]}},
		{"block_id":"equation","block_type":16,"equation":{"elements":[{"text_run":{"content":"x^2"}}]}},
		{"block_id":"file","block_type":23,"file":{"name":"report.pdf"}},
		{"block_id":"grid","block_type":24,"grid":{},"children":[]},
		{"block_id":"sheet","block_type":30,"sheet":{}},
		{"block_id":"chat","block_type":20,"chat_card":{}}
	]}`
	if err := decodeExactJSON([]byte(payload), &page); err != nil {
		t.Fatal(err)
	}
	blocks := make(map[string]docxBlock)
	for _, block := range page.Items {
		blocks[block.BlockID] = block
	}
	got, err := (docxRenderer{ctx: context.Background(), budget: newResourceBudget(ResourceLimits{}), blocks: blocks, normalizer: NewMarkdownNormalizer()}).renderBlock("root", 0, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Do it", "x^2", "report.pdf", "Unsupported sheet", "Unsupported chat card"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered %q missing %q", got, want)
		}
	}
}

func assertLoadCode(t *testing.T, err error, code ports.SourceLoadErrorCode) {
	t.Helper()
	var loadErr *ports.SourceLoadError
	if !errors.As(err, &loadErr) || loadErr.Code != code {
		t.Fatalf("error = %#v, want %s", err, code)
	}
}
