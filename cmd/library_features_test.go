// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"encoding/hex"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-dnp3/secauth"

	"github.com/otfabric/dnp3ctl/internal/station"
	"github.com/otfabric/dnp3ctl/internal/stationtest"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// timeModel is the test model of an outstation that asks for time synchronization.
var timeModel = strings.Replace(stationtest.Model, "  restart_delay: 1s\n", "  restart_delay: 1s\n  time_sync_interval: 1h\n", 1)

// functions returns the function codes of the requests the outstation logged.
func functions(st *stationtest.Station) []string {
	var out []string
	for _, e := range st.Events() {
		if e.Function != "" {
			out = append(out, e.Function)
		}
	}
	return out
}

func count(list []string, s string) int {
	n := 0
	for _, x := range list {
		if x == s {
			n++
		}
	}
	return n
}

// A one-shot command sends its request and nothing else: no integrity poll at connect and
// no time synchronization although the outstation asks for it (NEED_TIME).
func TestOneShotSendsOnlyItsRequest(t *testing.T) {
	st := stationtest.Start(t, timeModel)
	cli(t, st, "read", "--object", "g1v2").want(t, 0)
	got := functions(st)
	if count(got, "READ") != 1 || count(got, "RECORD_CURRENT_TIME") != 0 || count(got, "DELAY_MEASURE") != 0 || count(got, "WRITE") != 0 {
		t.Errorf("a one-shot read sent %v", got)
	}
	if !strings.Contains(cli(t, st, "read", "--object", "g1v2", "--format", "json").want(t, 0).stdout, "NEED_TIME") {
		t.Error("the one-shot read cleared NEED_TIME")
	}
}

// The monitor answers NEED_TIME by itself, unless --time-sync says otherwise.
func TestMonitorTimeSync(t *testing.T) {
	for _, tc := range []struct {
		mode string
		want string // the request that answers NEED_TIME, or "" for none
	}{
		{"", "RECORD_CURRENT_TIME"},
		{"lan", "RECORD_CURRENT_TIME"},
		{"non-lan", "DELAY_MEASURE"},
		{"clear-need-time", "WRITE"},
		{"none", ""},
	} {
		t.Run("mode "+tc.mode, func(t *testing.T) {
			st := stationtest.Start(t, timeModel)
			// The outstation reports NEED_TIME in the response to the integrity poll: run long
			// enough for the master to answer it.
			args := []string{"monitor", "--unsolicited", "none", "--integrity", "--duration", "1"}
			if tc.mode != "" {
				args = append(args, "--time-sync", tc.mode)
			}
			cli(t, st, args...).want(t, 0)
			got := functions(st)
			switch {
			case tc.want == "" && (count(got, "RECORD_CURRENT_TIME") != 0 || count(got, "DELAY_MEASURE") != 0):
				t.Errorf("--time-sync none sent %v", got)
			case tc.want == "WRITE" && (count(got, "WRITE") == 0 || count(got, "RECORD_CURRENT_TIME") != 0):
				t.Errorf("--time-sync clear-need-time sent %v", got)
			case tc.want != "" && count(got, tc.want) == 0:
				t.Errorf("--time-sync %q sent %v, want %s", tc.mode, got, tc.want)
			}
		})
	}
	st := stationtest.Start(t, stationtest.Model)
	cli(t, st, "monitor", "--time-sync", "sometimes").want(t, 3).errHas(t, "invalid --time-sync")
}

