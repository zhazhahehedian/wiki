package worker

import (
	"os"
	"strings"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/infra/tokenizer"
)

func TestMain(m *testing.M) {
	if err := tokenizer.Init("cl100k_base"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestSectionizeMarkdownBuildsHeadingPaths(t *testing.T) {
	text := "# 部署指南\n\n总体说明。\n\n## 环境变量\n\n配置 DATABASE_URL。\n\n## 启动\n\n运行 compose。\n\n# 附录\n\n其他。"
	sections := SectionizeMarkdown(text)
	if len(sections) != 4 {
		t.Fatalf("len(sections) = %d, want 4: %#v", len(sections), sections)
	}
	wantPaths := []string{"部署指南", "部署指南 > 环境变量", "部署指南 > 启动", "附录"}
	for i, want := range wantPaths {
		if sections[i].Path != want {
			t.Errorf("sections[%d].Path = %q, want %q", i, sections[i].Path, want)
		}
	}
	if sections[1].Body != "配置 DATABASE_URL。" {
		t.Errorf("sections[1].Body = %q", sections[1].Body)
	}
}

func TestSectionizeMarkdownPrefaceHasEmptyPath(t *testing.T) {
	text := "开头没有标题的前言。\n\n# 第一章\n\n正文。"
	sections := SectionizeMarkdown(text)
	if len(sections) != 2 {
		t.Fatalf("len(sections) = %d, want 2", len(sections))
	}
	if sections[0].Path != "" || sections[0].Body != "开头没有标题的前言。" {
		t.Errorf("preface section = %+v", sections[0])
	}
}

func TestSectionizeMarkdownIgnoresHashInsideCodeFence(t *testing.T) {
	text := "# 脚本\n\n```bash\n# 这是注释不是标题\necho hi\n```"
	sections := SectionizeMarkdown(text)
	if len(sections) != 1 {
		t.Fatalf("len(sections) = %d, want 1: %#v", len(sections), sections)
	}
	if sections[0].Path != "脚本" {
		t.Errorf("Path = %q, want 脚本", sections[0].Path)
	}
}

func TestSectionizeMarkdownSkipsHeadingWithEmptyBody(t *testing.T) {
	text := "# 只有标题\n\n## 也只有标题\n\n### 有正文\n\n内容。"
	sections := SectionizeMarkdown(text)
	if len(sections) != 1 {
		t.Fatalf("len(sections) = %d, want 1: %#v", len(sections), sections)
	}
	if sections[0].Path != "只有标题 > 也只有标题 > 有正文" {
		t.Errorf("Path = %q", sections[0].Path)
	}
}

func TestSectionizeMarkdownLengthAwareFenceTracking(t *testing.T) {
	text := "# 教程\n\n````\n```\n# 不是标题\n```\n````\n\n## 真标题\n\n正文。"
	sections := SectionizeMarkdown(text)
	if len(sections) != 2 {
		t.Fatalf("len(sections) = %d, want 2: %#v", len(sections), sections)
	}
	if sections[0].Path != "教程" {
		t.Errorf("sections[0].Path = %q, want 教程", sections[0].Path)
	}
	if sections[1].Path != "教程 > 真标题" {
		t.Errorf("sections[1].Path = %q, want 教程 > 真标题", sections[1].Path)
	}
	if !strings.Contains(sections[0].Body, "# 不是标题") {
		t.Errorf("fence content missing from body: %q", sections[0].Body)
	}
}
