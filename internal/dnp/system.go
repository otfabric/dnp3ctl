// SPDX-License-Identifier: MIT

package dnp

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/client"

	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// Time synchronization methods of `client time-sync`.
const (
	TimeLAN    = "lan"
	TimeNonLAN = "non-lan"
	TimeWrite  = "write"
)

// TimeMethods describes the values of --method of `client time-sync`.
func TimeMethods() map[string]string {
	return map[string]string{
		TimeLAN:    "RECORD_CURRENT_TIME, then WRITE g50v3 (the LAN procedure)",
		TimeNonLAN: "DELAY_MEASURE, then WRITE g50v1 corrected by the delay (serial links)",
		TimeWrite:  "WRITE g50v1 with --time, uncorrected",
	}
}

// RestartTypes describes the values of --type of `client restart`.
func RestartTypes() map[string]string {
	return map[string]string{
		"cold": "COLD_RESTART: the whole outstation restarts",
		"warm": "WARM_RESTART: the DNP3 application restarts",
	}
}

// confirmed runs a request that is answered by a null response and fills the common part of
// its result.
func confirmed(ctx context.Context, cc *config.ConnConfig, trace io.Writer, res *types.ConfirmedResult, fn func(context.Context, *Session) error) (*types.ConfirmedResult, error) {
	s, err := Dial(ctx, cc, trace)
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.Close() }()
	took, err := s.request(ctx, func(ctx context.Context) error { return fn(ctx, s) })
	if err != nil {
		return nil, WrapError(err)
	}
	res.Target, res.Outstation, res.RTTMs = s.Target.String(), s.Outstation, ms(took)
	return res, nil
}

func timeMethod(s string) (string, error) {
	switch m := strings.ToLower(strings.TrimSpace(s)); m {
	case "", TimeLAN:
		return TimeLAN, nil
	case TimeNonLAN, "nonlan", "serial":
		return TimeNonLAN, nil
	case TimeWrite:
		return TimeWrite, nil
	}
	return "", fmt.Errorf("invalid --method %q: use lan, non-lan or write", s)
}

// CheckTimeSync validates the configuration of `client time-sync`.
func CheckTimeSync(cfg *config.TimeSyncConfig) error {
	if err := Validate(&cfg.ConnConfig); err != nil {
		return err
	}
	m, err := timeMethod(cfg.Method)
	if err != nil {
		return err
	}
	if m != TimeWrite && strings.TrimSpace(cfg.Time) != "" && !strings.EqualFold(strings.TrimSpace(cfg.Time), "now") {
		return fmt.Errorf("--time applies to --method write; lan and non-lan send the current time")
	}
	_, err = ParseTime(cfg.Time, time.Now)
	return err
}

// TimeSync synchronizes the time of the outstation.
func TimeSync(ctx context.Context, cfg *config.TimeSyncConfig, trace io.Writer) (*types.ConfirmedResult, error) {
	m, err := timeMethod(cfg.Method)
	if err != nil {
		return nil, err
	}
	at, err := ParseTime(cfg.Time, time.Now)
	if err != nil {
		return nil, err
	}
	res := &types.ConfirmedResult{Request: "time-sync", Method: m}
	return confirmed(ctx, &cfg.ConnConfig, trace, res, func(ctx context.Context, s *Session) error {
		switch m {
		case TimeLAN:
			res.Time = Now()
			return s.SyncTime(ctx, client.TimeSyncLAN)
		case TimeNonLAN:
			res.Time = Now()
			return s.SyncTime(ctx, client.TimeSyncNonLAN)
		default:
			res.Time = at.UTC().Format(timeLayout)
			return s.WriteTime(ctx, at)
		}
	})
}

func restartType(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "cold":
		return true, nil
	case "warm":
		return false, nil
	}
	return false, fmt.Errorf("invalid --type %q: use cold or warm", s)
}

// CheckRestart validates the configuration of `client restart`.
func CheckRestart(cfg *config.RestartConfig) error {
	if err := Validate(&cfg.ConnConfig); err != nil {
		return err
	}
	_, err := restartType(cfg.Type)
	return err
}

// Restart asks the outstation for a cold or warm restart.
func Restart(ctx context.Context, cfg *config.RestartConfig, trace io.Writer) (*types.ConfirmedResult, error) {
	cold, err := restartType(cfg.Type)
	if err != nil {
		return nil, err
	}
	res := &types.ConfirmedResult{Request: "warm-restart"}
	if cold {
		res.Request = "cold-restart"
	}
	return confirmed(ctx, &cfg.ConnConfig, trace, res, func(ctx context.Context, s *Session) error {
		var delay time.Duration
		var rerr error
		if cold {
			delay, rerr = s.ColdRestart(ctx)
		} else {
			delay, rerr = s.WarmRestart(ctx)
		}
		d := ms(delay)
		res.DelayMs = &d
		return rerr
	})
}

