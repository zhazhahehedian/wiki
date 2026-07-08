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
		writeFencedCode(node, node.Language(src), src, w)
		return
	case *ast.CodeBlock:
		writeFencedCode(node, nil, src, w)
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
			// 首子节点若是嵌套列表, 交给下方循环渲染, 避免重复输出
			if _, ok := fc.(*ast.List); !ok {
				writeInlineText(fc, src, w)
			}
		}
		w.WriteString("\n")
		// 嵌套列表与代码块继续按块渲染
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			switch c.(type) {
			case *ast.List, *ast.FencedCodeBlock, *ast.CodeBlock:
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

// writeFencedCode 输出带围栏的代码块。围栏长度取 max(3, 内容中最长
// 行首反引号串+1), 保证围栏与内容中的 ``` 行不冲突, 输出始终成对闭合。
func writeFencedCode(n ast.Node, lang, src []byte, w *bytes.Buffer) {
	var code bytes.Buffer
	writeCodeLines(n, src, &code)

	fence := codeFence(code.Bytes())
	w.WriteString(fence)
	if lang != nil {
		w.Write(lang)
	}
	w.WriteString("\n")
	w.Write(code.Bytes())
	if code.Len() > 0 && !bytes.HasSuffix(code.Bytes(), []byte("\n")) {
		w.WriteString("\n")
	}
	w.WriteString(fence)
	w.WriteString("\n\n")
}

// codeFence 根据代码内容计算不冲突的围栏字符串。
func codeFence(code []byte) string {
	longest := 0
	for _, line := range bytes.Split(code, []byte("\n")) {
		trimmed := bytes.TrimLeft(line, " \t")
		run := 0
		for run < len(trimmed) && trimmed[run] == '`' {
			run++
		}
		if run > longest {
			longest = run
		}
	}
	n := longest + 1
	if n < 3 {
		n = 3
	}
	return strings.Repeat("`", n)
}
