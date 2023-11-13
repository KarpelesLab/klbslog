package klbslog

import (
	"context"
	"runtime"
	"strconv"
	"time"

	"log/slog"
)

type SHandler struct {
	opts *slog.HandlerOptions
}

func New(opts *slog.HandlerOptions) slog.Handler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	res := &SHandler{
		opts: opts,
	}
	return res
}

// Enabled returns if SHandler is enabled (note: it always is)
func (s *SHandler) Enabled(_ context.Context, lvl slog.Level) bool {
	minLevel := slog.LevelInfo
	if s.opts.Level != nil {
		minLevel = s.opts.Level.Level()
	}
	return lvl >= minLevel
}

func (s *SHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	panic("not implemented")
}

func (s *SHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return s
	}
	panic("not implemented")
}

func (s *SHandler) Handle(ctx context.Context, r slog.Record) error {
	// slog.Record has a number of attributes: Time, Message, Level, PC
	// attributes are not exported
	attrs := make(map[string]string)

	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()
		return true
	})

	// set or overwrite values for standard attributes
	attrs[slog.MessageKey] = r.Message
	attrs[slog.TimeKey] = r.Time.Format(time.RFC3339Nano)
	attrs[slog.LevelKey] = r.Level.String()

	if s.opts.AddSource {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		// we do not use slog's standard "source" attribute since that's go-specific data
		attrs["go.source.function"] = f.Function
		attrs["go.source.file"] = f.File
		attrs["go.source"] = f.File + ":" + strconv.Itoa(f.Line)
	}
	return nil
}