// The options of a long-running master: jitter, automatic event polls, the queue limit,
// unconfirmed unsolicited responses and the dual end point.
func TestMonitorOptions(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)
	cli(t, st, "monitor", "--integrity", "--count", "1", "--reconnect-jitter", "50", "--auto-event-poll", "--queue-limit", "8").want(t, 0)
	// An outstation that sends no unsolicited responses answers a master that does not
	// confirm them.
	quiet := stationtest.Start(t, strings.Replace(stationtest.Model, "unsolicited: {enabled: true}", "unsolicited: {enabled: false}", 1))
	cli(t, quiet, "monitor", "--unsolicited", "none", "--integrity", "--count", "1", "--no-unsolicited-confirm").want(t, 0)
	cli(t, st, "monitor", "--no-reconnect", "--reconnect-jitter", "10").want(t, 3).errHas(t, "--reconnect-jitter does not apply")
	cli(t, st, "monitor", "--reconnect-jitter", "101").want(t, 3).errHas(t, "0-100")

	port := stationtest.FreePort(t)
	r := cli(t, st, "monitor", "--integrity", "--count", "1", "--dual-endpoint", "127.0.0.1:"+itoa(port), "--dual-endpoint-allow", "127.0.0.1").want(t, 0)
	if !strings.Contains(r.stderr, "1 values") {
		t.Errorf("dual end point: %s", r.stderr)
	}
	cli(t, st, "monitor", "--dual-endpoint", "nowhere").want(t, 3).errHas(t, "invalid --dual-endpoint")
	cli(t, st, "monitor", "--dual-endpoint-allow", "127.0.0.1").want(t, 3).errHas(t, "needs --dual-endpoint")
	cli(t, st, "monitor", "--dual-endpoint", ":0", "--dual-endpoint-allow", "not-an-ip").want(t, 3).errHas(t, "--dual-endpoint-allow")
	run(t, "client", "monitor", "--url", "udp://127.0.0.1:20000", "--dual-endpoint", ":0").want(t, 3).errHas(t, "needs a tcp://")
	// The address of --dual-endpoint is taken.
	ln := stationtest.Start(t, stationtest.Model)
	cli(t, st, "monitor", "--count", "1", "--dual-endpoint", ln.Addr()).want(t, 4)
}

func itoa(n uint16) string { return strconv.Itoa(int(n)) }

// The connection options of every client command.
func TestConnectionOptions(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)
	cli(t, st, "poll", "--retries", "2", "--retry-jitter", "25").want(t, 0)
	cli(t, st, "poll", "--retry-jitter", "25").want(t, 3).errHas(t, "--retry-jitter needs --retries")
	cli(t, st, "poll", "--retries", "1", "--retry-jitter", "200").want(t, 3).errHas(t, "0-100")
	cli(t, st, "poll", "--max-fragment", "4096", "--max-request-fragment", "249").want(t, 0).has(t, "12 objects")
	cli(t, st, "poll", "--max-fragment", "100").want(t, 3)
	cli(t, st, "poll", "--max-request-fragment", "100").want(t, 3)
	// A request larger than --max-request-fragment is refused before it is sent.
	cli(t, st, "write-attribute", "--variation", "location", "--value", strings.Repeat("x", 250), "--max-request-fragment", "249").want(t, 3)
	cli(t, st, "poll", "--no-solicited-confirm").want(t, 0)
	cli(t, st, "poll", "--sa-challenge-macs", "hmac-sha256-16").want(t, 3).errHas(t, "--sa-challenge-macs needs secure authentication")
	cli(t, st, "poll", "--sa-key-change-count", "10").want(t, 3).errHas(t, "--sa-key-change-count needs an update key")
}

// Secure authentication: the MAC algorithms a challenge may ask for, the key change count
// and the master's security statistics at the end of a monitor.
func TestSecureAuthenticationOptions(t *testing.T) {
	update := bytes.Repeat([]byte{0x3c}, 16)
	st := stationtest.Start(t, stationtest.Model, func(o *station.Options) { o.Auth = &secauth.User{Number: 1, UpdateKey: update} })
	file := filepath.Join(t.TempDir(), "update.key")
	_ = os.WriteFile(file, []byte(hex.EncodeToString(update)), 0o600)

	cli(t, st, "operate", "--type", "crob", "--index", "1", "--op", "latch-on", "--mode", "direct",
		"--sa-update-key-file", file, "--sa-challenge-macs", "hmac-sha256-16,hmac-sha1-10", "--sa-key-change-count", "100").want(t, 0).has(t, "SUCCESS")
	// The outstation challenges with HMAC-SHA-256/16, which this master does not accept.
	cli(t, st, "operate", "--type", "crob", "--index", "1", "--op", "latch-on", "--mode", "direct",
		"--sa-update-key-file", file, "--sa-challenge-macs", "hmac-sha1-10").want(t, 6).errHas(t, "not among the accepted MAC algorithms")
	cli(t, st, "poll", "--sa-update-key-file", file, "--sa-challenge-macs", "md5").want(t, 3).errHas(t, "invalid --sa-challenge-macs")

	r := cli(t, st, "monitor", "--integrity", "--duration", "1", "--sa-update-key-file", file).want(t, 0)
	if !strings.Contains(r.stderr, "security statistics of the master:") || !strings.Contains(r.stderr, "session key changes 1") {
		t.Errorf("monitor did not report the master's security statistics:\n%s", r.stderr)
	}
}

