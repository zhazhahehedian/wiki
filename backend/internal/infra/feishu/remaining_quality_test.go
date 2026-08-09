package feishu

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

func TestSheetRowPlanRejectsUnsafeCountsBeforeAllocation(t *testing.T) {
	budget := newResourceBudget(ResourceLimits{MaxPages: 3, MaxRows: 1_000})
	if _, err := validateSheetRowPlan(-1, budget); err == nil {
		t.Fatal("negative row_count error = nil")
	}
	for _, count := range []int{1_001, math.MaxInt} {
		_, err := validateSheetRowPlan(count, budget)
		assertLoadCode(t, err, ports.SourceLoadTooLarge)
	}
	if pages, err := validateSheetRowPlan(1_000, budget); err != nil || pages != 2 {
		t.Fatalf("boundary plan = %d, %v", pages, err)
	}
	if _, err := validateSheetRowPlan(1_001, newResourceBudget(ResourceLimits{MaxPages: 2, MaxRows: 2_000})); err == nil {
		t.Fatal("page boundary error = nil")
	}
}

func TestSheetLoaderUsesActualReturnedPhysicalRanges(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/spreadsheets/book"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"spreadsheet":{"token":"book","title":"Book"}}}`))
		case strings.HasSuffix(r.URL.Path, "/sheets/query"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"sheets":[{"sheet_id":"sh1","title":"Data","grid_properties":{"row_count":1000}}]}}`))
		case strings.HasSuffix(r.URL.Path, "A1:ZZZ500"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"valueRange":{"range":"sh1!A1:B1","revision":1,"values":[["one"]]}}}`))
		case strings.HasSuffix(r.URL.Path, "A501:ZZZ1000"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"valueRange":{"range":"sh1!A750:B751","revision":1,"values":[["later"],["last"]]}}}`))
		default:
			t.Fatalf("unexpected request: %s", r.URL)
		}
	}))
	defer server.Close()
	doc, err := NewSheetLoader(NewClient(ClientConfig{BaseURL: server.URL}, server.Client())).Load(context.Background(), mustResourceRef(t, domain.ResourceSheet, "book", "sh1"), "token")
	if err != nil {
		t.Fatal(err)
	}
	locations := doc.SourceMetadata.Values().Locations
	if len(locations) != 2 || locations[0].RowStart != 1 || locations[0].RowEnd != 1 || locations[1].RowStart != 750 || locations[1].RowEnd != 751 {
		t.Fatalf("locations = %+v", locations)
	}
	if !strings.Contains(doc.Markdown, `rows="1-1,750-751"`) {
		t.Fatalf("markdown = %s", doc.Markdown)
	}
}

func TestSheetLoaderOmitsEmptyPhysicalSegments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/spreadsheets/book"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"spreadsheet":{"token":"book","title":"Book"}}}`))
		case strings.HasSuffix(r.URL.Path, "/sheets/query"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"sheets":[{"sheet_id":"sh1","title":"Data","grid_properties":{"row_count":501}}]}}`))
		case strings.HasSuffix(r.URL.Path, "A1:ZZZ500"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"valueRange":{"values":[]}}}`))
		default:
			_, _ = w.Write([]byte(`{"code":0,"data":{"valueRange":{"range":"sh1!A501:A501","values":[["last"]]}}}`))
		}
	}))
	defer server.Close()
	doc, err := NewSheetLoader(NewClient(ClientConfig{BaseURL: server.URL}, server.Client())).Load(context.Background(), mustResourceRef(t, domain.ResourceSheet, "book", "sh1"), "token")
	if err != nil {
		t.Fatal(err)
	}
	locations := doc.SourceMetadata.Values().Locations
	if len(locations) != 1 || locations[0].RowStart != 501 || locations[0].RowEnd != 501 {
		t.Fatalf("locations = %+v", locations)
	}
}

