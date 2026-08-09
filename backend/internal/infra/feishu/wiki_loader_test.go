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

func TestWikiLoaderResolvesAndDelegatesUnderlyingResource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		body, err := os.ReadFile("testdata/wiki_node.json")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	delegate := &recordingLoader{t: t}
	loader := NewWikiLoader(NewClient(ClientConfig{BaseURL: server.URL}, server.Client()), delegate, nil, nil)
	document, err := loader.Load(context.Background(), mustResourceRef(t, domain.ResourceWiki, "wikiA", ""), "access-token")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if delegate.ref.Type != domain.ResourceDocx || delegate.ref.Token != "docA" || delegate.token != "access-token" {
		t.Fatalf("delegation = ref %+v token %q", delegate.ref, delegate.token)
	}
	if document.Title != "Wiki 工程文档" || document.SafeSourceURL.String() != "https://acme.feishu.cn/wiki/wikiA" {
		t.Fatalf("document = %+v", document)
	}
	if metadata := document.SourceMetadata.Values(); metadata.SourceType != domain.ResourceWiki || metadata.SourceLocator.String() != "https://acme.feishu.cn/wiki/wikiA" {
		t.Fatalf("metadata = %+v", metadata)
	}
}

func TestWikiLoaderRejectsUnsupportedUnderlyingObjectWithoutDelegationCycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"node":{"node_token":"wikiA","obj_token":"wikiB","obj_type":"wiki","title":"Cycle"}}}`))
	}))
	defer server.Close()

	_, err := NewWikiLoader(NewClient(ClientConfig{BaseURL: server.URL}, server.Client()), &recordingLoader{t: t}, nil, nil).Load(context.Background(), mustResourceRef(t, domain.ResourceWiki, "wikiA", ""), "secret-token")
	var loadErr *ports.SourceLoadError
	if !errors.As(err, &loadErr) || loadErr.Code != ports.SourceLoadUnsupported {
		t.Fatalf("Load() error = %#v", err)
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error leaked token: %q", err)
	}
}

func TestWikiLoaderPreservesDelegateCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"node":{"node_token":"wikiA","obj_token":"docA","obj_type":"docx","title":"Title"}}}`))
	}))
	defer server.Close()

	_, err := NewWikiLoader(NewClient(ClientConfig{BaseURL: server.URL}, server.Client()), canceledLoader{}, nil, nil).Load(context.Background(), mustResourceRef(t, domain.ResourceWiki, "wikiA", ""), "token")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestWikiLoaderComposesStricterBitableLimits(t *testing.T) {
	for _, test := range []struct {
		name   string
		config BitableConfig
	}{
		{name: "rows", config: BitableConfig{MaxRows: 1, MaxOutputBytes: 10_000}},
		{name: "output", config: BitableConfig{MaxRows: 10, MaxOutputBytes: 20}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/spaces/get_node"):
					_, _ = w.Write([]byte(`{"code":0,"data":{"node":{"node_token":"wikiA","obj_token":"baseA","obj_type":"bitable","title":"Wiki Base"}}}`))
				case strings.HasSuffix(r.URL.Path, "/apps/baseA"):
					_, _ = w.Write([]byte(`{"code":0,"data":{"app":{"app_token":"baseA","name":"Base","revision":1}}}`))
				case strings.HasSuffix(r.URL.Path, "/tables"):
					_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"table_id":"tb1","name":"Table"}],"has_more":false}}`))
				case strings.HasSuffix(r.URL.Path, "/records"):
					_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"record_id":"rec1","fields":{"Name":"First"}},{"record_id":"rec2","fields":{"Name":"Second"}}],"has_more":false}}`))
				default:
					t.Fatalf("unexpected request: %s", r.URL)
				}
			}))
			defer server.Close()

			wikiClient := NewClient(ClientConfig{BaseURL: server.URL, ResourceLimits: ResourceLimits{MaxRows: 100, MaxOutputBytes: 100_000}}, server.Client())
			bitableClient := NewClient(ClientConfig{BaseURL: server.URL, ResourceLimits: ResourceLimits{MaxRows: 100, MaxOutputBytes: 100_000}}, server.Client())
			bitable := NewBitableLoader(bitableClient, test.config)
			_, err := NewWikiLoader(wikiClient, nil, nil, bitable).Load(context.Background(), mustResourceRef(t, domain.ResourceWiki, "wikiA", ""), "token")
			var loadErr *ports.SourceLoadError
			if !errors.As(err, &loadErr) || loadErr.Code != ports.SourceLoadTooLarge {
				t.Fatalf("Load() error = %#v, want too_large", err)
			}
		})
	}
}