// CheckUnsolicited validates the configuration of `client unsolicited enable|disable`.
func CheckUnsolicited(cfg *config.UnsolicitedConfig) error {
	if err := Validate(&cfg.ConnConfig); err != nil {
		return err
	}
	_, err := ParseClasses(cfg.Classes, 1, allEventClasses)
	return err
}

// Unsolicited enables or disables unsolicited responses.
func Unsolicited(ctx context.Context, cfg *config.UnsolicitedConfig, enable bool, trace io.Writer) (*types.ConfirmedResult, error) {
	classes, err := ParseClasses(cfg.Classes, 1, allEventClasses)
	if err != nil {
		return nil, err
	}
	res := &types.ConfirmedResult{Request: "disable-unsolicited", Classes: classes}
	if enable {
		res.Request = "enable-unsolicited"
	}
	return confirmed(ctx, &cfg.ConnConfig, trace, res, func(ctx context.Context, s *Session) error {
		if enable {
			return s.EnableUnsolicited(ctx, classes...)
		}
		return s.DisableUnsolicited(ctx, classes...)
	})
}

func objectsPlan(objects, rng string, def []app.GroupVariation) ([]app.GroupVariation, Selection, error) {
	gvs, err := ParseObjects(objects)
	if err != nil {
		return nil, Selection{}, err
	}
	if len(gvs) == 0 {
		gvs = def
	}
	if len(gvs) == 0 {
		return nil, Selection{}, fmt.Errorf("no object given: set --object (e.g. --object g30)")
	}
	for _, gv := range gvs {
		if gv.IsEvent() {
			return nil, Selection{}, fmt.Errorf("--object %s is an event object: name the static object (e.g. g30 for analog inputs)", gv)
		}
	}
	sel, err := ParseSelection(rng, 0)
	return gvs, sel, err
}

// CheckAssignClass validates the configuration of `client assign-class`.
func CheckAssignClass(cfg *config.AssignClassConfig) error {
	if err := Validate(&cfg.ConnConfig); err != nil {
		return err
	}
	if cfg.Class > 3 {
		return fmt.Errorf("invalid --class %d: use 0-3", cfg.Class)
	}
	_, _, err := objectsPlan(cfg.Objects, cfg.Range, nil)
	return err
}

// AssignClass assigns points to an event class.
func AssignClass(ctx context.Context, cfg *config.AssignClassConfig, trace io.Writer) (*types.ConfirmedResult, error) {
	gvs, sel, err := objectsPlan(cfg.Objects, cfg.Range, nil)
	if err != nil {
		return nil, err
	}
	class := int(cfg.Class)
	res := &types.ConfirmedResult{Request: "assign-class", Class: &class, Objects: ObjectNames(gvs), Range: sel.String()}
	return confirmed(ctx, &cfg.ConnConfig, trace, res, func(ctx context.Context, s *Session) error {
		return s.AssignClass(ctx, class, sel.Headers(gvs)...)
	})
}

var defaultFreeze = []app.GroupVariation{app.G20V0}

// freezePlan is a validated freeze request.
type freezePlan struct {
	gvs       []app.GroupVariation
	sel       Selection
	at        time.Time
	interval  time.Duration
	broadcast string
}

func planFreeze(cfg *config.FreezeConfig) (freezePlan, error) {
	gvs, sel, err := objectsPlan(cfg.Objects, cfg.Range, defaultFreeze)
	if err != nil {
		return freezePlan{}, err
	}
	p := freezePlan{gvs: gvs, sel: sel}
	if _, p.broadcast, err = checkBroadcast(cfg.Broadcast, &cfg.ConnConfig); err != nil {
		return p, err
	}
	at, every := strings.TrimSpace(cfg.At), strings.TrimSpace(cfg.Interval)
	switch {
	case at == "" && every != "":
		return p, fmt.Errorf("--interval needs --at")
	case at == "":
		return p, nil
	case cfg.Clear || cfg.NoAck || p.broadcast != "":
		return p, fmt.Errorf("--at cannot be combined with --clear, --no-ack or --broadcast")
	}
	if p.at, err = time.Parse(time.RFC3339Nano, at); err != nil {
		return p, fmt.Errorf("invalid --at %q: use an RFC 3339 time such as 2026-01-31T12:00:00Z", cfg.At)
	}
	if every != "" {
		if p.interval, err = time.ParseDuration(every); err != nil || p.interval <= 0 {
			return p, fmt.Errorf("invalid --interval %q: use a duration such as 15m or 1h", cfg.Interval)
		}
	}
	return p, nil
}

