package klbslog

import (
	"os"
)

const (
	PktLogJson     = 0x0100 // {"time":"2022-11-08T15:28:26.000000000-05:00","level":"INFO","msg":"hello","count":3}
	PktPipeRequest = 0x0200
)

// Packet is a wire protocol packet sent to the local logagent
type Packet struct {
	Type  uint16
	Flags uint32
	Data  []byte
	FDs   []*os.File // files to be passed (optional)
}