func TestDocxLoaderChargesRetainedOrphanBlockBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/documents/docA") {
			_, _ = w.Write([]byte(`{"code":0,"data":{"document":{"document_id":"docA","revision_id":1,"title":"Doc"}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"block_id":"root","block_type":1,"page":{"elements":[]},"children":[]},{"block_id":"orphan","block_type":2,"text":{"elements":[{"text_run":{"content":"` + strings.Repeat("x", 500) + `"}}]}}],"has_more":false}}`))
	}))
	defer server.Close()
	client := NewClient(ClientConfig{BaseURL: server.URL, ResourceLimits: ResourceLimits{MaxOutputBytes: 100}}, server.Client())
	_, err := NewDocxLoader(client).Load(context.Background(), mustResourceRef(t, domain.ResourceDocx, "docA", ""), "token")
	assertLoadCode(t, err, ports.SourceLoadTooLarge)
}

func TestBitableLoaderChargesEntitiesAndNamesBeforeRecordRequests(t *testing.T) {
	for _, test := range []struct {
		name   string
		limits ResourceLimits
		tables string
	}{
		{name: "huge name", limits: ResourceLimits{MaxOutputBytes: 100}, tables: `[{"table_id":"tb1","name":"` + strings.Repeat("x", 500) + `"}]`},
		{name: "many empty tables", limits: ResourceLimits{MaxBlocks: 2}, tables: `[{"table_id":"tb1","name":"One"},{"table_id":"tb2","name":"Two"},{"table_id":"tb3","name":"Three"}]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			recordsCalled := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/apps/baseA"):
					_, _ = w.Write([]byte(`{"code":0,"data":{"app":{"app_token":"baseA","name":"Base","revision":1}}}`))
				case strings.HasSuffix(r.URL.Path, "/tables"):
					_, _ = w.Write([]byte(`{"code":0,"data":{"items":` + test.tables + `,"has_more":false}}`))
				case strings.HasSuffix(r.URL.Path, "/records"):
					recordsCalled = true
					_, _ = w.Write([]byte(`{"code":0,"data":{"items":[],"has_more":false}}`))
				default:
					t.Fatalf("unexpected request: %s", r.URL)
				}
			}))
			defer server.Close()
			client := NewClient(ClientConfig{BaseURL: server.URL, ResourceLimits: test.limits}, server.Client())
			_, err := NewBitableLoader(client, BitableConfig{}).Load(context.Background(), mustResourceRef(t, domain.ResourceBitable, "baseA", ""), "token")
			assertLoadCode(t, err, ports.SourceLoadTooLarge)
			if recordsCalled {
				t.Fatal("records requested before retained input limit")
			}
		})
	}
}

func TestDocxRendererGenericVariantsAndTodoState(t *testing.T) {
	var page docxBlocksData
	payload := `{"items":[{"block_id":"root","block_type":1,"children":["todo","future","undefined","agenda","link_preview","synced","sub_page","ai","reference","project","meeting","vc","minutes"]},{"block_id":"todo","block_type":17,"todo":{"elements":[{"text_run":{"content":"Done"}}],"style":{"done":true}}},{"block_id":"future","block_type":60,"children":["child"]},{"block_id":"child","block_type":0},{"block_id":"undefined","block_type":0},{"block_id":"agenda","block_type":51,"agenda":{}},{"block_id":"link_preview","block_type":52,"link_preview":{}},{"block_id":"synced","block_type":53,"synced":{}},{"block_id":"sub_page","block_type":54,"sub_page":{}},{"block_id":"ai","block_type":55,"ai":{}},{"block_id":"reference","block_type":56,"reference":{}},{"block_id":"project","block_type":57,"project":{}},{"block_id":"meeting","block_type":58,"meeting":{}},{"block_id":"vc","block_type":59,"vc":{}},{"block_id":"minutes","block_type":61,"minutes":{}}]}`
	if err := decodeExactJSON([]byte(payload), &page); err != nil {
		t.Fatal(err)
	}
	blocks := map[string]docxBlock{}
	for _, block := range page.Items {
		if block.BlockID == "root" {
			block.Page = &docxText{}
		}
		blocks[block.BlockID] = block
	}
	got, err := (docxRenderer{ctx: context.Background(), budget: newResourceBudget(ResourceLimits{}), blocks: blocks, normalizer: NewMarkdownNormalizer()}).renderBlock("root", 0, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"- [x] Done", "Unsupported block type 60", "Unsupported block type 0"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered %q missing %q", got, want)
		}
	}
}

func TestDocxTableTraversalCarriesParentDepth(t *testing.T) {
	table := docxBlock{BlockID: "table", BlockType: 31, Table: &struct {
		Property struct {
			RowSize    int `json:"row_size"`
			ColumnSize int `json:"column_size"`
		} `json:"property"`
		Cells []string `json:"cells"`
	}{Cells: []string{"cell"}}}
	table.Table.Property.RowSize, table.Table.Property.ColumnSize = 1, 1
	blocks := map[string]docxBlock{"root": {BlockID: "root", BlockType: 1, Page: &docxText{}, Children: []string{"table"}}, "table": table, "cell": {BlockID: "cell", BlockType: 32, Children: []string{"text"}}, "text": {BlockID: "text", BlockType: 2, Text: &docxText{}}}
	renderer := docxRenderer{ctx: context.Background(), budget: newResourceBudget(ResourceLimits{MaxDepth: 2}), blocks: blocks, normalizer: NewMarkdownNormalizer()}
	_, err := renderer.renderBlock("root", 0, map[string]bool{})
	assertLoadCode(t, err, ports.SourceLoadTooLarge)
}

func TestBitableNormalizationEnforcesDepthAndCancellation(t *testing.T) {
	deep := []any{[]any{[]any{"value"}}}
	_, err := normalizeBitableValue(context.Background(), newResourceBudget(ResourceLimits{MaxDepth: 1}), deep, 0)
	assertLoadCode(t, err, ports.SourceLoadTooLarge)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = normalizeBitableValue(ctx, newResourceBudget(ResourceLimits{}), []any{"value"}, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("normalize error = %v", err)
	}
}

func TestDocxLoaderChargesAllRetainedInlineAndStyleFields(t *testing.T) {
	for _, test := range []struct{ name, orphan string }{
		{name: "code language", orphan: `{"block_id":"orphan","block_type":14,"code":{"elements":[],"style":{"language":"` + strings.Repeat("x", 500) + `"}}}`},
		{name: "equation mention", orphan: `{"block_id":"orphan","block_type":16,"equation":{"elements":[{"mention_doc":{"title":"` + strings.Repeat("x", 500) + `"}}]}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/documents/docA") {
					_, _ = w.Write([]byte(`{"code":0,"data":{"document":{"document_id":"docA","revision_id":1,"title":"Doc"}}}`))
					return
				}
				_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"block_id":"root","block_type":1,"page":{"elements":[]}},` + test.orphan + `],"has_more":false}}`))
			}))
			defer server.Close()
			client := NewClient(ClientConfig{BaseURL: server.URL, ResourceLimits: ResourceLimits{MaxOutputBytes: 100}}, server.Client())
			_, err := NewDocxLoader(client).Load(context.Background(), mustResourceRef(t, domain.ResourceDocx, "docA", ""), "token")
			assertLoadCode(t, err, ports.SourceLoadTooLarge)
		})
	}
}

func TestDocxLoaderRejectsContainerCodeLanguage(t *testing.T) {
	language := `[[` + strings.TrimSuffix(strings.Repeat(`null,`, 2_000), ",") + `]]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/documents/docA") {
			_, _ = w.Write([]byte(`{"code":0,"data":{"document":{"document_id":"docA","revision_id":1,"title":"Doc"}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"block_id":"root","block_type":1,"page":{"elements":[]},"children":["code"]},{"block_id":"code","block_type":14,"code":{"elements":[],"style":{"language":` + language + `}}}],"has_more":false}}`))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{BaseURL: server.URL, ResourceLimits: ResourceLimits{MaxOutputBytes: 100}}, server.Client())
	_, err := NewDocxLoader(client).Load(context.Background(), mustResourceRef(t, domain.ResourceDocx, "docA", ""), "token")
	var loadErr *ports.SourceLoadError
	if !errors.As(err, &loadErr) || (loadErr.Code != ports.SourceLoadMalformed && loadErr.Code != ports.SourceLoadTooLarge) {
		t.Fatalf("Load() error = %#v, want malformed or too_large", err)
	}
}