// time-sync --method clear-need-time clears NEED_TIME without setting the time.
func TestClearNeedTime(t *testing.T) {
	st := stationtest.Start(t, timeModel)
	var res types.ConfirmedResult
	cli(t, st, "time-sync", "--method", "clear-need-time", "--format", "json").want(t, 0).decode(t, &res)
	if res.Request != "time-sync" || res.Method != "clear-need-time" || res.Time != "" {
		t.Errorf("result %+v", res)
	}
	if got := functions(st); count(got, "WRITE") != 1 || count(got, "RECORD_CURRENT_TIME") != 0 {
		t.Errorf("clear-need-time sent %v", got)
	}
	if strings.Contains(cli(t, st, "read", "--object", "g1v2", "--format", "json").want(t, 0).stdout, "NEED_TIME") {
		t.Error("NEED_TIME is still set")
	}
	cli(t, st, "time-sync", "--method", "clear-need-time").want(t, 0).has(t, "time-sync (clear-need-time) of outstation 10")
	cli(t, st, "time-sync", "--method", "clear-need-time", "--time", "2026-01-01T00:00:00Z").want(t, 3)
}

// manage --function activate-config sends ACTIVATE_CONFIG and reports the outstation's
// status of requested operation.
func TestActivateConfig(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)
	var res types.ConfirmedResult
	cli(t, st, "manage", "--function", "activate-config", "--config-name", "/config/a.xml,/config/b.xml", "--config-string", "profile-2", "--format", "json").
		want(t, 0).decode(t, &res)
	if res.Request != "activate-config" || res.Function != "ACTIVATE_CONFIG" || len(res.Activation) == 0 {
		t.Fatalf("result %+v", res)
	}
	r := cli(t, st, "manage", "--function", "activate-config", "--config-name", "/config/a.xml").want(t, 0)
	r.has(t, "activate-config (ACTIVATE_CONFIG, status", "in effect after")
	cli(t, st, "manage", "--function", "activate-config", "--config-name", "/x", "--format", "table").want(t, 0).has(t, "ACTIVATE_CONFIG")

	cli(t, st, "manage", "--function", "activate-config").want(t, 3).errHas(t, "needs a configuration")
	cli(t, st, "manage", "--function", "activate-config", "--config-string", strings.Repeat("x", 256)).want(t, 3).errHas(t, "1-255 octets")
	cli(t, st, "manage", "--function", "save-configuration", "--config-name", "/x").want(t, 3).errHas(t, "apply to --function activate-config")

	refusing := stationtest.Start(t, strings.Replace(stationtest.Model, "  restart_delay: 1s\n", "  restart_delay: 1s\n  refused_functions: [activate-config]\n", 1))
	cli(t, refusing, "manage", "--function", "activate-config", "--config-name", "/x").want(t, 6).errHas(t, "NO_FUNC_CODE_SUPPORT")
}

// file put --created gives the file its time of creation.
func TestFilePutCreated(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)
	in := filepath.Join(t.TempDir(), "in.txt")
	_ = os.WriteFile(in, []byte("content"), 0o600)
	cli(t, st, "file", "put", "--input", in, "--path", "/created.txt", "--created", "2026-01-31T12:00:00Z").want(t, 0)
	var res types.FileInfoResult
	cli(t, st, "file", "info", "--path", "/created.txt", "--format", "json").want(t, 0).decode(t, &res)
	if res.Entry.Created != "2026-01-31T12:00:00.000Z" {
		t.Errorf("created %q", res.Entry.Created)
	}
	cli(t, st, "file", "put", "--input", in, "--path", "/x", "--created", "yesterday").want(t, 3).errHas(t, "invalid --created")
}

