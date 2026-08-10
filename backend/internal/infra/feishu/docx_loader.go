package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
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
	Todo      *docxText `json:"todo"`
	Equation  *struct {
		Content  string            `json:"content"`
		Elements []docxTextElement `json:"elements"`
	} `json:"equation"`
	File *struct {
		Name string `json:"name"`
	} `json:"file"`
	Callout        *struct{} `json:"callout"`
	Grid           *struct{} `json:"grid"`
	GridColumn     *struct{} `json:"grid_column"`
	QuoteContainer *struct{} `json:"quote_container"`
	Sheet          *struct{} `json:"sheet"`
	Bitable        *struct{} `json:"bitable"`
	ChatCard       *struct{} `json:"chat_card"`
	Diagram        *struct{} `json:"diagram"`
	Iframe         *struct{} `json:"iframe"`
	ISV            *struct{} `json:"isv"`
	Mindnote       *struct{} `json:"mindnote"`
	View           *struct{} `json:"view"`
	Task           *struct{} `json:"task"`
	OKR            *struct{} `json:"okr"`
	OKRObjective   *struct{} `json:"okr_objective"`
	OKRKeyResult   *struct{} `json:"okr_key_result"`
	OKRProgress    *struct{} `json:"okr_progress"`
	AddOns         *struct{} `json:"add_ons"`
	JiraIssue      *struct{} `json:"jira_issue"`
	WikiCatalog    *struct{} `json:"wiki_catalog"`
	Board          *struct{} `json:"board"`
	Table          *struct {
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
		Language docxLanguage `json:"language"`
		Done     bool         `json:"done"`
	} `json:"style"`
}

type docxLanguage struct {
	value         string
	retainedBytes int
}

const (
	docxInlineElementStructuralBytes = 64
	docxKnownInlinePayloadBytes      = 16
)

func (l *docxLanguage) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return errors.New("empty code language")
	}
	if data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*l = docxLanguage{retainedBytes: len(value)}
		if len(value) <= maxResourceIdentifierBytes && resourceIdentifierPattern.MatchString(value) {
			l.value = value
		}
		return nil
	}
	number, err := strconv.ParseFloat(string(data), 64)
	if err != nil {
		return errors.New("unsupported code language")
	}
	*l = docxLanguage{retainedBytes: 8}
	if number == 1 {
		l.value = "plaintext"
	}
	return nil
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
	unknownMembers int
}

