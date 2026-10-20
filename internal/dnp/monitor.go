package dnp

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dnp3 "github.com/otfabric/go-dnp3"
	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/client"
	"github.com/otfabric/go-dnp3/link"

	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/errs"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// monitorClasses parses --unsolicited of `client monitor`: classes 1-3, or none.
func monitorClasses(s string) ([]int, error) {
	if strings.EqualFold(strings.TrimSpace(s), "none") {
		return nil, nil
	}
	return ParseClasses(s, 1, allEventClasses)
}

// CheckMonitor validates the configuration of `client monitor`.
func CheckMonitor(cfg *config.MonitorConfig) error {
	if err := Validate(&cfg.ConnConfig); err != nil {
		return err
	}
	if _, err := monitorClasses(cfg.Unsolicited); err != nil {
		return err
	}
	if _, err := ParseObjectFilter(cfg.Objects); err != nil {
		return err
	}
	_, err := ParseIndexFilter(cfg.Indexes)
	return err
}

// Monitor stays connected and hands every value the outstation sends to emit: unsolicited
// responses, and the responses to the integrity poll and the periodic event polls of cfg.
// It ends when ctx ends, after --duration or --count, or, with --no-reconnect, when the
// connection is lost. Connection state changes go to progress.
func Monitor(ctx context.Context, cfg *config.MonitorConfig, progress io.Writer, emit func(*types.Measurement) error) (types.MonitorSummary, error) {
	summary := types.MonitorSummary{}
	classes, err := monitorClasses(cfg.Unsolicited)
	if err != nil {
		return summary, err
	}
	objects, err := ParseObjectFilter(cfg.Objects)
	if err != nil {
		return summary, err
	}
	indexes, err := ParseIndexFilter(cfg.Indexes)
	if err != nil {
		return summary, err
	}
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	if cfg.Duration != 0 {
		var stop context.CancelFunc
		ctx, stop = context.WithTimeout(ctx, time.Duration(cfg.Duration)*time.Second)
		defer stop()
	}

	var (
		mu     sync.Mutex
		failed error
	)
	// An outstation that restarted has forgotten which classes this master enabled for
	// unsolicited responses: enable them again when it reports the restart.
	restarted := make(chan struct{}, 1)
	watcher := &restartWatcher{onRestart: func() {
		if len(classes) == 0 {
			return
		}
		select {
		case restarted <- struct{}{}:
		default:
		}
	}}
	if cfg.Debug {
		watcher.trace = &Tracer{W: progress}
	}
	handler := client.HandlerFunc(func(info client.ResponseInfo, ms []client.Measurement) {
		mu.Lock()
		defer mu.Unlock()
		if failed != nil || ctx.Err() != nil {
			return
		}
		for _, m := range ms {
			if !objects.Match(m.GV) || !indexes.Match(m.Index) {
				continue
			}
			out := ToMeasurement(m)
			out.ReceivedAt, out.Unsolicited = Now(), info.Unsolicited
			if err := emit(&out); err != nil {
				failed = errs.Output(errs.CodeOutputWriteFailed, err)
				cancel(failed)
				return
			}
			summary.Values++
			if info.Unsolicited {
				summary.Unsolicited++
			}
			if cfg.Count != 0 && uint32(summary.Values) >= cfg.Count {
				cancel(nil)
				return
			}
		}
	})
	lost := make(chan error, 1)
	state := func(st dnp3.State, err error) {
		if err != nil && ctx.Err() == nil {
			_, _ = fmt.Fprintf(progress, "monitor: %s: %v\n", st, err)
		} else {
			_, _ = fmt.Fprintf(progress, "monitor: %s\n", st)
		}
		if st == dnp3.StateDisconnected && err != nil && cfg.NoReconnect {
			select {
			case lost <- err:
			default:
			}
		}
	}
	extra := []client.Option{
		client.WithHandler(handler),
		client.WithStateHandler(state),
		client.WithStartup(client.Startup{Integrity: cfg.Integrity, EnableUnsolicited: classes}),
		// A long-running master clears DEVICE_RESTART, as IEEE 1815-2012 expects of it.
		client.WithAutoClearRestart(true),
		client.WithMetrics(watcher),
	}
	if cfg.Integrity {
		// After a restart or an event buffer overflow the outstation's events are gone: an
		// integrity poll brings the stream up to date again.
		extra = append(extra, client.WithAutoIntegrity(true))
	}
	if !cfg.NoReconnect {
		extra = append(extra, client.WithReconnect(client.Reconnect{}))
	}
	if cfg.PollInterval != 0 {
		extra = append(extra, client.WithPoll(client.Poll{Interval: time.Duration(cfg.PollInterval) * time.Second, Headers: app.ClassHeaders(1, 2, 3)}))
	}
	begin := time.Now()
	if t, err := cfg.ResolveTarget(); err == nil {
		summary.Target = t.String()
	}
	s, err := Dial(ctx, &cfg.ConnConfig, progress, extra...)
	if err != nil {
		summary.Seconds = time.Since(begin).Seconds()
		mu.Lock()
		defer mu.Unlock()
		switch {
		case failed != nil:
			return summary, failed
		case ctx.Err() != nil:
			// Interrupted, or --count or --duration reached while connecting.
			return summary, nil
		}
		return summary, err
	}
	var result error
wait:
	for {
		select {
		case <-ctx.Done():
			break wait
		case err := <-lost:
			result = WrapError(err)
			break wait
		case <-restarted:
			rctx, stop := context.WithTimeout(ctx, s.Timeout)
			if err := s.EnableUnsolicited(rctx, classes...); err != nil && ctx.Err() == nil {
				_, _ = fmt.Fprintf(progress, "monitor: enabling unsolicited responses after a restart: %v\n", WrapError(err))
			} else if err == nil {
				_, _ = fmt.Fprintln(progress, "monitor: the outstation restarted: unsolicited responses enabled again")
			}
			stop()
		}
	}
	_ = s.Close()
	summary.Seconds = time.Since(begin).Seconds()
	mu.Lock()
	defer mu.Unlock()
	if failed != nil {
		return summary, failed
	}
	return summary, result
}

