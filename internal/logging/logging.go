package logging

import (
	"log/slog"
	"os"
)

// Setup creates a configured slog.Logger.
func Setup(format, level string) *slog.Logger {
	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: ParseLevel(level)}

	switch format {
	case "json":
		handler = slog.NewJSONHandler(os.Stderr, opts)
	default:
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	return slog.New(handler)
}

// ParseLevel converts a string level to slog.Level.
func ParseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
