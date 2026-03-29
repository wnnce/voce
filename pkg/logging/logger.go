package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"

	"gopkg.in/natefinch/lumberjack.v2"
)

var loggerLevelMap = map[string]slog.Level{
	"DEBUG": slog.LevelDebug,
	"INFO":  slog.LevelInfo,
	"WARN":  slog.LevelWarn,
	"ERROR": slog.LevelError,
}

func NewLogger(cfg Config) (*slog.Logger, error) {
	cfg.SetDefaults()
	var handler slog.Handler
	level, ok := loggerLevelMap[cfg.Level]
	if !ok {
		level = slog.LevelInfo
	}
	options := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.Source,
	}
	if cfg.Console {
		handler = newConsoleHandler(slog.NewTextHandler(io.Discard, options), os.Stdout, cfg.Source)
	} else {
		fileWrite, err := newLumberjackLogger(cfg)
		if err != nil {
			return nil, err
		}
		handler = slog.NewJSONHandler(io.MultiWriter(os.Stdout, fileWrite), options)
	}
	return slog.New(handler), nil
}

func NewLoggerWithContext(cfg Config, keys ...fmt.Stringer) (*slog.Logger, error) {
	logger, err := NewLogger(cfg)
	if err != nil {
		return nil, err
	}
	handler := newContextHandler(logger.Handler(), keys...)
	return slog.New(handler), nil
}

func newLumberjackLogger(cfg Config) (io.Writer, error) {
	if err := os.MkdirAll(cfg.FileDir, 0o777); err != nil {
		return nil, err
	}
	fullFilename := path.Join(cfg.FileDir, cfg.FileName)
	return &lumberjack.Logger{
		Filename:   fullFilename,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
	}, nil
}
