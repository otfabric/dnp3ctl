package cmd

import (
	"bytes"
	"context"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/otfabric/go-dnp3/secauth"

	"github.com/otfabric/dnp3ctl/internal/station"
	"github.com/otfabric/dnp3ctl/internal/stationtest"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// featureModel is the test model with command events on two outputs and two times and
// intervals.
var featureModel = strings.NewReplacer(
	"  - {type: binary-output, index: 1, value: false}\n",
	"  - {type: binary-output, index: 1, value: false, command_event_class: 2, command_event_variation: 1}\n",
	"  - {type: analog-output, index: 0, value: 11, static_variation: 3}\n",
	"  - {type: analog-output, index: 0, value: 11, static_variation: 3, command_event_class: 2}\n"+
		"  - {type: time-and-interval, index: 0, value: \"2026-01-04T02:00:00Z\", interval: 1, interval_unit: weeks}\n"+
		"  - {type: time-and-interval, index: 1, value: 2026-06-01T00:00:00Z}\n",
).Replace(stationtest.Model)

func TestTimeAndInterval(t *testing.T) {
	st := stationtest.Start(t, featureModel)

	cli(t, st, "read", "--object", "g50v4").want(t, 0).has(t, "2 objects",
		"g50v4   time-and-interval        0  2026-01-04T02:00:00.000Z  every 1 weeks", "every 0 none")
	cli(t, st, "read", "--object", "g50v4", "--format", "table").want(t, 0).has(t, "every 1 weeks")
	var res types.PollResult
	cli(t, st, "read", "--object", "50.4", "--range", "0", "--format", "json").want(t, 0).decode(t, &res)
	if len(res.Measurements) != 1 {
		t.Fatalf("result %+v", res)
	}
	if m := res.Measurements[0]; m.Type != "time-and-interval" || m.Value != "2026-01-04T02:00:00.000Z" || m.Interval == nil || *m.Interval != 1 || m.IntervalUnit != "weeks" {
		t.Errorf("time and interval: %+v", m)
	}
	// Like in the reference outstations, a time and interval is not in class 0.
	cli(t, st, "poll").want(t, 0).has(t, "12 objects")
}

func TestCommandEvents(t *testing.T) {
	st := stationtest.Start(t, featureModel)

	cli(t, st, "operate", "--type", "crob", "--index", "1", "--op", "latch-on", "--mode", "direct").want(t, 0)
	cli(t, st, "operate", "--type", "analog", "--index", "0", "--value", "50", "--mode", "direct").want(t, 0)
	var res types.PollResult
	cli(t, st, "poll", "--mode", "event", "--class", "2", "--format", "json").want(t, 0).decode(t, &res)
	got := map[string]types.Measurement{}
	for _, m := range res.Measurements {
		got[m.Object] = m
	}
	if m := got["g13v1"]; m.Type != "binary-output-command" || m.Index != 1 || m.Value != true || m.Status != "SUCCESS" || !m.Event {
		t.Errorf("binary command event: %+v", res.Measurements)
	}
	if m := got["g43v3"]; m.Type != "analog-output-command" || m.Value != float64(50) || m.Status != "SUCCESS" || m.Timestamp == "" {
		t.Errorf("analog command event: %+v", res.Measurements)
	}
	// A control of an output without command events reports none.
	cli(t, st, "operate", "--type", "crob", "--index", "0", "--op", "latch-off", "--mode", "sbo").want(t, 0)
	cli(t, st, "poll", "--mode", "event", "--format", "json").want(t, 0).decode(t, &res)
	for _, m := range res.Measurements {
		if m.Group == 13 {
			t.Errorf("command event of an output without them: %+v", m)
		}
	}
}

// Aggressive mode sends a critical request with its MAC, without waiting for a challenge.
func TestAggressiveModeAndSecurityStatistics(t *testing.T) {
	update := bytes.Repeat([]byte{0x6b}, 16)
	st := stationtest.Start(t, stationtest.Model, func(o *station.Options) { o.Auth = &secauth.User{Number: 1, UpdateKey: update} })
	updateFile := filepath.Join(t.TempDir(), "update.key")
	_ = os.WriteFile(updateFile, []byte(hex.EncodeToString(update)), 0o600)

	// SELECT is challenged; OPERATE goes in aggressive mode: g120v3 first, g120v9 last.
	r := cli(t, st, "--debug", "operate", "--type", "crob", "--index", "1", "--op", "latch-on", "--mode", "sbo",
		"--sa-update-key-file", updateFile, "--sa-aggressive").want(t, 0).has(t, "SUCCESS")
	if !strings.Contains(r.stderr, "app  OPERATE FIR FIN seq=3 headers=3\n") {
		t.Errorf("the OPERATE was not sent in aggressive mode (with the g120v3 and g120v9 headers):\n%s", r.stderr)
	}
	t.Setenv("DNP3CTL_SA_AGGRESSIVE", "true")
	cli(t, st, "restart", "--type", "warm", "--sa-update-key-file", updateFile).want(t, 0)
	kind, code := cli(t, st, "poll", "--format", "json").want(t, 3).errorCode(t)
	if kind != "invalid_input" || code != "invalid_flag_combination" {
		t.Errorf("aggressive mode without a key: %s/%s", kind, code)
	}
	cli(t, st, "poll").want(t, 3).errHas(t, "--sa-aggressive needs secure authentication")
	t.Setenv("DNP3CTL_SA_AGGRESSIVE", "")

	// The outstation keeps the security statistics of the association.
	var res types.PollResult
	cli(t, st, "read", "--object", "g121v1", "--format", "json").want(t, 0).decode(t, &res)
	if len(res.Measurements) != int(secauth.NumStatistics) {
		t.Fatalf("%d security statistics, want %d", len(res.Measurements), secauth.NumStatistics)
	}
	stats := map[string]float64{}
	for _, m := range res.Measurements {
		if m.Type != "security-statistic" || m.Statistic != secauth.Statistic(m.Index).String() {
			t.Errorf("statistic %+v", m)
		}
		stats[m.Statistic], _ = m.Value.(float64)
	}
	if stats["session key changes"] < 2 || stats["critical messages received"] < 3 || stats["authentication failures"] != 0 {
		t.Errorf("statistics %v", stats)
	}
	cli(t, st, "read", "--object", "g121", "--range", "13").want(t, 0).has(t, "g121v1", "security-statistic", "(session key changes)")
}

// background runs dnp3ctl until stop is called and returns its result on the channel.
func background(t *testing.T, args ...string) (stop func() result) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan result, 1)
	go func() { done <- runCtx(t, ctx, args...) }()
	return func() result {
		cancel()
		return <-done
	}
}

