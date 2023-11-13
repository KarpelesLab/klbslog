package klbslog

import (
	"context"
	"log"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"log/slog"

	"github.com/KarpelesLab/rest"
)

type SHandler struct {
	opts   *slog.HandlerOptions
	queue  []map[string]string
	maxlvl slog.Level
	qlk    sync.Mutex
	qcd    *sync.Cond
	parent slog.Handler
	common map[string]string
}

func New(opts *slog.HandlerOptions, parent slog.Handler) slog.Handler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	res := &SHandler{
		opts:   opts,
		parent: parent,
		common: make(map[string]string),
	}
	res.qcd = sync.NewCond(&res.qlk)

	if bi, ok := debug.ReadBuildInfo(); ok {
		res.common["go.project"] = bi.Path
		res.common["go.version"] = bi.GoVersion
	}

	// note: we can run this multiple times to have multiple parallel uploaders
	go res.run()

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

	if _, found := attrs["event"]; !found {
		attrs["event"] = "go.log"
	}

	// set or overwrite values for standard attributes
	attrs[slog.MessageKey] = r.Message
	attrs[slog.TimeKey] = r.Time.Format(time.RFC3339Nano)
	attrs[slog.LevelKey] = r.Level.String()

	for k, v := range s.common {
		attrs[k] = v
	}

	if s.opts.AddSource {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		// we do not use slog's standard "source" attribute since that's go-specific data
		attrs["go.source.function"] = f.Function
		attrs["go.source.file"] = f.File
		attrs["go.source"] = f.File + ":" + strconv.Itoa(f.Line)
	}

	s.append(attrs, r.Level)

	if s.parent != nil {
		return s.parent.Handle(ctx, r)
	}
	return nil
}

func (s *SHandler) append(v map[string]string, l slog.Level) {
	s.qlk.Lock()
	defer s.qlk.Unlock()

	s.queue = append(s.queue, v)

	if l > s.maxlvl {
		s.maxlvl = l
	}

	if l >= slog.LevelInfo {
		// do not broadcast for debug messages
		s.qcd.Broadcast()
	}
}

func (s *SHandler) run() {
	s.qlk.Lock()
	defer s.qlk.Unlock()

	// this runs in a separate goroutine
	for {
		if len(s.queue) == 0 || s.maxlvl < slog.LevelInfo {
			// nothing in queue
			s.qcd.Wait()
			continue
		}
		// take queue
		q := s.queue
		s.queue = nil
		s.maxlvl = slog.LevelDebug

		// run it (lock will be released during runQueue)
		s.runQueue(q)
	}
}

func (s *SHandler) runQueue(q []map[string]string) {
	// unlock qlk while running queue, but lock back afterward
	s.qlk.Unlock()
	defer s.qlk.Lock()

	// let's just call the rest function SLog:append with logs=q
	cnt := 0
	for {
		_, err := rest.Do(context.Background(), "SLog:append", "POST", map[string]any{"logs": q})
		if err == nil {
			// success
			return
		}
		log.Printf("[klbslog] Failed to push logs: %s", err)
		if cnt > 5 {
			return
		}
		cnt += 1
		time.Sleep(5 * time.Second)
	}
}
