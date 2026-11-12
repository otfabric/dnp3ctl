// SPDX-License-Identifier: MIT

package dnp

import (
	"bytes"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/secauth"

	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/errs"
)

func TestOneShotOptions(t *testing.T) {
	if n := len(oneShotOptions()); n != 4 {
		t.Errorf("%d one-shot options, want 4: startup, time synchronization, integrity, restart", n)
	}
}

func TestJitter(t *testing.T) {
	if j, err := jitter("--x", 25); err != nil || j != 0.25 {
		t.Errorf("jitter 25: %v, %v", j, err)
	}
	if _, err := jitter("--x", 101); err == nil {
		t.Error("jitter 101 was accepted")
	}
}

func TestParseChallengeMACs(t *testing.T) {
	macs, err := parseChallengeMACs(" HMAC-SHA256-16 ,, hmac-sha1-4")
	if err != nil || len(macs) != 2 || macs[0] != app.MACHMACSHA256Trunc16 || macs[1] != app.MACHMACSHA1Trunc4 {
		t.Errorf("macs %v, %v", macs, err)
	}
	if macs, err := parseChallengeMACs(""); err != nil || macs != nil {
		t.Errorf("empty: %v, %v", macs, err)
	}
	if _, err := parseChallengeMACs("aes-gmac"); err == nil {
		t.Error("aes-gmac was accepted")
	}
}

func TestMonitorOptionsValidation(t *testing.T) {
	base := func() *config.MonitorConfig {
		return &config.MonitorConfig{ConnConfig: config.ConnConfig{IP: "127.0.0.1", AddressConfig: config.AddressConfig{Master: 1, Outstation: 10}}}
	}
	for mode := range TimeSyncModes() {
		c := base()
		c.TimeSync = mode
		if _, _, err := monitorOptions(c, false); err != nil {
			t.Errorf("--time-sync %s: %v", mode, err)
		}
	}
	c := base()
	c.TimeSync, c.ReconnectJitter, c.AutoEventPoll, c.QueueLimit, c.NoUnsolicitedConfirm = "none", 10, true, 4, true
	if opts, _, err := monitorOptions(c, false); err != nil || len(opts) != 5 {
		t.Errorf("options: %d, %v", len(opts), err)
	}
	c = base()
	c.DualEndpoint, c.DualEndpointAllow = "127.0.0.1:0", []string{"127.0.0.1"}
	opts, l, err := monitorOptions(c, true)
	if err != nil || l == nil || len(opts) != 2 {
		t.Fatalf("dual end point: %d, %v, %v", len(opts), l, err)
	}
	_ = l.Close()
	for _, bad := range []func(*config.MonitorConfig){
		func(c *config.MonitorConfig) { c.TimeSync = "often" },
		func(c *config.MonitorConfig) { c.ReconnectJitter = 200 },
		func(c *config.MonitorConfig) { c.NoReconnect, c.ReconnectJitter = true, 1 },
		func(c *config.MonitorConfig) { c.DualEndpointAllow = []string{"x"} },
		func(c *config.MonitorConfig) { c.DualEndpointAllow = []string{"10.0.0.1"} },
		func(c *config.MonitorConfig) { c.DualEndpoint = "no-port" },
		func(c *config.MonitorConfig) { c.DualEndpoint, c.IP, c.URL = ":0", "", "udp://127.0.0.1" },
		func(c *config.MonitorConfig) { c.DualEndpoint, c.IP, c.URL = ":0", "", "bad://" },
	} {
		c := base()
		bad(c)
		if _, _, err := monitorOptions(c, false); err == nil {
			t.Errorf("%+v was accepted", c)
		} else if _, ok := errs.As(err); !ok {
			t.Errorf("%+v: %v is no *errs.Error", c, err)
		}
	}
	// The address of --dual-endpoint is taken.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	c = base()
	c.DualEndpoint = ln.Addr().String()
	if _, _, err := monitorOptions(c, true); err == nil {
		t.Error("listening on a taken address succeeded")
	}
}

