// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/client"
	"github.com/otfabric/go-dnp3/link"

	"github.com/otfabric/dnp3ctl/internal/stationtest"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// floatModel reports analog inputs 0 and 1 in single precision, static and as events.
var floatModel = strings.NewReplacer(
	"{type: analog-input, index: 0, value: 230.5, class: 2, static_variation: 5, deadband: 0.5}",
	"{type: analog-input, index: 0, value: 230.5, class: 2, static_variation: 5, event_variation: 5, deadband: 0.5}",
	"{type: analog-input, index: 1, value: 42, class: 2, flags: [ONLINE, OVER_RANGE]}",
	"{type: analog-input, index: 1, value: 42, class: 2, static_variation: 5, event_variation: 5, flags: [ONLINE]}",
).Replace(stationtest.Model)

// nonFinite returns the non_finite of the measurements of index in the JSON measurements.
func nonFinite(ms []types.Measurement, object string, index uint32) (string, bool) {
	for _, m := range ms {
		if m.Object == object && m.Index == index {
			return m.NonFinite, m.Value == nil
		}
	}
	return "<missing>", false
}

// TestNonFiniteValues: NaN and the infinities an outstation reports are written as a null
// value with non_finite in JSON, as NaN, +Inf and -Inf in text, and the commands go on.
func TestNonFiniteValues(t *testing.T) {
	if floatModel == stationtest.Model {
		t.Fatal("the test model has changed: floatModel does not apply")
	}
	st := stationtest.Start(t, floatModel)
	db := st.Server().Database()
	for i, v := range []float64{math.NaN(), math.Inf(1)} {
		if err := db.UpdateAnalogInput(uint32(i), app.Analog{Value: v, Flags: app.FlagOnline}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.UpdateAnalogOutput(0, app.Analog{Value: math.Inf(-1), Flags: app.FlagOnline}); err != nil {
		t.Fatal(err)
	}

	var res types.PollResult
	cli(t, st, "read", "--object", "g30v5,g40v3", "--format", "json").want(t, 0).decode(t, &res)
	for _, c := range []struct {
		object string
		index  uint32
		want   string
	}{{"g30v5", 0, "NaN"}, {"g30v5", 1, "+Inf"}, {"g40v3", 0, "-Inf"}} {
		if nf, null := nonFinite(res.Measurements, c.object, c.index); nf != c.want || !null {
			t.Errorf("read %s %d: non_finite %q, value null %t, want %s", c.object, c.index, nf, null, c.want)
		}
	}
	r := cli(t, st, "read", "--object", "g30v5,g40v3", "--format", "json").want(t, 0)
	if !strings.Contains(r.stdout, `"value": null,`) || !strings.Contains(r.stdout, `"non_finite": "NaN"`) {
		t.Errorf("the JSON form:\n%s", r.stdout)
	}
	cli(t, st, "read", "--object", "g30v5,g40v3").want(t, 0).has(t, "NaN", "+Inf", "-Inf")
	cli(t, st, "read", "--object", "g30v5,g40v3", "--format", "table").want(t, 0).has(t, "NaN", "+Inf", "-Inf")

	res = types.PollResult{}
	cli(t, st, "poll", "--format", "json").want(t, 0).decode(t, &res)
	for _, c := range []struct {
		object string
		index  uint32
		want   string
	}{{"g30v5", 0, "NaN"}, {"g30v5", 1, "+Inf"}} {
		if nf, null := nonFinite(res.Measurements, c.object, c.index); nf != c.want || !null {
			t.Errorf("poll %s %d: non_finite %q, value null %t, want %s", c.object, c.index, nf, null, c.want)
		}
	}

	t.Run("monitor", func(t *testing.T) {
		other := stationtest.Start(t, floatModel)
		enables := func() int {
			n := 0
			for _, e := range other.Events() {
				if e.Function == "ENABLE_UNSOLICITED" {
					n++
				}
			}
			return n
		}
		done := make(chan result, 1)
		go func() { done <- cli(t, other, "monitor", "--object", "g32", "--count", "4", "--format", "json") }()
		eventually(t, "unsolicited responses to be enabled", func() bool { return enables() == 1 })
		db := other.Server().Database()
		for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 7.5} {
			_ = db.UpdateAnalogInput(0, app.Analog{Value: v, Flags: app.FlagOnline})
		}
		lines := (<-done).want(t, 0).lines(t)
		want := []string{"NaN", "+Inf", "-Inf", ""}
		if len(lines) != len(want) {
			t.Fatalf("monitor: %v", lines)
		}
		for i, w := range want {
			nf, _ := lines[i]["non_finite"].(string)
			value, hasValue := lines[i]["value"]
			if nf != w || !hasValue || (w != "" && value != nil) || (w == "" && value != 7.5) {
				t.Errorf("monitor line %d: %v, want non_finite %q", i+1, lines[i], w)
			}
		}
	})
}

// TestNaNSetPointRefused: the simulated outstation refuses a NaN set-point of a control with
// min and max (OUT_OF_RANGE) as it refuses one past them, and its JSON event log has the
// control.
func TestNaNSetPointRefused(t *testing.T) {
	model := filepath.Join(t.TempDir(), "model.yaml")
	if err := os.WriteFile(model, []byte(stationtest.Model), 0o600); err != nil {
		t.Fatal(err)
	}
	port := stationtest.FreePort(t)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	done := make(chan result, 1)
	go func() {
		done <- runCtx(t, ctx, "server", "start", "--config", model, "--listen", "127.0.0.1", "--port", strconv.Itoa(int(port)), "--format", "json")
	}()
	addr := "127.0.0.1:" + strconv.Itoa(int(port))
	var c *client.Client
	eventually(t, "the server to answer", func() bool {
		cc, err := client.New(addr, client.WithAddresses(1, link.Address(10)), client.WithResponseTimeout(2*time.Second),
			client.WithStartup(client.Startup{}), client.WithTimeSync(client.TimeSyncNone))
		if err != nil {
			t.Fatal(err)
		}
		cctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if err := cc.Connect(cctx); err != nil {
			_ = cc.Close()
			return false
		}
		c = cc
		return true
	})
	for _, v := range []float64{math.NaN(), 1000, 50} {
		sts, err := c.Operate(ctx, client.DirectOperate, client.Command{Index: 0, GV: app.G41V3, Value: app.AnalogOutput{Value: v}})
		want := app.StatusOutOfRange
		if v == 50 {
			want = app.StatusSuccess
		}
		// A status other than SUCCESS comes with an error that names it.
		if len(sts) != 1 || sts[0] != want || (err != nil) != (want != app.StatusSuccess) {
			t.Errorf("set-point %v: %v, %v, want %s", v, sts, err, want)
		}
	}
	_ = c.Close()
	stop()
	r := (<-done).want(t, 0)
	var controls []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(r.stdout), "\n") {
		var e map[string]any
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("a line of the log is not JSON: %v\n%s", err, line)
		}
		if e["event"] == "control" {
			controls = append(controls, e)
		}
	}
	if len(controls) != 3 {
		t.Fatalf("controls in the log: %v", controls)
	}
	if _, has := controls[0]["value"]; has || controls[0]["non_finite"] != "NaN" || controls[0]["status"] != "OUT_OF_RANGE" {
		t.Errorf("the NaN set-point in the log: %v", controls[0])
	}
	if controls[1]["value"] != float64(1000) || controls[1]["status"] != "OUT_OF_RANGE" || controls[2]["status"] != "SUCCESS" {
		t.Errorf("the other set-points: %v", controls[1:])
	}
}
