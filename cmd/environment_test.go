// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"
	"testing"

	"github.com/otfabric/dnp3ctl/internal/stationtest"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// TestBroadcastAndOutstation: --broadcast and --outstation both on the command line are
// refused before anything is sent; --outstation on the command line wins over
// DNP3CTL_BROADCAST; the control parameters taken from the environment are named on stderr.
func TestBroadcastAndOutstation(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)
	crob := []string{"operate", "--type", "crob", "--index", "1", "--op", "latch-on", "--mode", "direct"}

	t.Run("both on the command line", func(t *testing.T) {
		for _, args := range [][]string{
			append(append([]string{}, crob...), "--broadcast", "no-confirm", "--outstation", "10"),
			{"freeze", "--broadcast", "no-confirm", "--outstation", "10", "--no-ack"},
		} {
			r := cli(t, st, append(args, "--format", "json")...).want(t, 3)
			if kind, code := r.errorCode(t); kind != "invalid_input" || code != "invalid_flag_combination" {
				t.Errorf("%v: %s/%s", args, kind, code)
			}
			cli(t, st, args...).want(t, 3).errHas(t, "--broadcast and --outstation cannot be combined")
		}
		// Refused before connecting: the outstation saw nothing.
		if st.HasEvent("connected") || st.HasEvent("request") {
			t.Errorf("the outstation saw a request: %+v", st.Events())
		}
	})

	t.Run("outstation on the command line wins", func(t *testing.T) {
		t.Setenv("DNP3CTL_BROADCAST", "no-confirm")
		var res types.OperateResult
		r := cli(t, st, append(append([]string{}, crob...), "--outstation", "10", "--format", "json")...).want(t, 0)
		r.decode(t, &res)
		if res.Broadcast != "" || res.Outstation != 10 || res.Status != "SUCCESS" {
			t.Errorf("operate: %+v, want SUCCESS from outstation 10", res)
		}
		r.errHas(t, "notice: DNP3CTL_BROADCAST=no-confirm is not used: --outstation 10 is on the command line")
		var fr types.ConfirmedResult
		cli(t, st, "freeze", "--outstation", "10", "--format", "json").want(t, 0).
			errHas(t, "DNP3CTL_BROADCAST=no-confirm is not used").decode(t, &fr)
		if fr.Broadcast != "" || fr.Request != "freeze" {
			t.Errorf("freeze: %+v", fr)
		}
	})

	t.Run("broadcast from the environment without --outstation", func(t *testing.T) {
		t.Setenv("DNP3CTL_BROADCAST", "no-confirm")
		var res types.OperateResult
		r := cli(t, st, append(append([]string{}, crob...), "--format", "json")...).want(t, 0)
		r.decode(t, &res)
		if res.Broadcast != "no-confirm" || res.Status != "not-reported" {
			t.Errorf("operate: %+v, want a broadcast", res)
		}
		r.errHas(t, "notice: --broadcast no-confirm is taken from DNP3CTL_BROADCAST")
		// DNP3CTL_OUTSTATION is not the command line: the broadcast stays.
		t.Setenv("DNP3CTL_OUTSTATION", "10")
		r = cli(t, st, append(append([]string{}, crob...), "--format", "json")...).want(t, 0)
		r.decode(t, &res)
		if res.Broadcast != "no-confirm" {
			t.Errorf("with DNP3CTL_OUTSTATION: %+v", res)
		}
	})

	t.Run("control parameters from the environment", func(t *testing.T) {
		t.Setenv("DNP3CTL_OP", "latch-off")
		t.Setenv("DNP3CTL_OPERATE_MODE", "direct")
		r := cli(t, st, "operate", "--type", "crob", "--index", "1").want(t, 0)
		r.errHas(t, "notice: --op latch-off is taken from DNP3CTL_OP", "notice: --mode direct is taken from DNP3CTL_OPERATE_MODE")
		if strings.Contains(r.stderr, "DNP3CTL_BROADCAST") || strings.Contains(r.stderr, "DNP3CTL_VALUE") {
			t.Errorf("a notice for a parameter not set: %s", r.stderr)
		}
		t.Setenv("DNP3CTL_OP", "")
		t.Setenv("DNP3CTL_VALUE", "50")
		cli(t, st, "operate", "--type", "analog", "--index", "0").want(t, 0).errHas(t, "notice: --value 50 is taken from DNP3CTL_VALUE")
		// A flag on the command line is not named.
		r = cli(t, st, "operate", "--type", "analog", "--index", "0", "--value", "40", "--mode", "sbo")
		r.want(t, 0)
		if strings.Contains(r.stderr, "notice:") {
			t.Errorf("a notice for a flag on the command line: %s", r.stderr)
		}
	})
}