// The outstation options of `server start`.
func TestServerOptions(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model, func(o *station.Options) {
		o.MaxControls, o.DeadbandIntegerOnly = 1, true
		o.MasterIPs = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}
	})
	// Two controls in a request that takes one: the second is TOO_MANY_OBJS.
	cli(t, st, "operate", "--type", "crob", "--index", "1,3", "--op", "latch-on", "--mode", "direct").want(t, 7).has(t, "TOO_MANY_OBJS")
	cli(t, st, "read", "--object", "g34v3").want(t, 6).errHas(t, "OBJECT_UNKNOWN")
	cli(t, st, "read", "--object", "g34v2").want(t, 0)

	other := stationtest.Start(t, stationtest.Model, func(o *station.Options) {
		o.MasterIPs = []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}
	})
	cli(t, other, "poll", "--timeout", "1000").want(t, 4)

	for _, args := range [][]string{
		{"--master-ips", "not-an-ip"},
		{"--file-timeout", "3601"},
		{"--file-timeout-class", "4"},
		{"--udp-fixed"},
		{"--max-tx-fragment", "100"},
		{"--max-rx-fragment", "248"},
		{"--sa-threshold", "3=10"},
		{"--sa-no-aggressive"},
		{"--sa-max-key-status-requests", "1"},
		{"--serial", "/dev/null", "--master-ips", "10.0.0.1"},
	} {
		r := run(t, append([]string{"server", "start", "--port", "0", "--duration", "1"}, args...)...)
		if r.code != 3 {
			t.Errorf("server start %v: exit code %d\n%s", args, r.code, r.stderr)
		}
	}
	key := filepath.Join(t.TempDir(), "sa.key")
	_ = os.WriteFile(key, []byte(strings.Repeat("ab", 16)), 0o600)
	for _, args := range [][]string{
		{"--sa-threshold", "18=10"},
		{"--sa-threshold", "3=0"},
		{"--sa-threshold", "x"},
		{"--sa-statistics-threshold", "70000"},
		{"--sa-max-key-status-requests", "1"},
	} {
		r := run(t, append([]string{"server", "start", "--port", "0", "--duration", "1", "--sa-key-file", key}, args...)...)
		if r.code != 3 {
			t.Errorf("server start %v: exit code %d\n%s", args, r.code, r.stderr)
		}
	}
	r := run(t, "server", "start", "--port", "0", "--duration", "1", "--sa-key-file", key, "--sa-threshold", "3=10,13=1",
		"--sa-no-aggressive", "--sa-expected-key-change-interval", "600", "--sa-expected-key-change-count", "500", "--sa-max-key-status-requests", "10",
		"--master-ips", "127.0.0.1,::1", "--max-controls", "4", "--max-tx-fragment", "1024", "--max-rx-fragment", "512",
		"--deadband-integer-only", "--file-timeout", "60", "--file-timeout-class", "2")
	if r.code != 0 {
		t.Errorf("server start with every option: exit code %d\n%s", r.code, r.stderr)
	}
}

// --udp-fixed answers the master at the address and port of --udp-peer.
func TestServerUDPFixed(t *testing.T) {
	port := itoa(stationtest.FreePort(t))
	master := itoa(stationtest.FreePort(t))
	stop := background(t, "server", "start", "--udp", "--listen", "127.0.0.1", "--port", port, "--udp-peer", "127.0.0.1:"+master, "--udp-fixed")
	deadline := time.Now().Add(5 * time.Second)
	var r result
	for time.Now().Before(deadline) {
		r = run(t, "client", "poll", "--url", "udp://127.0.0.1:"+port, "--udp-local-port", master, "--timeout", "1000")
		if r.code == 0 {
			break
		}
	}
	if r.code != 0 {
		t.Errorf("poll of a fixed-port UDP outstation: %d\n%s", r.code, r.stderr)
	}
	stop()
}

// looseFile writes the secret file name below dir, owner-only as every file is created, and
// then gives it mode: the permissions that others may read it with are what the test is
// about. It returns the path of the file.
func looseFile(t *testing.T, dir, name string, data []byte, mode os.FileMode) string {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile(name, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := root.Chmod(name, mode); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, name)
}

