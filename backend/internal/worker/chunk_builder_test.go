package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/splitter"
)

func TestBuildChunkPiecesPrefixesSectionPathForMarkdown(t *testing.T) {
	parsed := &ports.ParseResult{
		Text:     "# 部署指南\n\n## 环境变量\n\n配置 DATABASE_URL 即可。",
		Metadata: map[string]any{"format": "markdown"},
	}
	pieces, err := buildChunkPieces(context.Background(), splitter.New(), parsed, 400, 60)
	if err != nil {
		t.Fatalf("buildChunkPieces: %v", err)
	}
	if len(pieces) == 0 {
		t.Fatal("no pieces produced")
	}
	p := pieces[0]
	if !strings.HasPrefix(p.Content, "部署指南 > 环境变量\n\n") {
		t.Errorf("content missing section path prefix: %q", p.Content)
	}
	if p.Metadata["section_path"] != "部署指南 > 环境变量" {
		t.Errorf("metadata section_path = %v", p.Metadata["section_path"])
	}
	if p.TokenCount <= 0 {
		t.Errorf("TokenCount = %d, want > 0", p.TokenCount)
	}
}

func TestBuildChunkPiecesPassThroughForNonMarkdown(t *testing.T) {
	parsed := &ports.ParseResult{
		Text:     "纯文本内容，无结构。",
		Metadata: map[string]any{"format": "unknown"},
	}
	pieces, err := buildChunkPieces(context.Background(), splitter.New(), parsed, 400, 60)
	if err != nil {
		t.Fatalf("buildChunkPieces: %v", err)
	}
	if len(pieces) != 1 {
		t.Fatalf("len(pieces) = %d, want 1", len(pieces))
	}
	if pieces[0].Content != "纯文本内容，无结构。" {
		t.Errorf("content changed for non-markdown: %q", pieces[0].Content)
	}
	if got := pieces[0].Metadata["section_path"]; got != nil {
		t.Errorf("non-markdown should not set section_path, got %v", got)
	}
}

func TestBuildChunkPiecesFallsBackWhenNoSections(t *testing.T) {
	parsed := &ports.ParseResult{
		Text:     "# 只有标题一\n\n## 只有标题二",
		Metadata: map[string]any{"format": "markdown"},
	}
	pieces, err := buildChunkPieces(context.Background(), splitter.New(), parsed, 400, 60)
	if err != nil {
		t.Fatalf("buildChunkPieces: %v", err)
	}
	if len(pieces) == 0 {
		t.Fatal("expected fallback to whole-text split, got 0 pieces")
	}
}

func TestBuildChunkPiecesBoundsLongHeadingPath(t *testing.T) {
	longTitle := strings.Repeat("超长部署标题", 80)
	body := strings.Repeat("这是一个用于测试切分行为的完整句子。", 40)
	parsed := &ports.ParseResult{
		Text:     "# " + longTitle + "\n\n" + body,
		Metadata: map[string]any{"format": "markdown"},
	}
	chunkSize, overlap := 200, 150 // overlap 故意 >= 被前缀压缩后的有效预算
	pieces, err := buildChunkPieces(context.Background(), splitter.New(), parsed, chunkSize, overlap)
	if err != nil {
		t.Fatalf("buildChunkPieces: %v", err)
	}
	if len(pieces) == 0 {
		t.Fatal("no pieces produced")
	}
	// 未截断前缀 + 未收敛 overlap 会导致近重复切片泛滥（几十片）；修复后应保持有界。
	if len(pieces) > 20 {
		t.Errorf("len(pieces) = %d, want <= 20 (near-duplicate chunk flood)", len(pieces))
	}
	for i, p := range pieces {
		if p.TokenCount > chunkSize+chunkSize/5 {
			t.Errorf("pieces[%d].TokenCount = %d, want <= %d (prefix amplification)", i, p.TokenCount, chunkSize+chunkSize/5)
		}
		if p.Metadata["section_path"] != longTitle {
			t.Errorf("pieces[%d] metadata section_path should keep full original path", i)
		}
	}
}

func TestBuildChunkPiecesCollapsesMiddleSegmentsInPrefix(t *testing.T) {
	longMiddle := strings.Repeat("很长的中间标题", 60)
	fullPath := "第一章 > " + longMiddle + " > 收尾小节"
	parsed := &ports.ParseResult{
		Text:     "# 第一章\n\n## " + longMiddle + "\n\n### 收尾小节\n\n正文内容只有一句。",
		Metadata: map[string]any{"format": "markdown"},
	}
	pieces, err := buildChunkPieces(context.Background(), splitter.New(), parsed, 200, 60)
	if err != nil {
		t.Fatalf("buildChunkPieces: %v", err)
	}
	if len(pieces) != 1 {
		t.Fatalf("len(pieces) = %d, want 1", len(pieces))
	}
	if !strings.HasPrefix(pieces[0].Content, "第一章 > … > 收尾小节\n\n") {
		got := pieces[0].Content
		if len(got) > 80 {
			got = got[:80] + "..."
		}
		t.Errorf("content prefix should collapse middle segments, got %q", got)
	}
	if pieces[0].Metadata["section_path"] != fullPath {
		t.Errorf("metadata section_path should keep full original path")
	}
}