func TestDocxCodeLanguageSupportsBoundedScalars(t *testing.T) {
	for _, test := range []struct {
		name     string
		language string
		want     string
	}{
		{name: "numeric enum", language: `1`, want: "plaintext"},
		{name: "string identifier", language: `"go"`, want: "go"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var page docxBlocksData
			payload := `{"items":[{"block_id":"code","block_type":14,"code":{"elements":[],"style":{"language":` + test.language + `}}}]}`
			if err := decodeExactJSON([]byte(payload), &page); err != nil {
				t.Fatal(err)
			}
			got, err := (docxRenderer{blocks: map[string]docxBlock{"code": page.Items[0]}, normalizer: NewMarkdownNormalizer()}).renderBlock("code", 0, map[string]bool{})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(got, "```"+test.want+"\n") {
				t.Fatalf("rendered code = %q, want language %q", got, test.want)
			}
		})
	}
}

func TestDocxCodeLanguageRejectsNullAndObjectValues(t *testing.T) {
	for _, language := range []string{`null`, `{}`} {
		var page docxBlocksData
		payload := `{"items":[{"block_id":"code","block_type":14,"code":{"elements":[],"style":{"language":` + language + `}}}]}`
		if err := decodeExactJSON([]byte(payload), &page); err == nil {
			t.Fatalf("decode language %s error = nil", language)
		}
	}
}