func TestWikiLoaderComposesStricterDocxAndSheetLimits(t *testing.T) {
	t.Run("docx blocks", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/spaces/get_node"):
				_, _ = w.Write([]byte(`{"code":0,"data":{"node":{"node_token":"wikiA","obj_token":"docA","obj_type":"docx","title":"Wiki Doc"}}}`))
			case strings.HasSuffix(r.URL.Path, "/documents/docA"):
				_, _ = w.Write([]byte(`{"code":0,"data":{"document":{"document_id":"docA","revision_id":1,"title":"Doc"}}}`))
			case strings.HasSuffix(r.URL.Path, "/blocks"):
				_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"block_id":"one","block_type":1},{"block_id":"two","block_type":1}],"has_more":false}}`))
			default:
				t.Fatalf("unexpected request: %s", r.URL)
			}
		}))
		defer server.Close()
		wikiClient := NewClient(ClientConfig{BaseURL: server.URL, ResourceLimits: ResourceLimits{MaxBlocks: 100}}, server.Client())
		docxClient := NewClient(ClientConfig{BaseURL: server.URL, ResourceLimits: ResourceLimits{MaxBlocks: 1}}, server.Client())
		_, err := NewWikiLoader(wikiClient, NewDocxLoader(docxClient), nil, nil).Load(context.Background(), mustResourceRef(t, domain.ResourceWiki, "wikiA", ""), "token")
		assertLoadCode(t, err, ports.SourceLoadTooLarge)
	})

	t.Run("sheet rows", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/spaces/get_node"):
				_, _ = w.Write([]byte(`{"code":0,"data":{"node":{"node_token":"wikiA","obj_token":"book","obj_type":"sheet","title":"Wiki Sheet"}}}`))
			case strings.HasSuffix(r.URL.Path, "/spreadsheets/book"):
				_, _ = w.Write([]byte(`{"code":0,"data":{"spreadsheet":{"token":"book","title":"Book"}}}`))
			case strings.HasSuffix(r.URL.Path, "/sheets/query"):
				_, _ = w.Write([]byte(`{"code":0,"data":{"sheets":[{"sheet_id":"sh1","title":"Data"}]}}`))
			case strings.Contains(r.URL.Path, "/values/"):
				_, _ = w.Write([]byte(`{"code":0,"data":{"valueRange":{"revision":1,"values":[["one"],["two"]]}}}`))
			default:
				t.Fatalf("unexpected request: %s", r.URL)
			}
		}))
		defer server.Close()
		wikiClient := NewClient(ClientConfig{BaseURL: server.URL, ResourceLimits: ResourceLimits{MaxRows: 100}}, server.Client())
		sheetClient := NewClient(ClientConfig{BaseURL: server.URL, ResourceLimits: ResourceLimits{MaxRows: 1}}, server.Client())
		_, err := NewWikiLoader(wikiClient, nil, NewSheetLoader(sheetClient), nil).Load(context.Background(), mustResourceRef(t, domain.ResourceWiki, "wikiA", ""), "token")
		assertLoadCode(t, err, ports.SourceLoadTooLarge)
	})
}

type canceledLoader struct{}

func (canceledLoader) Load(context.Context, domain.ResourceRef, string) (domain.CanonicalDocument, error) {
	return domain.CanonicalDocument{}, context.Canceled
}

type recordingLoader struct {
	t     *testing.T
	ref   domain.ResourceRef
	token string
}

func (l *recordingLoader) Load(_ context.Context, ref domain.ResourceRef, token string) (domain.CanonicalDocument, error) {
	l.ref, l.token = ref, token
	metadata, err := domain.NewSourceMetadata(domain.SourceMetadataInput{SourceType: ref.Type, SectionPath: "Underlying", RemoteRevision: "3", SourceLocator: ref.CanonicalURL})
	if err != nil {
		l.t.Fatal(err)
	}
	document, err := domain.NewCanonicalDocument(domain.CanonicalDocumentInput{Title: "Underlying", Markdown: "# Underlying\n", RemoteRevision: "3", SourceMetadata: metadata, SafeSourceURL: ref.CanonicalURL})
	if err != nil {
		l.t.Fatal(err)
	}
	return document, nil
}
