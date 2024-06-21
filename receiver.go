package klbslog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type Receiver interface {
	ProcessLogs(logs []map[string]string) error
}

type PostReceiver string

var DefaultReceiver = PostReceiver("https://ws.atonline.com/_special/rest/SLog:append")

func (p PostReceiver) ProcessLogs(logs []map[string]string) error {
	if len(logs) == 0 {
		// nothing to do
		return nil
	}
	// let's just call the rest function SLog:append with logs=q
	body, err := json.Marshal(map[string]any{"logs": logs})
	if err != nil {
		// shouldn't happen
		return fmt.Errorf("failed to marshal logs: %w", err)
	}
	req, err := http.NewRequest("POST", string(p), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to prepare request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	var t time.Duration

	cnt := 0
	for {
		_, err := http.DefaultClient.Do(req)
		if err == nil {
			// success
			return nil
		}
		if cnt > 5 {
			return fmt.Errorf("failed to push logs: %w", err)
		}
		log.Printf("[klbslog] Failed to push logs: %s (retrying)", err)
		cnt += 1
		// wait increasingly longer (but very short)
		t = (t * 2) + 10*time.Millisecond
		time.Sleep(t)
	}
}
