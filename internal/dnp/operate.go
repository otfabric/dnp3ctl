// SPDX-License-Identifier: MIT

package dnp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	dnp3 "github.com/otfabric/go-dnp3"
	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/client"

	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/errs"
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

// parseIndexList parses a comma-separated list of point indexes, each given once.
func parseIndexList(s, flag string) ([]uint32, error) {
	var out []uint32
	seen := map[uint32]bool{}
	for _, part := range strings.Split(s, ",") {
		n, err := strconv.ParseUint(strings.TrimSpace(part), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid %s %q: use an index, or several comma-separated (0,1,4)", flag, s)
		}
		if seen[uint32(n)] {
			return nil, fmt.Errorf("invalid %s %q: index %d is given twice", flag, s, n)
		}
		seen[uint32(n)] = true
		out = append(out, uint32(n))
	}
	return out, nil
}

// parseValues parses --value: one number for every index, or one per index.
func parseValues(raw string, n int) ([]float64, error) {
	parts := strings.Split(raw, ",")
	if len(parts) != 1 && len(parts) != n {
		return nil, fmt.Errorf("--value has %d values for %d indexes: give one for all, or one per index", len(parts), n)
	}
	out := make([]float64, 0, n)
	for _, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("invalid --value %q: use a number", strings.TrimSpace(p))
		}
		out = append(out, v)
	}
	for len(out) < n {
		out = append(out, out[0])
	}
	return out, nil
}

// analogVariation returns the object of a set-point: the one of --variation, checked for
// every value, or g41v1 when every value is a whole number that fits it, g41v3 otherwise.
func analogVariation(variation uint8, values []float64) (app.GroupVariation, error) {
	if variation == 0 {
		for _, v := range values {
			if v != math.Trunc(v) || v < math.MinInt32 || v > math.MaxInt32 {
				return app.G41V3, nil
			}
		}
		return app.G41V1, nil
	}
	av, ok := analogVariations[variation]
	if !ok {
		return 0, fmt.Errorf("invalid --variation %d: use 1 (32-bit), 2 (16-bit), 3 (float) or 4 (double)", variation)
	}
	for _, v := range values {
		text := strconv.FormatFloat(v, 'g', -1, 64)
		if v < av.lo || v > av.hi {
			return 0, fmt.Errorf("--value %s is out of range for %s (%g to %g)", text, av.gv, av.lo, av.hi)
		}
		if variation <= 2 && v != math.Trunc(v) {
			return 0, fmt.Errorf("--value %s is not a whole number, which %s needs", text, av.gv)
		}
	}
	return av.gv, nil
}

// operatePlan validates cfg and returns the controls, the mode and the result to fill.
func operatePlan(cfg *config.OperateConfig) ([]client.Command, client.CommandMode, *types.OperateResult, error) {
	mode, modeName, err := operateMode(cfg.Mode)
	if err != nil {
		return nil, 0, nil, err
	}
	_, broadcast, err := checkBroadcast(cfg.Broadcast, &cfg.ConnConfig)
	if err != nil {
		return nil, 0, nil, err
	}
	if broadcast != "" && mode == client.SelectBeforeOperate {
		return nil, 0, nil, fmt.Errorf("--broadcast needs --mode direct or direct-no-ack: select-before-operate needs an answer, and nobody answers a broadcast")
	}
	indexes, err := parseIndexList(cfg.Index, "--index")
	if err != nil {
		return nil, 0, nil, err
	}
	res := &types.OperateResult{Mode: modeName, Index: indexes[0], Broadcast: broadcast}
	cmds := make([]client.Command, 0, len(indexes))
	switch strings.ToLower(strings.TrimSpace(cfg.Type)) {
	case ControlCROB, "binary", "g12", "g12v1":
		if strings.TrimSpace(cfg.Value) != "" || cfg.Variation != 0 {
			return nil, 0, nil, fmt.Errorf("--value and --variation are for --type analog; a crob takes --op")
		}
		o, ok := crobOps[strings.ToLower(strings.TrimSpace(cfg.Op))]
		if !ok {
			return nil, 0, nil, fmt.Errorf("invalid --op %q: use %s", cfg.Op, strings.Join(CROBOps(), ", "))
		}
		if cfg.Count == 0 {
			return nil, 0, nil, fmt.Errorf("invalid --count 0: a crob is executed at least once")
		}
		for _, i := range indexes {
			cmd := client.CROB(i, o.op, cfg.OnTime, cfg.OffTime)
			crob := cmd.Value.(app.CROB)
			crob.TripClose, crob.Count = o.tcc, cfg.Count
			cmd.Value, cmd.GV = crob, app.G12V1
			cmds = append(cmds, cmd)
			res.Controls = append(res.Controls, types.ControlStatus{Index: i})
		}
		res.Type, res.Object, res.Op, res.TripClose = ControlCROB, app.G12V1.String(), o.op.String(), o.tcc.String()
		res.Count, res.OnTimeMs, res.OffTimeMs = cfg.Count, cfg.OnTime, cfg.OffTime
		return cmds, mode, res, nil
	case ControlAnalog, "setpoint", "set-point", "g41":
		if strings.TrimSpace(cfg.Op) != "" {
			return nil, 0, nil, fmt.Errorf("--op is for --type crob; an analog control takes --value")
		}
		if strings.TrimSpace(cfg.Value) == "" {
			return nil, 0, nil, fmt.Errorf("an analog control needs --value")
		}
		values, err := parseValues(cfg.Value, len(indexes))
		if err != nil {
			return nil, 0, nil, err
		}
		gv, err := analogVariation(cfg.Variation, values)
		if err != nil {
			return nil, 0, nil, err
		}
		for k, i := range indexes {
			cmd := client.SetPoint(i, values[k])
			cmd.GV = gv
			cmds = append(cmds, cmd)
			v := values[k]
			res.Controls = append(res.Controls, types.ControlStatus{Index: i, Value: &v})
		}
		res.Type, res.Object, res.Value = ControlAnalog, gv.String(), res.Controls[0].Value
		return cmds, mode, res, nil
	}
	return nil, 0, nil, fmt.Errorf("invalid --type %q: use crob or analog", cfg.Type)
}

