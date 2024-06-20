package klbslog

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
)

// RequestLogHandler is a http handler that will log every requests it handles into slog. It will also update
// request.Context to return the request information so it will be populated in messages sent via slog.
type RequestLogHandler struct {
	Handler http.Handler
}

func (r *RequestLogHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	ri := &reqInfo{
		ResponseWriter: rw,
		Context:        req.Context(),
		reqId:          uuid.Must(uuid.NewRandom()),
		start:          time.Now(),
	}
	ri.req = req.WithContext(ri)

	defer ri.log()
	r.Handler.ServeHTTP(ri.pass())
}

type reqInfo struct {
	http.ResponseWriter
	context.Context

	req        *http.Request
	reqId      uuid.UUID
	start      time.Time
	statusCode int
	written    int
}

func (ri *reqInfo) WriteHeader(statusCode int) {
	ri.statusCode = statusCode
	ri.ResponseWriter.WriteHeader(statusCode)
}

func (ri *reqInfo) Write(b []byte) (int, error) {
	n, e := ri.ResponseWriter.Write(b)
	if n > 0 {
		ri.written += n
	}

	return n, e
}

func (ri *reqInfo) Peel() http.ResponseWriter {
	return ri.ResponseWriter
}

func (ri *reqInfo) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := ri.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (ri *reqInfo) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := ri.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (ri *reqInfo) Value(v any) any {
	switch sv := v.(type) {
	case **http.Request:
		*sv = ri.req
		return ri.req
	case string:
		switch sv {
		case "http_request":
			return ri.req
		case "request_id":
			return ri.reqId.String()
		}
	}
	return ri.Context.Value(v)
}

func (ri *reqInfo) pass() (http.ResponseWriter, *http.Request) {
	return ri, ri.req
}

func (ri *reqInfo) log() {
	// actually log request
	q_time := time.Since(ri.start)
	r := ri.req

	if err := recover(); err != nil {
		if err == http.ErrAbortHandler {
			slog.InfoContext(r.Context(), fmt.Sprintf("Request handler aborted (websocket?) during %s %s", r.Method, r.RequestURI), "event", "http:handler:abort")
			// pass this to the higher powers
			panic(err)
		}
		slog.ErrorContext(r.Context(), fmt.Sprintf("[http] Crash during %s %s: %s", r.Method, r.RequestURI, err), "event", "http:crash", "category", "go.panic")
		debug.PrintStack()
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// shouldn't happen
		ip = r.RemoteAddr
	}

	logEntry := map[string]any{
		"remote_addr": r.RemoteAddr,
		"host":        r.Host,
		"method":      r.Method,
		"uri":         r.RequestURI,
		"proto":       r.Proto,
		"status":      ri.statusCode,
		"bytes_sent":  ri.written,
		"time_us":     float64(q_time) / float64(time.Microsecond),
		"h_request":   censorHeaders(r.Header, "Cookie"),
		"h_response":  censorHeaders(ri.Header(), "Set-Cookie"),
	}
	buf, _ := json.Marshal(logEntry)

	slog.InfoContext(r.Context(), string(buf), "event", "http:log", "remote_ip", ip, "http.host", r.Host, "http.method", r.Method, "http.request_uri", r.RequestURI, "http.proto", r.Proto, "http.status", ri.statusCode, "http.written", ri.written)
}
