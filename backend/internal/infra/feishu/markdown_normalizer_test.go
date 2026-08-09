package feishu

import "testing"

func TestMarkdownNormalizerEscapesTextAndNormalizesStructure(t *testing.T) {
	n := NewMarkdownNormalizer()
	got := n.Normalize("####### Heading\n\n\n\nvalue *x* [y]\\z\n\n")
	want := "###### Heading\n\nvalue \\*x\\* \\[y\\]\\\\z\n"
	if got != want {
		t.Fatalf("Normalize() = %q, want %q", got, want)
	}
}

func TestMarkdownNormalizerBuildsUnicodeTableAndUnsupportedImage(t *testing.T) {
	n := NewMarkdownNormalizer()
	got := n.Table([][]string{{"姓名", "说明"}, {"张三", "a|b"}})
	want := "| 姓名 | 说明 |\n| --- | --- |\n| 张三 | a\\|b |"
	if got != want {
		t.Fatalf("Table() = %q, want %q", got, want)
	}
	if got := n.UnsupportedImage("架构图", "https://acme.feishu.cn/docx/doc"); got != "> [Image: 架构图](https://acme.feishu.cn/docx/doc)" {
		t.Fatalf("UnsupportedImage() = %q", got)
	}
}

func TestMarkdownNormalizerEscapesBlockAndTableDelimitersInText(t *testing.T) {
	n := NewMarkdownNormalizer()
	got := n.Paragraph("# heading - item + item | cell!")
	want := `\# heading \- item \+ item \| cell\!`
	if got != want {
		t.Fatalf("Paragraph() = %q, want %q", got, want)
	}
}

func TestMarkdownNormalizerKeepsMultilineCellInsideTableRow(t *testing.T) {
	got := NewMarkdownNormalizer().Table([][]string{{"H"}, {"line one\nline two"}})
	want := "| H |\n| --- |\n| line one line two |"
	if got != want {
		t.Fatalf("Table() = %q, want %q", got, want)
	}
}

func TestMarkdownNormalizerEscapesParagraphBlockOpeners(t *testing.T) {
	got := NewMarkdownNormalizer().Paragraph("1. ordered\n2) ordered\n~~~go\ncode\n~~~\n===")
	want := "1\\. ordered\n2\\) ordered\n\\~\\~\\~go\ncode\n\\~\\~\\~\n\\=\\=\\="
	if got != want {
		t.Fatalf("Paragraph() = %q, want %q", got, want)
	}
}
