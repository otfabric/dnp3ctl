package cmd

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/otfabric/dnp3ctl/internal/station"
	"github.com/otfabric/dnp3ctl/internal/stationtest"
	"github.com/otfabric/dnp3ctl/internal/types"
)

func TestVersionHelpAndUsage(t *testing.T) {
	SetBuildMeta("1.2.3", "v1.2.3", "abc1234", "2026-01-31T12:00:00Z")
	t.Cleanup(func() { SetBuildMeta("dev", "", "", "") })
	run(t, "version").want(t, 0).has(t, "dnp3ctl version: 1.2.3", "tag:         v1.2.3", "commit:      abc1234", "build date:  2026-01-31T12:00:00Z", "go version:", "platform:")
	SetBuildMeta(" ", "", "", "")
	r := run(t, "version").want(t, 0).has(t, "dnp3ctl version: (unknown)")
	if strings.Contains(r.stdout, "commit:") {
		t.Errorf("empty build metadata is printed:\n%s", r.stdout)
	}

	run(t).want(t, 0).has(t, "Available Commands", "client", "server", "discover", "completion", "version")
	run(t, "client").want(t, 0).has(t, "poll", "read", "monitor", "operate", "time-sync", "restart", "unsolicited", "assign-class", "freeze", "attributes", "ping", "file", "objects")
	run(t, "client", "file").want(t, 0).has(t, "ls", "info", "get", "put", "rm")
	run(t, "client", "unsolicited").want(t, 0).has(t, "enable", "disable")
	run(t, "server").want(t, 0).has(t, "start", "generate-config")
	run(t, "client", "operate", "--help").want(t, 0).has(t, "THIS CHANGES THE PROCESS", "DNP3CTL_URL", "--mode", "--sa-key-file")

	run(t, "nope").want(t, 2).errHas(t, `error: unknown command "nope"`)
	run(t, "client", "nope").want(t, 2)
	run(t, "server", "nope").want(t, 2)
	run(t, "client", "poll", "--nope").want(t, 2).errHas(t, "unknown flag")
	run(t, "client", "poll", "--port", "banana").want(t, 2)
	run(t, "client", "read").want(t, 2).errHas(t, `required flag(s) "object" not set`)
	run(t, "version", "extra").want(t, 2)
}

func TestCompletion(t *testing.T) {
	for shell, marker := range map[string]string{
		"bash":       "__start_dnp3ctl",
		"zsh":        "#compdef dnp3ctl",
		"fish":       "complete -c dnp3ctl",
		"powershell": "Register-ArgumentCompleter",
	} {
		run(t, "completion", shell).want(t, 0).has(t, marker)
	}
	run(t, "completion").want(t, 2)
	run(t, "completion", "tcsh").want(t, 2)

	// Flags with a fixed set of values complete to those values.
	for _, c := range []struct {
		args []string
		want []string
	}{
		{[]string{"client", "poll", "--format", ""}, []string{"json", "table", "text"}},
		{[]string{"client", "poll", "--mode", ""}, []string{"class", "event", "integrity"}},
		{[]string{"client", "operate", "--type", ""}, []string{"analog", "crob"}},
		{[]string{"client", "operate", "--mode", ""}, []string{"direct", "direct-no-ack", "sbo"}},
		{[]string{"client", "operate", "--op", "latch"}, []string{"latch-on", "latch-off"}},
		{[]string{"client", "read", "--object", "g30v"}, []string{"g30v1", "g30v5", "g30v6"}},
		{[]string{"client", "time-sync", "--method", ""}, []string{"lan", "non-lan", "write"}},
		{[]string{"client", "restart", "--type", ""}, []string{"cold", "warm"}},
		{[]string{"client", "attributes", "--variation", "manu"}, []string{"manufacturer"}},
		{[]string{"client", "poll", "--parity", ""}, []string{"none", "even", "odd"}},
		{[]string{"client", "poll", "--sa-mac", "hmac-sha256"}, []string{"hmac-sha256-16", "hmac-sha256-8"}},
		{[]string{"server", "generate-config", "--format", ""}, []string{"yaml", "json"}},
		{[]string{"completion", ""}, []string{"bash", "zsh", "fish", "powershell"}},
	} {
		r := run(t, append([]string{cobra.ShellCompNoDescRequestCmd}, c.args...)...).want(t, 0)
		for _, w := range c.want {
			if !strings.Contains("\n"+r.stdout, "\n"+w+"\n") {
				t.Errorf("completion of %v lacks %q:\n%s", c.args, w, r.stdout)
			}
		}
	}
}