// restartWatcher sees the IIN of every fragment the outstation sends, null responses
// included, which the measurement handler does not get. DEVICE_RESTART counts once it was
// seen clear: an outstation reports it from its start until a master clears it, and the
// startup sequence already enables the classes when connecting. Every callback is handed on
// to the --debug trace.
type restartWatcher struct {
	trace     dnp3.Metrics
	onRestart func()
	sawClear  atomic.Bool
}

func (w *restartWatcher) OnFragmentReceived(remote net.Addr, f *app.Fragment, size int) {
	switch {
	case !f.IIN.Has(app.IINDeviceRestart):
		w.sawClear.Store(true)
	case w.sawClear.Swap(false):
		w.onRestart()
	}
	if w.trace != nil {
		w.trace.OnFragmentReceived(remote, f, size)
	}
}

func (w *restartWatcher) OnConnect(remote net.Addr) {
	if w.trace != nil {
		w.trace.OnConnect(remote)
	}
}

func (w *restartWatcher) OnDisconnect(remote net.Addr, err error) {
	if w.trace != nil {
		w.trace.OnDisconnect(remote, err)
	}
}

func (w *restartWatcher) OnFrameSent(remote net.Addr, f *link.Frame, size int) {
	if w.trace != nil {
		w.trace.OnFrameSent(remote, f, size)
	}
}

func (w *restartWatcher) OnFrameReceived(remote net.Addr, f *link.Frame, size int) {
	if w.trace != nil {
		w.trace.OnFrameReceived(remote, f, size)
	}
}

func (w *restartWatcher) OnFramingError(remote net.Addr, err error) {
	if w.trace != nil {
		w.trace.OnFramingError(remote, err)
	}
}

func (w *restartWatcher) OnFragmentSent(remote net.Addr, f *app.Fragment, size int) {
	if w.trace != nil {
		w.trace.OnFragmentSent(remote, f, size)
	}
}

func (w *restartWatcher) OnDecodeError(remote net.Addr, err error) {
	if w.trace != nil {
		w.trace.OnDecodeError(remote, err)
	}
}
