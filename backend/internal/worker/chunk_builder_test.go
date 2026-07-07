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
