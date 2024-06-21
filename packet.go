package klbslog

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"syscall"
)

const (
	PktLogJson = 0x0100 // {"time":"2022-11-08T15:28:26.000000000-05:00","level":"INFO","msg":"hello","count":3}
)

// Packet is a wire protocol packet sent to the local logagent
type Packet struct {
	Type  uint16
	Flags uint32
	Data  []byte
	FDs   []*os.File // files to be passed (optional)
}

// SendTo will serialize and write the packet to the specified connection. Make sure
// to lock it so multiple packets aren't sent at the same time.
func (p *Packet) SendTo(c *net.UnixConn) error {
	hdr := make([]byte, 12)
	binary.BigEndian.PutUint16(hdr[:2], p.Type)
	binary.BigEndian.PutUint16(hdr[2:4], uint16(len(p.FDs)))
	binary.BigEndian.PutUint32(hdr[4:8], p.Flags)
	binary.BigEndian.PutUint32(hdr[8:12], uint32(len(p.Data)))
	if len(p.Data) > 0 {
		_, err := c.Write(append(hdr, p.Data...))
		if err != nil {
			return err
		}
	}
	if len(p.FDs) == 0 {
		return nil
	}

	sc, err := c.SyscallConn()
	if err != nil {
		return err
	}

	// send FDs
	fds := make([]int, len(p.FDs))
	for n, fd := range p.FDs {
		fds[n] = int(fd.Fd())
	}
	rights := syscall.UnixRights(fds...)
	return sc.Write(func(fd uintptr) bool {
		err := syscall.Sendmsg(int(fd), nil, rights, nil, 0)
		if err != nil {
			return false
		}
		return true
	})
}

// ReadFrom will receive a packet from the specified unixconn
func (p *Packet) ReadFrom(c *net.UnixConn) error {
	hdr := make([]byte, 12)
	_, err := io.ReadFull(c, hdr)
	if err != nil {
		return err
	}
	p.Type = binary.BigEndian.Uint16(hdr[:2])
	nfd := binary.BigEndian.Uint16(hdr[2:4])
	p.Flags = binary.BigEndian.Uint32(hdr[4:8])
	ln := binary.BigEndian.Uint32(hdr[8:12])
	if ln > 1024*1024 {
		return errors.New("packet is too large")
	}
	if ln == 0 {
		p.Data = nil
	} else {
		p.Data = make([]byte, ln)
		_, err = io.ReadFull(c, p.Data)
		if err != nil {
			return err
		}
	}

	if nfd == 0 {
		return nil
	}

	sc, err := c.SyscallConn()
	if err != nil {
		return err
	}

	// read fds
	buf := make([]byte, syscall.CmsgSpace(int(nfd)*4))
	sc.Read(func(fd uintptr) bool {
		_, _, _, _, err = syscall.Recvmsg(int(fd), nil, buf, 0)
		if err == syscall.EAGAIN {
			return false
		}
		return true
	})

	var msgs []syscall.SocketControlMessage
	msgs, err = syscall.ParseSocketControlMessage(buf)

	p.FDs = make([]*os.File, 0, len(msgs))
	for i := 0; i < len(msgs) && err == nil; i++ {
		var fds []int
		fds, err = syscall.ParseUnixRights(&msgs[i])

		for _, fd := range fds {
			p.FDs = append(p.FDs, os.NewFile(uintptr(fd), "file"))
		}
	}
	return nil
}
