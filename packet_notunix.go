//go:build !unix

package klbslog

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
)

// SendTo will serialize and write the packet to the specified connection. Make sure
// to lock it so multiple packets aren't sent at the same time.
func (p *Packet) SendTo(c *net.UnixConn) error {
	if len(p.FDs) != 0 {
		return errors.New("unable to send FDs over non-UNIX")
	}
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
	return nil
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
	if nfd != 0 {
		return errors.New("unable to receive FDs over non-UNIX")
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

	return nil
}
