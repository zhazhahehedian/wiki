package feishu

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

const (
	defaultBitableMaxRows        = 10_000
	defaultBitableMaxOutputBytes = 10 << 20
)

type BitableConfig struct {
	MaxRows        int
	MaxOutputBytes int
}

type BitableLoader struct {
	client     *Client
	config     BitableConfig
	normalizer MarkdownNormalizer
}

func NewBitableLoader(client *Client, config BitableConfig) *BitableLoader {
	if config.MaxRows <= 0 {
		config.MaxRows = defaultBitableMaxRows
		if client != nil && client.resourceLimits.MaxRows > 0 {
			config.MaxRows = client.resourceLimits.MaxRows
		}
	}
	if config.MaxOutputBytes <= 0 {
		config.MaxOutputBytes = defaultBitableMaxOutputBytes
		if client != nil && client.resourceLimits.MaxOutputBytes > 0 {
			config.MaxOutputBytes = client.resourceLimits.MaxOutputBytes
		}
	}
	return &BitableLoader{client: client, config: config, normalizer: NewMarkdownNormalizer()}
}

type bitableAppData struct {
	App struct {
		AppToken string `json:"app_token"`
		Name     string `json:"name"`
		Revision int64  `json:"revision"`
	} `json:"app"`
}

type bitableTable struct {
	TableID string `json:"table_id"`
	Name    string `json:"name"`
}

type bitableView struct {
	ViewID   string `json:"view_id"`
	ViewName string `json:"view_name"`
}

type bitableRecord struct {
	RecordID   string            `json:"record_id"`
	Fields     map[string]any    `json:"fields"`
	Normalized map[string]string `json:"-"`
}

type bitableTablePage struct {
	Items     []bitableTable `json:"items"`
	HasMore   bool           `json:"has_more"`
	PageToken string         `json:"page_token"`
}

type bitableViewPage struct {
	Items     []bitableView `json:"items"`
	HasMore   bool          `json:"has_more"`
	PageToken string        `json:"page_token"`
}

type bitableRecordPage struct {
	Items     []bitableRecord `json:"items"`
	HasMore   bool            `json:"has_more"`
	PageToken string          `json:"page_token"`
}

type loadedBitableView struct {
	table   bitableTable
	view    bitableView
	records []bitableRecord
}

