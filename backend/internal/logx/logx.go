// Package logx configures the process logger and request ids.
package logx

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"unicode/utf8"
)

// ParseLevel accepts debug, info, warn/warning, and error. Empty means info.
func ParseLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid LOG_LEVEL %q", raw)
	}
}

// Setup installs the process-wide structured logger. Output is text lines
// with a level, so LOG_LEVEL filters records below it.
func Setup(w io.Writer, level string) error {
	parsed, err := ParseLevel(level)
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: parsed})))
	return nil
}

const maxRequestIDRunes = 128

// SanitizeRequestID accepts a caller-supplied id only when it is a short
// single-line token. Anything else is rejected so it cannot break log lines.
func SanitizeRequestID(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || utf8.RuneCountInString(raw) > maxRequestIDRunes || !utf8.ValidString(raw) {
		return "", false
	}
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-' || r == ':':
		default:
			return "", false
		}
	}
	return raw, true
}

// NewRequestID returns an id the server generated itself.
func NewRequestID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "req-unavailable"
	}
	return "req-" + hex.EncodeToString(buf[:])
}
