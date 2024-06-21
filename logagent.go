package klbslog

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

type LogAgent struct {
	c  *net.UnixConn
	lk sync.Mutex
}

var _ Receiver = &LogAgent{}

func NewLogAgent() *LogAgent {
	return &LogAgent{}
}

// ProcessLogs sends the logs to the logagent daemon
func (p *LogAgent) ProcessLogs(logs []map[string]string) error {
	p.lk.Lock()
	defer p.lk.Unlock()

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

// GetLogfile returns a write-only os.File that can be used when using os/exec to start daemons.
//
// Remember to close the os.File after calling Start.
func (p *LogAgent) GetLogfile(name string) (*os.File, error) {
	p.lk.Lock()
	defer p.lk.Unlock()

	c, err := p.getConnection()
	if err != nil {
		return nil, err
	}
	req, err := json.Marshal(map[string]any{"name": name})
	if err != nil {
		return nil, err
	}

	pkt := &Packet{
		Type: PktPipeRequest,
		Data: req,
	}
	err = pkt.SendTo(c)
	if err != nil {
		return nil, err
	}
	res := &Packet{}
	err = res.ReadFrom(c)
	if err != nil {
		return nil, err
	}
	if res.Type != PktPipeResponse {
		return nil, fmt.Errorf("unexpected response %x", res.Type)
	}
	return res.FDs[0], nil
}

// getConnection returns the connection, getting a lock in the process
func (p *LogAgent) getConnection() (*net.UnixConn, error) {
	if p.c != nil {
		return p.c, nil
	}
	c, err := p.connect()
	if err != nil {
		return nil, err
	}
	p.c = c
	return c, nil
}

// connect establishes a connection to the local logagent daemon and returns the connection
func (p *LogAgent) connect() (*net.UnixConn, error) {
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
func (p *LogAgent) send(pkt *Packet) error {
	c, err := p.getConnection()
	if err != nil {
		return err
	}
	return pkt.SendTo(c)
}
