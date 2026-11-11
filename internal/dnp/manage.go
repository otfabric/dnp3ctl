// SPDX-License-Identifier: MIT

package dnp

import (
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/otfabric/go-dnp3/app"

	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/errs"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// managementFunctions are the management functions of IEEE 1815-2012 clause 4.4.8, by the
// names dnp3-interop's contract uses.
var managementFunctions = map[string]app.FunctionCode{
	"initialize-data":        app.FuncInitializeData,
	"initialize-application": app.FuncInitializeApplication,
	"start-application":      app.FuncStartApplication,
	"stop-application":       app.FuncStopApplication,
	"save-configuration":     app.FuncSaveConfiguration,
	"activate-config":        app.FuncActivateConfig,
}

// ManagementFunctions describes the values of --function of `client manage`.
func ManagementFunctions() map[string]string {
	return map[string]string{
		"initialize-data":        "INITIALIZE_DATA (15): set the outstation's data to its initial values",
		"initialize-application": "INITIALIZE_APPL (16): prepare the applications to start",
		"start-application":      "START_APPL (17): start the applications",
		"stop-application":       "STOP_APPL (18): stop the applications",
		"save-configuration":     "SAVE_CONFIG (19): save the configuration",
		"activate-config":        "ACTIVATE_CONFIG (31): activate the configurations of --config-name and --config-string",
	}
}

// ManagementFunction returns the function code of a name of ManagementFunctions.
func ManagementFunction(name string) (app.FunctionCode, error) {
	fc, ok := managementFunctions[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return 0, fmt.Errorf("invalid --function %q: use initialize-data, initialize-application, start-application, stop-application, save-configuration or activate-config", name)
	}
	return fc, nil
}

// managementHeaders returns the objects of a management function: the application
// functions name every application (g90v1, qualifier 0x06), the others carry none.
func managementHeaders(fc app.FunctionCode) []app.Header {
	switch fc {
	case app.FuncInitializeApplication, app.FuncStartApplication, app.FuncStopApplication:
		return []app.Header{app.AllObjects(app.GV(90, 1))}
	default:
		return nil
	}
}

// activateHeaders returns the objects of ACTIVATE_CONFIG (IEEE 1815-2012 clause 4.4.20):
// a file specification (g70v8) for every name of --config-name and an octet string (g110)
// for every value of --config-string.
func activateHeaders(names, octets []string) ([]app.Header, error) {
	var headers []app.Header
	var specs []app.Value
	for _, n := range names {
		if n = strings.TrimSpace(n); n == "" {
			continue
		}
		if len(n) > math.MaxUint16 {
			return nil, fmt.Errorf("--config-name of %d octets: the maximum is %d", len(n), math.MaxUint16)
		}
		specs = append(specs, app.FileSpecification{Name: n})
	}
	if len(specs) > 0 {
		headers = append(headers, app.FreeFormat(app.G70V8, specs...))
	}
	if len(octets) > math.MaxUint16 {
		return nil, fmt.Errorf("%d --config-string values: the maximum is %d", len(octets), math.MaxUint16)
	}
	var index uint32
	for _, o := range octets {
		n := len(o)
		if n < 1 || n > math.MaxUint8 {
			return nil, fmt.Errorf("--config-string %q: an octet string has 1-255 octets", o)
		}
		headers = append(headers, app.Ranged(app.GV(110, uint8(n)), index, app.OctetString{Data: []byte(o)}))
		index++
	}
	return headers, nil
}

// managePlan validates cfg and returns its function and, for activate-config, its objects.
func managePlan(cfg *config.ManageConfig) (app.FunctionCode, []app.Header, error) {
	fc, err := ManagementFunction(cfg.Function)
	if err != nil {
		return 0, nil, err
	}
	if fc != app.FuncActivateConfig {
		if len(cfg.ConfigNames) > 0 || len(cfg.ConfigStrings) > 0 {
			return 0, nil, errs.InvalidInput(errs.CodeInvalidFlagCombination, "--config-name and --config-string apply to --function activate-config", nil)
		}
		return fc, managementHeaders(fc), nil
	}
	headers, err := activateHeaders(cfg.ConfigNames, cfg.ConfigStrings)
	switch {
	case err != nil:
		return 0, nil, err
	case len(headers) == 0:
		return 0, nil, errs.InvalidInput(errs.CodeInvalidInput, "activate-config needs a configuration: set --config-name or --config-string", nil)
	}
	return fc, headers, nil
}

// CheckManage validates the configuration of `client manage`.
func CheckManage(cfg *config.ManageConfig) error {
	if err := Validate(&cfg.ConnConfig); err != nil {
		return err
	}
	_, _, err := managePlan(cfg)
	return err
}

// Manage sends a management function.
func Manage(ctx context.Context, cfg *config.ManageConfig, trace io.Writer) (*types.ConfirmedResult, error) {
	fc, headers, err := managePlan(cfg)
	if err != nil {
		return nil, err
	}
	res := &types.ConfirmedResult{Request: strings.ToLower(strings.TrimSpace(cfg.Function)), Function: fc.String()}
	return confirmed(ctx, &cfg.ConnConfig, trace, res, func(ctx context.Context, s *Session) error {
		if fc == app.FuncActivateConfig {
			sts, err := s.ActivateConfig(ctx, headers...)
			res.Activation = activationResults(sts)
			return err
		}
		_, err := s.Request(ctx, fc, headers...)
		return err
	})
}

// activationResults converts the g91v1 objects of an ACTIVATE_CONFIG response.
func activationResults(sts []app.ActivateConfigStatus) []types.ActivationStatus {
	out := make([]types.ActivationStatus, 0, len(sts))
	for _, st := range sts {
		a := types.ActivationStatus{DelayMs: ms(st.Delay), Results: make([]types.ActivationResult, 0, len(st.Results))}
		for _, r := range st.Results {
			a.Results = append(a.Results, types.ActivationResult{Code: int(r.Code), Text: r.Text})
		}
		out = append(out, a)
	}
	return out
}

// CheckClearRestart validates the configuration of `client clear-restart`.
func CheckClearRestart(cfg *config.ClearRestartConfig) error {
	return Validate(&cfg.ConnConfig)
}

// ClearRestart clears the outstation's DEVICE_RESTART indication (IIN1.7).
func ClearRestart(ctx context.Context, cfg *config.ClearRestartConfig, trace io.Writer) (*types.ConfirmedResult, error) {
	res := &types.ConfirmedResult{Request: "clear-restart"}
	return confirmed(ctx, &cfg.ConnConfig, trace, res, func(ctx context.Context, s *Session) error {
		return s.ClearRestart(ctx)
	})
}

// deadbandVariations are the variations of g34 with their largest value.
var deadbandVariations = map[uint8]struct {
	gv app.GroupVariation
	hi float64
}{
	1: {app.G34V1, math.MaxUint16},
	2: {app.G34V2, math.MaxUint32},
	3: {app.G34V3, math.MaxFloat32},
}

func deadbandPlan(cfg *config.WriteDeadbandConfig) ([]uint32, float64, app.GroupVariation, error) {
	indexes, err := parseIndexList(cfg.Indexes, "--index")
	if err != nil {
		return nil, 0, 0, err
	}
	raw := strings.TrimSpace(cfg.Value)
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return nil, 0, 0, fmt.Errorf("invalid --value %q: a deadband is a number of 0 or more", cfg.Value)
	}
	variation := cfg.Variation
	if variation == 0 {
		variation = 3
		if v == math.Trunc(v) && v <= math.MaxUint32 {
			variation = 2
		}
	}
	dv, ok := deadbandVariations[variation]
	switch {
	case !ok:
		return nil, 0, 0, fmt.Errorf("invalid --variation %d: use 1 (16-bit), 2 (32-bit) or 3 (float)", cfg.Variation)
	case v > dv.hi:
		return nil, 0, 0, fmt.Errorf("--value %s is out of range for %s (0 to %g)", raw, dv.gv, dv.hi)
	case variation <= 2 && v != math.Trunc(v):
		return nil, 0, 0, fmt.Errorf("--value %s is not a whole number, which %s needs", raw, dv.gv)
	}
	return indexes, v, dv.gv, nil
}

