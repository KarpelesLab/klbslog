package klbslog

import (
	"os"
)

const (
	PktSetInfo      = 0x0100 // set the process name/etc. {"name":"..."}
	PktLogJson      = 0x0101 // {"time":"2022-11-08T15:28:26.000000000-05:00","level":"INFO","msg":"hello","count":3}
	PktPipeRequest  = 0x0200 // request a pipe from the daemon
	PktPipeResponse = 0x8200 // pipe provided
)

// Packet is a wire protocol packet sent to the local logagent
type Packet struct {
	Type  uint16
	Flags uint32
	Data  []byte
	FDs   []*os.File // files to be passed (optional)
}