func (l *BitableLoader) Load(ctx context.Context, ref domain.ResourceRef, accessToken string) (domain.CanonicalDocument, error) {
	if l == nil || l.client == nil || ref.Type != domain.ResourceBitable {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	var app bitableAppData
	basePath := "/open-apis/bitable/v1/apps/" + ref.Token
	if err := l.client.Get(ctx, accessToken, basePath, nil, &app); err != nil {
		return domain.CanonicalDocument{}, err
	}
	if strings.TrimSpace(app.App.Name) == "" || app.App.Revision < 0 {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	limits := l.client.resourceLimits
	limits.MaxRows = l.config.MaxRows
	limits.MaxOutputBytes = l.config.MaxOutputBytes
	budget, err := resourceBudgetFromContext(ctx, limits)
	if err != nil {
		return domain.CanonicalDocument{}, err
	}
	if err := budget.RetainedBytes(len(app.App.AppToken) + len(app.App.Name)); err != nil {
		return domain.CanonicalDocument{}, err
	}
	tables, err := l.listTables(ctx, basePath, accessToken, budget)
	if err != nil {
		return domain.CanonicalDocument{}, err
	}
	loaded := make([]loadedBitableView, 0)
	estimatedOutputBytes := 0
	for _, table := range tables {
		if err := ctx.Err(); err != nil {
			return domain.CanonicalDocument{}, err
		}
		if ref.TableID != "" && ref.TableID != table.TableID {
			continue
		}
		if ref.ViewID == "" {
			records, err := l.listRecords(ctx, basePath, table.TableID, "", accessToken, budget, &estimatedOutputBytes)
			if err != nil {
				return domain.CanonicalDocument{}, err
			}
			loaded = append(loaded, loadedBitableView{table: table, records: records})
			continue
		}
		views, err := l.listViews(ctx, basePath, table.TableID, accessToken, budget)
		if err != nil {
			return domain.CanonicalDocument{}, err
		}
		for _, view := range views {
			if ref.ViewID != view.ViewID {
				continue
			}
			records, err := l.listRecords(ctx, basePath, table.TableID, view.ViewID, accessToken, budget, &estimatedOutputBytes)
			if err != nil {
				return domain.CanonicalDocument{}, err
			}
			loaded = append(loaded, loadedBitableView{table: table, view: view, records: records})
		}
	}
	if (ref.TableID != "" || ref.ViewID != "") && len(loaded) == 0 {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadNotFound, nil)
	}

	sectionLabels := make([]string, len(loaded))
	sectionIDs := make([]string, len(loaded))
	for i, item := range loaded {
		sectionLabels[i] = item.table.Name
		sectionIDs[i] = item.table.TableID
		if item.view.ViewID != "" {
			sectionLabels[i] += " / " + item.view.ViewName
			sectionIDs[i] += "/" + item.view.ViewID
		}
	}
	sectionHeadings := citationSectionPaths(sectionLabels, sectionIDs)
	sectionPaths := make([]string, len(sectionHeadings))
	for i, heading := range sectionHeadings {
		sectionPaths[i] = l.normalizer.headingText(heading)
	}
	parts := make([]string, 0, len(loaded))
	for i, item := range loaded {
		rowStart, rowEnd := rowBounds(len(item.records))
		heading := sectionHeadings[i]
		comment := fmt.Sprintf(`<!-- feishu-bitable table_id="%s" rows="%d-%d" -->`, item.table.TableID, rowStart, rowEnd)
		if item.view.ViewID != "" {
			comment = fmt.Sprintf(`<!-- feishu-bitable table_id="%s" view_id="%s" rows="%d-%d" -->`, item.table.TableID, item.view.ViewID, rowStart, rowEnd)
		}
		section := []string{l.normalizer.Heading(2, heading), comment}
		estimatedOutputBytes += len(heading) + len(comment) + 8
		if err := budget.CheckAdditionalOutputBytes(estimatedOutputBytes); err != nil {
			return domain.CanonicalDocument{}, err
		}
		table, err := l.recordsTable(ctx, item.records)
		if err != nil {
			return domain.CanonicalDocument{}, err
		}
		if table != "" {
			section = append(section, table)
		}
		parts = append(parts, strings.Join(section, "\n\n"))
	}
	markdown := l.normalizer.Finalize(parts)
	if err := budget.Bytes(len(markdown)); err != nil {
		return domain.CanonicalDocument{}, err
	}

	metadataInput := domain.SourceMetadataInput{
		SourceType: domain.ResourceBitable, SectionPath: app.App.Name,
		RemoteRevision: strconv.FormatInt(app.App.Revision, 10), SourceLocator: ref.CanonicalURL,
		Locations: make([]domain.SourceLocation, 0, len(loaded)),
	}
	if ref.TableID != "" {
		metadataInput.TableID = ref.TableID
	}
	for i, item := range loaded {
		rowStart, rowEnd := rowBounds(len(item.records))
		path := sectionPaths[i]
		if err := budget.RetainedBytes(len(path) + len(item.table.TableID) + len(item.view.ViewID)); err != nil {
			return domain.CanonicalDocument{}, err
		}
		metadataInput.Locations = append(metadataInput.Locations, domain.SourceLocation{
			SectionPath: path, TableID: item.table.TableID, ViewID: item.view.ViewID, RowStart: rowStart, RowEnd: rowEnd,
		})
	}
	if len(loaded) == 1 {
		metadataInput.TableID = loaded[0].table.TableID
		metadataInput.ViewID = loaded[0].view.ViewID
		metadataInput.RowStart, metadataInput.RowEnd = rowBounds(len(loaded[0].records))
	}
	metadata, err := domain.NewSourceMetadata(metadataInput)
	if err != nil {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	document, err := domain.NewCanonicalDocument(domain.CanonicalDocumentInput{
		Title: app.App.Name, Markdown: markdown, RemoteRevision: strconv.FormatInt(app.App.Revision, 10),
		SourceMetadata: metadata, SafeSourceURL: ref.CanonicalURL,
	})
	if err != nil {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
	}
	return document, nil
}

func (l *BitableLoader) listTables(ctx context.Context, basePath, accessToken string, budget *resourceBudget) ([]bitableTable, error) {
	items := make([]bitableTable, 0)
	err := l.paginate(ctx, budget, nil, 100, func(query url.Values) (bool, string, error) {
		var page bitableTablePage
		if err := l.client.Get(ctx, accessToken, basePath+"/tables", query, &page); err != nil {
			return false, "", err
		}
		if err := budget.Entities(len(page.Items)); err != nil {
			return false, "", err
		}
		for _, item := range page.Items {
			if err := ctx.Err(); err != nil {
				return false, "", err
			}
			if !validAPIIdentifier(item.TableID) || strings.TrimSpace(item.Name) == "" {
				return false, "", ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
			}
			if err := budget.RetainedBytes(len(item.TableID) + len(item.Name)); err != nil {
				return false, "", err
			}
		}
		items = append(items, page.Items...)
		return page.HasMore, page.PageToken, nil
	})
	return items, err
}

func (l *BitableLoader) listViews(ctx context.Context, basePath, tableID, accessToken string, budget *resourceBudget) ([]bitableView, error) {
	items := make([]bitableView, 0)
	path := basePath + "/tables/" + tableID + "/views"
	err := l.paginate(ctx, budget, nil, 100, func(query url.Values) (bool, string, error) {
		var page bitableViewPage
		if err := l.client.Get(ctx, accessToken, path, query, &page); err != nil {
			return false, "", err
		}
		if err := budget.Entities(len(page.Items)); err != nil {
			return false, "", err
		}
		for _, item := range page.Items {
			if err := ctx.Err(); err != nil {
				return false, "", err
			}
			if !validAPIIdentifier(item.ViewID) || strings.TrimSpace(item.ViewName) == "" {
				return false, "", ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
			}
			if err := budget.RetainedBytes(len(item.ViewID) + len(item.ViewName)); err != nil {
				return false, "", err
			}
		}
		items = append(items, page.Items...)
		return page.HasMore, page.PageToken, nil
	})
	return items, err
}

func (l *BitableLoader) listRecords(ctx context.Context, basePath, tableID, viewID, accessToken string, budget *resourceBudget, estimatedOutputBytes *int) ([]bitableRecord, error) {
	items := make([]bitableRecord, 0)
	path := basePath + "/tables/" + tableID + "/records"
	var baseQuery url.Values
	if viewID != "" {
		baseQuery = url.Values{"view_id": {viewID}}
	}
	err := l.paginate(ctx, budget, baseQuery, 500, func(query url.Values) (bool, string, error) {
		var page bitableRecordPage
		if err := l.client.Get(ctx, accessToken, path, query, &page); err != nil {
			return false, "", err
		}
		if err := budget.Rows(len(page.Items)); err != nil {
			return false, "", err
		}
		for i := range page.Items {
			item := &page.Items[i]
			if err := ctx.Err(); err != nil {
				return false, "", err
			}
			if item.RecordID == "" {
				return false, "", ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
			}
			if err := budget.RetainedBytes(len(item.RecordID)); err != nil {
				return false, "", err
			}
			item.Normalized = make(map[string]string, len(item.Fields))
			for field, value := range item.Fields {
				normalized, err := normalizeBitableValue(ctx, budget, value, 0)
				if err != nil {
					return false, "", err
				}
				if err := budget.RetainedBytes(len(field) + len(normalized)); err != nil {
					return false, "", err
				}
				item.Normalized[field] = normalized
				*estimatedOutputBytes += len(field) + len(normalized) + 6
				if err := budget.CheckAdditionalOutputBytes(*estimatedOutputBytes); err != nil {
					return false, "", err
				}
			}
			item.Fields = nil
		}
		items = append(items, page.Items...)
		return page.HasMore, page.PageToken, nil
	})
	return items, err
}

func (l *BitableLoader) paginate(ctx context.Context, budget *resourceBudget, base url.Values, pageSize int, fetch func(url.Values) (bool, string, error)) error {
	tracker := newPageTokenTracker()
	token := ""
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := budget.Page(); err != nil {
			return err
		}
		query := make(url.Values, len(base)+2)
		for key, values := range base {
			query[key] = append([]string(nil), values...)
		}
		query.Set("page_size", strconv.Itoa(pageSize))
		if token != "" {
			query.Set("page_token", token)
		}
		hasMore, next, err := fetch(query)
		if err != nil {
			return err
		}
		if !hasMore {
			return nil
		}
		if next == "" || tracker.Advance(next) != nil {
			return ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
		}
		token = next
	}
}

func (l *BitableLoader) recordsTable(ctx context.Context, records []bitableRecord) (string, error) {
	if len(records) == 0 {
		return "", nil
	}
	fieldSet := make(map[string]struct{})
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		for field := range record.Normalized {
			fieldSet[field] = struct{}{}
		}
	}
	fields := make([]string, 0, len(fieldSet))
	for field := range fieldSet {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	rows := make([][]string, 0, len(records)+1)
	rows = append(rows, fields)
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		row := make([]string, 0, len(fields))
		for _, field := range fields {
			row = append(row, record.Normalized[field])
		}
		rows = append(rows, row)
	}
	return l.normalizer.Table(rows), nil
}

