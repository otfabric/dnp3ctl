// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/otfabric/dnp3ctl/internal/stationtest"
)

// cutProxy forwards loopback TCP connections to upstream and cuts each one, both ways,
// once the outstation has sent after octets through it. It is deterministic: the cut
// falls at the same octet on every run.
func cutProxy(t *testing.T, upstream string, after int64) []string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	t.Cleanup(func() { _ = ln.Close(); wg.Wait() })
	wg.Go(func() {
		for {
			down, err := ln.Accept()
			if err != nil {
				return
			}
			up, err := net.Dial("tcp", upstream)
			if err != nil {
				_ = down.Close()
				continue
			}
			closeBoth := sync.OnceFunc(func() { _ = down.Close(); _ = up.Close() })
			wg.Go(func() { _, _ = io.Copy(up, down); closeBoth() })
			wg.Go(func() { _, _ = io.CopyN(down, up, after); closeBoth() })
		}
	})
	return peer(ln)
}

// garbagePeer answers every connection with the same octets that are no DNP3 frame,
// over and over, and reads whatever the master sends.
func garbagePeer(t *testing.T) []string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	noise := make([]byte, 512)
	for i := range noise {
		noise[i] = byte(i*37 + 11)
	}
	noise[0], noise[1] = 0x05, 0x64 // a start of a link frame, and then no valid header
	var wg sync.WaitGroup
	t.Cleanup(func() { _ = ln.Close(); wg.Wait() })
	wg.Go(func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Go(func() { _, _ = io.Copy(io.Discard, c) })
			wg.Go(func() {
				defer func() { _ = c.Close() }()
				for range 40 {
					if _, err := c.Write(noise); err != nil {
						return
					}
					time.Sleep(10 * time.Millisecond)
				}
			})
		}
	})
	return peer(ln)
}

// closingPeer accepts every connection and closes it at once.
func closingPeer(t *testing.T) []string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	t.Cleanup(func() { _ = ln.Close(); <-done })
	go func() {
		defer close(done)
		for {
			c, err := ln.Accept()
			if errors.Is(err, net.ErrClosed) {
				return
			}
			if err == nil {
				_ = c.Close()
			}
		}
	}()
	return peer(ln)
}

func peer(ln net.Listener) []string {
	a := ln.Addr().(*net.TCPAddr)
	return []string{"--ip", a.IP.String(), "--port", strconv.Itoa(a.Port)}
}

// TestChaos runs commands against connections that fail in the middle: each ends,
// within its timeout, with the documented exit code and error kind, and leaves no
// partial output file behind.
func TestChaos(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)
	dir := t.TempDir()
	type want struct {
		code        int
		kind, code2 string
	}
	for _, c := range []struct {
		name string
		to   func(t *testing.T) []string
		args []string
		want want
		out  string
	}{
		{"file get cut mid-transfer", func(t *testing.T) []string { return cutProxy(t, st.Addr(), 1500) },
			[]string{"client", "file", "get", "--path", "/records/big.dat", "--block-size", "256"}, want{4, "transport", "connection_lost"}, "big.dat"},
		{"poll cut after the first frame", func(t *testing.T) []string { return cutProxy(t, st.Addr(), 10) },
			[]string{"client", "poll"}, want{4, "transport", "connection_lost"}, ""},
		{"poll of a peer that sends noise", garbagePeer,
			[]string{"client", "poll", "--response-timeout", "300"}, want{5, "timeout", "request_timeout"}, ""},
		{"poll of a peer that hangs up", closingPeer,
			[]string{"client", "poll"}, want{4, "transport", "connection_lost"}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			args := append(append([]string{}, c.args...), c.to(t)...)
			args = append(args, "--timeout", "3000", "--format", "json")
			var out string
			if c.out != "" {
				out = filepath.Join(dir, c.out)
				args = append(args, "--output", out)
			}
			start := time.Now()
			r := run(t, args...)
			if took := time.Since(start); took > 10*time.Second {
				t.Errorf("took %v", took)
			}
			kind, code := r.want(t, c.want.code).errorCode(t)
			if kind != c.want.kind || code != c.want.code2 {
				t.Errorf("error %s/%s, want %s/%s", kind, code, c.want.kind, c.want.code2)
			}
			if out != "" {
				if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("a partial %s was left behind: %v", out, err)
				}
			}
		})
	}
	// The outstation serves the next master normally.
	cli(t, st, "poll").want(t, 0)
}

