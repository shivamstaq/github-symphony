package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// SetupJSONL creates a slog.Logger that writes JSON lines to a file.
// If the file's parent directory doesn't exist, it is created.
// Returns the logger and the file (caller should close on shutdown).
func SetupJSONL(path string, level string) (*slog.Logger, *os.File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, nil, fmt.Errorf("create log dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	opts := &slog.HandlerOptions{Level: ParseLevel(level)}
	handler := slog.NewJSONHandler(f, opts)
	return slog.New(handler), f, nil
}

// SetupMulti creates a logger that writes to both stderr (text) and a JSONL file.
// This is the default setup for Symphony: human-readable TUI output + structured file log.
func SetupMulti(jsonlPath string, level string) (*slog.Logger, *os.File, error) {
	dir := filepath.Dir(jsonlPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, nil, fmt.Errorf("create log dir: %w", err)
	}

	f, err := os.OpenFile(jsonlPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	opts := &slog.HandlerOptions{Level: ParseLevel(level)}
	handler := &multiHandler{
		file:    slog.NewJSONHandler(f, opts),
		console: slog.NewTextHandler(os.Stderr, opts),
	}
	return slog.New(handler), f, nil
}

// multiHandler writes to both file and console handlers.
type multiHandler struct {
	file    slog.Handler
	console slog.Handler
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.file.Enabled(ctx, level) || h.console.Enabled(ctx, level)
}

func (h *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	// Write to both; file errors are silent to avoid blocking
	_ = h.file.Handle(ctx, r)
	return h.console.Handle(ctx, r)
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &multiHandler{
		file:    h.file.WithAttrs(attrs),
		console: h.console.WithAttrs(attrs),
	}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	return &multiHandler{
		file:    h.file.WithGroup(name),
		console: h.console.WithGroup(name),
	}
}

// SetupFileOnly creates a JSONL logger that only writes to a file (no stderr).
// Used for per-agent session logs.
func SetupFileOnly(path string) (*slog.Logger, io.Closer, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, nil, fmt.Errorf("create log dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	opts := &slog.HandlerOptions{Level: slog.LevelDebug}
	handler := slog.NewJSONHandler(f, opts)
	return slog.New(handler), f, nil
}
