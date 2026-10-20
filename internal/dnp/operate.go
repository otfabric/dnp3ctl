package dnp

import (
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/client"

	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// Control types and modes of `client operate`.
const (
	ControlCROB   = "crob"
	ControlAnalog = "analog"

	ModeSBO         = "sbo"
	ModeDirect      = "direct"
	ModeDirectNoAck = "direct-no-ack"
)

// ControlTypes describes the values of --type of `client operate`.
func ControlTypes() map[string]string {
	return map[string]string{
		ControlCROB:   "control relay output block (g12v1): latch, pulse, trip and close",
		ControlAnalog: "analog output block (g41): a set-point",
	}
}

// OperateModes describes the values of --mode of `client operate`.
func OperateModes() map[string]string {
	return map[string]string{
		ModeSBO:         "SELECT, then OPERATE when the select was accepted",
		ModeDirect:      "DIRECT_OPERATE",
		ModeDirectNoAck: "DIRECT_OPERATE_NR: no response, no status",
	}
}

// crobOps maps --op to the operation type and trip-close code of a crob.
var crobOps = map[string]struct {
	op  app.OpType
	tcc app.TripClose
}{
	"latch-on":  {app.OpLatchOn, app.TCCNul},
	"latch-off": {app.OpLatchOff, app.TCCNul},
	"pulse-on":  {app.OpPulseOn, app.TCCNul},
	"pulse-off": {app.OpPulseOff, app.TCCNul},
	"close":     {app.OpPulseOn, app.TCCClose},
	"trip":      {app.OpPulseOn, app.TCCTrip},
	"nul":       {app.OpNul, app.TCCNul},
}

// CROBOps lists the values of --op.
func CROBOps() []string {
	return []string{"latch-on", "latch-off", "pulse-on", "pulse-off", "close", "trip", "nul"}
}

func operateMode(s string) (client.CommandMode, string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", ModeSBO, "select-before-operate":
		return client.SelectBeforeOperate, ModeSBO, nil
	case ModeDirect, "direct-operate":
		return client.DirectOperate, ModeDirect, nil
	case ModeDirectNoAck, "direct-operate-no-ack":
		return client.DirectOperateNoAck, ModeDirectNoAck, nil
	}
	return 0, "", fmt.Errorf("invalid --mode %q: use sbo, direct or direct-no-ack", s)
}

// analogVariations are the ranges of the integer variations of g41.
var analogVariations = map[uint8]struct {
	gv     app.GroupVariation
	lo, hi float64
}{
	1: {app.G41V1, math.MinInt32, math.MaxInt32},
	2: {app.G41V2, math.MinInt16, math.MaxInt16},
	3: {app.G41V3, -math.MaxFloat32, math.MaxFloat32},
	4: {app.G41V4, -math.MaxFloat64, math.MaxFloat64},
}

