package klbslog

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/KarpelesLab/pjson"
	"github.com/KarpelesLab/webutil"
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
		req:            req,
	}
	defer ri.log()

	req = req.WithContext(ri)

	r.Handler.ServeHTTP(ri, req)
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
		slog.ErrorContext(r.Context(), fmt.Sprintf("[http] Crash during %s %s: %s", r.Method, r.RequestURI, err), "event", "platform-fe:http:crash", "category", "go.panic")
		debug.PrintStack()
	}

	ipp := webutil.ParseIPPort(r.RemoteAddr)

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
	buf, _ := pjson.Marshal(logEntry)

	slog.InfoContext(r.Context(), string(buf), "event", "http:log", "remote_ip", ipp.IP.String(), "http.host", r.Host, "http.method", r.Method, "http.request_uri", r.RequestURI, "http.proto", r.Proto, "http.status", ri.statusCode, "http.written", ri.written)
}