// Every flag of every command says which environment variable sets it, and no two flags of
// a command share one.
func TestFlagsDocumentTheirEnvironment(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Name() == "help" || c.Name() == "completion" || strings.HasPrefix(c.Name(), "__") {
			return
		}
		if c.Long == "" && c.Runnable() && c.Name() != "version" {
			t.Errorf("%s has no long description", c.CommandPath())
		}
		if c.Runnable() && !c.HasSubCommands() && c.Example == "" && c.Name() != "version" {
			t.Errorf("%s has no examples", c.CommandPath())
		}
		if c.Runnable() && !c.HasSubCommands() && c.Name() != "version" {
			seen := map[string]bool{}
			c.LocalFlags().VisitAll(func(f *pflag.Flag) {
				if f.Name == "help" {
					return
				}
				i := strings.Index(f.Usage, "[env: ")
				if i < 0 {
					t.Errorf("%s --%s has no environment variable", c.CommandPath(), f.Name)
					return
				}
				name := f.Usage[i+6 : len(f.Usage)-1]
				if !strings.HasPrefix(name, "DNP3CTL_") || seen[name] {
					t.Errorf("%s --%s: environment variable %q is not DNP3CTL_* or is used twice", c.CommandPath(), f.Name, name)
				}
				seen[name] = true
			})
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(NewRootCmd())

	// Secrets have no flag: a key or a password on the command line ends up in the shell
	// history and the process list.
	for _, args := range [][]string{{"client", "poll", "--help"}, {"client", "file", "get", "--help"}, {"server", "start", "--help"}} {
		r := run(t, args...).want(t, 0)
		for _, secret := range []string{"--sa-key ", "--file-password ", "--password"} {
			if strings.Contains(r.stdout, secret) {
				t.Errorf("%v has a flag %q", args, secret)
			}
		}
	}

	// A malformed value is an error before anything runs, whatever the command.
	t.Setenv("DNP3CTL_PORT", "banana")
	run(t, "version").want(t, 3).errHas(t, `error: invalid environment: DNP3CTL_PORT="banana"`)
	defer func() {
		if recover() == nil {
			t.Error("a malformed environment value was accepted")
		}
	}()
	NewRootCmd()
}

func TestDiscover(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)
	// The outstation, and a port that is closed.
	ports := strconv.Itoa(int(st.Port)) + "," + strconv.Itoa(int(stationtest.FreePort(t)))

	var res types.DiscoverResult
	run(t, "discover", "--subnets", "127.0.0.1", "--ports", ports, "--timeout", "500", "--format", "json").want(t, 0).
		errHas(t, "discover: 127.0.0.1 port "+strconv.Itoa(int(st.Port))+": dnp3").decode(t, &res)
	if res.Hosts != 1 || res.Found != 1 || len(res.Devices) != 1 || res.Devices[0].Status != "dnp3" || res.Devices[0].Port != st.Port ||
		res.Devices[0].Outstation == nil || *res.Devices[0].Outstation != 10 || *res.Devices[0].Master != 1 {
		t.Fatalf("discover: %+v", res)
	}
	run(t, "discover", "--subnets", "127.0.0.1/32,127.0.0.1-1", "--ports", strconv.Itoa(int(st.Port))).want(t, 0).has(t, "1 DNP3 outstations among 1 hosts", "dnp3", "outstation 10")
	run(t, "discover", "--subnets", "127.0.0.1", "--ports", strconv.Itoa(int(st.Port)), "--format", "table").want(t, 0).has(t, "IP         PORT")
	// An outstation that is probed at another link address does not answer: the port is open.
	run(t, "discover", "--subnets", "127.0.0.1", "--ports", strconv.Itoa(int(st.Port)), "--outstations", "4", "--timeout", "300", "--debug").want(t, 0).
		has(t, "0 DNP3 outstations", "open").errHas(t, ": open")
	// The probe made no application layer request.
	if st.HasEvent("request") {
		t.Error("discover sent an application layer request")
	}

	for _, args := range [][]string{
		{"--subnets", "10.0.0.0/8"},
		{"--subnets", "10.0.0.0/33"},
		{"--subnets", "fd00::/64"},
		{"--subnets", "10.0.0.9-3"},
		{"--subnets", "10.0.0.x-3"},
		{"--subnets", " , "},
		{"--subnets", "127.0.0.1", "--ports", "0"},
		{"--subnets", "127.0.0.1", "--ports", " "},
		{"--subnets", "127.0.0.1", "--parallel", "0"},
		{"--subnets", "127.0.0.1", "--outstations", "65533"},
		{"--subnets", "127.0.0.1", "--outstations", " "},
		{"--subnets", "127.0.0.1", "--master", "65530"},
		{"--subnets", "127.0.0.1", "--format", "xml"},
	} {
		run(t, append([]string{"discover"}, args...)...).want(t, 3)
	}
	run(t, "discover").want(t, 2)
	run(t, "discover", "--subnets", "127.0.0.0/30", "--ports", strconv.Itoa(int(stationtest.FreePort(t))), "--force-large-scan").want(t, 0).has(t, "0 DNP3 outstations among 2 hosts")
}

