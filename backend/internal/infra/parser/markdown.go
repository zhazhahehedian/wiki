package parser

import (
	"bytes"
	"context"
	"io"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type Markdown struct{}

func (Markdown) Supports(mime string) bool {
	return mime == "text/markdown" || mime == "text/x-markdown"
}

func (Markdown) Parse(ctx context.Context, r io.Reader, mime string) (*ports.ParseResult, error) {
	_ = mime
	src, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	md := goldmark.New()
	root := md.Parser().Parse(text.NewReader(src))

	var buf bytes.Buffer
	walkText(root, src, &buf)

	return &ports.ParseResult{
		Text:     buf.String(),
		Metadata: map[string]any{"format": "markdown"},
	}, nil
}

func walkText(n ast.Node, src []byte, w *bytes.Buffer) {
	if n == nil {
		return
	}
	if t, ok := n.(*ast.Text); ok {
		w.Write(t.Segment.Value(src))
	}
	switch n.(type) {
	case *ast.Paragraph, *ast.Heading, *ast.ListItem, *ast.CodeBlock, *ast.FencedCodeBlock:
		defer w.WriteString("\n\n")
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		walkText(c, src, w)
	}
}
