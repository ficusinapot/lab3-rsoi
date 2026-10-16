package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/samber/oops"
	"gopkg.in/natefinch/lumberjack.v2"
)

func New(cfg Config) (*slog.Logger, func(), error) {
	if err := cfg.Validate(); err != nil {
		return nil, nil, oops.Wrapf(err, "validate logging configuration")
	}
	handlers := make([]slog.Handler, 0, len(cfg.Files)+1)
	closers := make([]io.Closer, 0, len(cfg.Files))
	closeFiles := func() {
		for _, closer := range closers {
			// Logging shutdown cannot report through a potentially closed destination.
			_ = closer.Close()
		}
	}
	if cfg.Stdout.Level != "" {
		handlers = append(handlers, slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: parseLevel(cfg.Stdout.Level),
		}))
	}
	for _, file := range cfg.Files {
		if err := os.MkdirAll(filepath.Dir(file.Path), 0o750); err != nil {
			closeFiles()
			return nil, nil, oops.Wrapf(err, "create log directory")
		}
		writer := &lumberjack.Logger{
			Filename:   file.Path,
			MaxSize:    file.MaxFileSizeMB,
			MaxBackups: file.MaxFilesCount,
			MaxAge:     file.MaxFileAgeInDays,
		}
		closers = append(closers, writer)
		handlers = append(handlers, levelHandler{
			level:   parseLevel(file.Level),
			handler: slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: slog.LevelDebug}),
		})
	}
	if len(handlers) == 0 {
		handlers = append(handlers, slog.NewJSONHandler(io.Discard, nil))
	}
	return slog.New(multiHandler{level: parseLevel(cfg.Level), handlers: handlers}), closeFiles, nil
}

func parseLevel(level string) slog.Level {
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