// A key or password file that others can read is used, with a warning on stderr.
func TestSecretFileWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permissions")
	}
	update := bytes.Repeat([]byte{0x5a}, 16)
	st := stationtest.Start(t, stationtest.Model, func(o *station.Options) { o.Auth = &secauth.User{Number: 1, UpdateKey: update} })
	dir := t.TempDir()
	loose := looseFile(t, dir, "loose.key", []byte(hex.EncodeToString(update)), 0o644)
	r := cli(t, st, "poll", "--sa-update-key-file", loose).want(t, 0)
	if strings.Count(r.stderr, "warning: the secure authentication update key file "+loose+" is accessible to other users (mode 0644)") != 1 {
		t.Errorf("no single warning about a readable key file:\n%s", r.stderr)
	}
	tight := filepath.Join(dir, "tight.key")
	_ = os.WriteFile(tight, []byte(hex.EncodeToString(update)), 0o600)
	if r := cli(t, st, "poll", "--sa-update-key-file", tight).want(t, 0); strings.Contains(r.stderr, "warning:") {
		t.Errorf("a warning about a key file of its owner only:\n%s", r.stderr)
	}

	pw := looseFile(t, dir, "password", []byte("secret\n"), 0o640)
	cli(t, st, "file", "ls", "--file-user", "operator", "--file-password-file", pw, "--sa-update-key-file", tight).want(t, 6).
		errHas(t, "warning: the file password file "+pw+" is accessible to other users (mode 0640)")
}

// --source-address binds the connections of the master; --dial-source-address and the
// --redial flags shape the connections of `server start --dial`.
func TestSourceAddressAndRedial(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)
	cli(t, st, "poll", "--source-address", "127.0.0.1").want(t, 0).has(t, "12 objects")
	// An address of no interface of this host: the bind fails, so the dialer is used.
	if r := cli(t, st, "poll", "--source-address", "192.0.2.1", "--timeout", "2000"); r.code == 0 || r.code == 3 {
		t.Errorf("poll from 192.0.2.1: exit %d\n%s", r.code, r.stderr)
	}
	cli(t, st, "poll", "--source-address", "nope").want(t, 3).errHas(t, "invalid --source-address")
	cli(t, st, "poll", "--source-address", "127.0.0.1:20000").want(t, 3).errHas(t, "without a port")
	run(t, "client", "poll", "--listen", "127.0.0.1", "--port", itoa(stationtest.FreePort(t)), "--source-address", "127.0.0.1").want(t, 3).errHas(t, "--source-address needs")
	run(t, "client", "poll", "--url", "serial:///dev/null", "--source-address", "127.0.0.1").want(t, 3).errHas(t, "--source-address needs")

	t.Run("UDP", func(t *testing.T) {
		port := itoa(stationtest.FreePort(t))
		stop := background(t, "server", "start", "--udp", "--listen", "127.0.0.1", "--port", port)
		deadline := time.Now().Add(5 * time.Second)
		var r result
		for time.Now().Before(deadline) {
			r = run(t, "client", "poll", "--url", "udp://127.0.0.1:"+port, "--source-address", "127.0.0.1", "--timeout", "1000")
			if r.code == 0 {
				break
			}
		}
		if r.code != 0 {
			t.Errorf("UDP poll from 127.0.0.1: %d\n%s", r.code, r.stderr)
		}
		stop()
	})

	t.Run("dial", func(t *testing.T) {
		port := itoa(stationtest.FreePort(t))
		stop := background(t, "server", "start", "--dial", "tcp://127.0.0.1:"+port, "--dial-source-address", "127.0.0.1",
			"--redial-min-delay", "50", "--redial-max-delay", "200", "--redial-jitter", "50")
		run(t, "client", "poll", "--listen", "127.0.0.1", "--port", port, "--timeout", "5000").want(t, 0).has(t, "14 objects")
		// The outstation connects again, after the redial delay, for the next command.
		run(t, "client", "poll", "--listen", "127.0.0.1", "--port", port, "--timeout", "5000").want(t, 0)
		stop().want(t, 0)
	})

	for _, c := range []struct {
		args []string
		msg  string
	}{
		{[]string{"--redial-jitter", "10"}, "need --dial"},
		{[]string{"--dial-source-address", "127.0.0.1"}, "need --dial"},
		{[]string{"--dial", "tcp://127.0.0.1", "--redial-jitter", "101"}, "0-100"},
		{[]string{"--dial", "tcp://127.0.0.1", "--redial-min-delay", "500", "--redial-max-delay", "100"}, "below --redial-min-delay"},
		{[]string{"--dial", "tcp://127.0.0.1", "--dial-source-address", "host"}, "invalid --dial-source-address"},
	} {
		run(t, append([]string{"server", "start"}, c.args...)...).want(t, 3).errHas(t, c.msg)
	}
}
