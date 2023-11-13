package klbslog

import (
	"context"
	"log"
	"runtime"
	"strconv"
	"sync"
	"time"

	"log/slog"

	"github.com/KarpelesLab/rest"
)

type SHandler struct {
	opts  *slog.HandlerOptions
	queue []map[string]string
	qlk   sync.Mutex
	qcd   *sync.Cond
}

func New(opts *slog.HandlerOptions) slog.Handler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	res := &SHandler{
		opts: opts,
	}
	res.qcd = sync.NewCond(&res.qlk)

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

	s.append(attrs)
	return nil
}

func (s *SHandler) append(v map[string]string) {
	s.qlk.Lock()
	defer s.qlk.Unlock()

	s.queue = append(s.queue, v)
	s.qcd.Broadcast()
}

func (s *SHandler) run() {
	s.qlk.Lock()
	defer s.qlk.Unlock()

	// this runs in a separate goroutine
	for {
		if len(s.queue) == 0 {
			// nothing in queue
			s.qcd.Wait()
			continue
		}
		// take queue
		q := s.queue
		s.queue = nil

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
