package parser

import (
	"bytes"
	"context"
	"io"
	"strings"

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
	decoded, err := decodeTextBytes(src)
	if err != nil {
		return nil, err
	}
	src = []byte(decoded)

	md := goldmark.New()
	root := md.Parser().Parse(text.NewReader(src))

	var buf bytes.Buffer
	renderNode(root, src, &buf)

	return &ports.ParseResult{
		Text:     strings.TrimRight(buf.String(), "\n") + "\n",
		Metadata: map[string]any{"format": "markdown"},
	}, nil
}

// renderNode 把 goldmark AST 渲染回保留结构的纯文本:
// 标题保留 # 层级标记, 代码块保留 ``` 围栏与全部内容, 列表保留 - 标记。
func renderNode(n ast.Node, src []byte, w *bytes.Buffer) {
	switch node := n.(type) {
	case *ast.Heading:
		w.WriteString(strings.Repeat("#", node.Level))
		w.WriteString(" ")
		writeInlineText(node, src, w)
		w.WriteString("\n\n")
		return
	case *ast.FencedCodeBlock:
		w.WriteString("```")
		if lang := node.Language(src); lang != nil {
			w.Write(lang)
		}
		w.WriteString("\n")
		writeCodeLines(node, src, w)
		w.WriteString("```\n\n")
		return
	case *ast.CodeBlock:
		w.WriteString("```\n")
		writeCodeLines(node, src, w)
		w.WriteString("```\n\n")
		return
	case *ast.Paragraph:
		writeInlineText(node, src, w)
		w.WriteString("\n\n")
		return
	case *ast.List:
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			renderNode(c, src, w)
		}
		w.WriteString("\n")
		return
	case *ast.ListItem:
		w.WriteString("- ")
		if fc := n.FirstChild(); fc != nil {
			writeInlineText(fc, src, w)
		}
		w.WriteString("\n")
		// 嵌套列表继续按列表渲染
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if _, ok := c.(*ast.List); ok {
				renderNode(c, src, w)
			}
		}
		return
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		renderNode(c, src, w)
	}
}

func writeInlineText(n ast.Node, src []byte, w *bytes.Buffer) {
	if t, ok := n.(*ast.Text); ok {
		w.Write(t.Segment.Value(src))
		if t.SoftLineBreak() || t.HardLineBreak() {
			w.WriteString(" ")
		}
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		writeInlineText(c, src, w)
	}
}

func writeCodeLines(n ast.Node, src []byte, w *bytes.Buffer) {
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		w.Write(seg.Value(src))
	}
}
