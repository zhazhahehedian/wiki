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
	}
	if config.MaxOutputBytes <= 0 {
		config.MaxOutputBytes = defaultBitableMaxOutputBytes
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
	RecordID string         `json:"record_id"`
	Fields   map[string]any `json:"fields"`
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
	tables, err := l.listTables(ctx, basePath, accessToken)
	if err != nil {
		return domain.CanonicalDocument{}, err
	}
	loaded := make([]loadedBitableView, 0)
	totalRows := 0
	outputBytes := 0
	for _, table := range tables {
		if ref.TableID != "" && ref.TableID != table.TableID {
			continue
		}
		views, err := l.listViews(ctx, basePath, table.TableID, accessToken)
		if err != nil {
			return domain.CanonicalDocument{}, err
		}
		if ref.ViewID == "" {
			records, err := l.listRecords(ctx, basePath, table.TableID, "", accessToken, &totalRows, &outputBytes)
			if err != nil {
				return domain.CanonicalDocument{}, err
			}
			loaded = append(loaded, loadedBitableView{table: table, records: records})
			continue
		}
		for _, view := range views {
			if ref.ViewID != view.ViewID {
				continue
			}
			records, err := l.listRecords(ctx, basePath, table.TableID, view.ViewID, accessToken, &totalRows, &outputBytes)
			if err != nil {
				return domain.CanonicalDocument{}, err
			}
			loaded = append(loaded, loadedBitableView{table: table, view: view, records: records})
		}
	}
	if (ref.TableID != "" || ref.ViewID != "") && len(loaded) == 0 {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadNotFound, nil)
	}

	parts := make([]string, 0, len(loaded))
	for _, item := range loaded {
		rowStart, rowEnd := rowBounds(len(item.records))
		heading := item.table.Name
		comment := fmt.Sprintf(`<!-- feishu-bitable table_id="%s" rows="%d-%d" -->`, item.table.TableID, rowStart, rowEnd)
		if item.view.ViewID != "" {
			heading += " / " + item.view.ViewName
			comment = fmt.Sprintf(`<!-- feishu-bitable table_id="%s" view_id="%s" rows="%d-%d" -->`, item.table.TableID, item.view.ViewID, rowStart, rowEnd)
		}
		section := []string{l.normalizer.Heading(2, heading), comment}
		if table := l.recordsTable(item.records); table != "" {
			section = append(section, table)
		}
		parts = append(parts, strings.Join(section, "\n\n"))
	}
	markdown := l.normalizer.Finalize(parts)
	if len(markdown) > l.config.MaxOutputBytes {
		return domain.CanonicalDocument{}, ports.NewSourceLoadError(ports.SourceLoadTooLarge, nil)
	}

	metadataInput := domain.SourceMetadataInput{
		SourceType: domain.ResourceBitable, SectionPath: app.App.Name,
		RemoteRevision: strconv.FormatInt(app.App.Revision, 10), SourceLocator: ref.CanonicalURL,
	}
	if ref.TableID != "" {
		metadataInput.TableID = ref.TableID
	}
	if len(loaded) == 1 {
		metadataInput.TableID = loaded[0].table.TableID
		metadataInput.ViewID = loaded[0].view.ViewID
		metadataInput.RowStart, metadataInput.RowEnd = rowBounds(len(loaded[0].records))
	} else if totalRows > 0 {
		metadataInput.RowStart, metadataInput.RowEnd = 1, totalRows
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

func (l *BitableLoader) listTables(ctx context.Context, basePath, accessToken string) ([]bitableTable, error) {
	items := make([]bitableTable, 0)
	err := l.paginate(nil, 100, func(query url.Values) (bool, string, error) {
		var page bitableTablePage
		if err := l.client.Get(ctx, accessToken, basePath+"/tables", query, &page); err != nil {
			return false, "", err
		}
		for _, item := range page.Items {
			if !validAPIIdentifier(item.TableID) || strings.TrimSpace(item.Name) == "" {
				return false, "", ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
			}
		}
		items = append(items, page.Items...)
		return page.HasMore, page.PageToken, nil
	})
	return items, err
}

func (l *BitableLoader) listViews(ctx context.Context, basePath, tableID, accessToken string) ([]bitableView, error) {
	items := make([]bitableView, 0)
	path := basePath + "/tables/" + tableID + "/views"
	err := l.paginate(nil, 100, func(query url.Values) (bool, string, error) {
		var page bitableViewPage
		if err := l.client.Get(ctx, accessToken, path, query, &page); err != nil {
			return false, "", err
		}
		for _, item := range page.Items {
			if !validAPIIdentifier(item.ViewID) || strings.TrimSpace(item.ViewName) == "" {
				return false, "", ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
			}
		}
		items = append(items, page.Items...)
		return page.HasMore, page.PageToken, nil
	})
	return items, err
}

func (l *BitableLoader) listRecords(ctx context.Context, basePath, tableID, viewID, accessToken string, totalRows, outputBytes *int) ([]bitableRecord, error) {
	items := make([]bitableRecord, 0)
	path := basePath + "/tables/" + tableID + "/records"
	baseQuery := url.Values{"view_id": {viewID}}
	err := l.paginate(baseQuery, 500, func(query url.Values) (bool, string, error) {
		var page bitableRecordPage
		if err := l.client.Get(ctx, accessToken, path, query, &page); err != nil {
			return false, "", err
		}
		if *totalRows+len(page.Items) > l.config.MaxRows {
			return false, "", ports.NewSourceLoadError(ports.SourceLoadTooLarge, nil)
		}
		for _, item := range page.Items {
			if item.RecordID == "" {
				return false, "", ports.NewSourceLoadError(ports.SourceLoadMalformed, nil)
			}
			*outputBytes += len(item.RecordID)
			for field, value := range item.Fields {
				*outputBytes += len(field) + len(flattenBitableValue(value))
			}
			if *outputBytes > l.config.MaxOutputBytes {
				return false, "", ports.NewSourceLoadError(ports.SourceLoadTooLarge, nil)
			}
		}
		*totalRows += len(page.Items)
		items = append(items, page.Items...)
		return page.HasMore, page.PageToken, nil
	})
	return items, err
}

func (l *BitableLoader) paginate(base url.Values, pageSize int, fetch func(url.Values) (bool, string, error)) error {
	tracker := newPageTokenTracker()
	token := ""
	for {
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

func (l *BitableLoader) recordsTable(records []bitableRecord) string {
	if len(records) == 0 {
		return ""
	}
	fieldSet := make(map[string]struct{})
	for _, record := range records {
		for field := range record.Fields {
			fieldSet[field] = struct{}{}
		}
	}
	fields := make([]string, 0, len(fieldSet))
	for field := range fieldSet {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	rows := make([][]string, 0, len(records)+1)
	rows = append(rows, append([]string{"Record ID"}, fields...))
	for _, record := range records {
		row := make([]string, 1, len(fields)+1)
		row[0] = record.RecordID
		for _, field := range fields {
			row = append(row, flattenBitableValue(record.Fields[field]))
		}
		rows = append(rows, row)
	}
	return l.normalizer.Table(rows)
}

func flattenBitableValue(value any) string {
	switch value := value.(type) {
	case nil:
		return ""
	case string:
		return value
	case bool:
		return strconv.FormatBool(value)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case []any:
		parts := make([]string, len(value))
		for i, item := range value {
			parts[i] = flattenBitableValue(item)
		}
		return strings.Join(parts, "; ")
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, key+"="+flattenBitableValue(value[key]))
		}
		return strings.Join(parts, "; ")
	default:
		return canonicalCell(value)
	}
}

var _ ports.SourceLoader = (*BitableLoader)(nil)
