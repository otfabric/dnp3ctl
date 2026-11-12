// SPDX-License-Identifier: MIT

package dnp

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
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

// timeSyncModes are the values of --time-sync of `client monitor`; auto leaves go-dnp3's
// choice (LAN over TCP, TLS and UDP, non-LAN over other streams).
var timeSyncModes = map[string]client.TimeSync{
	"lan":             client.TimeSyncLAN,
	"non-lan":         client.TimeSyncNonLAN,
	"clear-need-time": client.TimeSyncClearNeedTime,
	"none":            client.TimeSyncNone,
}

// TimeSyncModes describes the values of --time-sync, for completion.
func TimeSyncModes() map[string]string {
	return map[string]string{
		"auto":            "LAN procedure over TCP, TLS and UDP, non-LAN over serial (default)",
		"lan":             "RECORD_CURRENT_TIME, then the time of that request",
		"non-lan":         "DELAY_MEASURE, then the time corrected by the delay",
		"clear-need-time": "clear NEED_TIME without setting the time",
		"none":            "leave NEED_TIME to the operator",
	}
}

// monitorOptions returns the options of cfg that only a long-running master has, and the
// listener of --dual-endpoint, which the caller closes when it is not handed to a client.
func monitorOptions(cfg *config.MonitorConfig, listen bool) ([]client.Option, net.Listener, error) {
	var opts []client.Option
	switch mode := strings.ToLower(strings.TrimSpace(cfg.TimeSync)); mode {
	case "", "auto":
	default:
		ts, ok := timeSyncModes[mode]
		if !ok {
			return nil, nil, errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("invalid --time-sync %q: use auto, lan, non-lan, clear-need-time or none", cfg.TimeSync), nil)
		}
		opts = append(opts, client.WithTimeSync(ts))
	}
	j, err := jitter("--reconnect-jitter", cfg.ReconnectJitter)
	if err != nil {
		return nil, nil, err
	}
	switch {
	case !cfg.NoReconnect:
		opts = append(opts, client.WithReconnect(client.Reconnect{Jitter: j}))
	case cfg.ReconnectJitter != 0:
		return nil, nil, errs.InvalidInput(errs.CodeInvalidFlagCombination, "--reconnect-jitter does not apply with --no-reconnect", nil)
	}
	if cfg.AutoEventPoll {
		opts = append(opts, client.WithAutoEventPoll(true))
	}
	if cfg.QueueLimit != 0 {
		opts = append(opts, client.WithQueueLimit(int(cfg.QueueLimit)))
	}
	if cfg.NoUnsolicitedConfirm {
		opts = append(opts, client.WithUnsolicitedConfirm(false))
	}
	allow, err := ParsePrefixes(cfg.DualEndpointAllow, "--dual-endpoint-allow")
	if err != nil {
		return nil, nil, errs.InvalidInput(errs.CodeInvalidInput, err.Error(), err)
	}
	addr := strings.TrimSpace(cfg.DualEndpoint)
	if addr == "" {
		if len(allow) > 0 {
			return nil, nil, errs.InvalidInput(errs.CodeInvalidFlagCombination, "--dual-endpoint-allow needs --dual-endpoint", nil)
		}
		return opts, nil, nil
	}
	target, err := cfg.ResolveTarget()
	if err != nil {
		return nil, nil, errs.InvalidInput(errs.CodeInvalidURL, err.Error(), err)
	}
	if target.Listen || target.Scheme != config.SchemeTCP {
		return nil, nil, errs.InvalidInput(errs.CodeInvalidFlagCombination, fmt.Sprintf("--dual-endpoint needs a tcp:// outstation the master connects to; the target is %s", target), nil)
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return nil, nil, errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("invalid --dual-endpoint %q: use host:port or :port", cfg.DualEndpoint), err)
	}
	if !listen {
		return opts, nil, nil
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, errs.New(errs.KindTransport, errs.CodeListenFailed, fmt.Sprintf("cannot listen for the outstation: %v", err), err)
	}
	return append(opts, client.WithDualEndpoint(l, allowFunc(allow))), l, nil
}

