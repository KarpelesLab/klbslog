package klbslog

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

type logagentPipe struct {
	c *net.UnixConn
}

func LogAgent() Receiver {
	return &logagentPipe{}
}

func (p *logagentPipe) ProcessLogs(logs []map[string]string) error {
	for _, l := range logs {
		obj, err := json.Marshal(l)
		if err != nil {
			log.Printf("failed to marshal log: %s", err)
		}
		pkt := &Packet{
			Type:  PktLogJson,
			Flags: 0,
			Data:  obj,
		}

		var t time.Duration
		cnt := 0
		for {
			err := p.send(pkt)
			if err == nil {
				// success
				break
			}
			if cnt > 5 {
				return fmt.Errorf("failed to push logs: %w", err)
			}
			log.Printf("[klbslog] Failed to push log: %s (retrying)", err)
			cnt += 1
			// wait increasingly longer (but very short)
			t = (t * 2) + 10*time.Millisecond
			time.Sleep(t)
		}
	}
	return nil
}

func (p *logagentPipe) connect() (*net.UnixConn, error) {
	id := os.Getuid()
	if id == 0 {
		return net.DialUnix("unix", nil, &net.UnixAddr{Name: "/run/logagent.sock"})
	}

	c, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: "/run/logagent.sock"})
	if err == nil {
		return c, nil
	}

	// attempt per-user
	sockpath := fmt.Sprintf("/tmp/.logagent-%d.sock", id)
	return net.DialUnix("unix", nil, &net.UnixAddr{Name: sockpath})
}

// send sends a single packet to the local logagent
func (p *logagentPipe) send(pkt *Packet) error {
	if p.c == nil {
		c, err := p.connect()
		if err == nil {
			return err
		}
		p.c = c
	}
	return pkt.SendTo(p.c)
}
