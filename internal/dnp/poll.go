package dnp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	dnp3 "github.com/otfabric/go-dnp3"
	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/client"

	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// Poll modes of `client poll`.
const (
	PollIntegrity = "integrity"
	PollEvent     = "event"
	PollClass     = "class"
)

// PollModes describes the values of --mode of `client poll`.
func PollModes() map[string]string {
	return map[string]string{
		PollIntegrity: "events of classes 1-3, then all static data (class 0)",
		PollEvent:     "the events of --class",
		PollClass:     "exactly the classes of --class, 0 included",
	}
}

var allEventClasses = []int{1, 2, 3}

// pollPlan returns the request name and the classes of the poll of cfg.
func pollPlan(cfg *config.PollConfig) (request string, classes []int, err error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Mode)) {
	case "", PollIntegrity:
		if strings.TrimSpace(cfg.Classes) != "" {
			return "", nil, fmt.Errorf("--class does not apply to an integrity poll: use --mode event or --mode class")
		}
		return "integrity-poll", []int{1, 2, 3, 0}, nil
	case PollEvent:
		classes, err := ParseClasses(cfg.Classes, 1, allEventClasses)
		return "event-poll", classes, err
	case PollClass:
		classes, err := ParseClasses(cfg.Classes, 0, []int{1, 2, 3, 0})
		return "class-poll", classes, err
	}
	return "", nil, fmt.Errorf("invalid --mode %q: use integrity, event or class", cfg.Mode)
}

// CheckPoll validates the configuration of `client poll`.
func CheckPoll(cfg *config.PollConfig) error {
	if err := Validate(&cfg.ConnConfig); err != nil {
		return err
	}
	if _, _, err := pollPlan(cfg); err != nil {
		return err
	}
	if _, err := ParseObjectFilter(cfg.Objects); err != nil {
		return err
	}
	_, err := ParseIndexFilter(cfg.Indexes)
	return err
}

// Poll runs an integrity, event or class poll.
func Poll(ctx context.Context, cfg *config.PollConfig, trace io.Writer) (*types.PollResult, error) {
	request, classes, err := pollPlan(cfg)
	if err != nil {
		return nil, err
	}
	objects, err := ParseObjectFilter(cfg.Objects)
	if err != nil {
		return nil, err
	}
	indexes, err := ParseIndexFilter(cfg.Indexes)
	if err != nil {
		return nil, err
	}
	s, err := Dial(ctx, &cfg.ConnConfig, trace)
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.Close() }()
	var res *client.Result
	took, err := s.request(ctx, func(ctx context.Context) error {
		var rerr error
		switch request {
		case "integrity-poll":
			res, rerr = s.IntegrityPoll(ctx)
		case "event-poll":
			res, rerr = s.EventPoll(ctx, classes...)
		default:
			res, rerr = s.Read(ctx, app.ClassHeaders(classes...)...)
		}
		return rerr
	})
	out := &types.PollResult{Target: s.Target.String(), Outstation: s.Outstation, Request: request, Classes: classes}
	return finishPoll(out, res, took, err, objects, indexes)
}

// finishPoll fills the result of a poll or read from the response. An IINError with
// values is a partial result; without values, or any other error, it is fatal.
func finishPoll(out *types.PollResult, res *client.Result, took time.Duration, err error, objects ObjectFilter, indexes IndexFilter) (*types.PollResult, error) {
	var iinErr *dnp3.IINError
	if err != nil && (res == nil || !errors.As(err, &iinErr) || len(measurementsOf(res)) == 0) {
		return nil, WrapError(err)
	}
	out.DurationMs = ms(took)
	out.IIN = res.IIN.String()
	out.Measurements = []types.Measurement{}
	for _, m := range measurementsOf(res) {
		out.Received++
		if !objects.Match(m.GV) || !indexes.Match(m.Index) {
			continue
		}
		out.Measurements = append(out.Measurements, ToMeasurement(m))
	}
	out.Count = len(out.Measurements)
	out.Error = EmbeddedErrorInfo(err)
	return out, nil
}

// measurementsOf returns the values of a response in the order of its headers: the
// measurements of go-dnp3, and the times and intervals (g50v4), which it leaves out with
// the other time objects.
func measurementsOf(res *client.Result) []client.Measurement {
	var out []client.Measurement
	for _, h := range res.Headers() {
		if h.GV != app.G50V4 {
			out = append(out, client.Measurements(&app.Fragment{Headers: []app.Header{h}})...)
			continue
		}
		for _, o := range h.Objects {
			out = append(out, client.Measurement{GV: h.GV, Index: o.Index, Value: o.Value})
		}
	}
	return out
}

// CheckRead validates the configuration of `client read`.
func CheckRead(cfg *config.ReadConfig) error {
	if err := Validate(&cfg.ConnConfig); err != nil {
		return err
	}
	_, _, err := readPlan(cfg)
	return err
}

func readPlan(cfg *config.ReadConfig) ([]app.GroupVariation, Selection, error) {
	gvs, err := ParseObjects(cfg.Objects)
	if err != nil {
		return nil, Selection{}, err
	}
	if len(gvs) == 0 {
		return nil, Selection{}, fmt.Errorf("no object given: set --object (e.g. --object g30v5)")
	}
	sel, err := ParseSelection(cfg.Range, cfg.Count)
	return gvs, sel, err
}

// Read reads objects by group and variation.
func Read(ctx context.Context, cfg *config.ReadConfig, trace io.Writer) (*types.PollResult, error) {
	gvs, sel, err := readPlan(cfg)
	if err != nil {
		return nil, err
	}
	s, err := Dial(ctx, &cfg.ConnConfig, trace)
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.Close() }()
	var res *client.Result
	took, err := s.request(ctx, func(ctx context.Context) error {
		var rerr error
		res, rerr = s.Read(ctx, sel.Headers(gvs)...)
		return rerr
	})
	out := &types.PollResult{Target: s.Target.String(), Outstation: s.Outstation, Request: "read", Objects: ObjectNames(gvs), Range: sel.String()}
	return finishPoll(out, res, took, err, ObjectFilter{}, IndexFilter{})
}
