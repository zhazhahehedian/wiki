package parser

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type XLSX struct{}

func (XLSX) Supports(mime string) bool {
	return mime == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
}

func (XLSX) Parse(ctx context.Context, r io.Reader, mime string) (*ports.ParseResult, error) {
	_ = mime
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	f, err := excelize.OpenReader(bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("excelize open: %w", err)
	}
	defer f.Close()

	var out strings.Builder
	sheets := f.GetSheetList()
	for _, sheet := range sheets {
		out.WriteString("# Sheet: ")
		out.WriteString(sheet)
		out.WriteString("\n\n")
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		for _, row := range rows {
			out.WriteString(strings.Join(row, "\t"))
			out.WriteString("\n")
		}
		out.WriteString("\n")
	}

	return &ports.ParseResult{
		Text:     out.String(),
		Metadata: map[string]any{"format": "xlsx", "sheets": len(sheets)},
	}, nil
}