func TestServer(t *testing.T) {
	dir := t.TempDir()

	t.Run("generate-config", func(t *testing.T) {
		r := run(t, "server", "generate-config").want(t, 0).has(t, "# dnp3ctl outstation model", "address: 10")
		if _, err := station.Parse([]byte(r.stdout)); err != nil {
			t.Errorf("the generated YAML is not a model: %v", err)
		}
		r = run(t, "server", "generate-config", "--format", "json").want(t, 0)
		m, err := station.Parse([]byte(r.stdout))
		if err != nil || m.Validate() != nil || len(m.Points) != len(station.Demo().Points) {
			t.Errorf("the generated JSON is not the demo model: %v", err)
		}
		path := filepath.Join(dir, "outstation.yaml")
		run(t, "server", "generate-config", "--output", path).want(t, 0).errHas(t, "wrote "+path)
		run(t, "server", "generate-config", "--output", path).want(t, 3).errHas(t, "pass --force")
		run(t, "server", "generate-config", "--output", path, "--force", "--format", "json").want(t, 0)
		run(t, "server", "generate-config", "--output", filepath.Join(dir, "no", "such", "dir.yaml")).want(t, 10)
		run(t, "server", "generate-config", "--format", "toml").want(t, 3)
		if _, err := station.Load(path); err != nil {
			t.Errorf("the written file does not load: %v", err)
		}
	})

	t.Run("start", func(t *testing.T) {
		model := filepath.Join(dir, "model.yaml")
		if err := os.WriteFile(model, []byte(stationtest.Model), 0o600); err != nil {
			t.Fatal(err)
		}
		port := strconv.Itoa(int(stationtest.FreePort(t)))
		ctx, stop := context.WithCancel(context.Background())
		done := make(chan result, 1)
		go func() {
			done <- runCtx(t, ctx, "--debug", "server", "start", "--config", model, "--listen", "127.0.0.1", "--port", port, "--format", "json")
		}()
		target := []string{"--ip", "127.0.0.1", "--port", port}
		eventually(t, "the server to answer", func() bool {
			return run(t, append([]string{"client", "ping", "--count", "1", "--timeout", "300"}, target...)...).code == 0
		})
		run(t, append([]string{"client", "poll"}, target...)...).want(t, 0).has(t, "12 objects")
		run(t, append([]string{"client", "operate", "--type", "crob", "--index", "1", "--op", "latch-on"}, target...)...).want(t, 0)
		run(t, append([]string{"client", "time-sync"}, target...)...).want(t, 0)
		// A second server cannot take the port.
		run(t, "server", "start", "--listen", "127.0.0.1", "--port", port, "--duration", "1").want(t, 4).errHas(t, "error: cannot listen")
		stop()
		r := (<-done).want(t, 0)
		events := map[string]bool{}
		for _, e := range r.lines(t) {
			events[e["event"].(string)] = true
			if e["timestamp"] == "" {
				t.Errorf("event without timestamp: %v", e)
			}
		}
		for _, want := range []string{"listening", "connected", "request", "control", "time-sync", "unsolicited", "disconnected", "stopping"} {
			if !events[want] {
				t.Errorf("the event log has no %q event:\n%s", want, r.stdout)
			}
		}
		r.errHas(t, "debug:", "link PRI", "app  ", "debug: log ", "component=dnp3.server")
	})

	t.Run("demo outstation for a while", func(t *testing.T) {
		port := strconv.Itoa(int(stationtest.FreePort(t)))
		begin := time.Now()
		r := run(t, "server", "start", "--listen", "127.0.0.1", "--port", port, "--duration", "1").want(t, 0)
		r.has(t, "listening", "Demo feeder RTU, link address 10", "stopping")
		if d := time.Since(begin); d < time.Second || d > 10*time.Second {
			t.Errorf("--duration 1 ran for %s", d)
		}
		run(t, "server", "start", "--listen", "127.0.0.1", "--port", port, "--duration", "1", "--format", "table").want(t, 0).has(t, "TIMESTAMP", "EVENT")
	})

	t.Run("UDP", func(t *testing.T) {
		port := strconv.Itoa(int(stationtest.FreePort(t)))
		ctx, stop := context.WithCancel(context.Background())
		done := make(chan result, 1)
		go func() {
			done <- runCtx(t, ctx, "server", "start", "--listen", "127.0.0.1", "--port", port, "--udp")
		}()
		url := "udp://127.0.0.1:" + port
		eventually(t, "the UDP server to answer", func() bool {
			return run(t, "client", "ping", "--count", "1", "--timeout", "300", "--url", url).code == 0
		})
		run(t, "client", "read", "--url", url, "--object", "g30", "--range", "0-1").want(t, 0).has(t, "udp://127.0.0.1:"+port, "11.02")
		// A master that sends from a fixed port, as outstations configured with a master
		// endpoint expect.
		local := strconv.Itoa(int(stationtest.FreePort(t)))
		run(t, "client", "read", "--url", url, "--object", "g1v2", "--udp-local-port", local).want(t, 0).has(t, "3 objects")
		stop()
		r := (<-done).want(t, 0).has(t, "listening", "udp://127.0.0.1:"+port)
		// The event log names the master the outstation answers, not its own socket.
		remotes := map[string]bool{}
		for _, line := range strings.Split(r.stdout, "\n") {
			if f := strings.Fields(line); len(f) > 2 && f[1] == "request" {
				remotes[f[2]] = true
			}
		}
		if !remotes["127.0.0.1:"+local] || remotes["127.0.0.1:"+port] {
			t.Errorf("remotes of the requests: %v\n%s", remotes, r.stdout)
		}
	})

	t.Run("UDP with a known master", func(t *testing.T) {
		// With one configured master the outstation sends its null unsolicited response at
		// once: --udp-peer tells it where, before the master has sent anything.
		model := filepath.Join(dir, "udp-master.yaml")
		_ = os.WriteFile(model, []byte(strings.Replace(stationtest.Model, "  address: 10\n", "  address: 10\n  masters: [1]\n", 1)), 0o600)
		master, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = master.Close() }()
		port := strconv.Itoa(int(stationtest.FreePort(t)))
		ctx, stop := context.WithCancel(context.Background())
		done := make(chan result, 1)
		go func() {
			done <- runCtx(t, ctx, "server", "start", "--config", model, "--listen", "127.0.0.1", "--port", port, "--udp", "--udp-peer", master.LocalAddr().String())
		}()
		_ = master.SetReadDeadline(time.Now().Add(5 * time.Second))
		buf := make([]byte, 300)
		n, from, err := master.ReadFrom(buf)
		if err != nil || n < 10 || buf[0] != 0x05 || buf[1] != 0x64 || from.String() != "127.0.0.1:"+port {
			t.Errorf("no link frame from the outstation: %d octets from %v, %v", n, from, err)
		}
		stop()
		(<-done).want(t, 0).has(t, master.LocalAddr().String())
	})

	t.Run("secure authentication with key change", func(t *testing.T) {
		keyFile := filepath.Join(dir, "update.key")
		_ = os.WriteFile(keyFile, []byte(strings.Repeat("a5", 16)), 0o600)
		port := strconv.Itoa(int(stationtest.FreePort(t)))
		ctx, stop := context.WithCancel(context.Background())
		done := make(chan result, 1)
		go func() {
			done <- runCtx(t, ctx, "server", "start", "--listen", "127.0.0.1", "--port", port, "--sa-update-key-file", keyFile,
				"--sa-reply-timeout", "5000", "--sa-statistics-class", "3", "--sa-statistics-threshold", "2")
		}()
		target := []string{"--ip", "127.0.0.1", "--port", port}
		eventually(t, "the server to answer", func() bool {
			return run(t, append([]string{"client", "ping", "--count", "1", "--timeout", "300"}, target...)...).code == 0
		})
		run(t, append([]string{"client", "operate", "--type", "crob", "--index", "1", "--op", "latch-on", "--mode", "direct"}, target...)...).want(t, 6)
		run(t, append([]string{"client", "operate", "--type", "crob", "--index", "1", "--op", "latch-on", "--mode", "direct", "--sa-update-key-file", keyFile}, target...)...).want(t, 0)
		run(t, append([]string{"client", "operate", "--type", "crob", "--index", "1", "--op", "latch-off", "--mode", "sbo", "--sa-update-key-file", keyFile, "--sa-aggressive"}, target...)...).
			want(t, 0).has(t, "SUCCESS")
		run(t, append([]string{"client", "read", "--object", "g121v1", "--range", "12-13"}, target...)...).want(t, 0).has(t, "(successful authentications)", "(session key changes)")
		stop()
		(<-done).want(t, 0)
	})

	t.Run("several outstations behind one port", func(t *testing.T) {
		first, second := filepath.Join(dir, "rtu10.yaml"), filepath.Join(dir, "rtu11.yaml")
		_ = os.WriteFile(first, []byte(stationtest.Model), 0o600)
		_ = os.WriteFile(second, []byte("outstation: {name: Second, address: 11}\npoints: [{type: binary-input, index: 0, value: false}, {type: counter, index: 0, value: 5}]\n"), 0o600)
		for _, transport := range []string{"tcp", "udp"} {
			t.Run(transport, func(t *testing.T) {
				port := strconv.Itoa(int(stationtest.FreePort(t)))
				args := []string{"server", "start", "--config", first, "--config", second, "--listen", "127.0.0.1", "--port", port, "--format", "json"}
				target := []string{"--ip", "127.0.0.1", "--port", port}
				if transport == "udp" {
					args = append(args, "--udp")
					target = []string{"--url", "udp://127.0.0.1:" + port}
				}
				// The event log is read while the server runs: a broadcast is not answered, so the
				// test waits for both outstations to log it before stopping the server.
				var stdout, stderr syncBuffer
				ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
				defer stop()
				done := make(chan result, 1)
				go func() {
					code := Run(ctx, args, &stdout, &stderr)
					done <- result{stdout.String(), stderr.String(), code}
				}()
				frozen := func() map[float64]bool {
					seen := map[float64]bool{}
					for _, e := range (result{stdout: stdout.String()}).lines(t) {
						if e["event"] == "request" && e["function"] == "IMMED_FREEZE_NR" {
							seen[e["outstation"].(float64)] = true
						}
					}
					return seen
				}
				eventually(t, "the server to answer", func() bool {
					return run(t, append([]string{"client", "ping", "--count", "1", "--timeout", "300"}, target...)...).code == 0
				})
				// Each outstation answers at its own link address, with its own points.
				var a, b types.PollResult
				run(t, append([]string{"client", "poll", "--outstation", "10", "--format", "json"}, target...)...).want(t, 0).decode(t, &a)
				run(t, append([]string{"client", "poll", "--outstation", "11", "--format", "json"}, target...)...).want(t, 0).decode(t, &b)
				if a.Count != 12 || b.Count != 2 || b.Outstation != 11 {
					t.Errorf("outstation 10: %d values, outstation 11: %d values", a.Count, b.Count)
				}
				run(t, append([]string{"client", "poll", "--outstation", "12", "--timeout", "1000", "--response-timeout", "300"}, target...)...).want(t, 5)
				// A broadcast reaches both.
				run(t, append([]string{"client", "freeze", "--broadcast", "no-confirm", "--no-ack"}, target...)...).want(t, 0)
				eventually(t, "both outstations to log the broadcast freeze", func() bool {
					seen := frozen()
					return seen[10] && seen[11]
				})
				stop()
				r := (<-done).want(t, 0)
				for _, e := range r.lines(t) {
					if e["event"] == "listening" && !strings.Contains(e["detail"].(string), "Test outstation, link address 10; Second, link address 11") {
						t.Errorf("listening: %v", e)
					}
					// Over UDP the event log names the master, as with one outstation, not the
					// server's own socket.
					if remote, _ := e["remote"].(string); transport == "udp" && e["event"] != "listening" && remote == "127.0.0.1:"+port {
						t.Errorf("%s names the server's socket as the remote: %v", e["event"], e)
					}
				}
			})
		}
		run(t, "server", "start", "--config", first+", ,"+first).want(t, 3).errHas(t, "both have link address 10")
		run(t, "server", "start", "--config", first, "--config", second, "--dial", "tcp://127.0.0.1:1").want(t, 3).errHas(t, "serve one outstation")
		run(t, "server", "start", "--config", first, "--config", filepath.Join(dir, "missing.yaml")).want(t, 3)
	})

	t.Run("TLS and allow-list", func(t *testing.T) {
		certs := stationtest.NewCerts(t)
		port := strconv.Itoa(int(stationtest.FreePort(t)))
		ctx, stop := context.WithCancel(context.Background())
		done := make(chan result, 1)
		go func() {
			done <- runCtx(t, ctx, "server", "start", "--listen", "127.0.0.1", "--port", port, "--tls-cert", certs.ServerCert, "--tls-key", certs.ServerKey,
				"--tls-client-ca", certs.CA, "--allow", "127.0.0.1,10.0.0.0/8", "--max-sessions", "4")
		}()
		url := "tls://127.0.0.1:" + port
		eventually(t, "the TLS server to answer", func() bool {
			return run(t, "client", "ping", "--count", "1", "--timeout", "300", "--url", url, "--tls-ca", certs.CA, "--tls-cert", certs.ClientCert, "--tls-key", certs.ClientKey).code == 0
		})
		// Without a client certificate the handshake fails.
		run(t, "client", "poll", "--url", url, "--tls-ca", certs.CA, "--timeout", "1500").want(t, 4)
		stop()
		(<-done).want(t, 0).has(t, "tls://127.0.0.1:"+port)

		// A client that is not on the allow-list is turned away before any exchange.
		ctx, stop = context.WithCancel(context.Background())
		go func() {
			done <- runCtx(t, ctx, "server", "start", "--listen", "127.0.0.1", "--port", port, "--allow", "192.0.2.1")
		}()
		eventually(t, "the server to listen", func() bool {
			conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 200*time.Millisecond)
			if err == nil {
				_ = conn.Close()
			}
			return err == nil
		})
		run(t, "client", "ping", "--count", "1", "--timeout", "500", "--ip", "127.0.0.1", "--port", port).want(t, 4)
		stop()
		(<-done).want(t, 0).has(t, "refused", "not in --allow")
	})

	t.Run("invalid", func(t *testing.T) {
		bad := filepath.Join(dir, "bad.yaml")
		_ = os.WriteFile(bad, []byte("outstation: {address: 65530}\npoints: []\n"), 0o600)
		run(t, "server", "start", "--config", bad).want(t, 3).errHas(t, "invalid model", "address 65530", "at least one point")
		run(t, "server", "start", "--config", filepath.Join(dir, "missing.yaml")).want(t, 3)
		run(t, "server", "start", "--tls-cert", "x.pem").want(t, 3)
		run(t, "server", "start", "--tls-client-ca", "x.pem").want(t, 3)
		run(t, "server", "start", "--tls-cert", bad, "--tls-key", bad).want(t, 3)
		run(t, "server", "start", "--allow", "everyone").want(t, 3)
		run(t, "server", "start", "--udp", "--allow", "10.0.0.1").want(t, 3)
		run(t, "server", "start", "--udp-peer", "127.0.0.1:20000").want(t, 3).errHas(t, "needs --udp")
		run(t, "server", "start", "--udp", "--udp-peer", "nowhere").want(t, 3)
		run(t, "server", "start", "--serial", "/dev/null", "--udp").want(t, 3)
		run(t, "server", "start", "--serial", "/dev/null", "--parity", "mark").want(t, 3)
		run(t, "server", "start", "--sa-key-file", filepath.Join(dir, "missing.key")).want(t, 3)
		run(t, "server", "start", "--format", "xml").want(t, 3)
		run(t, "server", "start", "--sa-reply-timeout", "100").want(t, 3).errHas(t, "need secure authentication")
		run(t, "server", "start", "--sa-statistics-threshold", "5").want(t, 3)
		keyFile := filepath.Join(dir, "invalid-test.key")
		_ = os.WriteFile(keyFile, []byte(strings.Repeat("a5", 16)), 0o600)
		run(t, "server", "start", "--sa-update-key-file", keyFile, "--sa-statistics-class", "4").want(t, 3).errHas(t, "use 1-3")
	})
}
