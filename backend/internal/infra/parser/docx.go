package parser

import (
	"bytes"
	"context"
	"fmt"
	"io"

	docconv "code.sajari.com/docconv/v2"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type DOCX struct{}

func (DOCX) Supports(mime string) bool {
	return mime == "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
}

func (DOCX) Parse(ctx context.Context, r io.Reader, mime string) (*ports.ParseResult, error) {
	_ = mime
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	text, meta, err := docconv.ConvertDocx(bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("docconv: %w", err)
	}
	out := map[string]any{"format": "docx"}
	for k, v := range meta {
		out[k] = v
	}
	return &ports.ParseResult{
		Text:     text,
		Metadata: out,
	}, nil
}
