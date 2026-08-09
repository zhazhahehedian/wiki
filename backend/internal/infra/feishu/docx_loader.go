package feishu

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type DocxLoader struct {
	client     *Client
	normalizer MarkdownNormalizer
}

func NewDocxLoader(client *Client) *DocxLoader {
	return &DocxLoader{client: client, normalizer: NewMarkdownNormalizer()}
}

type docxDocumentData struct {
	Document struct {
		DocumentID string `json:"document_id"`
		RevisionID int64  `json:"revision_id"`
		Title      string `json:"title"`
	} `json:"document"`
}

type docxBlocksData struct {
	Items     []docxBlock `json:"items"`
	HasMore   bool        `json:"has_more"`
	PageToken string      `json:"page_token"`
}

type docxBlock struct {
	BlockID   string    `json:"block_id"`
	BlockType int       `json:"block_type"`
	Children  []string  `json:"children"`
	Text      *docxText `json:"text"`
	Page      *docxText `json:"page"`
	Heading1  *docxText `json:"heading1"`
	Heading2  *docxText `json:"heading2"`
	Heading3  *docxText `json:"heading3"`
	Heading4  *docxText `json:"heading4"`
	Heading5  *docxText `json:"heading5"`
	Heading6  *docxText `json:"heading6"`
	Heading7  *docxText `json:"heading7"`
	Heading8  *docxText `json:"heading8"`
	Heading9  *docxText `json:"heading9"`
	Bullet    *docxText `json:"bullet"`
	Ordered   *docxText `json:"ordered"`
	Quote     *docxText `json:"quote"`
	Code      *docxText `json:"code"`
	Table     *struct {
		Property struct {
			RowSize    int `json:"row_size"`
			ColumnSize int `json:"column_size"`
		} `json:"property"`
		Cells []string `json:"cells"`
	} `json:"table"`
	Image *struct {
		Token string `json:"token"`
	} `json:"image"`
}

type docxText struct {
	Elements []docxTextElement `json:"elements"`
	Style    struct {
		Language any `json:"language"`
	} `json:"style"`
}

type docxTextElement struct {
	TextRun *struct {
		Content string `json:"content"`
	} `json:"text_run"`
	MentionUser *struct {
		UserID string `json:"user_id"`
	} `json:"mention_user"`
	MentionDoc *struct {
		Title string `json:"title"`
	} `json:"mention_doc"`
	Equation *struct {
		Content string `json:"content"`
	} `json:"equation"`
	File *struct {
		Name string `json:"name"`
	} `json:"file"`
}

