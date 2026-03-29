package logging

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const (
	contextKey = "context"
)

type contextHandler struct {
	slog.Handler
	keys []fmt.Stringer
}

func newContextHandler(parent slog.Handler, keys ...fmt.Stringer) slog.Handler {
	return &contextHandler{
		keys:    keys,
		Handler: parent,
	}
}

func (c *contextHandler) Handle(ctx context.Context, record slog.Record) error {
	if len(c.keys) == 0 {
		return c.Handler.Handle(ctx, record)
	}
	attrs := make([]slog.Attr, 0, len(c.keys))
	for _, key := range c.keys {
		value := ctx.Value(key)
		if value == nil {
			continue
		}
		// 适配各种可能的value类型
		switch v := value.(type) {
		case string:
			attrs = append(attrs, slog.String(key.String(), v))
		case int64:
			attrs = append(attrs, slog.Int64(key.String(), v))
		case uint64:
			attrs = append(attrs, slog.Uint64(key.String(), v))
		case bool:
			attrs = append(attrs, slog.Bool(key.String(), v))
		case time.Duration:
			attrs = append(attrs, slog.Duration(key.String(), v))
		case time.Time:
			attrs = append(attrs, slog.Time(key.String(), v))
		default:
			attrs = append(attrs, slog.Any(key.String(), v))
		}
	}
	record.AddAttrs(slog.Any(contextKey, slog.GroupValue(attrs...)))
	return c.Handler.Handle(ctx, record)
}