func TestAllowFunc(t *testing.T) {
	if allowFunc(nil) != nil {
		t.Error("no prefixes should allow any address")
	}
	allow := allowFunc([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	if !allow(&net.TCPAddr{IP: net.ParseIP("10.1.2.3"), Port: 1}) || allow(&net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 1}) {
		t.Error("allow does not follow the prefix")
	}
	if !allow(&net.TCPAddr{IP: net.ParseIP("::ffff:10.0.0.1"), Port: 1}) {
		t.Error("an IPv4-mapped address is not unmapped")
	}
	if allow(serialAddr("COM1")) {
		t.Error("an address without a port was allowed")
	}
}

func TestParsePrefixes(t *testing.T) {
	ps, err := ParsePrefixes([]string{" 10.1.2.3/8 ", "", "::1", "fe80::1%eth0"}, "--x")
	if err != nil || len(ps) != 3 || ps[0].String() != "10.0.0.0/8" || ps[1].String() != "::1/128" || !ps[2].IsValid() {
		t.Errorf("prefixes %v, %v", ps, err)
	}
	if _, err := ParsePrefixes([]string{"host"}, "--x"); err == nil || !strings.Contains(err.Error(), "--x") {
		t.Errorf("a host name: %v", err)
	}
}

func TestSecurityStatisticsText(t *testing.T) {
	stats := make([]uint32, secauth.NumStatistics)
	if SecurityStatisticsText(stats) != "" {
		t.Error("zero statistics are not empty")
	}
	stats[secauth.StatSessionKeyChanges], stats[secauth.StatTotalMessagesSent] = 2, 9
	if got := SecurityStatisticsText(stats); got != "total messages sent 9, session key changes 2" {
		t.Errorf("text %q", got)
	}
}

func TestManagePlan(t *testing.T) {
	fc, headers, err := managePlan(&config.ManageConfig{Function: "activate-config", ConfigNames: []string{"/a.xml", " ", "/b.xml"}, ConfigStrings: []string{"p1", "profile-two"}})
	if err != nil || fc != app.FuncActivateConfig || len(headers) != 3 || headers[0].GV != app.G70V8 || headers[1].GV != app.GV(110, 2) || headers[2].GV != app.GV(110, 11) {
		t.Fatalf("plan %v, %+v, %v", fc, headers, err)
	}
	if _, h, err := managePlan(&config.ManageConfig{Function: "start-application"}); err != nil || len(h) != 1 {
		t.Errorf("start-application: %v, %v", h, err)
	}
	for _, c := range []config.ManageConfig{
		{Function: "activate-config"},
		{Function: "activate-config", ConfigStrings: []string{""}},
		{Function: "activate-config", ConfigNames: []string{strings.Repeat("x", 70000)}},
		{Function: "save-configuration", ConfigStrings: []string{"x"}},
		{Function: "reboot"},
	} {
		if _, _, err := managePlan(&c); err == nil {
			t.Errorf("%+v was accepted", c)
		}
	}
	got := activationResults([]app.ActivateConfigStatus{{Delay: 1500 * time.Millisecond, Results: []app.ActivateConfigResult{{Code: 0}, {Code: 4, Text: "bad"}}}})
	if len(got) != 1 || got[0].DelayMs != 1500 || len(got[0].Results) != 2 || got[0].Results[1].Text != "bad" {
		t.Errorf("activation %+v", got)
	}
}

func TestParseCreated(t *testing.T) {
	if c, err := parseCreated(""); err != nil || !c.IsZero() {
		t.Errorf("empty: %v, %v", c, err)
	}
	if c, err := parseCreated("2026-01-31T12:00:00Z"); err != nil || c.Year() != 2026 {
		t.Errorf("a time: %v, %v", c, err)
	}
	if _, err := parseCreated("soon"); err == nil || !strings.Contains(err.Error(), "--created") {
		t.Errorf("soon: %v", err)
	}
}

func TestTimeMethodClearNeedTime(t *testing.T) {
	if m, err := timeMethod(" Clear-Need-Time "); err != nil || m != TimeClearNeedTime {
		t.Errorf("method %q, %v", m, err)
	}
	cfg := &config.TimeSyncConfig{ConnConfig: config.ConnConfig{IP: "127.0.0.1", AddressConfig: config.AddressConfig{Master: 1, Outstation: 10}}, Method: TimeClearNeedTime, Time: "2026-01-01T00:00:00Z"}
	if err := CheckTimeSync(cfg); err == nil {
		t.Error("--time with clear-need-time was accepted")
	}
}

func TestConnectionOptionCombinations(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "key")
	_ = os.WriteFile(key, []byte(strings.Repeat("ab", 16)), 0o600)
	base := func() *config.ConnConfig {
		return &config.ConnConfig{IP: "127.0.0.1", AddressConfig: config.AddressConfig{Master: 1, Outstation: 10}}
	}
	c := base()
	c.SAUpdateKeyFile, c.SAChallengeMACs, c.SAKeyChangeCount = key, "hmac-sha256-16", 50
	c.Retries, c.RetryJitter, c.MaxFragment, c.MaxRequestFragment, c.NoSolicitedConfirm = 1, 50, 4096, 300, true
	if _, opts, _, err := options(c, nil); err != nil || len(opts) < 9 {
		t.Errorf("options: %d, %v", len(opts), err)
	}
	for _, bad := range []func(*config.ConnConfig){
		func(c *config.ConnConfig) { c.RetryJitter = 5 },
		func(c *config.ConnConfig) { c.Retries, c.RetryJitter = 1, 101 },
		func(c *config.ConnConfig) { c.SAChallengeMACs = "hmac-sha256-16" },
		func(c *config.ConnConfig) { c.SAKeyChangeCount = 5 },
		func(c *config.ConnConfig) { c.SAKeyFile, c.SAKeyChangeCount = key, 5 },
		func(c *config.ConnConfig) { c.SAKeyFile, c.SAChallengeMACs = key, "x" },
	} {
		c := base()
		bad(c)
		if _, _, _, err := options(c, nil); err == nil {
			t.Errorf("%+v was accepted", c)
		}
	}
}

