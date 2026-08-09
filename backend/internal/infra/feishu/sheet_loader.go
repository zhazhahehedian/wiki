package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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
	Range    string  `json:"range"`
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

	budget, err := resourceBudgetFromContext(ctx, l.client.resourceLimits)
	if err != nil {
		return domain.CanonicalDocument{}, err
	}
	if err := budget.RetainedBytes(len(workbook.Spreadsheet.Token) + len(workbook.Spreadsheet.Title)); err != nil {
		return domain.CanonicalDocument{}, err
	}
	sheets, err := l.listSheets(ctx, ref, accessToken, budget)
	if err != nil {
		return domain.CanonicalDocument{}, err
	}
	loaded := make([]loadedSheet, 0, len(sheets))
	estimatedOutputBytes := 0
	for _, sheet := range sheets {
		if ref.SheetID != "" && ref.SheetID != sheet.SheetID {
			continue
		}
		rowCount := sheet.RowCount
		if rowCount == 0 {
			rowCount = sheet.GridProperties.RowCount
		}
		segments, revision, err := l.loadRows(ctx, ref.Token, sheet.SheetID, rowCount, accessToken, budget, &estimatedOutputBytes)
		if err != nil {
			return domain.CanonicalDocument{}, err
		}
		loaded = append(loaded, loadedSheet{id: sheet.SheetID, title: sheet.Title, segments: segments, revision: revision})
	}
	if ref.SheetID != "" && len(loaded) == 0 {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadNotFound, nil)
	}

	sectionLabels := make([]string, len(loaded))
	sectionIDs := make([]string, len(loaded))
	for i, sheet := range loaded {
		sectionLabels[i], sectionIDs[i] = sheet.title, sheet.id
	}
	sectionHeadings := citationSectionPaths(sectionLabels, sectionIDs)
	sectionPaths := make([]string, len(sectionHeadings))
	for i, heading := range sectionHeadings {
		sectionPaths[i] = l.normalizer.headingText(heading)
	}
	parts := make([]string, 0, len(loaded))
	for i, sheet := range loaded {
		rows := flattenSheetSegments(sheet.segments)
		rangeLabel := segmentRangeLabel(sheet.segments)
		estimatedOutputBytes += len(sectionPaths[i]) + len(sheet.id) + len(rangeLabel) + 50
		if err := budget.CheckAdditionalOutputBytes(estimatedOutputBytes); err != nil {
			return domain.CanonicalDocument{}, err
		}
		section := []string{
			l.normalizer.Heading(2, sectionHeadings[i]),
			fmt.Sprintf(`<!-- feishu-sheet sheet_id="%s" rows="%s" -->`, sheet.id, rangeLabel),
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
	for i, sheet := range loaded {
		sectionPath := sectionPaths[i]
		if len(sheet.segments) == 0 {
			if err := budget.RetainedBytes(len(sectionPath) + len(sheet.title) + len(sheet.id)); err != nil {
				return domain.CanonicalDocument{}, err
			}
			metadataInput.Locations = append(metadataInput.Locations, domain.SourceLocation{SectionPath: sectionPath, SheetName: sheet.title, SheetID: sheet.id})
		}
		for _, segment := range sheet.segments {
			if err := budget.RetainedBytes(len(sectionPath) + len(sheet.title) + len(sheet.id)); err != nil {
				return domain.CanonicalDocument{}, err
			}
			metadataInput.Locations = append(metadataInput.Locations, domain.SourceLocation{
				SectionPath: sectionPath,
				SheetName:   sheet.title, SheetID: sheet.id, RowStart: segment.start, RowEnd: segment.end,
			})
		}
	}
	if len(loaded) == 1 {
		metadataInput.SheetID = loaded[0].id
		metadataInput.SheetName = loaded[0].title
		metadataInput.RowStart, metadataInput.RowEnd = contiguousSegmentBounds(loaded[0].segments)
	}
	metadata, err := domain.NewSourceMetadata(metadataInput)
	if err != nil {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	markdown := l.normalizer.Finalize(parts)
	if err := budget.Bytes(len(markdown)); err != nil {
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
		if sheet.RowCount < 0 || sheet.GridProperties.RowCount < 0 {
			return nil, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
		}
		if err := budget.RetainedBytes(len(sheet.SheetID) + len(sheet.Title)); err != nil {
			return nil, err
		}
	}
	if err := budget.Entities(len(data.Sheets)); err != nil {
		return nil, err
	}
	return data.Sheets, nil
}

func (l *SheetLoader) loadRows(ctx context.Context, workbookToken, sheetID string, rowCount int, accessToken string, budget *resourceBudget, estimatedOutputBytes *int) ([]sheetRowSegment, int64, error) {
	segments := make([]sheetRowSegment, 0)
	revision := int64(0)
	if _, err := validateSheetRowPlan(rowCount, budget); err != nil {
		return nil, 0, err
	}
	if rowCount > 0 {
		if err := budget.Rows(rowCount); err != nil {
			return nil, 0, err
		}
	}
	loadRange := func(start, end int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := budget.Page(); err != nil {
			return err
		}
		path := "/open-apis/sheets/v2/spreadsheets/" + workbookToken + "/values/" + sheetID
		if start > 0 {
			path += "!A" + strconv.Itoa(start) + ":ZZZ" + strconv.Itoa(end)
		}
		var page sheetValuesData
		if err := l.client.Get(ctx, accessToken, path, nil, &page); err != nil {
			return err
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
		if rowCount == 0 {
			if err := budget.Rows(len(valueRange.Values)); err != nil {
				return err
			}
		}
		actualStart, actualEnd, hasRows, err := actualSheetSegment(valueRange.Range, start, end, len(valueRange.Values))
		if err != nil {
			return err
		}
		if !hasRows {
			return nil
		}
		if err := budget.Entities(1); err != nil {
			return err
		}
		if err := budget.RetainedBytes(len(valueRange.Range)); err != nil {
			return err
		}
		rows := make([][]string, 0, len(valueRange.Values))
		for _, values := range valueRange.Values {
			if err := ctx.Err(); err != nil {
				return err
			}
			*estimatedOutputBytes += 4 + len(values)*3
			if err := budget.CheckAdditionalOutputBytes(*estimatedOutputBytes); err != nil {
				return err
			}
			row := make([]string, len(values))
			for i, value := range values {
				row[i] = canonicalCell(value)
				if err := budget.RetainedBytes(len(row[i])); err != nil {
					return err
				}
				*estimatedOutputBytes += len(row[i])
				if err := budget.CheckAdditionalOutputBytes(*estimatedOutputBytes); err != nil {
					return err
				}
			}
			rows = append(rows, row)
		}
		segments = append(segments, sheetRowSegment{start: actualStart, end: actualEnd, rows: rows})
		return nil
	}
	if rowCount == 0 {
		if err := loadRange(0, 0); err != nil {
			return nil, 0, err
		}
		return segments, revision, nil
	}
	for start := 1; ; {
		end := start + min(499, rowCount-start)
		if err := loadRange(start, end); err != nil {
			return nil, 0, err
		}
		if end == rowCount {
			break
		}
		start = end + 1
	}
	return segments, revision, nil
}

func validateSheetRowPlan(rowCount int, budget *resourceBudget) (int, error) {
	if rowCount < 0 {
		return 0, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	pages := 1
	if rowCount > 0 {
		pages = (rowCount-1)/500 + 1
	}
	if err := budget.CheckRows(rowCount); err != nil {
		return 0, err
	}
	if err := budget.CheckPages(pages); err != nil {
		return 0, err
	}
	return pages, nil
}

func actualSheetSegment(responseRange string, requestedStart, requestedEnd, count int) (int, int, bool, error) {
	if count == 0 {
		return 0, 0, false, nil
	}
	start := requestedStart
	if start == 0 {
		start = 1
	}
	if responseRange != "" {
		cellRange := responseRange
		if bang := strings.LastIndex(cellRange, "!"); bang >= 0 {
			cellRange = cellRange[bang+1:]
		}
		first := strings.SplitN(cellRange, ":", 2)[0]
		digit := strings.IndexFunc(first, func(r rune) bool { return r >= '0' && r <= '9' })
		if digit < 0 {
			return 0, 0, false, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
		}
		parsed, err := strconv.ParseInt(first[digit:], 10, 64)
		if err != nil || parsed <= 0 || parsed > int64(math.MaxInt) {
			return 0, 0, false, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
		}
		start = int(parsed)
	}
	if requestedStart > 0 && (start < requestedStart || start > requestedEnd) {
		return 0, 0, false, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	if count-1 > math.MaxInt-start {
		return 0, 0, false, ports.NewSourceLoadError(ports.SourceLoadTooLarge, nil)
	}
	end := start + count - 1
	if requestedEnd > 0 && end > requestedEnd {
		return 0, 0, false, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	return start, end, true, nil
}

func flattenSheetSegments(segments []sheetRowSegment) [][]string {
	var rows [][]string
	for _, segment := range segments {
		rows = append(rows, segment.rows...)
	}
	return rows
}

func contiguousSegmentBounds(segments []sheetRowSegment) (int, int) {
	if len(segments) == 0 {
		return 0, 0
	}
	for i := 1; i < len(segments); i++ {
		if segments[i-1].end == math.MaxInt || segments[i].start != segments[i-1].end+1 {
			return 0, 0
		}
	}
	return segments[0].start, segments[len(segments)-1].end
}

func segmentRangeLabel(segments []sheetRowSegment) string {
	if len(segments) == 0 {
		return "0-0"
	}
	parts := make([]string, len(segments))
	for i, segment := range segments {
		parts[i] = strconv.Itoa(segment.start) + "-" + strconv.Itoa(segment.end)
	}
	return strings.Join(parts, ",")
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
