package splitter

import (
	"context"
	"os"
	"strings"
	"testing"

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
