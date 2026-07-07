package splitter

import (
	"context"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/tokenizer"
)

func TestMain(m *testing.M) {
	if err := tokenizer.Init("cl100k_base"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestSplitShortTextReturnsSingleChunk(t *testing.T) {
	s := New()
	chunks, err := s.Split(context.Background(), "hello world", ports.SplitOptions{ChunkSize: 800, Overlap: 120})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Content != "hello world" {
		t.Errorf("content mismatch: %q", chunks[0].Content)
	}
}

func TestSplitLongTextProducesMultipleChunks(t *testing.T) {
	para := strings.Repeat("This is a test sentence used to verify the splitter behavior. ", 100)
	text := para + "\n\n" + para
	s := New()
	chunks, err := s.Split(context.Background(), text, ports.SplitOptions{ChunkSize: 400, Overlap: 50})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("want >= 2 chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if c.TokenCount > 400+50 {
			t.Errorf("chunk %d token count %d exceeds limit", i, c.TokenCount)
		}
		if c.Seq != i {
			t.Errorf("chunk %d seq = %d", i, c.Seq)
		}
	}
}

func TestSplitChineseText(t *testing.T) {
	text := strings.Repeat("这是一段中文测试。", 200)
	s := New()
	chunks, err := s.Split(context.Background(), text, ports.SplitOptions{ChunkSize: 400, Overlap: 50})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("want >= 2 chunks, got %d", len(chunks))
	}
}

func TestSplitWithZeroOverlapReconstructsOriginalText(t *testing.T) {
	text := strings.Repeat("这是一段中文测试。", 200)
	s := New()
	chunks, err := s.Split(context.Background(), text, ports.SplitOptions{ChunkSize: 400, Overlap: 0})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("want >= 2 chunks, got %d", len(chunks))
	}
	var joined strings.Builder
	for _, c := range chunks {
		joined.WriteString(c.Content)
	}
	if joined.String() != text {
		t.Errorf("joined chunks do not reconstruct original text:\n  original length %d\n  joined length %d\n  first chunk: %q",
			len(text), joined.Len(), chunks[0].Content)
	}
}

func TestSplitChunksAreContiguousSubstringsOfOriginal(t *testing.T) {
	text := strings.Repeat("第一段的内容用于验证分隔符保留。", 30) +
		"\n\n" +
		strings.Repeat("Second paragraph content for separator checks. ", 40)
	s := New()
	chunks, err := s.Split(context.Background(), text, ports.SplitOptions{ChunkSize: 200, Overlap: 30})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("want >= 2 chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if !strings.Contains(text, c.Content) {
			t.Errorf("chunk %d is not a contiguous substring of the original text: %q", i, c.Content)
		}
	}
}

func TestSplitLongSentenceEndingWithSeparatorFallsBackToHardSplit(t *testing.T) {
	// 唯一的分隔符出现在文本末尾，SplitAfter 无法把文本切小，必须回退到硬切。
	text := strings.Repeat("长", 300) + "。"
	s := New()
	chunks, err := s.Split(context.Background(), text, ports.SplitOptions{ChunkSize: 80, Overlap: 0})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("want >= 2 chunks, got %d", len(chunks))
	}
	var joined strings.Builder
	for _, c := range chunks {
		joined.WriteString(c.Content)
	}
	if joined.String() != text {
		t.Errorf("joined chunks do not reconstruct original text: joined length %d, want %d", joined.Len(), len(text))
	}
}

func TestHardSplitChineseTextReturnsValidUTF8Chunks(t *testing.T) {
	text := strings.Repeat("\u6597\u5730\u4e3b\u6d41\u7a0b\u8bf4\u660e", 300)
	s := New()

	chunks, err := s.Split(context.Background(), text, ports.SplitOptions{ChunkSize: 80, Overlap: 10})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("want >= 2 chunks, got %d", len(chunks))
	}
	for i, chunk := range chunks {
		if !utf8.ValidString(chunk.Content) {
			t.Fatalf("chunk %d is not valid UTF-8: %q", i, chunk.Content)
		}
	}
}
