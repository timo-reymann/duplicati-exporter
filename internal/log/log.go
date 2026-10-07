// Package log provides helpers to configure the standard library structured
// logger (log/slog) for the exporter.
package log

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Options describes the desired logging behavior.
type Options struct {
	// Level is one of debug, info, warn, error.
	Level string
	// Format is one of logfmt or json.
	Format string
	// Writer defaults to os.Stderr when nil.
	Writer io.Writer
}

// New builds a slog.Logger according to opts.
func New(opts Options) (*slog.Logger, error) {
	level, err := parseLevel(opts.Level)
	if err != nil {
		return nil, err
	}

	writer := opts.Writer
	if writer == nil {
		writer = os.Stderr
	}

	handlerOpts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(opts.Format)) {
	case "", "logfmt", "text":
		handler = slog.NewTextHandler(writer, handlerOpts)
	case "json":
		handler = slog.NewJSONHandler(writer, handlerOpts)
	default:
		return nil, fmt.Errorf("unsupported log format %q (want logfmt or json)", opts.Format)
	}

	return slog.New(handler), nil
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q (want debug, info, warn or error)", s)
	}
}
