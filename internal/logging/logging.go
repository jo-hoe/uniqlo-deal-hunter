// Package logging bootstraps the application's structured logger. Uses the
// stdlib log/slog package with a JSON handler so that Grafana / Loki can
// parse fields natively.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// New builds a slog.Logger that writes JSON to stderr at the requested level.
// The level string is case-insensitive and matches slog's four canonical
// levels. Unknown values fall back to Info with a warning.
func New(level string) *slog.Logger {
	return NewWithWriter(os.Stderr, level)
}

// NewWithWriter is the seam for tests: build a JSON logger against any
// io.Writer.
func NewWithWriter(w io.Writer, level string) *slog.Logger {
	lvl, parseErr := parseLevel(level)
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lvl})
	logger := slog.New(h)
	if parseErr != nil {
		logger.Warn("unknown log level, defaulting to info", "requested", level)
	}
	return logger
}

// parseLevel converts a config string to a slog.Level. Returns the default
// (Info) and a non-nil error when the input is unrecognised.
func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level %q", s)
	}
}
