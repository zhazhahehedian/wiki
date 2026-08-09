package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type SheetLoader struct {
	client     *Client
	normalizer MarkdownNormalizer
}

func NewSheetLoader(client *Client) *SheetLoader {
	return &SheetLoader{client: client, normalizer: NewMarkdownNormalizer()}
}

type sheetWorkbookData struct {
	Spreadsheet struct {
		SpreadsheetToken string `json:"spreadsheet_token"`
		Title            string `json:"title"`
		Revision         int64  `json:"revision"`
	} `json:"spreadsheet"`
}

type sheetListData struct {
	Sheets    []sheetInfo `json:"sheets"`
	HasMore   bool        `json:"has_more"`
	PageToken string      `json:"page_token"`
}

type sheetInfo struct {
	SheetID        string `json:"sheet_id"`
	Title          string `json:"title"`
	RowCount       int    `json:"row_count"`
	GridProperties struct {
		RowCount int `json:"row_count"`
	} `json:"grid_properties"`
}

type sheetValuesData struct {
	ValueRange      sheetValueRange `json:"value_range"`
	ValueRangeCamel sheetValueRange `json:"valueRange"`
	Revision        int64           `json:"revision"`
	HasMore         bool            `json:"has_more"`
	PageToken       string          `json:"page_token"`
}

type sheetValueRange struct {
	Revision int64   `json:"revision"`
	Values   [][]any `json:"values"`
}

type loadedSheet struct {
	id       string
	title    string
	rows     [][]string
	revision int64
}

func (l *SheetLoader) Load(ctx context.Context, ref domain.ResourceRef, accessToken string) (domain.CanonicalDocument, error) {
	if l == nil || l.client == nil || ref.Type != domain.ResourceSheet {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	var workbook sheetWorkbookData
	metaPath := "/open-apis/sheets/v3/spreadsheets/" + ref.Token
	if err := l.client.Get(ctx, accessToken, metaPath, nil, &workbook); err != nil {
		return domain.CanonicalDocument{}, err
	}
	if strings.TrimSpace(workbook.Spreadsheet.Title) == "" || workbook.Spreadsheet.Revision < 0 {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}

	sheets, err := l.listSheets(ctx, ref, accessToken)
	if err != nil {
		return domain.CanonicalDocument{}, err
	}
	loaded := make([]loadedSheet, 0, len(sheets))
	for _, sheet := range sheets {
		if ref.SheetID != "" && ref.SheetID != sheet.SheetID {
			continue
		}
		rowCount := sheet.RowCount
		if rowCount == 0 {
			rowCount = sheet.GridProperties.RowCount
		}
		rows, revision, err := l.loadRows(ctx, ref.Token, sheet.SheetID, rowCount, accessToken)
		if err != nil {
			return domain.CanonicalDocument{}, err
		}
		loaded = append(loaded, loadedSheet{id: sheet.SheetID, title: sheet.Title, rows: rows, revision: revision})
	}
	if ref.SheetID != "" && len(loaded) == 0 {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadNotFound, nil)
	}

	parts := make([]string, 0, len(loaded))
	for _, sheet := range loaded {
		rowStart, rowEnd := rowBounds(len(sheet.rows))
		section := []string{
			l.normalizer.Heading(2, sheet.title),
			fmt.Sprintf(`<!-- feishu-sheet sheet_id="%s" rows="%d-%d" -->`, sheet.id, rowStart, rowEnd),
		}
		if table := l.normalizer.Table(sheet.rows); table != "" {
			section = append(section, table)
		}
		parts = append(parts, strings.Join(section, "\n\n"))
	}

	revision := workbook.Spreadsheet.Revision
	for _, sheet := range loaded {
		if sheet.revision > revision {
			revision = sheet.revision
		}
	}
	metadataInput := domain.SourceMetadataInput{
		SourceType: domain.ResourceSheet, SectionPath: workbook.Spreadsheet.Title,
		RemoteRevision: strconv.FormatInt(revision, 10), SourceLocator: ref.CanonicalURL,
	}
	if len(loaded) == 1 {
		metadataInput.SheetID = loaded[0].id
		metadataInput.SheetName = loaded[0].title
		metadataInput.RowStart, metadataInput.RowEnd = rowBounds(len(loaded[0].rows))
	}
	metadata, err := domain.NewSourceMetadata(metadataInput)
	if err != nil {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	document, err := domain.NewCanonicalDocument(domain.CanonicalDocumentInput{
		Title: workbook.Spreadsheet.Title, Markdown: l.normalizer.Finalize(parts),
		RemoteRevision: strconv.FormatInt(revision, 10),
		SourceMetadata: metadata, SafeSourceURL: ref.CanonicalURL,
	})
	if err != nil {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	return document, nil
}

func (l *SheetLoader) listSheets(ctx context.Context, ref domain.ResourceRef, accessToken string) ([]sheetInfo, error) {
	var result []sheetInfo
	tracker := newPageTokenTracker()
	token := ""
	for {
		query := url.Values{"page_size": {"100"}}
		if token != "" {
			query.Set("page_token", token)
		}
		var page sheetListData
		path := "/open-apis/sheets/v3/spreadsheets/" + ref.Token + "/sheets/query"
		if err := l.client.Get(ctx, accessToken, path, query, &page); err != nil {
			return nil, err
		}
		for _, sheet := range page.Sheets {
			if !validAPIIdentifier(sheet.SheetID) || strings.TrimSpace(sheet.Title) == "" {
				return nil, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
			}
		}
		result = append(result, page.Sheets...)
		if !page.HasMore {
			return result, nil
		}
		if page.PageToken == "" || tracker.Advance(page.PageToken) != nil {
			return nil, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
		}
		token = page.PageToken
	}
}

func (l *SheetLoader) loadRows(ctx context.Context, workbookToken, sheetID string, rowCount int, accessToken string) ([][]string, int64, error) {
	rows := make([][]string, 0)
	revision := int64(0)
	ranges := [][2]int{{0, 0}}
	if rowCount > 0 {
		ranges = ranges[:0]
		for start := 1; start <= rowCount; start += 500 {
			end := start + 499
			if end > rowCount {
				end = rowCount
			}
			ranges = append(ranges, [2]int{start, end})
		}
	}
	for _, rowRange := range ranges {
		path := "/open-apis/sheets/v2/spreadsheets/" + workbookToken + "/values/" + sheetID
		if rowRange[0] > 0 {
			path += "!A" + strconv.Itoa(rowRange[0]) + ":ZZZ" + strconv.Itoa(rowRange[1])
		}
		var page sheetValuesData
		if err := l.client.Get(ctx, accessToken, path, nil, &page); err != nil {
			return nil, 0, err
		}
		valueRange := page.ValueRange
		if valueRange.Values == nil && page.ValueRangeCamel.Values != nil {
			valueRange = page.ValueRangeCamel
		}
		if valueRange.Revision > revision {
			revision = valueRange.Revision
		}
		if page.Revision > revision {
			revision = page.Revision
		}
		for _, values := range valueRange.Values {
			row := make([]string, len(values))
			for i, value := range values {
				row[i] = canonicalCell(value)
			}
			rows = append(rows, row)
		}
	}
	return rows, revision, nil
}

func canonicalCell(value any) string {
	switch value := value.(type) {
	case nil:
		return ""
	case string:
		return value
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(value)
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return string(encoded)
	}
}

func rowBounds(count int) (int, int) {
	if count == 0 {
		return 0, 0
	}
	return 1, count
}

var _ ports.SourceLoader = (*SheetLoader)(nil)
