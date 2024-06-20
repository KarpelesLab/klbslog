package klbslog

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"log/slog"
)

type SHandler struct {
	opts   *slog.HandlerOptions
	queue  []map[string]string
	maxlvl slog.Level
	qlk    sync.Mutex
	qcd    *sync.Cond
	parent slog.Handler
	common map[string]string
	client *http.Client
	target string
}

func New(opts *slog.HandlerOptions, parent slog.Handler) slog.Handler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	res := &SHandler{
		opts:   opts,
		parent: parent,
		common: make(map[string]string),
		client: http.DefaultClient,
		target: "https://ws.atonline.com/_special/rest/SLog:append", // default target
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

// SetTarget sets the target url for logs to be sent to
func (s *SHandler) SetTarget(targetUrl string) {
	s.target = targetUrl
}

// SetHttpClient sets the HTTP client to use to send requests
func (s *SHandler) SetHttpClient(client *http.Client) {
	s.client = client
}

func (s *SHandler) Handle(ctx context.Context, r slog.Record) error {
	// slog.Record has a number of attributes: Time, Message, Level, PC
	// attributes are not exported
	attrs := make(map[string]string)

	if xtra := getExtraAttrs(ctx); len(xtra) > 0 {
		r = r.Clone()
		r.AddAttrs(xtra...)
	}

	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()
		return true
	})

	if _, found := attrs["event"]; !found {
		attrs["event"] = "go.log"
	}

	// attempt to get any info from a http request
	var req *http.Request
	ctx.Value(&req)
	if req != nil {
		attrs["http.host"] = req.Host
		attrs["http.method"] = req.Method
		attrs["http.request_uri"] = req.RequestURI
		attrs["http.proto"] = req.Proto
		ip, _, err := net.SplitHostPort(req.RemoteAddr)
		if err != nil {
			// shouldn't happen
			ip = req.RemoteAddr
		}
		attrs["remote_ip"] = ip
		if trace := req.Header.Get("Sec-Trace-Id"); trace != "" {
			attrs["http.trace"] = trace
		}
	}
	if reqId, ok := ctx.Value("request_id").(string); ok {
		attrs["req"] = reqId
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

// takeQueue waits for elements to be added to the log queue, and will take
// what it finds there
func (s *SHandler) takeQueue() []map[string]string {
	s.qlk.Lock()
	defer s.qlk.Unlock()

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

		return q
	}
}

func (s *SHandler) run() {
	// this runs in a separate goroutine
	for {
		q := s.takeQueue()

		// run it (lock will be released during runQueue)
		s.runQueue(q)
	}
}

func (s *SHandler) runQueue(q []map[string]string) {
	if len(q) == 0 {
		// nothing to do
		return
	}
	// let's just call the rest function SLog:append with logs=q
	body, err := json.Marshal(map[string]any{"logs": q})
	if err != nil {
		// shouldn't happen
		log.Printf("[klbslog] failed to prepare POST body: %s", err)
		return
	}
	req, err := http.NewRequest("POST", s.target, bytes.NewReader(body))
	if err != nil {
		log.Printf("[klbslog] failed to prepare POST request: %s", err)
		return
	}
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	var t time.Duration

	cnt := 0
	for {
		_, err := s.client.Do(req)
		if err == nil {
			// success
			return
		}
		log.Printf("[klbslog] Failed to push logs: %s", err)
		if cnt > 5 {
			return
		}
		cnt += 1
		// wait increasingly longer (but very short)
		t = (t * 2) + 10*time.Millisecond
		time.Sleep(t)
	}
}