// CheckOperate validates the configuration of `client operate`.
func CheckOperate(cfg *config.OperateConfig) error {
	if err := Validate(&cfg.ConnConfig); err != nil {
		return err
	}
	_, _, _, err := operatePlan(cfg)
	return err
}

// Operate executes controls, all in one request. They are sent once and never repeated.
// When a direct operate of several controls is refused for some of them, the result lists
// every status with the error: the others were executed.
func Operate(ctx context.Context, cfg *config.OperateConfig, trace io.Writer) (*types.OperateResult, error) {
	cmds, mode, res, err := operatePlan(cfg)
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
			statuses, rerr = s.Operate(ctx, mode, cmds...)
			return rerr
		}
		fc := app.FuncDirectOperate
		if mode == client.DirectOperateNoAck {
			fc = app.FuncDirectOperateNoAck
		}
		objs := make([]app.Object, 0, len(cmds))
		for _, c := range cmds {
			objs = append(objs, app.Object{Index: c.Index, Value: c.Value})
		}
		return s.Broadcast(ctx, broadcastAddresses[res.Broadcast], fc, app.Indexed(cmds[0].GV, objs...))
	})
	res.Target, res.Outstation, res.RTTMs = s.Target.String(), s.Outstation, ms(took)
	res.Status = "not-reported"
	if mode != client.DirectOperateNoAck && res.Broadcast == "" && len(statuses) == len(cmds) {
		res.Status = app.StatusSuccess.String()
		for k, st := range statuses {
			res.Controls[k].Status = st.String()
			if st != app.StatusSuccess && res.Status == app.StatusSuccess.String() {
				res.Status = st.String()
			}
		}
	} else {
		for k := range res.Controls {
			res.Controls[k].Status = res.Status
		}
	}
	if err == nil {
		return res, nil
	}
	var cmdErr *dnp3.CommandError
	if !errors.As(err, &cmdErr) || len(cmds) == 1 || len(statuses) != len(cmds) {
		return nil, WrapError(err)
	}
	executed := 0
	for _, st := range statuses {
		if st == app.StatusSuccess {
			executed++
		}
	}
	if mode != client.DirectOperate || executed == 0 {
		// With select-before-operate a refused select ends the operation, and the statuses
		// do not tell at which step it was refused: nothing is reported as executed.
		e, _ := errs.As(WrapError(err))
		return nil, errs.New(e.Kind, e.Code, e.Message+"; statuses: "+controlStatuses(res.Controls), err)
	}
	res.Error = EmbeddedErrorInfo(err)
	return res, nil
}

// controlStatuses renders the statuses of several controls for an error message.
func controlStatuses(cs []types.ControlStatus) string {
	parts := make([]string, 0, len(cs))
	for _, c := range cs {
		parts = append(parts, fmt.Sprintf("index %d %s", c.Index, c.Status))
	}
	return strings.Join(parts, ", ")
}
