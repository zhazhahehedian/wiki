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
		parts := strings.Split(text, sep)
		var result []string
		for _, p := range parts {
			if p == "" {
				continue
			}
			result = append(result, s.splitRecursive(p, chunkSize, tok)...)
		}
		return result
	}
	return []string{text}
}

func hardSplitByToken(text string, chunkSize int, tok *tokenizer.Tiktoken) []string {
	ids := tok.Encode(text)
	var out []string
	for i := 0; i < len(ids); i += chunkSize {
		end := i + chunkSize
		if end > len(ids) {
			end = len(ids)
		}
		out = append(out, tok.Decode(ids[i:end]))
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
	ids := tok.Encode(s)
	if len(ids) <= n {
		return s
	}
	return tok.Decode(ids[len(ids)-n:])
}