// looseFile writes the secret file name below dir, owner-only as every file is created, and
// then gives it mode: the permissions that others may read it with are what the test is
// about.
func looseFile(t *testing.T, dir, name string, data []byte, mode os.FileMode) {
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
}

func TestSecretFileWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permissions")
	}
	var out bytes.Buffer
	restore := SetWarningOutput(&out)
	dir := t.TempDir()
	loose := filepath.Join(dir, "loose")
	looseFile(t, dir, "loose", []byte("00112233445566778899aabbccddeeff"), 0o604)
	for range 2 {
		if _, err := readKey(loose, "", "secure authentication key", "--sa-key-file", "DNP3CTL_SA_KEY"); err != nil {
			t.Fatal(err)
		}
	}
	if n := strings.Count(out.String(), "warning: the secure authentication key file "+loose+" is accessible to other users (mode 0604)"); n != 1 {
		t.Errorf("%d warnings:\n%s", n, out.String())
	}
	out.Reset()
	checkSecretFile(dir, "key")
	checkSecretFile(filepath.Join(dir, "missing"), "key")
	if out.Len() != 0 {
		t.Errorf("a warning about a directory or a missing file: %s", out.String())
	}
	// A second output starts with a fresh record of the files it warned about; the first
	// one's restore leaves it in place.
	var second bytes.Buffer
	restoreSecond := SetWarningOutput(&second)
	restore()
	checkSecretFile(loose, "key")
	if !strings.Contains(second.String(), "warning:") {
		t.Errorf("the second output did not get the warning: %q", second.String())
	}
	restoreSecond()
}

func TestParseSourceAddress(t *testing.T) {
	for in, want := range map[string]string{"": "invalid IP", "127.0.0.1": "127.0.0.1", " ::1 ": "::1", "::ffff:10.0.0.1": "10.0.0.1", "fe80::1%en0": "fe80::1%en0"} {
		a, err := ParseSourceAddress("--source-address", in)
		if err != nil || a.String() != want {
			t.Errorf("%q: %v, %v", in, a, err)
		}
	}
	for _, in := range []string{"x", "1.2.3.4:5", "10.0.0.0/8"} {
		if _, err := ParseSourceAddress("--source-address", in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
	if d := SourceDialer(netip.MustParseAddr("127.0.0.1")); d.Timeout != dialTimeout || d.LocalAddr.String() != "127.0.0.1:0" {
		t.Errorf("dialer %+v", d)
	}
}
