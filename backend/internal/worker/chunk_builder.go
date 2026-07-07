package worker

import (
	"context"
	"fmt"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/tokenizer"
)

// chunkPiece 是一个待嵌入的切片：内容已含标题路径前缀（若有），
// TokenCount 为前缀+正文的总 token 数。
type chunkPiece struct {
	Content    string
	TokenCount int
	Metadata   map[string]any
}

// minSectionChunkSize 防止超长标题路径把切片的有效预算压没。
const minSectionChunkSize = 100

// buildChunkPieces 把解析结果切成待嵌入的切片。
// Markdown 先按标题分节, 每个切片前拼接标题路径前缀(前缀计入 token 预算),
// 路径同时写入 metadata["section_path"]; 其他格式保持整体切分。
func buildChunkPieces(ctx context.Context, split ports.Splitter, parsed *ports.ParseResult, chunkSize, overlap int) ([]chunkPiece, error) {
	if parsed.Metadata["format"] != "markdown" {
		chunks, err := split.Split(ctx, parsed.Text, ports.SplitOptions{ChunkSize: chunkSize, Overlap: overlap})
		if err != nil {
			return nil, err
		}
		pieces := make([]chunkPiece, 0, len(chunks))
		for _, c := range chunks {
			pieces = append(pieces, chunkPiece{Content: c.Content, TokenCount: c.TokenCount, Metadata: map[string]any{}})
		}
		return pieces, nil
	}

	sections := SectionizeMarkdown(parsed.Text)
	if len(sections) == 0 {
		sections = []Section{{Path: "", Body: parsed.Text}}
	}

	tok := tokenizer.Default()
	var pieces []chunkPiece
	for _, sec := range sections {
		prefix := ""
		if sec.Path != "" {
			prefix = sec.Path + "\n\n"
		}
		effSize := chunkSize - tok.Count(prefix)
		if effSize < minSectionChunkSize {
			effSize = minSectionChunkSize
		}
		chunks, err := split.Split(ctx, sec.Body, ports.SplitOptions{ChunkSize: effSize, Overlap: overlap})
		if err != nil {
			return nil, fmt.Errorf("split section %q: %w", sec.Path, err)
		}
		for _, c := range chunks {
			content := prefix + c.Content
			pieces = append(pieces, chunkPiece{
				Content:    content,
				TokenCount: tok.Count(content),
				Metadata:   map[string]any{"section_path": sec.Path},
			})
		}
	}
	return pieces, nil
}