// CheckFreeze validates the configuration of `client freeze`.
func CheckFreeze(cfg *config.FreezeConfig) error {
	if err := Validate(&cfg.ConnConfig); err != nil {
		return err
	}
	_, err := planFreeze(cfg)
	return err
}

// freezeFunction returns the function code of an immediate freeze.
func freezeFunction(clear, noAck bool) app.FunctionCode {
	switch {
	case clear && noAck:
		return app.FuncFreezeClearNoAck
	case clear:
		return app.FuncFreezeClear
	case noAck:
		return app.FuncImmediateFreezeNoAck
	default:
		return app.FuncImmediateFreeze
	}
}

// Freeze freezes counters or analog inputs: at once, optionally clearing the counters, or
// at a time, optionally repeated; to the outstation or broadcast.
func Freeze(ctx context.Context, cfg *config.FreezeConfig, trace io.Writer) (*types.ConfirmedResult, error) {
	p, err := planFreeze(cfg)
	if err != nil {
		return nil, err
	}
	res := &types.ConfirmedResult{Request: "freeze", Objects: ObjectNames(p.gvs), Range: p.sel.String(), NoAck: cfg.NoAck, Broadcast: p.broadcast}
	switch {
	case !p.at.IsZero():
		res.Request, res.Time = "freeze-at-time", p.at.UTC().Format(timeLayout)
		if p.interval != 0 {
			res.Interval = p.interval.String()
		}
	case cfg.Clear:
		res.Request = "freeze-clear"
	}
	headers := p.sel.Headers(p.gvs)
	return confirmed(ctx, &cfg.ConnConfig, trace, res, func(ctx context.Context, s *Session) error {
		switch {
		case !p.at.IsZero():
			return s.FreezeAtTime(ctx, p.at, p.interval, headers...)
		case p.broadcast != "":
			return s.Broadcast(ctx, broadcastAddresses[p.broadcast], freezeFunction(cfg.Clear, cfg.NoAck), headers...)
		default:
			return s.Freeze(ctx, cfg.Clear, cfg.NoAck, headers...)
		}
	})
}

// CheckPing validates the configuration of `client ping`.
func CheckPing(cfg *config.PingConfig) error {
	if err := Validate(&cfg.ConnConfig); err != nil {
		return err
	}
	if cfg.Interval < 10 {
		return fmt.Errorf("invalid --interval %d: use at least 10 milliseconds", cfg.Interval)
	}
	return nil
}

// Ping measures the delay to the outstation with DELAY_MEASURE, --count times or until ctx
// ends. emit receives each sample as it completes.
func Ping(ctx context.Context, cfg *config.PingConfig, trace io.Writer, emit func(*types.PingSample) error) (types.PingSummary, error) {
	summary := types.PingSummary{}
	s, err := Dial(ctx, &cfg.ConnConfig, trace)
	if err != nil {
		return summary, err
	}
	defer func() { _ = s.Close() }()
	summary.Target = s.Target.String()
	var total float64
	tick := time.NewTicker(time.Duration(cfg.Interval) * time.Millisecond)
	defer tick.Stop()
	for seq := uint32(1); cfg.Count == 0 || seq <= cfg.Count; seq++ {
		if seq > 1 {
			select {
			case <-ctx.Done():
				return finishPing(summary, total), nil
			case <-tick.C:
			}
		}
		sample := &types.PingSample{Seq: seq, Timestamp: Now()}
		var processing, rtt time.Duration
		_, err := s.request(ctx, func(ctx context.Context) error {
			var rerr error
			processing, rtt, rerr = s.DelayMeasure(ctx)
			return rerr
		})
		if ctx.Err() != nil {
			return finishPing(summary, total), nil
		}
		summary.Sent++
		if err != nil {
			summary.Failed++
			sample.Error = EmbeddedErrorInfo(err)
		} else {
			sample.RTTMs, sample.ProcessingMs = ms(rtt), ms(processing)
			total += sample.RTTMs
			if summary.Sent-summary.Failed == 1 || sample.RTTMs < summary.Min {
				summary.Min = sample.RTTMs
			}
			summary.Max = max(summary.Max, sample.RTTMs)
		}
		if werr := emit(sample); werr != nil {
			return finishPing(summary, total), werr
		}
		if err != nil && Fatal(err) {
			return finishPing(summary, total), WrapError(err)
		}
	}
	return finishPing(summary, total), nil
}

// finishPing fills the average round trip of a summary.
func finishPing(s types.PingSummary, total float64) types.PingSummary {
	if ok := s.Sent - s.Failed; ok > 0 {
		s.Avg = total / float64(ok)
	}
	return s
}
