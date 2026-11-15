// SPDX-License-Identifier: MIT

package station

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/server"

	"github.com/otfabric/dnp3ctl/internal/types"
)

const restartModel = "outstation: {restart_delay: 2s}\npoints: [{type: binary-input, index: 0, value: true}]\n"

// restartStation is a station of restartModel, with its log.
func restartStation(t *testing.T, log func(types.ServerEvent)) *Station {
	t.Helper()
	m, err := Parse([]byte(restartModel))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	st, err := New(m, Options{Log: log})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// count returns how many events of the log have a detail that holds detail.
func (l *eventLog) count(detail string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, e := range l.events {
		if e.Event == "restart" && strings.Contains(e.Detail, detail) {
			n++
		}
	}
	return n
}

// TestRestartPending: requests while a restart is pending are answered with the time left
// and add no restart; the outstation restarts once, after restart_delay.
func TestRestartPending(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := &eventLog{}
		st := restartStation(t, l.log)
		defer func() { _ = st.Close() }()
		h := &handler{st: st}
		for i := range 1000 {
			d, err := h.Restart(context.Background(), false)
			if err != nil || d != 2*time.Second {
				t.Fatalf("request %d: %s, %v", i+1, d, err)
			}
		}
		time.Sleep(1500 * time.Millisecond)
		if d, err := h.Restart(context.Background(), false); err != nil || d != 500*time.Millisecond {
			t.Errorf("a request 1.5 s later: %s, %v, want 500ms", d, err)
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if n := l.count("restart done"); n != 1 {
			t.Errorf("%d restarts after 1001 requests, want 1", n)
		}
		// After the restart, a request schedules the next one.
		if d, err := h.Restart(context.Background(), false); err != nil || d != 2*time.Second {
			t.Errorf("a request after the restart: %s, %v", d, err)
		}
		time.Sleep(3 * time.Second)
		synctest.Wait()
		if n := l.count("warm restart done"); n != 2 {
			t.Errorf("%d restarts, want 2", n)
		}
	})
}

// TestRestartColdWins: a cold request makes a pending warm restart cold, at the time the
// warm one was due; the cold restart puts the points back to the model.
func TestRestartColdWins(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := &eventLog{}
		st := restartStation(t, l.log)
		defer func() { _ = st.Close() }()
		h := &handler{st: st}
		bi := pointTypes["binary-input"]
		if err := st.srv.Database().Update(bi, 0, app.Binary{Value: false, Flags: app.FlagOnline}, server.EventSuppress); err != nil {
			t.Fatal(err)
		}
		if d, err := h.Restart(context.Background(), false); err != nil || d != 2*time.Second {
			t.Fatalf("warm: %s, %v", d, err)
		}
		time.Sleep(500 * time.Millisecond)
		if d, err := h.Restart(context.Background(), true); err != nil || d != 1500*time.Millisecond {
			t.Errorf("cold 0.5 s later: %s, %v, want 1.5s", d, err)
		}
		if !l.has("restart", "the pending restart is now cold") {
			t.Error("no event for the cold request")
		}
		if d, err := h.Restart(context.Background(), false); err != nil || d != 1500*time.Millisecond {
			t.Errorf("warm after cold: %s, %v", d, err)
		}
		time.Sleep(1499 * time.Millisecond)
		synctest.Wait()
		if n := l.count("restart done"); n != 0 {
			t.Errorf("%d restarts before the time that was due", n)
		}
		time.Sleep(time.Millisecond)
		synctest.Wait()
		if l.count("cold restart done") != 1 || l.count("warm restart done") != 0 {
			t.Errorf("want one cold restart: %+v", l.events)
		}
		if v, _ := st.srv.Database().Get(bi, 0); !v.(app.Binary).Value {
			t.Errorf("binary input 0 after the cold restart: %+v, want the model's true", v)
		}
	})
}

// TestRestartAfterClose: a restart pending at Close is not carried out, and a request after
// Close is refused.
func TestRestartAfterClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := &eventLog{}
		st := restartStation(t, l.log)
		h := &handler{st: st}
		if _, err := h.Restart(context.Background(), true); err != nil {
			t.Fatal(err)
		}
		if err := st.Close(); err != nil {
			t.Fatal(err)
		}
		if d, err := h.Restart(context.Background(), false); err == nil {
			t.Errorf("a request after Close: %s, nil", d)
		}
		if !l.has("restart", "warm restart refused: the outstation is stopping") {
			t.Errorf("no event for the refused request: %+v", l.events)
		}
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if n := l.count("restart done"); n != 0 {
			t.Errorf("%d restarts after Close", n)
		}
		if err := st.Close(); err != nil {
			t.Errorf("Close again: %v", err)
		}
	})
}

// TestRestartRacesClose: requests from several goroutines while Close runs; no restart is
// carried out after Close returns, and Close does not return before one in progress ends.
func TestRestartRacesClose(t *testing.T) {
	for _, after := range []time.Duration{0, 2 * time.Second, 2*time.Second + time.Millisecond} {
		t.Run(after.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var closed atomic.Bool
				var late atomic.Int32
				st := restartStation(t, func(e types.ServerEvent) {
					if closed.Load() && strings.HasSuffix(e.Detail, "restart done") {
						late.Add(1)
					}
				})
				h := &handler{st: st}
				var wg sync.WaitGroup
				for i := range 8 {
					wg.Go(func() {
						for j := range 50 {
							_, _ = h.Restart(context.Background(), (i+j)%3 == 0)
							time.Sleep(time.Duration(i+1) * 10 * time.Millisecond)
						}
					})
				}
				time.Sleep(after)
				wg.Go(func() {
					_ = st.Close()
					closed.Store(true)
				})
				wg.Go(func() { _ = st.Close() })
				wg.Wait()
				closed.Store(true)
				time.Sleep(10 * time.Second)
				synctest.Wait()
				if n := late.Load(); n != 0 {
					t.Errorf("%d restarts after Close returned", n)
				}
			})
		})
	}
}
