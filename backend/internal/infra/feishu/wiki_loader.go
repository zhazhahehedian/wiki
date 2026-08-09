package feishu

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type WikiLoader struct {
	client  *Client
	docx    ports.SourceLoader
	sheet   ports.SourceLoader
	bitable ports.SourceLoader
}

func NewWikiLoader(client *Client, docx, sheet, bitable ports.SourceLoader) *WikiLoader {
	return &WikiLoader{client: client, docx: docx, sheet: sheet, bitable: bitable}
}

type wikiNodeData struct {
	Node struct {
		NodeToken string `json:"node_token"`
		ObjToken  string `json:"obj_token"`
		ObjType   string `json:"obj_type"`
		Title     string `json:"title"`
	} `json:"node"`
}

func (l *WikiLoader) Load(ctx context.Context, ref domain.ResourceRef, accessToken string) (domain.CanonicalDocument, error) {
	if l == nil || l.client == nil || ref.Type != domain.ResourceWiki {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	var node wikiNodeData
	query := url.Values{"token": {ref.Token}}
	if err := l.client.Get(ctx, accessToken, "/open-apis/wiki/v2/spaces/get_node", query, &node); err != nil {
		return domain.CanonicalDocument{}, err
	}
	if node.Node.NodeToken == "" || node.Node.ObjToken == "" || strings.TrimSpace(node.Node.Title) == "" {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}

	resourceType, path, delegate := l.delegateFor(node.Node.ObjType)
	if delegate == nil {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadUnsupported, nil)
	}
	parsed, err := url.Parse(ref.CanonicalURL.String())
	if err != nil {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	underlyingURL, err := domain.NewSafeURL("https://" + parsed.Hostname() + "/" + path + "/" + node.Node.ObjToken)
	if err != nil {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	underlyingRef, err := domain.NewResourceRef(domain.ResourceRefInput{
		Type: resourceType, ProviderHost: ref.ProviderHost, Tenant: ref.Tenant, Token: node.Node.ObjToken,
		CanonicalURL: underlyingURL, OriginalURL: underlyingURL,
	})
	if err != nil {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	document, err := delegate.Load(ctx, underlyingRef, accessToken)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return domain.CanonicalDocument{}, err
		}
		var typed *ports.SourceLoadError
		if errors.As(err, &typed) {
			return domain.CanonicalDocument{}, err
		}
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadAPIError, nil)
	}

	values := document.SourceMetadata.Values()
	values.SourceType = domain.ResourceWiki
	values.SectionPath = node.Node.Title
	values.RemoteRevision = document.RemoteRevision
	values.SourceLocator = ref.CanonicalURL
	metadata, err := domain.NewSourceMetadata(values)
	if err != nil {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	canonical, err := domain.NewCanonicalDocument(domain.CanonicalDocumentInput{
		Title: node.Node.Title, Markdown: document.Markdown, RemoteRevision: document.RemoteRevision,
		SourceMetadata: metadata, SafeSourceURL: ref.CanonicalURL,
	})
	if err != nil {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	return canonical, nil
}

func (l *WikiLoader) delegateFor(objectType string) (domain.ResourceType, string, ports.SourceLoader) {
	switch objectType {
	case "docx":
		return domain.ResourceDocx, "docx", l.docx
	case "sheet":
		return domain.ResourceSheet, "sheets", l.sheet
	case "bitable":
		return domain.ResourceBitable, "base", l.bitable
	default:
		return "", "", nil
	}
}

var _ ports.SourceLoader = (*WikiLoader)(nil)
