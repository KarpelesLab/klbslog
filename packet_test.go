package klbslog_test

import (
	"bytes"
	"testing"

	"github.com/KarpelesLab/klbslog"
)

func TestPacketSerialize(t *testing.T) {
	buf := &bytes.Buffer{}

	pkt := &klbslog.Packet{
		Type:  42,
		Flags: 0xdeadbeef,
		Data:  []byte("hello world"),
	}
	err := pkt.SendTo(buf)
	if err != nil {
		t.Errorf("failed to serialize packet: %s", err)
		return
	}

	//log.Printf("packet = %s", hex.Dump(buf.Bytes()))

	pkt2 := &klbslog.Packet{}
	err = pkt2.ReadFrom(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Errorf("failed to read packet: %s", err)
		return
	}

	if pkt.Type != pkt2.Type || !bytes.Equal(pkt.Data, pkt2.Data) || pkt.Flags != pkt2.Flags {
		t.Errorf("failed")
	}
}
