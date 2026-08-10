package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

func TestDocxLoaderTraversesPaginatedBlocksRecursively(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var fixture string
		switch {
		case strings.HasSuffix(r.URL.Path, "/documents/docA"):
			fixture = "testdata/docx_document.json"
		case r.URL.Query().Get("page_token") == "page-2":
			fixture = "testdata/docx_blocks_page2.json"
		default:
			fixture = "testdata/docx_blocks_page1.json"
		}
		body, err := os.ReadFile(fixture)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	loader := NewDocxLoader(NewClient(ClientConfig{BaseURL: server.URL}, server.Client()))
	document, err := loader.Load(context.Background(), mustResourceRef(t, domain.ResourceDocx, "docA", ""), "access-token")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	wantFragments := []string{
		"# 概览 \\*安全\\*", "Unicode 内容", "- 父项", "  1. 子项", "> 引用",
		"```go\nfmt.Println(\"hi\")\n```", "| Name | Value |", "| A\\|B | 1 |",
		"> [Image: unsupported image](https://acme.feishu.cn/docx/docA)",
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(document.Markdown, fragment) {
			t.Fatalf("Markdown missing %q:\n%s", fragment, document.Markdown)
		}
	}
	if document.Title != "工程文档" || document.RemoteRevision != "42" {
		t.Fatalf("document identity = %+v", document)
	}
	metadata := document.SourceMetadata.Values()
	if metadata.SourceType != domain.ResourceDocx || metadata.SectionPath != "工程文档" {
		t.Fatalf("metadata = %+v", metadata)
	}
}

func TestDocxLoaderRejectsMalformedBlock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/documents/docA") {
			_, _ = w.Write([]byte(`{"code":0,"data":{"document":{"document_id":"docA","revision_id":1,"title":"Title"}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"block_id":"bad","block_type":999}],"has_more":false}}`))
	}))
	defer server.Close()

	_, err := NewDocxLoader(NewClient(ClientConfig{BaseURL: server.URL}, server.Client())).Load(context.Background(), mustResourceRef(t, domain.ResourceDocx, "docA", ""), "token")
	var loadErr *ports.SourceLoadError
	if !errors.As(err, &loadErr) || loadErr.Code != ports.SourceLoadMalformed {
		t.Fatalf("Load() error = %#v", err)
	}
}

func TestDocxCodeBlockAcceptsNumericProviderLanguageEnum(t *testing.T) {
	var page docxBlocksData
	payload := strings.ReplaceAll(`{"items":[{"block_id":"code","block_type":14,"code":{"elements":[{"text_run":{"content":"CODE"}}],"style":{"language":1}}}]}`, "CODE", "x```y")
	err := json.Unmarshal([]byte(payload), &page)
	if err != nil {
		t.Fatalf("decode block: %v", err)
	}
	rendered, err := (docxRenderer{blocks: map[string]docxBlock{"code": page.Items[0]}, normalizer: NewMarkdownNormalizer()}).renderBlock("code", 0, map[string]bool{})
	if err != nil || rendered != "````plaintext\nx```y\n````" {
		t.Fatalf("renderBlock() = %q, %v", rendered, err)
	}
}

func TestDocxTableCellImageRetainsSafeSourceLink(t *testing.T) {
	table := docxBlock{BlockID: "table", BlockType: 31}
	table.Table = &struct {
		Property struct {
			RowSize    int `json:"row_size"`
			ColumnSize int `json:"column_size"`
		} `json:"property"`
		Cells []string `json:"cells"`
	}{}
	table.Table.Property.RowSize = 1
	table.Table.Property.ColumnSize = 1
	table.Table.Cells = []string{"cell"}
	cell := docxBlock{BlockID: "cell", BlockType: 32, Children: []string{"image"}}
	image := docxBlock{BlockID: "image", BlockType: 27, Image: &struct {
		Token string `json:"token"`
	}{Token: "private-image-token"}}
	renderer := docxRenderer{
		blocks:     map[string]docxBlock{"table": table, "cell": cell, "image": image},
		normalizer: NewMarkdownNormalizer(), sourceURL: "https://acme.feishu.cn/docx/docA",
	}

	got, err := renderer.renderBlock("table", 0, map[string]bool{})
	if err != nil {
		t.Fatalf("renderBlock() error = %v", err)
	}
	want := "| [Image: unsupported image](https://acme.feishu.cn/docx/docA) |\n| --- |"
	if got != want {
		t.Fatalf("renderBlock() = %q, want %q", got, want)
	}
	if strings.Contains(got, "private-image-token") {
		t.Fatalf("renderBlock() leaked image token: %q", got)
	}
}

func mustResourceRef(t *testing.T, resourceType domain.ResourceType, token, selector string) domain.ResourceRef {
	t.Helper()
	path := map[domain.ResourceType]string{domain.ResourceDocx: "docx", domain.ResourceSheet: "sheets", domain.ResourceBitable: "base", domain.ResourceWiki: "wiki"}[resourceType]
	raw := "https://acme.feishu.cn/" + path + "/" + token
	input := domain.ResourceRefInput{Type: resourceType, ProviderHost: "feishu.cn", Token: token}
	if resourceType == domain.ResourceSheet && selector != "" {
		input.SheetID = selector
		raw += "?sheet=" + url.QueryEscape(selector)
	}
	if resourceType == domain.ResourceBitable && selector != "" {
		input.TableID = selector
		raw += "?table=" + url.QueryEscape(selector)
	}
	safe, err := domain.NewSafeURL(raw)
	if err != nil {
		t.Fatal(err)
	}
	input.CanonicalURL, input.OriginalURL = safe, safe
	ref, err := domain.NewResourceRef(input)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}
