package logx

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := []struct {
		raw  string
		want slog.Level
		ok   bool
	}{
		{"", slog.LevelInfo, true},
		{"info", slog.LevelInfo, true},
		{" DEBUG ", slog.LevelDebug, true},
		{"warn", slog.LevelWarn, true},
		{"warning", slog.LevelWarn, true},
		{"error", slog.LevelError, true},
		{"trace", 0, false},
		{"info\nerror", 0, false},
	}
	for _, tc := range cases {
		got, err := ParseLevel(tc.raw)
		if tc.ok {
			if err != nil || got != tc.want {
				t.Fatalf("ParseLevel(%q)=%v,%v", tc.raw, got, err)
			}
			continue
		}
		if err == nil {
			t.Fatalf("ParseLevel(%q) accepted", tc.raw)
		}
	}
}

func TestSetupFiltersBelowLevel(t *testing.T) {
	var buf bytes.Buffer
	if err := Setup(&buf, "error"); err != nil {
		t.Fatal(err)
	}
	slog.Info("hidden")
	slog.Error("visible")
	if strings.Contains(buf.String(), "hidden") || !strings.Contains(buf.String(), "visible") {
		t.Fatalf("logs=%q", buf.String())
	}
}

func TestSanitizeRequestID(t *testing.T) {
	if got, ok := SanitizeRequestID("req-123.abc:def"); !ok || got != "req-123.abc:def" {
		t.Fatalf("valid id rejected: %q %v", got, ok)
	}
	for _, raw := range []string{"", "bad id", "line\nbreak", "ta\tb", "quote\"", strings.Repeat("a", 129), "中文"} {
		if _, ok := SanitizeRequestID(raw); ok {
			t.Fatalf("accepted %q", raw)
		}
	}
	if id := NewRequestID(); !strings.HasPrefix(id, "req-") || len(id) < 8 {
		t.Fatalf("generated id %q", id)
	}
}
