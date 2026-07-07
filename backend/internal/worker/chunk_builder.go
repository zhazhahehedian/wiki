package worker

import (
	"context"
	"fmt"
	"strings"

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
			// 前缀最多占预算一半，防止超长标题路径在每个切片上无限放大。
			prefix = capSectionPath(sec.Path, chunkSize/2, tok) + "\n\n"
		}
		effSize := chunkSize - tok.Count(prefix)
		if effSize < minSectionChunkSize {
			effSize = minSectionChunkSize
		}
		// overlap >= 有效预算时切片几乎全是上一片的尾巴，会产生近重复泛滥；收敛到预算的 1/3。
		effOverlap := overlap
		if effOverlap >= effSize {
			effOverlap = effSize / 3
		}
		chunks, err := split.Split(ctx, sec.Body, ports.SplitOptions{ChunkSize: effSize, Overlap: effOverlap})
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

// capSectionPath 把标题路径限制在 budget 个 token 内：先用 "…" 折叠中间层级
// （保留首尾段），仍超长时按 rune 硬截断。返回值仅用于切片内容前缀；
// metadata 中的 section_path 始终保留原始完整路径。
func capSectionPath(path string, budget int, tok *tokenizer.Tiktoken) string {
	if tok.Count(path) <= budget {
		return path
	}
	segs := strings.Split(path, " > ")
	if len(segs) >= 2 {
		collapsed := segs[0] + " > … > " + segs[len(segs)-1]
		if tok.Count(collapsed) <= budget {
			return collapsed
		}
		path = collapsed
	}
	return truncateToTokens(path, budget, tok)
}

// truncateToTokens 按 rune 从尾部截断 s，直到 token 数不超过 budget。
// 每轮至少去掉 1/8 的 rune，保证几何收敛且不破坏 UTF-8 边界。
func truncateToTokens(s string, budget int, tok *tokenizer.Tiktoken) string {
	runes := []rune(s)
	for len(runes) > 0 && tok.Count(string(runes)) > budget {
		cut := len(runes) / 8
		if cut < 1 {
			cut = 1
		}
		runes = runes[:len(runes)-cut]
	}
	return string(runes)
}