func flattenBitableValue(value any) string {
	result, _ := normalizeBitableValue(context.Background(), newResourceBudget(ResourceLimits{}), value, 0)
	return result
}

func normalizeBitableValue(ctx context.Context, budget *resourceBudget, value any, depth int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := budget.Depth(depth); err != nil {
		return "", err
	}
	switch value := value.(type) {
	case nil:
		return "", nil
	case string:
		if err := budget.CheckAdditionalRetainedBytes(len(value)); err != nil {
			return "", err
		}
		return value, nil
	case bool:
		return strconv.FormatBool(value), nil
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64), nil
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			part, err := normalizeBitableValue(ctx, budget, item, depth+1)
			if err != nil {
				return "", err
			}
			if part != "" {
				parts = append(parts, part)
			}
		}
		total := 0
		for _, part := range parts {
			total += len(part)
		}
		if len(parts) > 1 {
			total += (len(parts) - 1) * 2
		}
		if err := budget.CheckAdditionalRetainedBytes(total); err != nil {
			return "", err
		}
		return strings.Join(parts, "; "), nil
	case map[string]any:
		parts := make([]string, 0, 4)
		for _, key := range []string{"text", "name", "display_name"} {
			nested, exists := value[key]
			if !exists {
				continue
			}
			part, err := normalizeBitableValue(ctx, budget, nested, depth+1)
			if err != nil {
				return "", err
			}
			if part != "" {
				parts = append(parts, part)
			}
		}
		for _, key := range []string{"url", "link"} {
			if raw, ok := value[key].(string); ok && permanentBitableURL(raw) {
				if err := budget.Depth(depth + 1); err != nil {
					return "", err
				}
				parts = append(parts, raw)
			}
		}
		total := 0
		for _, part := range parts {
			total += len(part)
		}
		if len(parts) > 1 {
			total += (len(parts) - 1) * 2
		}
		if err := budget.CheckAdditionalRetainedBytes(total); err != nil {
			return "", err
		}
		return strings.Join(parts, "; "), nil
	default:
		return canonicalCell(value), nil
	}
}

func permanentBitableURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil && parsed.Port() == "" && parsed.RawQuery == "" && parsed.Fragment == ""
}

var _ ports.SourceLoader = (*BitableLoader)(nil)
