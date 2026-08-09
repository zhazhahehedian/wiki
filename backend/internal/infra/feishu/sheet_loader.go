package feishu

import (
	"context"
	"encoding/json"
	"fmt"
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
		Token string `json:"token"`
		Title string `json:"title"`
	} `json:"spreadsheet"`
}

type sheetListData struct {
	Sheets []sheetInfo `json:"sheets"`
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
	segments []sheetRowSegment
	revision int64
}

type sheetRowSegment struct {
	start int
	end   int
	rows  [][]string
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
	if workbook.Spreadsheet.Token != ref.Token || strings.TrimSpace(workbook.Spreadsheet.Title) == "" {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}

	budget := resourceBudgetFromContext(ctx, l.client.resourceLimits)
	sheets, err := l.listSheets(ctx, ref, accessToken, budget)
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
		segments, revision, err := l.loadRows(ctx, ref.Token, sheet.SheetID, rowCount, accessToken, budget)
		if err != nil {
			return domain.CanonicalDocument{}, err
		}
		loaded = append(loaded, loadedSheet{id: sheet.SheetID, title: sheet.Title, segments: segments, revision: revision})
	}
	if ref.SheetID != "" && len(loaded) == 0 {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadNotFound, nil)
	}

	parts := make([]string, 0, len(loaded))
	for _, sheet := range loaded {
		rows := flattenSheetSegments(sheet.segments)
		rowStart, rowEnd := segmentBounds(sheet.segments)
		section := []string{
			l.normalizer.Heading(2, sheet.title),
			fmt.Sprintf(`<!-- feishu-sheet sheet_id="%s" rows="%d-%d" -->`, sheet.id, rowStart, rowEnd),
		}
		if table := l.normalizer.Table(rows); table != "" {
			section = append(section, table)
		}
		parts = append(parts, strings.Join(section, "\n\n"))
	}

	revision := int64(0)
	for _, sheet := range loaded {
		if sheet.revision > revision {
			revision = sheet.revision
		}
	}
	metadataInput := domain.SourceMetadataInput{
		SourceType: domain.ResourceSheet, SectionPath: workbook.Spreadsheet.Title,
		RemoteRevision: strconv.FormatInt(revision, 10), SourceLocator: ref.CanonicalURL,
		Locations: make([]domain.SourceLocation, 0, len(loaded)),
	}
	for _, sheet := range loaded {
		if len(sheet.segments) == 0 {
			metadataInput.Locations = append(metadataInput.Locations, domain.SourceLocation{SectionPath: workbook.Spreadsheet.Title + " / " + sheet.title, SheetName: sheet.title, SheetID: sheet.id})
		}
		for _, segment := range sheet.segments {
			metadataInput.Locations = append(metadataInput.Locations, domain.SourceLocation{
				SectionPath: workbook.Spreadsheet.Title + " / " + sheet.title,
				SheetName:   sheet.title, SheetID: sheet.id, RowStart: segment.start, RowEnd: segment.end,
			})
		}
	}
	if len(loaded) == 1 {
		metadataInput.SheetID = loaded[0].id
		metadataInput.SheetName = loaded[0].title
		metadataInput.RowStart, metadataInput.RowEnd = segmentBounds(loaded[0].segments)
	}
	metadata, err := domain.NewSourceMetadata(metadataInput)
	if err != nil {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	markdown := l.normalizer.Finalize(parts)
	if err := budget.OutputBytes(len(markdown)); err != nil {
		return domain.CanonicalDocument{}, err
	}
	document, err := domain.NewCanonicalDocument(domain.CanonicalDocumentInput{
		Title: workbook.Spreadsheet.Title, Markdown: markdown,
		RemoteRevision: strconv.FormatInt(revision, 10),
		SourceMetadata: metadata, SafeSourceURL: ref.CanonicalURL,
	})
	if err != nil {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	return document, nil
}

func (l *SheetLoader) listSheets(ctx context.Context, ref domain.ResourceRef, accessToken string, budget *resourceBudget) ([]sheetInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := budget.Page(); err != nil {
		return nil, err
	}
	var data sheetListData
	path := "/open-apis/sheets/v3/spreadsheets/" + ref.Token + "/sheets/query"
	if err := l.client.Get(ctx, accessToken, path, nil, &data); err != nil {
		return nil, err
	}
	for _, sheet := range data.Sheets {
		if !validAPIIdentifier(sheet.SheetID) || strings.TrimSpace(sheet.Title) == "" {
			return nil, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
		}
	}
	return data.Sheets, nil
}

func (l *SheetLoader) loadRows(ctx context.Context, workbookToken, sheetID string, rowCount int, accessToken string, budget *resourceBudget) ([]sheetRowSegment, int64, error) {
	segments := make([]sheetRowSegment, 0)
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
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		if err := budget.Page(); err != nil {
			return nil, 0, err
		}
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
		if err := budget.Rows(len(valueRange.Values)); err != nil {
			return nil, 0, err
		}
		rows := make([][]string, 0, len(valueRange.Values))
		for _, values := range valueRange.Values {
			if err := ctx.Err(); err != nil {
				return nil, 0, err
			}
			if err := budget.Bytes(4 + len(values)*3); err != nil {
				return nil, 0, err
			}
			row := make([]string, len(values))
			for i, value := range values {
				row[i] = canonicalCell(value)
				if err := budget.Bytes(len(row[i])); err != nil {
					return nil, 0, err
				}
			}
			rows = append(rows, row)
		}
		if rowRange[0] > 0 {
			segments = append(segments, sheetRowSegment{start: rowRange[0], end: rowRange[1], rows: rows})
		} else if len(rows) > 0 {
			segments = append(segments, sheetRowSegment{start: 1, end: len(rows), rows: rows})
		}
	}
	return segments, revision, nil
}

func flattenSheetSegments(segments []sheetRowSegment) [][]string {
	var rows [][]string
	for _, segment := range segments {
		rows = append(rows, segment.rows...)
	}
	return rows
}

func segmentBounds(segments []sheetRowSegment) (int, int) {
	if len(segments) == 0 {
		return 0, 0
	}
	return segments[0].start, segments[len(segments)-1].end
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