// operatePlan validates cfg and returns the command, the mode and the result to fill.
func operatePlan(cfg *config.OperateConfig) (client.Command, client.CommandMode, *types.OperateResult, error) {
	mode, modeName, err := operateMode(cfg.Mode)
	if err != nil {
		return client.Command{}, 0, nil, err
	}
	_, broadcast, err := checkBroadcast(cfg.Broadcast, &cfg.ConnConfig)
	if err != nil {
		return client.Command{}, 0, nil, err
	}
	if broadcast != "" && mode == client.SelectBeforeOperate {
		return client.Command{}, 0, nil, fmt.Errorf("--broadcast needs --mode direct or direct-no-ack: select-before-operate needs an answer, and nobody answers a broadcast")
	}
	res := &types.OperateResult{Mode: modeName, Index: cfg.Index, Broadcast: broadcast}
	switch strings.ToLower(strings.TrimSpace(cfg.Type)) {
	case ControlCROB, "binary", "g12", "g12v1":
		if strings.TrimSpace(cfg.Value) != "" || cfg.Variation != 0 {
			return client.Command{}, 0, nil, fmt.Errorf("--value and --variation are for --type analog; a crob takes --op")
		}
		o, ok := crobOps[strings.ToLower(strings.TrimSpace(cfg.Op))]
		if !ok {
			return client.Command{}, 0, nil, fmt.Errorf("invalid --op %q: use %s", cfg.Op, strings.Join(CROBOps(), ", "))
		}
		count := cfg.Count
		if count == 0 {
			return client.Command{}, 0, nil, fmt.Errorf("invalid --count 0: a crob is executed at least once")
		}
		cmd := client.CROB(cfg.Index, o.op, cfg.OnTime, cfg.OffTime)
		crob := cmd.Value.(app.CROB)
		crob.TripClose, crob.Count = o.tcc, count
		cmd.Value, cmd.GV = crob, app.G12V1
		res.Type, res.Object, res.Op, res.TripClose = ControlCROB, app.G12V1.String(), o.op.String(), o.tcc.String()
		res.Count, res.OnTimeMs, res.OffTimeMs = count, cfg.OnTime, cfg.OffTime
		return cmd, mode, res, nil
	case ControlAnalog, "setpoint", "set-point", "g41":
		if strings.TrimSpace(cfg.Op) != "" {
			return client.Command{}, 0, nil, fmt.Errorf("--op is for --type crob; an analog control takes --value")
		}
		raw := strings.TrimSpace(cfg.Value)
		if raw == "" {
			return client.Command{}, 0, nil, fmt.Errorf("an analog control needs --value")
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return client.Command{}, 0, nil, fmt.Errorf("invalid --value %q: use a number", cfg.Value)
		}
		cmd := client.SetPoint(cfg.Index, v)
		if cfg.Variation != 0 {
			av, ok := analogVariations[cfg.Variation]
			if !ok {
				return client.Command{}, 0, nil, fmt.Errorf("invalid --variation %d: use 1 (32-bit), 2 (16-bit), 3 (float) or 4 (double)", cfg.Variation)
			}
			if v < av.lo || v > av.hi {
				return client.Command{}, 0, nil, fmt.Errorf("--value %s is out of range for %s (%g to %g)", raw, av.gv, av.lo, av.hi)
			}
			if cfg.Variation <= 2 && v != math.Trunc(v) {
				return client.Command{}, 0, nil, fmt.Errorf("--value %s is not a whole number, which %s needs", raw, av.gv)
			}
			cmd.GV = av.gv
		}
		if cmd.GV == 0 {
			cmd.GV = app.G41V3
			if v == math.Trunc(v) && v >= math.MinInt32 && v <= math.MaxInt32 {
				cmd.GV = app.G41V1
			}
		}
		res.Type, res.Object, res.Value = ControlAnalog, cmd.GV.String(), &v
		return cmd, mode, res, nil
	}
	return client.Command{}, 0, nil, fmt.Errorf("invalid --type %q: use crob or analog", cfg.Type)
}

// CheckOperate validates the configuration of `client operate`.
func CheckOperate(cfg *config.OperateConfig) error {
	if err := Validate(&cfg.ConnConfig); err != nil {
		return err
	}
	_, _, _, err := operatePlan(cfg)
	return err
}

// Operate executes one control. It is sent once and never repeated.
func Operate(ctx context.Context, cfg *config.OperateConfig, trace io.Writer) (*types.OperateResult, error) {
	cmd, mode, res, err := operatePlan(cfg)
	if err != nil {
		return nil, err
	}
	s, err := Dial(ctx, &cfg.ConnConfig, trace)
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.Close() }()
	var statuses []app.CommandStatus
	took, err := s.request(ctx, func(ctx context.Context) error {
		if res.Broadcast == "" {
			var rerr error
			statuses, rerr = s.Operate(ctx, mode, cmd)
			return rerr
		}
		addr := broadcastAddresses[res.Broadcast]
		fc := app.FuncDirectOperate
		if mode == client.DirectOperateNoAck {
			fc = app.FuncDirectOperateNoAck
		}
		return s.Broadcast(ctx, addr, fc, app.Indexed(cmd.GV, app.Object{Index: cmd.Index, Value: cmd.Value}))
	})
	if err != nil {
		return nil, WrapError(err)
	}
	res.Target, res.Outstation, res.RTTMs = s.Target.String(), s.Outstation, ms(took)
	res.Status = "not-reported"
	if mode != client.DirectOperateNoAck && len(statuses) > 0 {
		res.Status = statuses[0].String()
	}
	return res, nil
}
