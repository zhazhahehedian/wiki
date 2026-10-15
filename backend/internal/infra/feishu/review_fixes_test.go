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
		"a": {BlockID: "a", BlockType: 1, Page: &docxText{}, Children: []string{"b"}},
		"b": {BlockID: "b", BlockType: 1, Page: &docxText{}, Children: []string{"c"}},
		"c": {BlockID: "c", BlockType: 1, Page: &docxText{}},
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
