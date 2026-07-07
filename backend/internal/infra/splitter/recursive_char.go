package splitter

import (
	"context"
	"strings"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/tokenizer"
)

var defaultSeparators = []string{"\n\n", "。", "！", "？", ". ", "! ", "? ", "\n", " ", ""}

type RecursiveChar struct {
	separators []string
}

func New() *RecursiveChar {
	return &RecursiveChar{separators: defaultSeparators}
}

func (s *RecursiveChar) Split(ctx context.Context, text string, opts ports.SplitOptions) ([]ports.SplitChunk, error) {
	tok := tokenizer.Default()
	pieces := s.splitRecursive(text, opts.ChunkSize, tok)
	chunks := mergeWithOverlap(pieces, opts.ChunkSize, opts.Overlap, tok)

	out := make([]ports.SplitChunk, 0, len(chunks))
	for i, c := range chunks {
		out = append(out, ports.SplitChunk{
			Seq:        i,
			Content:    c,
			TokenCount: tok.Count(c),
		})
	}
	return out, nil
}

func (s *RecursiveChar) splitRecursive(text string, chunkSize int, tok *tokenizer.Tiktoken) []string {
	if tok.Count(text) <= chunkSize {
		return []string{text}
	}
	for _, sep := range s.separators {
		if sep == "" {
			return hardSplitByToken(text, chunkSize, tok)
		}
		if !strings.Contains(text, sep) {
			continue
		}
		parts := strings.SplitAfter(text, sep)
		var result []string
		for _, p := range parts {
			if p == "" {
				continue
			}
			// SplitAfter 保留分隔符；若整段无法再被该分隔符切小（如超长单句以
			// 该分隔符结尾），换下一级分隔符，避免无限递归。
			if p == text {
				result = nil
				break
			}
			result = append(result, s.splitRecursive(p, chunkSize, tok)...)
		}
		if result == nil {
			continue
		}
		return result
	}
	return []string{text}
}

func hardSplitByToken(text string, chunkSize int, tok *tokenizer.Tiktoken) []string {
	var out []string
	var current strings.Builder
	for _, r := range text {
		next := current.String() + string(r)
		if current.Len() > 0 && tok.Count(next) > chunkSize {
			out = append(out, current.String())
			current.Reset()
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		out = append(out, current.String())
	}
	return out
}

func mergeWithOverlap(pieces []string, chunkSize, overlap int, tok *tokenizer.Tiktoken) []string {
	if len(pieces) == 0 {
		return nil
	}
	var chunks []string
	var current strings.Builder
	curCount := 0

	for _, p := range pieces {
		pCount := tok.Count(p)
		if curCount+pCount > chunkSize && current.Len() > 0 {
			chunks = append(chunks, current.String())
			tail := tailTokens(current.String(), overlap, tok)
			current.Reset()
			current.WriteString(tail)
			curCount = tok.Count(tail)
		}
		current.WriteString(p)
		curCount += pCount
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

func tailTokens(s string, n int, tok *tokenizer.Tiktoken) string {
	if n <= 0 {
		return ""
	}
	if tok.Count(s) <= n {
		return s
	}

	runes := []rune(s)
	start := len(runes)
	for start > 0 {
		candidate := string(runes[start-1:])
		if tok.Count(candidate) > n {
			break
		}
		start--
	}
	return string(runes[start:])
}
