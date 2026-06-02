package parser

import (
	"context"
	"fmt"
	"io"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type Dispatcher struct {
	parsers []ports.Parser
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		parsers: []ports.Parser{
			Markdown{},
			PDF{},
			DOCX{},
			XLSX{},
		},
	}
}

func (d *Dispatcher) Supports(mime string) bool {
	for _, p := range d.parsers {
		if p.Supports(mime) {
			return true
		}
	}
	return false
}

func (d *Dispatcher) Parse(ctx context.Context, r io.Reader, mime string) (*ports.ParseResult, error) {
	for _, p := range d.parsers {
		if p.Supports(mime) {
			return p.Parse(ctx, r, mime)
		}
	}
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("fallback read: %w", err)
		}
	}
	return &ports.ParseResult{
		Text:     string(buf),
		Metadata: map[string]any{"format": "unknown", "mime": mime},
	}, nil
}
