package worker

import "testing"

func TestEffectiveMimeTypeInfersMarkdownFromExtension(t *testing.T) {
	tests := []struct {
		name  string
		mime  string
		title string
		want  string
	}{
		{"octet-stream md", "application/octet-stream", "部署指南.md", "text/markdown"},
		{"octet-stream markdown", "application/octet-stream", "README.markdown", "text/markdown"},
		{"empty mime md", "", "CLAUDE.MD", "text/markdown"},
		{"explicit mime kept", "text/plain", "notes.md", "text/plain"},
		{"pdf untouched", "application/pdf", "报告.pdf", "application/pdf"},
		{"unknown extension kept generic", "application/octet-stream", "data.bin", "application/octet-stream"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveMimeType(tt.mime, tt.title); got != tt.want {
				t.Errorf("effectiveMimeType(%q, %q) = %q, want %q", tt.mime, tt.title, got, tt.want)
			}
		})
	}
}
