// SPDX-License-Identifier: MIT

package cmd

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/otfabric/dnp3ctl/internal/stationtest"
)

// requestCounter forwards loopback TCP connections to upstream and drops everything the
// outstation sends back: requests reach the outstation, responses never reach the master.
// It counts, per connection, the link frames with user data the master sends: one for each
// time it sends a request.
type requestCounter struct {
	mu     sync.Mutex
	frames []int           // per connection, in the order they were accepted
	ended  []chan struct{} // closed when the master's side of the connection has ended
}

func newRequestCounter(t *testing.T, upstream string) (*requestCounter, []string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c := &requestCounter{}
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
			c.mu.Lock()
			conn, ended := len(c.frames), make(chan struct{})
			c.frames, c.ended = append(c.frames, 0), append(c.ended, ended)
			c.mu.Unlock()
			closeBoth := sync.OnceFunc(func() { _ = down.Close(); _ = up.Close() })
			wg.Go(func() { _, _ = io.Copy(io.Discard, up); closeBoth() })
			wg.Go(func() {
				defer close(ended)
				defer closeBoth()
				var pending []byte
				buf := make([]byte, 4096)
				for {
					n, err := down.Read(buf)
					if n > 0 {
						if _, werr := up.Write(buf[:n]); werr != nil {
							return
						}
						pending = append(pending, buf[:n]...)
						var frames int
						pending, frames = dataFrames(pending)
						c.mu.Lock()
						c.frames[conn] += frames
						c.mu.Unlock()
					}
					if err != nil {
						return
					}
				}
			})
		}
	})
	return c, peer(ln)
}

// dataFrames counts the complete link frames at the start of b that carry user data, and
// returns what follows them.
func dataFrames(b []byte) ([]byte, int) {
	n := 0
	for len(b) >= 3 {
		if b[0] != 0x05 || b[1] != 0x64 || b[2] < 5 {
			b = b[1:]
			continue
		}
		data := int(b[2]) - 5
		size := 10 + data + 2*((data+15)/16)
		if len(b) < size {
			break
		}
		if data > 0 {
			n++
		}
		b = b[size:]
	}
	return b, n
}

// last returns the count of the last connection, once the master has closed it.
func (c *requestCounter) last(t *testing.T) int {
	t.Helper()
	c.mu.Lock()
	if len(c.frames) == 0 {
		c.mu.Unlock()
		return -1
	}
	ended := c.ended[len(c.ended)-1]
	c.mu.Unlock()
	select {
	case <-ended:
	case <-time.After(10 * time.Second):
		t.Fatal("the master's connection is still open 10 s after the command ended")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.frames[len(c.frames)-1]
}

// TestControlSentOnceWithRetries: --retries repeats a request that is safe to repeat; a
// control, a freeze and a restart whose response does not arrive are sent once, and the
// command ends with a timeout (exit code 5).
func TestControlSentOnceWithRetries(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)
	counter, to := newRequestCounter(t, st.Addr())
	for _, c := range []struct {
		args  []string
		sends int
	}{
		{[]string{"client", "operate", "--type", "crob", "--index", "1", "--op", "latch-on", "--mode", "direct"}, 1},
		{[]string{"client", "operate", "--type", "crob", "--index", "1", "--op", "latch-on"}, 1}, // the SELECT
		{[]string{"client", "operate", "--type", "analog", "--index", "0", "--value", "5", "--mode", "direct"}, 1},
		{[]string{"client", "freeze"}, 1},
		{[]string{"client", "restart", "--type", "cold"}, 1},
		// A read is safe to repeat: it is sent once and three times more.
		{[]string{"client", "read", "--object", "g1v0"}, 4},
	} {
		full := append(append(append([]string{}, c.args...), to...), "--retries", "3", "--response-timeout", "200", "--timeout", "10000", "--format", "json")
		r := run(t, full...).want(t, 5)
		if kind, _ := r.errorCode(t); kind != "timeout" {
			t.Errorf("%v: error kind %s, want timeout", c.args, kind)
		}
		if got := counter.last(t); got != c.sends {
			t.Errorf("%v: the master sent %d requests, want %d", c.args, got, c.sends)
		}
	}
	_ = st.Close()
	counts := map[string]int{}
	for _, e := range st.Events() {
		if e.Event == "request" {
			counts[e.Function]++
		}
	}
	for fc, want := range map[string]int{"DIRECT_OPERATE": 2, "SELECT": 1, "OPERATE": 0, "IMMED_FREEZE": 1, "COLD_RESTART": 1} {
		if counts[fc] != want {
			t.Errorf("the outstation handled %s %d times, want %d: %v", fc, counts[fc], want, counts)
		}
	}
}