// slowProxy forwards loopback TCP connections to upstream; what the outstation sends goes
// to the master one octet at a time, delay apart: an outstation behind a slow or
// congested link. What the master sends goes through at once.
func slowProxy(t *testing.T, upstream string, delay time.Duration) []string {
	t.Helper()
	return proxy(t, upstream, func(down, up net.Conn) {
		b := make([]byte, 1)
		for {
			if _, err := io.ReadFull(up, b); err != nil {
				return
			}
			time.Sleep(delay)
			if _, err := down.Write(b); err != nil {
				return
			}
		}
	})
}

// silentProxy forwards loopback TCP connections to upstream; once the outstation has sent
// after octets, the rest of what it sends is dropped and the connection stays open: a
// response cut short, then silence.
func silentProxy(t *testing.T, upstream string, after int64) []string {
	t.Helper()
	return proxy(t, upstream, func(down, up net.Conn) {
		if _, err := io.CopyN(down, up, after); err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, up)
	})
}

// proxy forwards loopback TCP connections to upstream: the master's octets as they come,
// the outstation's through back, which returns when either side is done.
func proxy(t *testing.T, upstream string, back func(down, up net.Conn)) []string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	t.Cleanup(func() { _ = ln.Close(); wg.Wait() })
	wg.Go(func() {
		for {
			down, err := ln.Accept()
			if err != nil {
				return
			}
			up, err := net.Dial("tcp", upstream)
			if err != nil {
				_ = down.Close()
				continue
			}
			closeBoth := sync.OnceFunc(func() { _ = down.Close(); _ = up.Close() })
			wg.Go(func() { _, _ = io.Copy(up, down); closeBoth() })
			wg.Go(func() { back(down, up); closeBoth() })
		}
	})
	return peer(ln)
}

// TestChaosSlowAndSilent runs commands through an outstation that drips its responses and
// one that stops in the middle of a response without closing the connection.
func TestChaosSlowAndSilent(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)
	for _, c := range []struct {
		name string
		to   func(t *testing.T) []string
		args []string
		code int
		kind string
	}{
		// About 200 octets at 2 ms each: slow, but every fragment within its timeout.
		{"poll of a slow outstation", func(t *testing.T) []string { return slowProxy(t, st.Addr(), 2*time.Millisecond) },
			[]string{"client", "poll", "--response-timeout", "3000"}, 0, ""},
		// 40 ms an octet: a link frame of the response takes longer than the response timeout.
		{"poll of an outstation too slow for the response timeout", func(t *testing.T) []string { return slowProxy(t, st.Addr(), 40*time.Millisecond) },
			[]string{"client", "poll", "--response-timeout", "300"}, 5, "timeout"},
		// The response to the READ stops after its first octets, and nothing follows.
		{"poll of an outstation that stops mid-response", func(t *testing.T) []string { return silentProxy(t, st.Addr(), 20) },
			[]string{"client", "poll", "--response-timeout", "300"}, 5, "timeout"},
		{"file get from an outstation that stops mid-transfer", func(t *testing.T) []string { return silentProxy(t, st.Addr(), 600) },
			[]string{"client", "file", "get", "--path", "/records/big.dat", "--block-size", "256", "--response-timeout", "300", "--output", "-"}, 5, "timeout"},
	} {
		t.Run(c.name, func(t *testing.T) {
			args := append(append([]string{}, c.args...), c.to(t)...)
			args = append(args, "--timeout", "8000", "--format", "json")
			start := time.Now()
			r := run(t, args...)
			if took := time.Since(start); took > 12*time.Second {
				t.Errorf("took %v", took)
			}
			r.want(t, c.code)
			if c.code == 0 {
				return
			}
			if kind, _ := r.errorCode(t); kind != c.kind {
				t.Errorf("error kind %s, want %s", kind, c.kind)
			}
		})
	}
	cli(t, st, "poll").want(t, 0)
}
