// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"

	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/link"
	"github.com/otfabric/go-dnp3/transport"
)

// vtOutstation is a scripted outstation for the virtual terminal, which the station does
// not have: it answers every request with an empty response and, once the master has
// enabled unsolicited responses, sends one unsolicited response with the virtual terminal
// event data "hi" of port 2 (g113v2, IEEE 1815-2012 A.44).
type vtOutstation struct {
	l    net.Listener
	mu   sync.Mutex
	errs []error
}

func newVTOutstation(t *testing.T) *vtOutstation {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	o := &vtOutstation{l: l}
	go o.accept()
	t.Cleanup(func() {
		_ = l.Close()
		o.mu.Lock()
		defer o.mu.Unlock()
		for _, err := range o.errs {
			t.Error(err)
		}
	})
	return o
}

func (o *vtOutstation) flags() []string {
	return []string{"--ip", "127.0.0.1", "--port", strconv.Itoa(o.l.Addr().(*net.TCPAddr).Port), "--timeout", "5000"}
}

func (o *vtOutstation) accept() {
	for {
		c, err := o.l.Accept()
		if err != nil {
			return
		}
		go o.serve(c)
	}
}

func (o *vtOutstation) fail(err error) {
	o.mu.Lock()
	o.errs = append(o.errs, err)
	o.mu.Unlock()
}

func (o *vtOutstation) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	r := link.NewReader(c)
	re := transport.NewReassembler(2048)
	var seg transport.Segmenter
	var unsSeq uint8
	send := func(to link.Frame, fragment []byte) error {
		for _, s := range seg.Segment(fragment) {
			b, err := (&link.Frame{Function: link.PrimaryUnconfirmedUserData, Dest: to.Src, Src: to.Dest, Data: s}).Encode()
			if err != nil {
				return err
			}
			if _, err := c.Write(b); err != nil {
				return err
			}
		}
		return nil
	}
	for {
		f, err := r.ReadFrame()
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				o.fail(err)
			}
			return
		}
		reply, answer := link.SecondaryAck, true
		switch f.Function {
		case link.PrimaryRequestLinkStatus:
			reply = link.SecondaryLinkStatus
		case link.PrimaryResetLinkStates, link.PrimaryTestLinkStates, link.PrimaryConfirmedUserData:
		default:
			answer = false
		}
		if answer {
			b, _ := (&link.Frame{Function: reply, Dest: f.Src, Src: f.Dest}).Encode()
			if _, err := c.Write(b); err != nil {
				return
			}
		}
		if !f.Function.HasData() {
			continue
		}
		frag, err := re.Add(f.Data)
		if err != nil || frag == nil {
			continue
		}
		req, err := app.Decode(frag)
		if err != nil || req.Function == app.FuncConfirm {
			continue
		}
		// An empty response with the request's sequence number.
		if err := send(f, []byte{0xC0 | req.Control.Seq, byte(app.FuncResponse), 0, 0}); err != nil {
			return
		}
		if req.Function == app.FuncEnableUnsolicited {
			// FIR, FIN, CON and UNS; g113v2, qualifier 0x17: one object, index 2, "hi".
			uns := []byte{0xF0 | unsSeq, byte(app.FuncUnsolicitedResponse), 0, 0, 113, 2, 0x17, 1, 2, 'h', 'i'}
			unsSeq = (unsSeq + 1) & 0x0F
			if err := send(f, uns); err != nil {
				return
			}
		}
	}
}

// TestMonitorVirtualTerminal: the monitor reports the virtual terminal event data of an
// unsolicited response (go-dnp3's FragmentHandler), with the kind virtual-terminal, the
// port as its index and the octets as text; --object selects it.
func TestMonitorVirtualTerminal(t *testing.T) {
	o := newVTOutstation(t)
	args := append(append([]string{"client", "monitor"}, o.flags()...), "--object", "g113", "--count", "1", "--format", "json")
	r := run(t, args...).want(t, 0)
	lines := r.lines(t)
	if len(lines) != 1 {
		t.Fatalf("monitor: %v\n%s", lines, r.stderr)
	}
	l := lines[0]
	if l["object"] != "g113v2" || l["type"] != "virtual-terminal" || l["index"] != float64(2) ||
		l["value"] != "hi" || l["event"] != true || l["unsolicited"] != true {
		t.Errorf("virtual terminal record: %v", l)
	}
}