func (l *DocxLoader) Load(ctx context.Context, ref domain.ResourceRef, accessToken string) (domain.CanonicalDocument, error) {
	if l == nil || l.client == nil || ref.Type != domain.ResourceDocx {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	basePath := "/open-apis/docx/v1/documents/" + ref.Token
	var info docxDocumentData
	if err := l.client.Get(ctx, accessToken, basePath, nil, &info); err != nil {
		return domain.CanonicalDocument{}, err
	}
	if info.Document.DocumentID == "" || strings.TrimSpace(info.Document.Title) == "" || info.Document.RevisionID <= 0 {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}

	blocks := make(map[string]docxBlock)
	order := make([]string, 0)
	tracker := newPageTokenTracker()
	pageToken := ""
	for {
		query := url.Values{"page_size": {"500"}}
		if pageToken != "" {
			query.Set("page_token", pageToken)
		}
		var page docxBlocksData
		if err := l.client.Get(ctx, accessToken, basePath+"/blocks", query, &page); err != nil {
			return domain.CanonicalDocument{}, err
		}
		for _, block := range page.Items {
			if block.BlockID == "" {
				return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
			}
			if _, exists := blocks[block.BlockID]; exists {
				return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
			}
			blocks[block.BlockID] = block
			order = append(order, block.BlockID)
		}
		if !page.HasMore {
			break
		}
		if page.PageToken == "" || tracker.Advance(page.PageToken) != nil {
			return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
		}
		pageToken = page.PageToken
	}

	renderer := docxRenderer{blocks: blocks, normalizer: l.normalizer, sourceURL: ref.CanonicalURL.String()}
	parts, err := renderer.renderRoots(order)
	if err != nil {
		return domain.CanonicalDocument{}, err
	}
	metadata, err := domain.NewSourceMetadata(domain.SourceMetadataInput{
		SourceType: domain.ResourceDocx, SectionPath: info.Document.Title,
		RemoteRevision: strconv.FormatInt(info.Document.RevisionID, 10), SourceLocator: ref.CanonicalURL,
	})
	if err != nil {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	document, err := domain.NewCanonicalDocument(domain.CanonicalDocumentInput{
		Title: info.Document.Title, Markdown: l.normalizer.Finalize(parts),
		RemoteRevision: strconv.FormatInt(info.Document.RevisionID, 10),
		SourceMetadata: metadata, SafeSourceURL: ref.CanonicalURL,
	})
	if err != nil {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	return document, nil
}

type docxRenderer struct {
	blocks     map[string]docxBlock
	normalizer MarkdownNormalizer
	sourceURL  string
}

func (r docxRenderer) renderRoots(order []string) ([]string, error) {
	for _, id := range order {
		if block := r.blocks[id]; block.Page != nil || block.BlockType == 1 {
			return r.renderChildren(block.Children, 0, make(map[string]bool))
		}
	}
	referenced := make(map[string]bool)
	for _, block := range r.blocks {
		for _, id := range block.Children {
			referenced[id] = true
		}
		if block.Table != nil {
			for _, id := range block.Table.Cells {
				referenced[id] = true
			}
		}
	}
	roots := make([]string, 0)
	for _, id := range order {
		if !referenced[id] {
			roots = append(roots, id)
		}
	}
	return r.renderChildren(roots, 0, make(map[string]bool))
}

func (r docxRenderer) renderChildren(ids []string, depth int, stack map[string]bool) ([]string, error) {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		part, err := r.renderBlock(id, depth, stack)
		if err != nil {
			return nil, err
		}
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts, nil
}

func (r docxRenderer) renderBlock(id string, depth int, stack map[string]bool) (string, error) {
	block, ok := r.blocks[id]
	if !ok || stack[id] {
		return "", ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	stack[id] = true
	defer delete(stack, id)
	text := func(value *docxText) string { return docxPlainText(value) }
	var rendered string
	switch {
	case block.Text != nil:
		rendered = r.normalizer.Paragraph(text(block.Text))
	case block.Heading1 != nil:
		rendered = r.normalizer.Heading(1, text(block.Heading1))
	case block.Heading2 != nil:
		rendered = r.normalizer.Heading(2, text(block.Heading2))
	case block.Heading3 != nil:
		rendered = r.normalizer.Heading(3, text(block.Heading3))
	case block.Heading4 != nil:
		rendered = r.normalizer.Heading(4, text(block.Heading4))
	case block.Heading5 != nil:
		rendered = r.normalizer.Heading(5, text(block.Heading5))
	case block.Heading6 != nil:
		rendered = r.normalizer.Heading(6, text(block.Heading6))
	case block.Heading7 != nil:
		rendered = r.normalizer.Heading(6, text(block.Heading7))
	case block.Heading8 != nil:
		rendered = r.normalizer.Heading(6, text(block.Heading8))
	case block.Heading9 != nil:
		rendered = r.normalizer.Heading(6, text(block.Heading9))
	case block.Bullet != nil:
		rendered = strings.Repeat("  ", depth) + "- " + r.normalizer.Paragraph(text(block.Bullet))
	case block.Ordered != nil:
		rendered = strings.Repeat("  ", depth) + "1. " + r.normalizer.Paragraph(text(block.Ordered))
	case block.Quote != nil:
		rendered = "> " + r.normalizer.Paragraph(text(block.Quote))
	case block.Code != nil:
		content := text(block.Code)
		fence := codeFence(content)
		rendered = fence + docxCodeLanguage(block.Code.Style.Language) + "\n" + content + "\n" + fence
	case block.Table != nil:
		rows, err := r.renderTable(block, stack)
		if err != nil {
			return "", err
		}
		rendered = r.normalizer.Table(rows)
	case block.Image != nil:
		rendered = r.normalizer.UnsupportedImage("", r.sourceURL)
	case block.BlockType == 22:
		rendered = "---"
	case block.BlockType == 1 || block.BlockType == 32:
		// Containers only render their children.
	default:
		return "", ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	if len(block.Children) > 0 {
		children, err := r.renderChildren(block.Children, depth+1, stack)
		if err != nil {
			return "", err
		}
		if len(children) > 0 {
			if rendered == "" {
				rendered = strings.Join(children, "\n")
			} else {
				rendered += "\n" + strings.Join(children, "\n")
			}
		}
	}
	return rendered, nil
}

func (r docxRenderer) renderTable(block docxBlock, stack map[string]bool) ([][]string, error) {
	rows, columns := block.Table.Property.RowSize, block.Table.Property.ColumnSize
	if rows <= 0 || columns <= 0 || len(block.Table.Cells) != rows*columns {
		return nil, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	result := make([][]string, rows)
	for row := 0; row < rows; row++ {
		result[row] = make([]string, columns)
		for column := 0; column < columns; column++ {
			cellID := block.Table.Cells[row*columns+column]
			cell, ok := r.blocks[cellID]
			if !ok || cell.BlockType != 32 {
				return nil, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
			}
			parts, err := r.plainChildren(cell.Children, stack)
			if err != nil {
				return nil, err
			}
			result[row][column] = strings.Join(parts, " ")
		}
	}
	return result, nil
}

func (r docxRenderer) plainChildren(ids []string, stack map[string]bool) ([]string, error) {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		block, ok := r.blocks[id]
		if !ok || stack[id] {
			return nil, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
		}
		stack[id] = true
		var value string
		switch {
		case block.Text != nil:
			value = docxPlainText(block.Text)
		case block.Heading1 != nil:
			value = docxPlainText(block.Heading1)
		case block.Heading2 != nil:
			value = docxPlainText(block.Heading2)
		case block.Heading3 != nil:
			value = docxPlainText(block.Heading3)
		case block.Bullet != nil:
			value = docxPlainText(block.Bullet)
		case block.Ordered != nil:
			value = docxPlainText(block.Ordered)
		case block.Quote != nil:
			value = docxPlainText(block.Quote)
		case block.Code != nil:
			value = docxPlainText(block.Code)
		case block.Image != nil:
			value = "unsupported image"
		case block.BlockType != 32 && block.BlockType != 1:
			delete(stack, id)
			return nil, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
		}
		children, err := r.plainChildren(block.Children, stack)
		delete(stack, id)
		if err != nil {
			return nil, err
		}
		if value != "" {
			parts = append(parts, value)
		}
		parts = append(parts, children...)
	}
	return parts, nil
}

func docxCodeLanguage(value any) string {
	language, ok := value.(string)
	if ok && resourceIdentifierPattern.MatchString(language) {
		return language
	}
	if number, ok := value.(float64); ok && number == 1 {
		return "plaintext"
	}
	return ""
}

func codeFence(content string) string {
	longest, current := 0, 0
	for _, char := range content {
		if char == '`' {
			current++
			if current > longest {
				longest = current
			}
		} else {
			current = 0
		}
	}
	if longest < 3 {
		longest = 2
	}
	return strings.Repeat("`", longest+1)
}

func docxPlainText(value *docxText) string {
	if value == nil {
		return ""
	}
	var builder strings.Builder
	for _, element := range value.Elements {
		switch {
		case element.TextRun != nil:
			builder.WriteString(element.TextRun.Content)
		case element.MentionUser != nil:
			builder.WriteString("@")
			builder.WriteString(element.MentionUser.UserID)
		case element.MentionDoc != nil:
			builder.WriteString(element.MentionDoc.Title)
		case element.Equation != nil:
			builder.WriteString(element.Equation.Content)
		case element.File != nil:
			builder.WriteString("[File: ")
			builder.WriteString(element.File.Name)
			builder.WriteString("]")
		default:
			builder.WriteString("[Unsupported inline element]")
		}
	}
	return builder.String()
}

var _ ports.SourceLoader = (*DocxLoader)(nil)
