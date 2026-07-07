package parser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestMarkdownParse(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "hello.md"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	res, err := (Markdown{}).Parse(context.Background(), f, "text/markdown")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.Contains(res.Text, "Hello") {
		t.Errorf("expected 'Hello' in text, got %q", res.Text)
	}
	if !strings.Contains(res.Text, "Item one") {
		t.Errorf("expected 'Item one' in text, got %q", res.Text)
	}
}

func TestMarkdownParsePreservesCodeBlockContent(t *testing.T) {
	md := "# 标题\n\n正文段落。\n\n```bash\ngoose -dir migrations up\n```\n\n结尾。"
	res, err := (Markdown{}).Parse(context.Background(), strings.NewReader(md), "text/markdown")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.Contains(res.Text, "goose -dir migrations up") {
		t.Errorf("code block content missing from parsed text: %q", res.Text)
	}
	if !strings.Contains(res.Text, "```bash") {
		t.Errorf("code fence with language missing: %q", res.Text)
	}
}

func TestMarkdownParsePreservesHeadingMarkers(t *testing.T) {
	md := "# 部署指南\n\n## 环境变量\n\n配置 DATABASE_URL。"
	res, err := (Markdown{}).Parse(context.Background(), strings.NewReader(md), "text/markdown")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.Contains(res.Text, "# 部署指南") {
		t.Errorf("level-1 heading marker missing: %q", res.Text)
	}
	if !strings.Contains(res.Text, "## 环境变量") {
		t.Errorf("level-2 heading marker missing: %q", res.Text)
	}
}

func TestMarkdownParseRendersListItems(t *testing.T) {
	md := "前言\n\n- 第一项\n- 第二项"
	res, err := (Markdown{}).Parse(context.Background(), strings.NewReader(md), "text/markdown")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.Contains(res.Text, "- 第一项") || !strings.Contains(res.Text, "- 第二项") {
		t.Errorf("list items missing or unmarked: %q", res.Text)
	}
}

func TestDispatcherSupports(t *testing.T) {
	d := NewDispatcher()
	cases := map[string]bool{
		"text/markdown":   true,
		"application/pdf": true,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":       true,
		"application/octet-stream": false,
	}
	for mime, want := range cases {
		if got := d.Supports(mime); got != want {
			t.Errorf("Supports(%q) = %v, want %v", mime, got, want)
		}
	}
}

func TestDispatcherFallback(t *testing.T) {
	d := NewDispatcher()
	res, err := d.Parse(context.Background(), strings.NewReader("plain text"), "application/octet-stream")
	if err != nil {
		t.Fatalf("fallback parse: %v", err)
	}
	if res.Text != "plain text" {
		t.Errorf("fallback text mismatch: %q", res.Text)
	}
}

func TestDispatcherFallbackDecodesGB18030Text(t *testing.T) {
	raw, err := simplifiedchinese.GB18030.NewEncoder().String("斗地主流程说明")
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}

	res, err := NewDispatcher().Parse(context.Background(), strings.NewReader(raw), "application/octet-stream")
	if err != nil {
		t.Fatalf("fallback parse: %v", err)
	}
	if !utf8.ValidString(res.Text) {
		t.Fatalf("parsed text is not valid UTF-8: %q", res.Text)
	}
	if res.Text != "斗地主流程说明" {
		t.Fatalf("parsed text = %q, want decoded Chinese text", res.Text)
	}
}

func TestMarkdownParseDecodesGB18030Text(t *testing.T) {
	raw, err := simplifiedchinese.GB18030.NewEncoder().String("# 斗地主流程说明\n\n正文")
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}

	res, err := (Markdown{}).Parse(context.Background(), strings.NewReader(raw), "text/markdown")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !utf8.ValidString(res.Text) {
		t.Fatalf("parsed text is not valid UTF-8: %q", res.Text)
	}
	if !strings.Contains(res.Text, "斗地主流程说明") {
		t.Fatalf("parsed text = %q, want decoded heading", res.Text)
	}
}