// allowFunc returns the address check of --dual-endpoint-allow: nil (any address) for no
// prefixes.
func allowFunc(allow []netip.Prefix) func(net.Addr) bool {
	if len(allow) == 0 {
		return nil
	}
	return func(a net.Addr) bool {
		ap, err := netip.ParseAddrPort(a.String())
		if err != nil {
			return false
		}
		for _, p := range allow {
			if p.Contains(ap.Addr().Unmap()) {
				return true
			}
		}
		return false
	}
}

// ParsePrefixes parses a list of IP addresses and CIDR prefixes, as --allow, --master-ips
// and --dual-endpoint-allow take them; an address is a prefix of its full length. flag
// names the list in errors.
func ParsePrefixes(list []string, flag string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, s := range list {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p.Masked())
			continue
		}
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("invalid %s %q: use an address or a CIDR prefix", flag, s)
		}
		out = append(out, netip.PrefixFrom(addr.WithZone(""), addr.BitLen()))
	}
	return out, nil
}

// CheckMonitor validates the configuration of `client monitor`.
func CheckMonitor(cfg *config.MonitorConfig) error {
	if err := Validate(&cfg.ConnConfig); err != nil {
		return err
	}
	if _, _, err := monitorOptions(cfg, false); err != nil {
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
	// unsolicited responses: go-dnp3 runs the restart procedure of IEEE 1815-2012 clause
	// 5.1.1.1.2 (clear IIN1.7, time, integrity poll, ENABLE_UNSOLICITED of the startup
	// classes) by itself; the monitor reports the restart.
	watcher := &restartWatcher{onRestart: func() {
		_, _ = fmt.Fprintln(progress, "monitor: the outstation reported a restart")
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
			if cfg.Count != 0 && int64(summary.Values) >= int64(cfg.Count) {
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
	mopts, dual, err := monitorOptions(cfg, true)
	if err != nil {
		return summary, err
	}
	if dual != nil {
		// The client closes the listener of WithDualEndpoint; this covers a client that
		// was never created.
		defer func() { _ = dual.Close() }()
	}
	extra := []client.Option{
		client.WithHandler(handler),
		client.WithStateHandler(state),
		client.WithStartup(client.Startup{Integrity: cfg.Integrity, EnableUnsolicited: classes}),
		// A long-running master clears DEVICE_RESTART, as IEEE 1815-2012 expects of it.
		client.WithAutoClearRestart(true),
		client.WithMetrics(watcher),
		// After a restart or an event buffer overflow the outstation's events are gone: with
		// --integrity an integrity poll brings the stream up to date again.
		client.WithAutoIntegrity(cfg.Integrity),
	}
	extra = append(extra, mopts...)
	if cfg.PollInterval != 0 {
		extra = append(extra, client.WithPoll(client.Poll{Interval: time.Duration(cfg.PollInterval) * time.Second, Headers: app.ClassHeaders(1, 2, 3)}))
	}
	begin := time.Now()
	if t, err := cfg.ResolveTarget(); err == nil {
		summary.Target = t.String()
	}
	s, err := DialMonitor(ctx, &cfg.ConnConfig, progress, extra...)
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
		}
	}
	if cfg.SAKeyFile != "" || cfg.SAUpdateKeyFile != "" || cfg.SAKey != "" || cfg.SAUpdateKey != "" {
		if line := SecurityStatisticsText(s.SecurityStatistics()); line != "" {
			_, _ = fmt.Fprintf(progress, "monitor: security statistics of the master: %s\n", line)
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

// SecurityStatisticsText renders the master's security statistics (IEEE 1815-2012 Table
// 7-6) that are not zero, "name n" comma-separated, in the order of the table.
func SecurityStatisticsText(stats []uint32) string {
	var parts []string
	for i, n := range stats {
		if n != 0 {
			parts = append(parts, fmt.Sprintf("%s %d", statisticName(uint32(i)), n))
		}
	}
	return strings.Join(parts, ", ")
}