// CheckWriteDeadband validates the configuration of `client write-deadband`.
func CheckWriteDeadband(cfg *config.WriteDeadbandConfig) error {
	if err := Validate(&cfg.ConnConfig); err != nil {
		return err
	}
	_, _, _, err := deadbandPlan(cfg)
	return err
}

// WriteDeadband writes the deadband of analog inputs (WRITE of g34).
func WriteDeadband(ctx context.Context, cfg *config.WriteDeadbandConfig, trace io.Writer) (*types.ConfirmedResult, error) {
	indexes, v, gv, err := deadbandPlan(cfg)
	if err != nil {
		return nil, err
	}
	objs := make([]app.Object, 0, len(indexes))
	names := make([]string, 0, len(indexes))
	for _, i := range indexes {
		objs = append(objs, app.Object{Index: i, Value: app.AnalogDeadband{Value: v}})
		names = append(names, strconv.FormatUint(uint64(i), 10))
	}
	res := &types.ConfirmedResult{Request: "write-deadband", Objects: []string{gv.String()}, Range: strings.Join(names, ","), Value: &v}
	return confirmed(ctx, &cfg.ConnConfig, trace, res, func(ctx context.Context, s *Session) error {
		_, err := s.Write(ctx, app.Indexed(gv, objs...))
		return err
	})
}