func TestDocxRendererUsesOfficialBlockTypes39Through43(t *testing.T) {
	for _, test := range []struct {
		blockType int
		payload   string
		want      string
	}{
		{blockType: 39, payload: `"okr_progress":{}`, want: "Unsupported OKR progress"},
		{blockType: 40, payload: `"add_ons":{}`, want: `Unsupported add\-on`},
		{blockType: 41, payload: `"jira_issue":{}`, want: "Unsupported Jira issue"},
		{blockType: 42, payload: `"wiki_catalog":{}`, want: "Unsupported wiki catalog"},
		{blockType: 43, payload: `"board":{}`, want: "Unsupported board"},
	} {
		t.Run(strconv.Itoa(test.blockType), func(t *testing.T) {
			var page docxBlocksData
			payload := `{"items":[{"block_id":"leaf","block_type":` + strconv.Itoa(test.blockType) + `,` + test.payload + `}]}`
			if err := decodeExactJSON([]byte(payload), &page); err != nil {
				t.Fatal(err)
			}
			got, err := (docxRenderer{ctx: context.Background(), budget: newResourceBudget(ResourceLimits{}), blocks: map[string]docxBlock{"leaf": page.Items[0]}, normalizer: NewMarkdownNormalizer()}).renderBlock("leaf", 0, map[string]bool{})
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("renderBlock() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDocxLoaderRejectsMultipleInlineUnionMembers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/documents/docA") {
			_, _ = w.Write([]byte(`{"code":0,"data":{"document":{"document_id":"docA","revision_id":1,"title":"Doc"}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"block_id":"root","block_type":1,"page":{"elements":[]},"children":["bad"]},{"block_id":"bad","block_type":2,"text":{"elements":[{"text_run":{"content":"visible"},"mention_doc":{"title":"also"}}]}}],"has_more":false}}`))
	}))
	defer server.Close()
	_, err := NewDocxLoader(NewClient(ClientConfig{BaseURL: server.URL}, server.Client())).Load(context.Background(), mustResourceRef(t, domain.ResourceDocx, "docA", ""), "token")
	assertLoadCode(t, err, ports.SourceLoadMalformed)
}

func TestDocxRendererRejectsKnownTypesMissingPayload(t *testing.T) {
	for _, blockType := range []int{2, 14, 16, 27, 31} {
		t.Run(strconv.Itoa(blockType), func(t *testing.T) {
			block := docxBlock{BlockID: "bad", BlockType: blockType}
			_, err := (docxRenderer{ctx: context.Background(), budget: newResourceBudget(ResourceLimits{}), blocks: map[string]docxBlock{"bad": block}, normalizer: NewMarkdownNormalizer()}).renderBlock("bad", 0, map[string]bool{})
			assertLoadCode(t, err, ports.SourceLoadMalformed)
		})
	}
}

func TestDocxTableCellsRenderValidKnownAndFutureVariants(t *testing.T) {
	table := docxBlock{BlockID: "table", BlockType: 31, Table: &struct {
		Property struct {
			RowSize    int `json:"row_size"`
			ColumnSize int `json:"column_size"`
		} `json:"property"`
		Cells []string `json:"cells"`
	}{Cells: []string{"cell"}}}
	table.Table.Property.RowSize, table.Table.Property.ColumnSize = 1, 1
	heading := &docxText{}
	heading.Elements = []docxTextElement{{TextRun: &struct {
		Content string `json:"content"`
	}{Content: "Heading"}}}
	blocks := map[string]docxBlock{
		"table":   table,
		"cell":    {BlockID: "cell", BlockType: 32, Children: []string{"heading", "divider", "future"}},
		"heading": {BlockID: "heading", BlockType: 6, Heading4: heading},
		"divider": {BlockID: "divider", BlockType: 22},
		"future":  {BlockID: "future", BlockType: 60},
	}
	got, err := (docxRenderer{ctx: context.Background(), budget: newResourceBudget(ResourceLimits{}), blocks: blocks, normalizer: NewMarkdownNormalizer()}).renderBlock("table", 0, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Heading", "---", "Unsupported block type 60"} {
		if !strings.Contains(got, want) {
			t.Fatalf("table %q missing %q", got, want)
		}
	}
}

func TestDocxUnknownInlineVariantsRenderSafePlaceholders(t *testing.T) {
	var page docxBlocksData
	payload := `{"items":[{"block_id":"paragraph","block_type":2,"text":{"elements":[{"reminder":{"access_token":"paragraph-secret"}}]}},{"block_id":"table","block_type":31,"table":{"property":{"row_size":1,"column_size":1},"cells":["cell"]}},{"block_id":"cell","block_type":32,"children":["inline"]},{"block_id":"inline","block_type":2,"text":{"elements":[{"inline_component":{"download_url":"https://secret.invalid/file"}}]}}]}`
	if err := decodeExactJSON([]byte(payload), &page); err != nil {
		t.Fatal(err)
	}
	blocks := make(map[string]docxBlock)
	for _, block := range page.Items {
		blocks[block.BlockID] = block
	}
	renderer := docxRenderer{ctx: context.Background(), budget: newResourceBudget(ResourceLimits{}), blocks: blocks, normalizer: NewMarkdownNormalizer()}
	paragraph, err := renderer.renderBlock("paragraph", 0, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	table, err := renderer.renderBlock("table", 0, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range []string{paragraph, table} {
		if !strings.Contains(got, "Unsupported inline element") {
			t.Fatalf("rendered = %q", got)
		}
		if strings.Contains(got, "secret") || strings.Contains(got, "download") {
			t.Fatalf("unknown payload leaked: %q", got)
		}
	}
}

func TestDocxUnknownInlineUnionRejectsEmptyNullAndMultipleMembers(t *testing.T) {
	for _, element := range []string{
		`{}`,
		`{"reminder":null}`,
		`{"text_run":{"content":"known"},"reminder":{"value":"unknown"}}`,
		`{"reminder":{"value":"one"},"inline_component":{"value":"two"}}`,
	} {
		var page docxBlocksData
		payload := `{"items":[{"block_id":"bad","block_type":2,"text":{"elements":[` + element + `]}}]}`
		if err := decodeExactJSON([]byte(payload), &page); err != nil {
			t.Fatal(err)
		}
		_, err := (docxRenderer{ctx: context.Background(), budget: newResourceBudget(ResourceLimits{}), blocks: map[string]docxBlock{"bad": page.Items[0]}, normalizer: NewMarkdownNormalizer()}).renderBlock("bad", 0, map[string]bool{})
		assertLoadCode(t, err, ports.SourceLoadMalformed)
	}
}
