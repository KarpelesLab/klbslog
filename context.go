package klbslog

import (
	"context"
	"log/slog"
)

type contextAttribute int

const extraSlogAttrs contextAttribute = 1

type ctxAttributes struct {
	context.Context
	attrs []slog.Attr
}

func WithAttributes(ctx context.Context, args ...any) context.Context {
	// convert args → attrs
	var attrs []slog.Attr
	for len(args) > 0 {
		switch x := args[0].(type) {
		case string:
			if len(args) == 1 {
				attrs = append(attrs, slog.String("!BADKEY", x))
				args = nil
			} else {
				attrs = append(attrs, slog.Any(x, args[1]))
				args = args[2:]
			}
		case slog.Attr:
			attrs = append(attrs, x)
			args = args[1:]
		default:
			attrs = append(attrs, slog.Any("!BADKEY", x))
			args = args[1:]
		}
	}
	return &ctxAttributes{ctx, attrs}
}

func (c *ctxAttributes) Value(v any) any {
	if v == extraSlogAttrs {
		if m, ok := c.Context.Value(extraSlogAttrs).([]slog.Attr); ok {
			return append(c.attrs, m...)
		}
		return c.attrs
	}
	return c.Context.Value(v)
}

func getExtraAttrs(ctx context.Context) []slog.Attr {
	if m, ok := ctx.Value(extraSlogAttrs).([]slog.Attr); ok {
		return m
	}
	return nil
}
