package logging

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/metadata"
)

func TestNewLogger(t *testing.T) {
	t.Run("console logger level mapping", func(t *testing.T) {
		cfg := Config{
			Level:   "DEBUG",
			Console: true,
		}
		logger, err := NewLogger(cfg)
		require.NoError(t, err)
		assert.True(t, logger.Enabled(context.Background(), slog.LevelDebug))
	})

	t.Run("file logger initialization", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := Config{
			Console:  false,
			FileDir:  tmpDir,
			FileName: "test.log",
		}
		logger, err := NewLogger(cfg)
		require.NoError(t, err)
		assert.NotNil(t, logger)
		logger.Info("trigger file write")
	})

	t.Run("invalid level defaults to info", func(t *testing.T) {
		cfg := Config{
			Level:   "UNKNOWN",
			Console: true,
		}
		logger, _ := NewLogger(cfg)
		assert.False(t, logger.Enabled(context.Background(), slog.LevelDebug))
		assert.True(t, logger.Enabled(context.Background(), slog.LevelInfo))
	})
}

func TestNewLoggerWithContext(t *testing.T) {
	cfg := Config{
		Console: true,
		Source:  false,
	}
	t.Run("multiple context keys", func(t *testing.T) {
		key1 := metadata.ContextTraceKey
		logger, err := NewLoggerWithContext(cfg, key1)
		require.NoError(t, err)

		ctx := context.WithValue(context.Background(), key1, "trace-1")
		logger.InfoContext(ctx, "multi-key message")
	})
}
