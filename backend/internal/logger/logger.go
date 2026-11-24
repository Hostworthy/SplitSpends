// Package logger configures structured logging. Messages are stable dotted
// keys such as "api.server.listening"; details go in attributes.
package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// New returns a JSON logger (or text when format is "text") at level.
func New(level, format string) *slog.Logger {
	return NewTo(os.Stderr, level, format)
}

// NewTo is New with an explicit writer.
func NewTo(w io.Writer, level, format string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}
	if strings.ToLower(format) == "text" {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}