func (e *docxTextElement) UnmarshalJSON(data []byte) error {
	type knownElement docxTextElement
	var known knownElement
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(data, &members); err != nil {
		return err
	}
	unknownMembers := 0
	for key, payload := range members {
		switch key {
		case "text_run", "mention_user", "mention_doc", "equation", "file":
			continue
		}
		if !bytes.Equal(bytes.TrimSpace(payload), []byte("null")) {
			unknownMembers++
		}
	}
	*e = docxTextElement(known)
	e.unknownMembers = unknownMembers
	return nil
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
	budget, err := resourceBudgetFromContext(ctx, l.client.resourceLimits)
	if err != nil {
		return domain.CanonicalDocument{}, err
	}
	if err := budget.RetainedBytes(len(info.Document.DocumentID) + len(info.Document.Title)); err != nil {
		return domain.CanonicalDocument{}, err
	}
	tracker := newPageTokenTracker()
	pageToken := ""
	for {
		if err := ctx.Err(); err != nil {
			return domain.CanonicalDocument{}, err
		}
		if err := budget.Page(); err != nil {
			return domain.CanonicalDocument{}, err
		}
		query := url.Values{"page_size": {"500"}}
		if pageToken != "" {
			query.Set("page_token", pageToken)
		}
		var page docxBlocksData
		if err := l.client.Get(ctx, accessToken, basePath+"/blocks", query, &page); err != nil {
			return domain.CanonicalDocument{}, err
		}
		if err := budget.Blocks(len(page.Items)); err != nil {
			return domain.CanonicalDocument{}, err
		}
		for _, block := range page.Items {
			if !validAPIIdentifier(block.BlockID) || block.BlockType < 0 || block.BlockType > 255 {
				return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
			}
			if !validDocxReferences(block) {
				return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
			}
			retainedBytes, err := docxRetainedBytes(block)
			if err != nil {
				return domain.CanonicalDocument{}, err
			}
			if err := budget.RetainedBytes(retainedBytes); err != nil {
				return domain.CanonicalDocument{}, err
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

	renderer := docxRenderer{ctx: ctx, budget: budget, blocks: blocks, normalizer: l.normalizer, sourceURL: ref.CanonicalURL.String()}
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
	markdown := l.normalizer.Finalize(parts)
	if err := budget.Bytes(len(markdown)); err != nil {
		return domain.CanonicalDocument{}, err
	}
	document, err := domain.NewCanonicalDocument(domain.CanonicalDocumentInput{
		Title: info.Document.Title, Markdown: markdown,
		RemoteRevision: strconv.FormatInt(info.Document.RevisionID, 10),
		SourceMetadata: metadata, SafeSourceURL: ref.CanonicalURL,
	})
	if err != nil {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	return document, nil
}

type docxRenderer struct {
	ctx        context.Context
	budget     *resourceBudget
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
	totalBytes := 0
	for _, id := range ids {
		part, err := r.renderBlock(id, depth, stack)
		if err != nil {
			return nil, err
		}
		if part != "" {
			totalBytes += len(part) + 1
			if r.budget != nil {
				if err := r.budget.CheckAdditionalOutputBytes(totalBytes); err != nil {
					return nil, err
				}
			}
			parts = append(parts, part)
		}
	}
	return parts, nil
}

func (r docxRenderer) renderBlock(id string, depth int, stack map[string]bool) (string, error) {
	if r.ctx != nil {
		if err := r.ctx.Err(); err != nil {
			return "", err
		}
	}
	if r.budget != nil {
		if err := r.budget.Depth(depth); err != nil {
			return "", err
		}
	}
	block, ok := r.blocks[id]
	if !ok || stack[id] {
		return "", ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	if err := validateDocxBlockPayload(block); err != nil {
		return "", err
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
	case block.Todo != nil:
		marker := " "
		if block.Todo.Style.Done {
			marker = "x"
		}
		rendered = "- [" + marker + "] " + r.normalizer.Paragraph(text(block.Todo))
	case block.Equation != nil:
		rendered = r.normalizer.Paragraph(docxEquationText(block.Equation.Content, block.Equation.Elements))
	case block.File != nil:
		name := strings.TrimSpace(block.File.Name)
		if name == "" {
			name = "file"
		}
		rendered = r.normalizer.Paragraph("Unsupported file: " + name)
	case block.Table != nil:
		rows, err := r.renderTable(block, depth, stack)
		if err != nil {
			return "", err
		}
		rendered = r.normalizer.tableCells(rows)
	case block.Image != nil:
		rendered = r.normalizer.UnsupportedImage("", r.sourceURL)
	case block.Sheet != nil:
		rendered = r.normalizer.Paragraph("Unsupported sheet")
	case block.Bitable != nil:
		rendered = r.normalizer.Paragraph("Unsupported bitable")
	case documentedDocxLeafLabel(block) != "":
		rendered = r.normalizer.Paragraph("Unsupported " + documentedDocxLeafLabel(block))
	case block.BlockType == 22:
		rendered = "---"
	case block.BlockType == 1 || block.BlockType == 32 || block.Callout != nil || block.Grid != nil || block.GridColumn != nil || block.QuoteContainer != nil:
		// Containers only render their children.
	default:
		rendered = r.normalizer.Paragraph("Unsupported block type " + strconv.Itoa(block.BlockType))
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
	if r.budget != nil {
		if err := r.budget.CheckAdditionalOutputBytes(len(rendered)); err != nil {
			return "", err
		}
	}
	return rendered, nil
}

func (r docxRenderer) renderTable(block docxBlock, depth int, stack map[string]bool) ([][]markdownTableCell, error) {
	rows, columns := block.Table.Property.RowSize, block.Table.Property.ColumnSize
	if rows <= 0 || columns <= 0 || rows > len(block.Table.Cells)/columns || len(block.Table.Cells) != rows*columns {
		return nil, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	result := make([][]markdownTableCell, rows)
	for row := 0; row < rows; row++ {
		result[row] = make([]markdownTableCell, columns)
		for column := 0; column < columns; column++ {
			cellID := block.Table.Cells[row*columns+column]
			cell, ok := r.blocks[cellID]
			if !ok || cell.BlockType != 32 {
				return nil, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
			}
			if r.budget != nil {
				if err := r.budget.Depth(depth + 1); err != nil {
					return nil, err
				}
			}
			parts, err := r.plainChildren(cell.Children, depth+2, stack)
			if err != nil {
				return nil, err
			}
			result[row][column] = r.normalizer.combineTableCells(parts)
		}
	}
	return result, nil
}

func docxRetainedBytes(block docxBlock) (int, error) {
	if err := validateDocxBlockPayload(block); err != nil {
		return 0, err
	}
	total := 0
	if err := addDocxRetainedBytes(&total, len(block.BlockID)); err != nil {
		return 0, err
	}
	if err := addDocxRetainedBytes(&total, 8); err != nil {
		return 0, err
	}
	for _, child := range block.Children {
		if err := addDocxRetainedBytes(&total, len(child)); err != nil {
			return 0, err
		}
	}
	texts := []*docxText{block.Text, block.Page, block.Heading1, block.Heading2, block.Heading3, block.Heading4, block.Heading5, block.Heading6, block.Heading7, block.Heading8, block.Heading9, block.Bullet, block.Ordered, block.Quote, block.Code, block.Todo}
	for _, text := range texts {
		textBytes, err := docxTextRetainedBytes(text)
		if err != nil {
			return 0, err
		}
		if err := addDocxRetainedBytes(&total, textBytes); err != nil {
			return 0, err
		}
	}
	if block.Equation != nil {
		if err := addDocxRetainedBytes(&total, len(block.Equation.Content)); err != nil {
			return 0, err
		}
		elementBytes, err := docxTextRetainedBytes(&docxText{Elements: block.Equation.Elements})
		if err != nil {
			return 0, err
		}
		if err := addDocxRetainedBytes(&total, elementBytes); err != nil {
			return 0, err
		}
	}
	if block.File != nil {
		if err := addDocxRetainedBytes(&total, len(block.File.Name)); err != nil {
			return 0, err
		}
	}
	if block.Image != nil {
		if err := addDocxRetainedBytes(&total, len(block.Image.Token)); err != nil {
			return 0, err
		}
	}
	if block.Table != nil {
		if err := addDocxRetainedBytes(&total, 16); err != nil {
			return 0, err
		}
		for _, cell := range block.Table.Cells {
			if err := addDocxRetainedBytes(&total, len(cell)); err != nil {
				return 0, err
			}
		}
	}
	return total, nil
}

func docxTextRetainedBytes(text *docxText) (int, error) {
	if text == nil {
		return 0, nil
	}
	total := 0
	if err := addDocxRetainedBytes(&total, text.Style.Language.retainedBytes); err != nil {
		return 0, err
	}
	if err := addDocxRetainedBytes(&total, 1); err != nil {
		return 0, err
	}
	for _, element := range text.Elements {
		if err := addDocxRetainedBytes(&total, docxInlineElementStructuralBytes); err != nil {
			return 0, err
		}
		members := element.unknownMembers
		if element.TextRun != nil {
			members++
			if err := addDocxRetainedBytes(&total, docxKnownInlinePayloadBytes); err != nil {
				return 0, err
			}
			if err := addDocxRetainedBytes(&total, len(element.TextRun.Content)); err != nil {
				return 0, err
			}
		}
		if element.MentionUser != nil {
			members++
			if err := addDocxRetainedBytes(&total, docxKnownInlinePayloadBytes); err != nil {
				return 0, err
			}
			if err := addDocxRetainedBytes(&total, len(element.MentionUser.UserID)); err != nil {
				return 0, err
			}
		}
		if element.MentionDoc != nil {
			members++
			if err := addDocxRetainedBytes(&total, docxKnownInlinePayloadBytes); err != nil {
				return 0, err
			}
			if err := addDocxRetainedBytes(&total, len(element.MentionDoc.Title)); err != nil {
				return 0, err
			}
		}
		if element.Equation != nil {
			members++
			if err := addDocxRetainedBytes(&total, docxKnownInlinePayloadBytes); err != nil {
				return 0, err
			}
			if err := addDocxRetainedBytes(&total, len(element.Equation.Content)); err != nil {
				return 0, err
			}
		}
		if element.File != nil {
			members++
			if err := addDocxRetainedBytes(&total, docxKnownInlinePayloadBytes); err != nil {
				return 0, err
			}
			if err := addDocxRetainedBytes(&total, len(element.File.Name)); err != nil {
				return 0, err
			}
		}
		if members != 1 {
			return 0, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
		}
	}
	return total, nil
}

func addDocxRetainedBytes(total *int, amount int) error {
	if amount < 0 || *total > math.MaxInt-amount {
		return ports.NewSourceLoadError(ports.SourceLoadTooLarge, nil)
	}
	*total += amount
	return nil
}

func validateDocxBlockPayload(block docxBlock) error {
	if err := validateDocxInlineElements(block); err != nil {
		return err
	}
	payloads := 0
	for _, present := range []bool{
		block.Page != nil, block.Text != nil, block.Heading1 != nil, block.Heading2 != nil, block.Heading3 != nil,
		block.Heading4 != nil, block.Heading5 != nil, block.Heading6 != nil, block.Heading7 != nil, block.Heading8 != nil,
		block.Heading9 != nil, block.Bullet != nil, block.Ordered != nil, block.Code != nil, block.Quote != nil,
		block.Equation != nil, block.Todo != nil, block.Bitable != nil, block.Callout != nil, block.ChatCard != nil,
		block.Diagram != nil, block.File != nil, block.Grid != nil, block.GridColumn != nil, block.Iframe != nil,
		block.Image != nil, block.ISV != nil, block.Mindnote != nil, block.Sheet != nil, block.Table != nil,
		block.View != nil, block.QuoteContainer != nil, block.Task != nil, block.OKR != nil, block.OKRObjective != nil,
		block.OKRKeyResult != nil, block.OKRProgress != nil, block.AddOns != nil, block.JiraIssue != nil, block.WikiCatalog != nil,
		block.Board != nil,
	} {
		if present {
			payloads++
		}
	}
	expected := false
	switch block.BlockType {
	case 1:
		expected = block.Page != nil
	case 2:
		expected = block.Text != nil
	case 3:
		expected = block.Heading1 != nil
	case 4:
		expected = block.Heading2 != nil
	case 5:
		expected = block.Heading3 != nil
	case 6:
		expected = block.Heading4 != nil
	case 7:
		expected = block.Heading5 != nil
	case 8:
		expected = block.Heading6 != nil
	case 9:
		expected = block.Heading7 != nil
	case 10:
		expected = block.Heading8 != nil
	case 11:
		expected = block.Heading9 != nil
	case 12:
		expected = block.Bullet != nil
	case 13:
		expected = block.Ordered != nil
	case 14:
		expected = block.Code != nil
	case 15:
		expected = block.Quote != nil
	case 16:
		expected = block.Equation != nil
	case 17:
		expected = block.Todo != nil
	case 18:
		expected = block.Bitable != nil
	case 19:
		expected = block.Callout != nil
	case 20:
		expected = block.ChatCard != nil
	case 21:
		expected = block.Diagram != nil
	case 22, 32:
		if payloads != 0 {
			return ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
		}
		return nil
	case 23:
		expected = block.File != nil
	case 24:
		expected = block.Grid != nil
	case 25:
		expected = block.GridColumn != nil
	case 26:
		expected = block.Iframe != nil
	case 27:
		expected = block.Image != nil
	case 28:
		expected = block.ISV != nil
	case 29:
		expected = block.Mindnote != nil
	case 30:
		expected = block.Sheet != nil
	case 31:
		expected = block.Table != nil
	case 33:
		expected = block.View != nil
	case 34:
		expected = block.QuoteContainer != nil
	case 35:
		expected = block.Task != nil
	case 36:
		expected = block.OKR != nil
	case 37:
		expected = block.OKRObjective != nil
	case 38:
		expected = block.OKRKeyResult != nil
	case 39:
		expected = block.OKRProgress != nil
	case 40:
		expected = block.AddOns != nil
	case 41:
		expected = block.JiraIssue != nil
	case 42:
		expected = block.WikiCatalog != nil
	case 43:
		expected = block.Board != nil
	default:
		if payloads != 0 {
			return ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
		}
		return nil
	}
	if !expected || payloads != 1 {
		return ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	return nil
}

func validateDocxInlineElements(block docxBlock) error {
	texts := []*docxText{block.Text, block.Page, block.Heading1, block.Heading2, block.Heading3, block.Heading4, block.Heading5, block.Heading6, block.Heading7, block.Heading8, block.Heading9, block.Bullet, block.Ordered, block.Quote, block.Code, block.Todo}
	for _, text := range texts {
		if _, err := docxTextRetainedBytes(text); err != nil {
			return err
		}
	}
	if block.Equation != nil {
		if _, err := docxTextRetainedBytes(&docxText{Elements: block.Equation.Elements}); err != nil {
			return err
		}
	}
	return nil
}

func validDocxReferences(block docxBlock) bool {
	for _, id := range block.Children {
		if !validAPIIdentifier(id) {
			return false
		}
	}
	if block.Table != nil {
		for _, id := range block.Table.Cells {
			if !validAPIIdentifier(id) {
				return false
			}
		}
	}
	return true
}

func (r docxRenderer) plainChildren(ids []string, depth int, stack map[string]bool) ([]markdownTableCell, error) {
	parts := make([]markdownTableCell, 0, len(ids))
	for _, id := range ids {
		if r.ctx != nil {
			if err := r.ctx.Err(); err != nil {
				return nil, err
			}
		}
		if r.budget != nil {
			if err := r.budget.Depth(depth); err != nil {
				return nil, err
			}
		}
		block, ok := r.blocks[id]
		if !ok || stack[id] {
			return nil, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
		}
		if err := validateDocxBlockPayload(block); err != nil {
			return nil, err
		}
		stack[id] = true
		var value markdownTableCell
		switch {
		case block.Text != nil:
			value.text = docxPlainText(block.Text)
		case block.Heading1 != nil:
			value.text = docxPlainText(block.Heading1)
		case block.Heading2 != nil:
			value.text = docxPlainText(block.Heading2)
		case block.Heading3 != nil:
			value.text = docxPlainText(block.Heading3)
		case block.Heading4 != nil:
			value.text = docxPlainText(block.Heading4)
		case block.Heading5 != nil:
			value.text = docxPlainText(block.Heading5)
		case block.Heading6 != nil:
			value.text = docxPlainText(block.Heading6)
		case block.Heading7 != nil:
			value.text = docxPlainText(block.Heading7)
		case block.Heading8 != nil:
			value.text = docxPlainText(block.Heading8)
		case block.Heading9 != nil:
			value.text = docxPlainText(block.Heading9)
		case block.Bullet != nil:
			value.text = docxPlainText(block.Bullet)
		case block.Ordered != nil:
			value.text = docxPlainText(block.Ordered)
		case block.Quote != nil:
			value.text = docxPlainText(block.Quote)
		case block.Code != nil:
			value.text = docxPlainText(block.Code)
		case block.Todo != nil:
			value.text = docxPlainText(block.Todo)
		case block.Equation != nil:
			value.text = docxEquationText(block.Equation.Content, block.Equation.Elements)
		case block.File != nil:
			value.text = block.File.Name
		case block.Image != nil:
			value.safeMarkdown = r.normalizer.UnsupportedInlineImage("", r.sourceURL)
		case block.BlockType == 22:
			value.text = "---"
		case documentedDocxLeafLabel(block) != "":
			value.text = "Unsupported " + documentedDocxLeafLabel(block)
		case block.Table != nil:
			value.text = "Unsupported table"
		case block.BlockType == 1 || block.BlockType == 32 || block.Callout != nil || block.Grid != nil || block.GridColumn != nil || block.QuoteContainer != nil:
			// Containers contribute their children only.
		default:
			value.text = "Unsupported block type " + strconv.Itoa(block.BlockType)
		}
		children, err := r.plainChildren(block.Children, depth+1, stack)
		delete(stack, id)
		if err != nil {
			return nil, err
		}
		if value.text != "" || value.safeMarkdown != "" {
			parts = append(parts, value)
		}
		parts = append(parts, children...)
	}
	return parts, nil
}

func docxEquationText(content string, elements []docxTextElement) string {
	if content != "" {
		return content
	}
	text := &docxText{Elements: elements}
	return docxPlainText(text)
}

func documentedDocxLeafLabel(block docxBlock) string {
	switch {
	case block.Bitable != nil:
		return "bitable"
	case block.ChatCard != nil:
		return "chat card"
	case block.Diagram != nil:
		return "diagram"
	case block.Iframe != nil:
		return "iframe"
	case block.ISV != nil:
		return "extension"
	case block.Mindnote != nil:
		return "mindnote"
	case block.Sheet != nil:
		return "sheet"
	case block.View != nil:
		return "view"
	case block.Task != nil:
		return "task"
	case block.OKR != nil:
		return "OKR"
	case block.OKRObjective != nil:
		return "OKR objective"
	case block.OKRKeyResult != nil:
		return "OKR key result"
	case block.OKRProgress != nil:
		return "OKR progress"
	case block.AddOns != nil:
		return "add-on"
	case block.JiraIssue != nil:
		return "Jira issue"
	case block.WikiCatalog != nil:
		return "wiki catalog"
	case block.Board != nil:
		return "board"
	default:
		return ""
	}
}

func docxCodeLanguage(value docxLanguage) string {
	return value.value
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
