package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

// loadTZEuropeMoscow is swappable in tests (LoadLocation error → time.Local).
var loadTZEuropeMoscow = func() (*time.Location, error) {
	return time.LoadLocation("Europe/Moscow")
}

// NewFromEnv creates a slog.Logger configured via environment variables.
//
// - LOG_LEVEL: debug|info (default: info)
// - LOG_FORMAT: json|text (default: text)
//
// Time format is always: 02/01/2006 15:04:05 (Europe/Moscow)
func NewFromEnv() *slog.Logger {
	return NewWithWriter(os.Stdout)
}

// NewWithWriter is like NewFromEnv but writes to w (tests use a buffer).
func NewWithWriter(w io.Writer) *slog.Logger {
	level := slog.LevelInfo
	if strings.EqualFold(strings.TrimSpace(os.Getenv("LOG_LEVEL")), "debug") {
		level = slog.LevelDebug
	}

	loc, err := loadTZEuropeMoscow()
	if err != nil {
		loc = time.Local
	}

	timeReplacer := func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			if t, ok := a.Value.Any().(time.Time); ok {
				a.Value = slog.StringValue(t.In(loc).Format("02/01/2006 15:04:05"))
			}
		}
		return a
	}

	opts := &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: timeReplacer,
	}

	format := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_FORMAT")))
	if format == "json" {
		return slog.New(slog.NewJSONHandler(w, opts))
	}
	return slog.New(slog.NewTextHandler(w, opts))
}
