// SPDX-License-Identifier: MIT

//go:build e2e

package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// model is the outstation the binary serves for these tests: known values, a control and a
// file.
const model = `
outstation:
  name: e2e outstation
  address: 10
points:
  - {type: binary-input, index: 0, value: true, class: 1}
  - {type: binary-output, index: 0, value: false}
  - {type: analog-input, index: 0, value: 42.5, class: 2, static_variation: 5}
controls:
  - {type: crob, index: 0, target: {type: binary-output, index: 0}}
files:
  - {name: /readme.txt, content: "hello from the e2e outstation\n"}
`

// outstation runs `dnp3ctl server start` of the binary under test on a free loopback port
// until the test ends, and returns the --ip and --port flags that reach it.
func outstation(t *testing.T) []string {
	t.Helper()
	bin := os.Getenv("DNP3CTL_E2E_BIN")
	if bin == "" {
		t.Fatal("DNP3CTL_E2E_BIN is not set; run make e2e")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "model.yaml")
	if err := os.WriteFile(file, []byte(model), 0o600); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	_ = ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, "server", "start", "--config", file, "--listen", "127.0.0.1", "--port", port, "--format", "json")
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 5 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	listening := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(stdout)
		once := false
		for sc.Scan() {
			var e struct{ Event string }
			if json.Unmarshal(sc.Bytes(), &e) == nil && e.Event == "listening" && !once {
				once = true
				close(listening)
			}
		}
	}()
	t.Cleanup(func() {
		cancel()
		if err := cmd.Wait(); err != nil && ctx.Err() == nil {
			t.Errorf("server start: %v", err)
		}
	})
	select {
	case <-listening:
	case <-time.After(10 * time.Second):
		t.Fatal("the outstation did not start listening")
	}
	return []string{"--ip", "127.0.0.1", "--port", port, "--timeout", "5000"}
}

func TestReadFromASpawnedOutstation(t *testing.T) {
	target := outstation(t)
	out, code := run(t, append([]string{"client", "read", "--object", "g30", "--format", "json"}, target...)...)
	if code != 0 {
		t.Fatalf("exit code %d\n%s", code, out)
	}
	var res struct {
		Measurements []struct {
			Object string
			Index  int
			Value  float64
		}
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(res.Measurements) != 1 || res.Measurements[0].Value != 42.5 {
		t.Errorf("read: %s", out)
	}
}

func TestOperateASpawnedOutstation(t *testing.T) {
	target := outstation(t)
	out, code := run(t, append([]string{"client", "operate", "--type", "crob", "--index", "0", "--op", "latch-on", "--mode", "sbo"}, target...)...)
	if code != 0 || !strings.Contains(out, "SUCCESS") {
		t.Fatalf("operate: exit code %d\n%s", code, out)
	}
	// The control's target follows.
	out, code = run(t, append([]string{"client", "read", "--object", "g10", "--format", "json"}, target...)...)
	if code != 0 || !strings.Contains(out, `"value": true`) {
		t.Errorf("binary output after the control: exit code %d\n%s", code, out)
	}
	// A control of an index the outstation does not have is refused: exit code 6.
	if out, code := run(t, append([]string{"client", "operate", "--type", "crob", "--index", "9", "--op", "latch-on", "--mode", "direct"}, target...)...); code != 6 {
		t.Errorf("operate of an unknown index: exit code %d\n%s", code, out)
	}
}

func TestFileGetFromASpawnedOutstation(t *testing.T) {
	target := outstation(t)
	local := filepath.Join(t.TempDir(), "readme.txt")
	out, code := run(t, append([]string{"client", "file", "get", "--path", "/readme.txt", "--output", local}, target...)...)
	if code != 0 {
		t.Fatalf("file get: exit code %d\n%s", code, out)
	}
	data, err := os.ReadFile(local)
	if err != nil || string(data) != "hello from the e2e outstation\n" {
		t.Errorf("downloaded %q, %v", data, err)
	}
	if info, err := os.Stat(local); err == nil && info.Mode().Perm() != 0o600 {
		t.Errorf("the downloaded file has mode %v, want 0600", info.Mode().Perm())
	}
	// A file the outstation does not have: exit code 6.
	if out, code := run(t, append([]string{"client", "file", "get", "--path", "/missing", "--output", "-"}, target...)...); code != 6 {
		t.Errorf("file get of a missing file: exit code %d\n%s", code, out)
	}
}

// The one-shot read leaves NEED_TIME and DEVICE_RESTART as the outstation reports them.
func TestOneShotLeavesIndicationsAlone(t *testing.T) {
	target := outstation(t)
	for range 2 {
		out, code := run(t, append([]string{"client", "read", "--object", "g1", "--format", "json"}, target...)...)
		if code != 0 || !strings.Contains(out, "DEVICE_RESTART") {
			t.Fatalf("read: exit code %d\n%s", code, out)
		}
	}
}
