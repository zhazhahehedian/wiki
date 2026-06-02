package parser

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	pdfreader "github.com/ledongthuc/pdf"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type PDF struct{}

func (PDF) Supports(mime string) bool {
	return mime == "application/pdf"
}

func (PDF) Parse(ctx context.Context, r io.Reader, mime string) (*ports.ParseResult, error) {
	_ = mime
	tmp, err := os.CreateTemp("", "pdf-*.pdf")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	size, err := io.Copy(tmp, r)
	if err != nil {
		return nil, fmt.Errorf("copy pdf to tmp: %w", err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	pdfDoc, err := pdfreader.NewReader(tmp, size)
	if err != nil {
		return nil, fmt.Errorf("pdf reader: %w", err)
	}

	var buf bytes.Buffer
	pages := pdfDoc.NumPage()
	for i := 1; i <= pages; i++ {
		p := pdfDoc.Page(i)
		if p.V.IsNull() {
			continue
		}
		t, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		buf.WriteString(t)
		buf.WriteString("\n\n")
	}

	return &ports.ParseResult{
		Text:     buf.String(),
		Metadata: map[string]any{"format": "pdf", "pages": pages},
	}, nil
}