// The dual end point: the outstation connects to the master.
func TestDualEndPoint(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model.yaml")
	_ = os.WriteFile(model, []byte(featureModel), 0o600)

	t.Run("TCP", func(t *testing.T) {
		port := strconv.Itoa(int(stationtest.FreePort(t)))
		stop := background(t, "server", "start", "--config", model, "--dial", "tcp://127.0.0.1:"+port, "--format", "json")
		var res types.PollResult
		run(t, "client", "poll", "--listen", "127.0.0.1", "--port", port, "--format", "json").want(t, 0).decode(t, &res)
		if res.Count != 12 || !strings.HasPrefix(res.Target, "tcp://127.0.0.1:") || res.Target == "tcp://127.0.0.1:"+port {
			t.Errorf("result %+v", res)
		}
		// The outstation connects again for the next command.
		t.Setenv("DNP3CTL_MASTER_LISTEN", "127.0.0.1:"+port)
		run(t, "client", "read", "--object", "g50v4", "--timeout", "5000").want(t, 0).has(t, "every 1 weeks")
		run(t, "client", "monitor", "--integrity", "--count", "3", "--format", "json").want(t, 0)
		events := map[string]bool{}
		for _, e := range stop().want(t, 0).lines(t) {
			events[e["event"].(string)] = true
			if e["event"] == "dialing" && e["remote"] != "tcp://127.0.0.1:"+port {
				t.Errorf("dialing event %v", e)
			}
		}
		for _, want := range []string{"dialing", "connected", "request", "disconnected", "stopping"} {
			if !events[want] {
				t.Errorf("the event log has no %q event", want)
			}
		}
	})

	t.Run("TLS", func(t *testing.T) {
		certs := stationtest.NewCerts(t)
		port := strconv.Itoa(int(stationtest.FreePort(t)))
		stop := background(t, "server", "start", "--config", model, "--dial", "tls://127.0.0.1:"+port,
			"--tls-ca", certs.CA, "--tls-cert", certs.ClientCert, "--tls-key", certs.ClientKey)
		run(t, "client", "poll", "--listen", ":"+port, "--tls-cert", certs.ServerCert, "--tls-key", certs.ServerKey, "--tls-ca", certs.CA).
			want(t, 0).has(t, "at tls://127.0.0.1:", "12 objects")
		stop().want(t, 0).has(t, "dialing", "tls://127.0.0.1:"+port)
		// A CA file without certificates.
		run(t, "client", "poll", "--listen", ":"+port, "--tls-cert", certs.ServerCert, "--tls-key", certs.ServerKey, "--tls-ca", model).
			want(t, 3).errHas(t, "no certificates found")
	})

	t.Run("the outstation does not connect", func(t *testing.T) {
		port := strconv.Itoa(int(stationtest.FreePort(t)))
		r := run(t, "client", "poll", "--listen", "127.0.0.1:"+port, "--timeout", "300", "--format", "json").want(t, 4)
		if kind, code := r.errorCode(t); kind != "transport" || code != "transport_connect_failed" {
			t.Errorf("error %s/%s", kind, code)
		}
		run(t, "client", "monitor", "--listen", "127.0.0.1:"+port, "--timeout", "300", "--no-reconnect").want(t, 4).errHas(t, "did not connect to 127.0.0.1:"+port)
	})

	t.Run("the port is taken", func(t *testing.T) {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = l.Close() }()
		kind, code := run(t, "client", "poll", "--listen", l.Addr().String(), "--format", "json").want(t, 4).errorCode(t)
		if kind != "transport" || code != "listen_failed" {
			t.Errorf("error %s/%s", kind, code)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		for _, args := range [][]string{
			{"--listen", "127.0.0.1", "--url", "tcp://127.0.0.1"},
			{"--listen", "127.0.0.1", "--ip", "127.0.0.1"},
			{"--listen", "127.0.0.1:x"},
			{"--listen", "127.0.0.1", "--tls"},
			{"--listen", "127.0.0.1", "--tls-cert", model, "--tls-key", model, "--tls-insecure"},
		} {
			kind, code := run(t, append([]string{"client", "poll", "--format", "json"}, args...)...).want(t, 3).errorCode(t)
			if kind != "invalid_input" {
				t.Errorf("%v: %s/%s", args, kind, code)
			}
		}
		for _, args := range [][]string{
			{"--dial", "http://127.0.0.1"},
			{"--dial", "udp://127.0.0.1"},
			{"--dial", "tcp://127.0.0.1", "--listen", "127.0.0.1"},
			{"--dial", "tcp://127.0.0.1", "--port", "20000"},
			{"--dial", "tcp://127.0.0.1", "--udp"},
			{"--dial", "tcp://127.0.0.1", "--allow", "127.0.0.1"},
			{"--dial", "tcp://127.0.0.1", "--tls-ca", model},
			{"--dial", "tls://127.0.0.1", "--tls-client-ca", model},
			{"--dial", "tls://127.0.0.1", "--tls-cert", model},
			{"--dial", "tls://127.0.0.1", "--tls-ca", model},
			{"--tls-ca", model},
		} {
			run(t, append([]string{"server", "start"}, args...)...).want(t, 3)
		}
	})

	t.Run("server: command line over the environment", func(t *testing.T) {
		// A connection style of the server on the command line wins over another one in the
		// environment; two on the command line stay mutually exclusive.
		free := strconv.Itoa(int(stationtest.FreePort(t)))
		for _, c := range []struct {
			name string
			env  map[string]string
			args []string
			want string
		}{
			{"--dial over DNP3CTL_PORT and DNP3CTL_UDP", map[string]string{"DNP3CTL_PORT": free, "DNP3CTL_UDP": "true", "DNP3CTL_UDP_PEER": "127.0.0.1:1"}, []string{"--dial", "tcp://127.0.0.1:" + free}, "dialing"},
			{"--listen over DNP3CTL_DIAL", map[string]string{"DNP3CTL_DIAL": "tcp://127.0.0.1:" + free}, []string{"--listen", "127.0.0.1", "--port", free}, "tcp://127.0.0.1:" + free},
			{"--udp over DNP3CTL_SERIAL", map[string]string{"DNP3CTL_SERIAL": "/dev/null"}, []string{"--udp", "--listen", "127.0.0.1", "--port", free}, "udp://127.0.0.1:" + free},
		} {
			t.Run(c.name, func(t *testing.T) {
				for k, v := range c.env {
					t.Setenv(k, v)
				}
				run(t, append([]string{"server", "start", "--duration", "1"}, c.args...)...).want(t, 0).has(t, c.want)
			})
		}
		// Two styles both in the environment are refused as well.
		t.Setenv("DNP3CTL_DIAL", "tcp://127.0.0.1:"+free)
		t.Setenv("DNP3CTL_PORT", free)
		run(t, "server", "start").want(t, 3).errHas(t, "--dial cannot be combined")
		t.Setenv("DNP3CTL_DIAL", "")
		t.Setenv("DNP3CTL_PORT", "")
		run(t, "server", "start", "--dial", "tcp://127.0.0.1:"+free, "--port", free).want(t, 3).errHas(t, "--dial cannot be combined")
	})
}
