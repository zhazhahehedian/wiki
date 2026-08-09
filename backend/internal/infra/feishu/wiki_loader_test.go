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
