package cmd

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/secauth"
	"github.com/otfabric/go-dnp3/server"

	"github.com/otfabric/dnp3ctl/internal/station"
	"github.com/otfabric/dnp3ctl/internal/stationtest"
	"github.com/otfabric/dnp3ctl/internal/types"
)

func TestPoll(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)

	t.Run("text", func(t *testing.T) {
		cli(t, st, "poll").want(t, 0).has(t,
			"Integrity poll of outstation 10 at tcp://127.0.0.1:", "12 objects", "IIN: ",
			"g1v2    binary-input             0  on", "[ONLINE|CHATTER_FILTER]", "g30v5", "230.5", "[ONLINE|OVER_RANGE]", "hello")
	})
	t.Run("json", func(t *testing.T) {
		var res types.PollResult
		cli(t, st, "poll", "--format", "json").want(t, 0).decode(t, &res)
		if res.Count != 12 || res.Received != 12 || len(res.Measurements) != 12 || res.Outstation != 10 || res.Request != "integrity-poll" ||
			!strings.Contains(res.IIN, "DEVICE_RESTART") || res.Error != nil || len(res.Classes) != 4 {
			t.Fatalf("result %+v", res)
		}
		byKey := map[string]types.Measurement{}
		for _, m := range res.Measurements {
			byKey[m.Type+strconv.Itoa(int(m.Index))] = m
		}
		if m := byKey["binary-input0"]; m.Value != true || m.Flags != "ONLINE" || m.Object != "g1v2" || m.Group != 1 || m.Variation != 2 {
			t.Errorf("binary input 0: %+v", m)
		}
		if m := byKey["double-bit-input0"]; m.Value != "on" {
			t.Errorf("double-bit input 0: %+v", m)
		}
		if m := byKey["analog-input0"]; m.Value != 230.5 || m.Object != "g30v5" {
			t.Errorf("analog input 0: %+v", m)
		}
		if m := byKey["counter0"]; m.Value != float64(123456) {
			t.Errorf("counter 0: %+v", m)
		}
		if m := byKey["analog-output0"]; m.Value != float64(11) || m.Object != "g40v3" {
			t.Errorf("analog output 0: %+v", m)
		}
		if m := byKey["octet-string0"]; m.Value != "hello" || m.Flags != "" {
			t.Errorf("octet string 0: %+v", m)
		}
	})
	t.Run("table", func(t *testing.T) {
		r := cli(t, st, "poll", "--format", "table").want(t, 0).has(t, "OBJECT  TYPE", "g1v2    binary-input")
		if n := len(strings.Split(strings.TrimSpace(r.stdout), "\n")); n != 14 {
			t.Errorf("%d table lines, want a header, a rule and 12 rows:\n%s", n, r.stdout)
		}
	})
	t.Run("modes and filters", func(t *testing.T) {
		var res types.PollResult
		cli(t, st, "poll", "--mode", "class", "--class", "0", "--object", "g30,g1v2", "--index", "0", "--format", "json").want(t, 0).decode(t, &res)
		if res.Request != "class-poll" || res.Received != 12 || res.Count != 2 {
			t.Errorf("filtered class 0: %+v", res)
		}
		cli(t, st, "poll", "--mode", "event", "--format", "json").want(t, 0).decode(t, &res)
		if res.Request != "event-poll" || len(res.Classes) != 3 {
			t.Errorf("event poll: %+v", res)
		}
		cli(t, st, "poll", "--mode", "event", "--class", "1,1,2").want(t, 0).has(t, "Event poll of class 1,2")
		cli(t, st, "poll", "--object", "g1").want(t, 0).has(t, "12 objects", "2 shown")
	})
	t.Run("events", func(t *testing.T) {
		// A change of a class 2 point is an event the next event poll reports, with time.
		if err := st.Server().Database().UpdateAnalogInput(1, app.Analog{Value: 43, Flags: app.FlagOnline}); err != nil {
			t.Fatal(err)
		}
		var res types.PollResult
		cli(t, st, "poll", "--mode", "event", "--class", "2", "--format", "json").want(t, 0).decode(t, &res)
		if res.Count != 1 || !res.Measurements[0].Event || res.Measurements[0].Value != float64(43) || res.Measurements[0].Timestamp == "" || res.Measurements[0].Group != 32 {
			t.Errorf("event: %+v", res)
		}
	})
	t.Run("invalid input", func(t *testing.T) {
		for _, args := range [][]string{
			{"--mode", "sometimes"},
			{"--class", "2"},
			{"--mode", "event", "--class", "0"},
			{"--mode", "class", "--class", "4"},
			{"--object", "g999"},
			{"--object", "g7v99"},
			{"--index", "9-1"},
			{"--format", "xml"},
		} {
			cli(t, st, append([]string{"poll"}, args...)...).want(t, 3)
		}
	})
}

func TestRead(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)

	var res types.PollResult
	cli(t, st, "read", "--object", "g30v1", "--format", "json").want(t, 0).decode(t, &res)
	if res.Request != "read" || res.Count != 2 || res.Objects[0] != "g30v1" || res.Range != "" || res.Measurements[0].Value != 230.5 && res.Measurements[0].Value != float64(231) {
		t.Errorf("read all: %+v", res)
	}
	cli(t, st, "read", "--object", "30.5", "--range", "1", "--format", "json").want(t, 0).decode(t, &res)
	if res.Count != 1 || res.Measurements[0].Index != 1 || res.Range != "1" {
		t.Errorf("read one: %+v", res)
	}
	cli(t, st, "read", "--object", "g1,g10", "--range", "0-1").want(t, 0).has(t, "Read of g1v0,g10v0 0-1", "4 objects", "g10v2")
	cli(t, st, "read", "--object", "g20", "--range", "0,0").want(t, 0).has(t, "123456")
	cli(t, st, "read", "--object", "g2", "--count", "5").want(t, 0).has(t, "count 5")
	cli(t, st, "read", "--object", "g80v1").want(t, 0)

	t.Run("partial", func(t *testing.T) {
		// Points 0-5 of which only 0 and 1 exist: the outstation answers with them and sets
		// PARAMETER_ERROR. The values are listed and the exit code is 7.
		r := cli(t, st, "read", "--object", "g30", "--range", "0-5", "--format", "json").want(t, 7)
		r.decode(t, &res)
		if res.Count != 2 || res.Error == nil || res.Error.Kind != "refused" {
			t.Errorf("partial: %+v", res)
		}
		cli(t, st, "read", "--object", "g30", "--range", "0-5").want(t, 7).has(t, "error: the outstation refused READ")
	})
	t.Run("refused", func(t *testing.T) {
		r := cli(t, st, "read", "--object", "g30", "--range", "7").want(t, 6).errHas(t, "error: the outstation refused READ")
		if r.stdout != "" {
			t.Errorf("stdout after a fatal error: %q", r.stdout)
		}
		kind, code := cli(t, st, "read", "--object", "g30", "--range", "7", "--format", "json").want(t, 6).errorCode(t)
		if kind != "refused" || (code != "parameter_error" && code != "object_unknown") {
			t.Errorf("error %s/%s", kind, code)
		}
	})
	t.Run("invalid input", func(t *testing.T) {
		for _, args := range [][]string{
			{"--object", ""},
			{"--object", "gx"},
			{"--object", "g30", "--range", "a-b"},
			{"--object", "g30", "--range", "5-1"},
			{"--object", "g30", "--range", "1,x"},
			{"--object", "g30", "--range", "1", "--count", "3"},
		} {
			cli(t, st, append([]string{"read"}, args...)...).want(t, 3)
		}
	})
}

func TestOperate(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)
	db := st.Server().Database()
	value := func(pt server.PointType, i uint32) app.Value {
		v, _ := db.Get(pt, i)
		return v
	}

	t.Run("crob, select before operate", func(t *testing.T) {
		var res types.OperateResult
		cli(t, st, "operate", "--type", "crob", "--index", "0", "--op", "latch-off", "--format", "json").want(t, 0).decode(t, &res)
		if res.Status != "SUCCESS" || res.Mode != "sbo" || res.Object != "g12v1" || res.Op != "LATCH_OFF" || res.Count != 1 {
			t.Errorf("result %+v", res)
		}
		if b := value(server.BinaryOutput, 0).(app.Binary); b.Value {
			t.Error("the target did not take the commanded state")
		}
		cli(t, st, "operate", "--type", "crob", "--index", "0", "--op", "close", "--on-time", "200").want(t, 0).
			has(t, "Operate (sbo) g12v1 index 0: PULSE_ON CLOSE count=1 on=200ms off=0ms on outstation 10", "SUCCESS")
		if b := value(server.BinaryOutput, 0).(app.Binary); !b.Value {
			t.Error("close did not set the target")
		}
		cli(t, st, "operate", "--type", "crob", "--index", "3", "--op", "latch-on", "--format", "table").want(t, 0).has(t, "OBJECT", "SUCCESS")
		if b := value(server.BinaryInput, 1).(app.Binary); !b.Value {
			t.Error("a binary input target did not take the commanded state")
		}
	})
	t.Run("select required", func(t *testing.T) {
		r := cli(t, st, "operate", "--type", "crob", "--index", "0", "--op", "trip", "--mode", "direct").want(t, 6)
		r.errHas(t, "error: the outstation refused the control: g12v1 index 0: NO_SELECT")
		kind, code := cli(t, st, "operate", "--type", "crob", "--index", "0", "--op", "trip", "--mode", "direct", "--format", "json").want(t, 6).errorCode(t)
		if kind != "refused" || code != "command_refused" {
			t.Errorf("error %s/%s", kind, code)
		}
		cli(t, st, "operate", "--type", "crob", "--index", "1", "--op", "pulse-on", "--mode", "direct", "--count", "2").want(t, 0).has(t, "count=2")
	})
	t.Run("refusals", func(t *testing.T) {
		cli(t, st, "operate", "--type", "crob", "--index", "2", "--op", "latch-on").want(t, 6).errHas(t, "LOCAL")
		cli(t, st, "operate", "--type", "crob", "--index", "9", "--op", "latch-on").want(t, 6).errHas(t, "NOT_SUPPORTED")
		cli(t, st, "operate", "--type", "analog", "--index", "0", "--value", "150").want(t, 6).errHas(t, "OUT_OF_RANGE")
	})
	t.Run("analog", func(t *testing.T) {
		var res types.OperateResult
		cli(t, st, "operate", "--type", "analog", "--index", "0", "--value", "12.5", "--format", "json").want(t, 0).decode(t, &res)
		if res.Object != "g41v3" || res.Value == nil || *res.Value != 12.5 || res.Status != "SUCCESS" {
			t.Errorf("result %+v", res)
		}
		if a := value(server.AnalogOutput, 0).(app.Analog); a.Value != 12.5 {
			t.Errorf("analog output 0 = %v", a.Value)
		}
		cli(t, st, "operate", "--type", "analog", "--index", "0", "--value", "40").want(t, 0).has(t, "g41v1 index 0: 40")
		cli(t, st, "operate", "--type", "setpoint", "--index", "0", "--value", "41", "--variation", "2", "--mode", "direct").want(t, 0).has(t, "g41v2")
		cli(t, st, "operate", "--type", "analog", "--index", "0", "--value", "42", "--variation", "4").want(t, 0).has(t, "g41v4")
	})
	t.Run("no acknowledgement", func(t *testing.T) {
		var res types.OperateResult
		cli(t, st, "operate", "--type", "crob", "--index", "1", "--op", "latch-on", "--mode", "direct-no-ack", "--format", "json").want(t, 0).decode(t, &res)
		if res.Status != "not-reported" {
			t.Errorf("result %+v", res)
		}
		// Wait for the outstation to finish the request, not only for the value: it logs
		// the control and the request after it has set the value.
		eventually(t, "the control to be handled", func() bool {
			for _, e := range st.Events() {
				if e.Event == "request" && e.Function == "DIRECT_OPERATE_NR" {
					return true
				}
			}
			return false
		})
		if b, _ := value(server.BinaryOutput, 1).(app.Binary); !b.Value {
			t.Error("the control did not set the output")
		}
	})
	t.Run("nothing is sent when the input is wrong", func(t *testing.T) {
		controls := func() int {
			n := 0
			for _, e := range st.Events() {
				// Requests and controls, not connections: a connection of an earlier
				// subtest can be logged late.
				if e.Event == "control" || e.Event == "request" {
					n++
				}
			}
			return n
		}
		before := controls()
		for _, args := range [][]string{
			{"--type", "relay", "--index", "0"},
			{"--type", "crob", "--index", "0"},
			{"--type", "crob", "--index", "0", "--op", "smash"},
			{"--type", "crob", "--index", "0", "--op", "latch-on", "--value", "1"},
			{"--type", "crob", "--index", "0", "--op", "latch-on", "--count", "0"},
			{"--type", "crob", "--index", "0", "--op", "latch-on", "--mode", "now"},
			{"--type", "analog", "--index", "0"},
			{"--type", "analog", "--index", "0", "--value", "x"},
			{"--type", "analog", "--index", "0", "--value", "1", "--op", "latch-on"},
			{"--type", "analog", "--index", "0", "--value", "1.5", "--variation", "1"},
			{"--type", "analog", "--index", "0", "--value", "70000", "--variation", "2"},
			{"--type", "analog", "--index", "0", "--value", "1", "--variation", "9"},
		} {
			cli(t, st, append([]string{"operate"}, args...)...).want(t, 3)
		}
		if after := controls(); after != before {
			t.Errorf("an invalid invocation reached the outstation: %d requests or controls", after-before)
		}
		run(t, "client", "operate", "--ip", "127.0.0.1", "--type", "crob").want(t, 2)
	})
}

func TestSystemRequests(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)

	t.Run("time-sync", func(t *testing.T) {
		var res types.ConfirmedResult
		cli(t, st, "time-sync", "--format", "json").want(t, 0).decode(t, &res)
		if res.Request != "time-sync" || res.Method != "lan" || res.Time == "" {
			t.Errorf("lan: %+v", res)
		}
		cli(t, st, "time", "--method", "non-lan").want(t, 0).has(t, "time-sync (non-lan")
		cli(t, st, "time-sync", "--method", "write", "--time", "2026-01-31T12:00:00Z", "--format", "table").want(t, 0).has(t, "write 2026-01-31T12:00:00.000Z")
		eventually(t, "the clock to be set", func() bool {
			return st.Server().Now().Year() == 2026 && st.Server().Now().Month() == time.January
		})
		cli(t, st, "time-sync", "--method", "never").want(t, 3)
		cli(t, st, "time-sync", "--time", "2026-01-31T12:00:00Z").want(t, 3)
		cli(t, st, "time-sync", "--method", "write", "--time", "yesterday").want(t, 3)
	})
	t.Run("restart", func(t *testing.T) {
		var res types.ConfirmedResult
		cli(t, st, "restart", "--type", "warm", "--format", "json").want(t, 0).decode(t, &res)
		if res.Request != "warm-restart" || res.DelayMs == nil || *res.DelayMs != 1000 {
			t.Errorf("warm: %+v", res)
		}
		cli(t, st, "restart", "--type", "cold").want(t, 0).has(t, "cold-restart (poll again after 1000 ms)")
		cli(t, st, "restart", "--type", "hot").want(t, 3)
		run(t, "client", "restart", "--ip", "127.0.0.1").want(t, 2)
		eventually(t, "the restart to be done", func() bool {
			n := 0
			for _, e := range st.Events() {
				if e.Event == "restart" && strings.Contains(e.Detail, "done") {
					n++
				}
			}
			return n == 2
		})
	})
	t.Run("restart refused", func(t *testing.T) {
		other := stationtest.Start(t, strings.Replace(stationtest.Model, "  restart_delay: 1s\n", "", 1))
		cli(t, other, "restart", "--type", "warm").want(t, 6)
	})
	t.Run("unsolicited", func(t *testing.T) {
		var res types.ConfirmedResult
		cli(t, st, "unsolicited", "enable", "--format", "json").want(t, 0).decode(t, &res)
		if res.Request != "enable-unsolicited" || len(res.Classes) != 3 {
			t.Errorf("enable: %+v", res)
		}
		cli(t, st, "unsolicited", "disable", "--class", "2").want(t, 0).has(t, "disable-unsolicited (class 2)")
		cli(t, st, "unsolicited", "enable", "--class", "0").want(t, 3)
	})
	t.Run("assign-class", func(t *testing.T) {
		var res types.ConfirmedResult
		cli(t, st, "assign-class", "--class", "3", "--object", "g30", "--range", "0-1", "--format", "json").want(t, 0).decode(t, &res)
		if res.Request != "assign-class" || res.Class == nil || *res.Class != 3 || res.Objects[0] != "g30v0" || res.Range != "0-1" {
			t.Errorf("assign: %+v", res)
		}
		cli(t, st, "assign-class", "--class", "0", "--object", "g1").want(t, 0).has(t, "g1v0 to class 0")
		cli(t, st, "assign-class", "--class", "4", "--object", "g1").want(t, 3)
		cli(t, st, "assign-class", "--class", "1", "--object", "g32").want(t, 3).errHas(t, "event object")
		run(t, "client", "assign-class", "--ip", "127.0.0.1", "--class", "1").want(t, 2)
	})
	t.Run("freeze", func(t *testing.T) {
		var res types.ConfirmedResult
		cli(t, st, "freeze", "--format", "json").want(t, 0).decode(t, &res)
		if res.Request != "freeze" || res.Objects[0] != "g20v0" {
			t.Errorf("freeze: %+v", res)
		}
		eventually(t, "the frozen counter", func() bool {
			v, _ := st.Server().Database().Get(server.FrozenCounter, 0)
			c, _ := v.(app.Counter)
			return c.Value == 123456
		})
		cli(t, st, "freeze", "--clear", "--range", "0").want(t, 0).has(t, "freeze-clear (g20v0 0)")
		cli(t, st, "freeze", "--no-ack").want(t, 0).has(t, "sent without acknowledgement")
		cli(t, st, "freeze", "--object", "g22").want(t, 3)
	})
	t.Run("attributes", func(t *testing.T) {
		var res types.AttributesResult
		cli(t, st, "attributes", "--format", "json").want(t, 0).decode(t, &res)
		names := map[string]any{}
		for _, a := range res.Attributes {
			names[a.Name] = a.Value
		}
		if names["manufacturer"] != "otfabric" || names["serial-number"] != "SN-1" || res.Count != len(res.Attributes) {
			t.Errorf("attributes: %+v", res)
		}
		cli(t, st, "attributes", "--variation", "manufacturer").want(t, 0).has(t, "252  manufacturer", "VSTR  otfabric")
		cli(t, st, "attributes", "--variation", "list").want(t, 0).has(t, "LIST", "252(")
		cli(t, st, "attrs", "--variation", "250", "--format", "table").want(t, 0).has(t, "VARIATION", "product", "test outstation")
		cli(t, st, "attributes", "--variation", "favourite-colour").want(t, 3)
	})
	t.Run("ping", func(t *testing.T) {
		r := cli(t, st, "ping", "--count", "2", "--interval", "10").want(t, 0).has(t, "seq=1", "seq=2", "rtt=")
		r.errHas(t, "delay measurement: 2 sent, 0 failed, rtt min/avg/max")
		lines := cli(t, st, "ping", "--count", "1", "--format", "json").want(t, 0).lines(t)
		if len(lines) != 1 || lines[0]["seq"] != float64(1) || lines[0]["error"] != nil {
			t.Errorf("ping json: %v", lines)
		}
		cli(t, st, "ping", "--count", "1", "--format", "table").want(t, 0).has(t, "TIMESTAMP", "PROCESSING_MS")
		cli(t, st, "ping", "--interval", "1").want(t, 3)
	})
	t.Run("objects", func(t *testing.T) {
		var res types.ObjectsResult
		run(t, "client", "objects", "--format", "json").want(t, 0).decode(t, &res)
		if res.Count < 100 || res.Count != len(res.Objects) {
			t.Errorf("%d objects", res.Count)
		}
		run(t, "client", "objects", "--group", "30").want(t, 0).has(t, "g30v5    Analog Input - Single-precision With Flag")
		r := run(t, "client", "objects", "--events", "--format", "table").want(t, 0).has(t, "g32v7", "true")
		if strings.Contains(r.stdout, "g30v5") {
			t.Error("--events lists a static object")
		}
	})
}

func TestFiles(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)
	dir := t.TempDir()

	t.Run("ls and info", func(t *testing.T) {
		var res types.FileListResult
		cli(t, st, "file", "ls", "--format", "json").want(t, 0).decode(t, &res)
		if res.Path != "/" || res.Count != 2 || res.Entries[0].Name != "readme.txt" || res.Entries[1].Type != "directory" {
			t.Errorf("ls: %+v", res)
		}
		cli(t, st, "file", "ls", "--path", "/records").want(t, 0).has(t, "big.dat", "5000")
		cli(t, st, "file", "ls", "--path", "/records", "--format", "table").want(t, 0).has(t, "NAME", "big.dat")
		var info types.FileInfoResult
		cli(t, st, "file", "info", "--path", "/records/big.dat", "--format", "json").want(t, 0).decode(t, &info)
		if info.Entry.Size != 5000 || info.Entry.Type != "file" || info.Entry.Permissions != "rw-r--r--" {
			t.Errorf("info: %+v", info)
		}
		cli(t, st, "file", "info", "--path", "/records").want(t, 0).has(t, "directory")
		cli(t, st, "file", "info", "--path", "/nope").want(t, 6).errHas(t, "FILE_NOT_FOUND")
		cli(t, st, "file", "info", "--path", "/nope", "--format", "table").want(t, 6)
	})
	t.Run("get", func(t *testing.T) {
		out := filepath.Join(dir, "big.dat")
		var res types.FileTransferResult
		cli(t, st, "file", "get", "--path", "/records/big.dat", "--output", out, "--format", "json").want(t, 0).decode(t, &res)
		data, err := os.ReadFile(out)
		if err != nil || len(data) != 5000 || res.Length != 5000 || res.Direction != "download" || res.Local != out || len(res.SHA256) != 64 {
			t.Fatalf("get: %+v, %d octets, %v", res, len(data), err)
		}
		cli(t, st, "file", "get", "--path", "/records/big.dat", "--output", out).want(t, 3).errHas(t, "pass --force")
		cli(t, st, "file", "get", "--path", "/records/big.dat", "--output", out, "--force", "--block-size", "512").want(t, 0).has(t, "Fetched /records/big.dat", "5000 octets")
		r := cli(t, st, "file", "get", "--path", "/readme.txt", "--output", "-").want(t, 0)
		if r.stdout != "hello from the outstation\n" || !strings.Contains(r.stderr, "sha256:") {
			t.Errorf("get to stdout: %q / %q", r.stdout, r.stderr)
		}
		// Without --output the file is named after the path, in the current directory.
		wd, _ := os.Getwd()
		t.Chdir(dir)
		cli(t, st, "file", "get", "--path", "/readme.txt").want(t, 0)
		if _, err := os.Stat(filepath.Join(dir, "readme.txt")); err != nil {
			t.Error(err)
		}
		t.Chdir(wd)
		cli(t, st, "file", "get", "--path", "/nope", "--output", filepath.Join(dir, "x")).want(t, 6)
		cli(t, st, "file", "get", "--path", "/", "--output", "").want(t, 3)
		cli(t, st, "file", "get", "--path", "/readme.txt", "--output", filepath.Join(dir, "no", "dir", "x")).want(t, 10)
	})
	t.Run("put and rm", func(t *testing.T) {
		in := filepath.Join(dir, "settings.xml")
		content := bytes.Repeat([]byte("<setting/>\n"), 300)
		if err := os.WriteFile(in, content, 0o600); err != nil {
			t.Fatal(err)
		}
		var res types.FileTransferResult
		cli(t, st, "file", "put", "--input", in, "--path", "/config/settings.xml", "--permissions", "rw-------", "--format", "json").want(t, 0).decode(t, &res)
		if res.Length != int64(len(content)) || res.Direction != "upload" {
			t.Errorf("put: %+v", res)
		}
		cli(t, st, "file", "info", "--path", "/config/settings.xml").want(t, 0).has(t, "rw-------", strconv.Itoa(len(content)))
		r := cli(t, st, "file", "get", "--path", "/config/settings.xml", "--output", "-").want(t, 0)
		if r.stdout != string(content) {
			t.Error("the file read back differs from the one written")
		}
		cli(t, st, "file", "put", "--input", in, "--path", "/config/s2.xml", "--permissions", "600", "--block-size", "256").want(t, 0).has(t, "Delivered")
		cli(t, st, "file", "rm", "--path", "/config/s2.xml").want(t, 0).has(t, "delete-file (/config/s2.xml)")
		cli(t, st, "file", "rm", "--path", "/config/s2.xml").want(t, 6).errHas(t, "FILE_NOT_FOUND")

		for _, args := range [][]string{
			{"put", "--input", filepath.Join(dir, "missing"), "--path", "/x"},
			{"put", "--input", dir, "--path", "/x"},
			{"put", "--input", in, "--path", "/x", "--permissions", "rwxrwxrwz"},
			{"put", "--input", in, "--path", " "},
		} {
			cli(t, st, append([]string{"file"}, args...)...).want(t, 3)
		}
	})
	t.Run("read-only outstation", func(t *testing.T) {
		ro := stationtest.Start(t, strings.Replace(stationtest.Model, "uploads: {accept: true}", "", 1))
		in := filepath.Join(dir, "settings.xml")
		cli(t, ro, "file", "put", "--input", in, "--path", "/x").want(t, 6).errHas(t, "PERMISSION_DENIED")
		cli(t, ro, "file", "rm", "--path", "/readme.txt").want(t, 6)
	})
	t.Run("credentials", func(t *testing.T) {
		pw := filepath.Join(dir, "password")
		_ = os.WriteFile(pw, []byte("secret\n"), 0o600)
		// The outstation has no authenticator: it refuses the credentials.
		cli(t, st, "file", "ls", "--file-user", "operator", "--file-password-file", pw).want(t, 6).errHas(t, "refused file operation authenticate")
		t.Setenv("DNP3CTL_FILE_PASSWORD", "secret")
		cli(t, st, "file", "ls", "--file-user", "operator").want(t, 6)
		cli(t, st, "file", "ls", "--file-user", "operator", "--file-password-file", pw).want(t, 3).errHas(t, "mutually exclusive")
		cli(t, st, "file", "ls").want(t, 3).errHas(t, "needs --file-user")
		t.Setenv("DNP3CTL_FILE_PASSWORD", "")
		cli(t, st, "file", "ls", "--file-user", "operator", "--file-password-file", filepath.Join(dir, "missing")).want(t, 3)
	})
}

func TestMonitor(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)

	t.Run("integrity first", func(t *testing.T) {
		r := cli(t, st, "monitor", "--integrity", "--count", "3", "--format", "json").want(t, 0)
		lines := r.lines(t)
		if len(lines) != 3 || lines[0]["received_at"] == "" || lines[0]["object"] == "" {
			t.Errorf("monitor: %v", lines)
		}
		r.errHas(t, "monitor: connected", "3 values")
	})
	t.Run("unsolicited changes", func(t *testing.T) {
		done := make(chan result, 1)
		go func() {
			done <- cli(t, st, "monitor", "--object", "g32", "--count", "2", "--format", "json")
		}()
		eventually(t, "unsolicited responses to be enabled", func() bool {
			for _, e := range st.Events() {
				if e.Function == "ENABLE_UNSOLICITED" {
					return true
				}
			}
			return false
		})
		for i := 0; i < 2; i++ {
			_ = st.Server().Database().UpdateAnalogInput(1, app.Analog{Value: float64(100 + i), Flags: app.FlagOnline})
			time.Sleep(50 * time.Millisecond)
		}
		r := (<-done).want(t, 0)
		lines := r.lines(t)
		if len(lines) != 2 || lines[0]["unsolicited"] != true || lines[0]["value"] != float64(100) || lines[1]["value"] != float64(101) {
			t.Errorf("unsolicited: %v", lines)
		}
	})
	t.Run("outstation restart", func(t *testing.T) {
		other := stationtest.Start(t, stationtest.Model)
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
		go func() { done <- cli(t, other, "monitor", "--object", "g32", "--count", "1", "--format", "json") }()
		eventually(t, "unsolicited responses to be enabled", func() bool { return enables() == 1 })
		eventually(t, "DEVICE_RESTART to be cleared", func() bool {
			for _, e := range other.Events() {
				if e.Function == "WRITE" {
					return true
				}
			}
			return false
		})
		// After a restart the outstation has forgotten the enabled classes: the monitor
		// enables them again, and events keep coming.
		other.Server().Restart()
		eventually(t, "unsolicited responses to be enabled again", func() bool { return enables() == 2 })
		_ = other.Server().Database().UpdateAnalogInput(1, app.Analog{Value: 7, Flags: app.FlagOnline})
		r := (<-done).want(t, 0).errHas(t, "unsolicited responses enabled again")
		if lines := r.lines(t); len(lines) != 1 || lines[0]["unsolicited"] != true {
			t.Errorf("after the restart: %v", lines)
		}
	})
	t.Run("poll interval, duration and table", func(t *testing.T) {
		cli(t, st, "monitor", "--unsolicited", "none", "--poll-interval", "1", "--duration", "1", "--index", "0", "--format", "table").want(t, 0)
		cli(t, st, "monitor", "--integrity", "--count", "1").want(t, 0).has(t, "resp")
		cli(t, st, "--debug", "monitor", "--integrity", "--count", "1").want(t, 0).errHas(t, "debug:", "link PRI", "app  ")
	})
	t.Run("connection lost", func(t *testing.T) {
		other := stationtest.Start(t, stationtest.Model)
		done := make(chan result, 1)
		go func() { done <- cli(t, other, "monitor", "--no-reconnect") }()
		eventually(t, "the monitor to connect", func() bool { return other.HasEvent("connected") })
		other.Server().CloseConnections()
		(<-done).want(t, 4).errHas(t, "error: connection error")
	})
	t.Run("invalid", func(t *testing.T) {
		cli(t, st, "monitor", "--unsolicited", "4").want(t, 3)
		cli(t, st, "monitor", "--object", "gx").want(t, 3)
		cli(t, st, "monitor", "--index", "x").want(t, 3)
	})
}

func TestTargetsAndErrors(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)
	port := strconv.Itoa(int(st.Port))

	t.Run("url", func(t *testing.T) {
		run(t, "client", "poll", "--url", "tcp://127.0.0.1:"+port).want(t, 0).has(t, "tcp://127.0.0.1:"+port)
	})
	t.Run("environment", func(t *testing.T) {
		t.Setenv("DNP3CTL_URL", "tcp://127.0.0.1:"+port)
		t.Setenv("DNP3CTL_OUTSTATION", "10")
		run(t, "client", "read", "--object", "g1v2").want(t, 0).has(t, "2 objects")
	})
	t.Run("invalid", func(t *testing.T) {
		for _, args := range [][]string{
			{},
			{"--url", "http://x"},
			{"--url", "tcp://x", "--ip", "y"},
			{"--url", "serial://"},
			{"--url", "tcp://127.0.0.1", "--tls-insecure"},
			{"--url", "tcp://127.0.0.1", "--udp-local-port", "20000"},
			{"--ip", "127.0.0.1", "--master", "10"},
			{"--ip", "127.0.0.1", "--outstation", "65530"},
			{"--ip", "127.0.0.1", "--master", "65532"},
			{"--url", "serial:///dev/ttyX", "--parity", "mark"},
			{"--ip", "127.0.0.1", "--tls-cert", "x.pem"},
			{"--ip", "127.0.0.1", "--tls-ca", "missing.pem"},
		} {
			kind, code := run(t, append([]string{"client", "poll", "--format", "json"}, args...)...).want(t, 3).errorCode(t)
			if kind != "invalid_input" || code == "" {
				t.Errorf("%v: %s/%s", args, kind, code)
			}
		}
	})
	t.Run("nothing listening", func(t *testing.T) {
		free := strconv.Itoa(int(stationtest.FreePort(t)))
		kind, code := run(t, "client", "poll", "--ip", "127.0.0.1", "--port", free, "--format", "json").want(t, 4).errorCode(t)
		if kind != "transport" || code != "transport_connect_failed" {
			t.Errorf("error %s/%s", kind, code)
		}
	})
	t.Run("wrong outstation address", func(t *testing.T) {
		kind, code := cli(t, st, "poll", "--outstation", "4", "--response-timeout", "200", "--timeout", "1000", "--format", "json").want(t, 5).errorCode(t)
		if kind != "timeout" || code != "request_timeout" {
			t.Errorf("error %s/%s", kind, code)
		}
		cli(t, st, "read", "--object", "g1", "--outstation", "4", "--response-timeout", "100", "--timeout", "2000", "--retries", "2").want(t, 5)
	})
	t.Run("debug", func(t *testing.T) {
		cli(t, st, "--debug", "read", "--object", "g1").want(t, 0).errHas(t, "debug:", "-> ", "link PRI UNCONFIRMED_USER_DATA", "app  ")
		run(t, "--debug", "client", "poll", "--ip", "127.0.0.1", "--port", strconv.Itoa(int(stationtest.FreePort(t)))).want(t, 4).errHas(t, "cause: ")
	})
	t.Run("link layer settings", func(t *testing.T) {
		cli(t, st, "poll", "--link-confirmed", "--link-timeout", "1000", "--link-retries", "1", "--keep-alive", "30").want(t, 0)
	})
}

func TestTLS(t *testing.T) {
	certs := stationtest.NewCerts(t)
	st := stationtest.Start(t, stationtest.Model, func(o *station.Options) { o.TLS = certs.ServerTLS })
	url := "tls://" + st.Addr()
	tlsFlags := []string{"--url", url, "--tls-ca", certs.CA, "--tls-cert", certs.ClientCert, "--tls-key", certs.ClientKey}

	run(t, append([]string{"client", "poll"}, tlsFlags...)...).want(t, 0).has(t, url, "12 objects")
	run(t, append([]string{"client", "poll", "--ip", "127.0.0.1", "--port", strconv.Itoa(int(st.Port))}, tlsFlags[2:]...)...).want(t, 0).has(t, url)
	run(t, "client", "poll", "--url", url, "--tls-insecure", "--tls-cert", certs.ClientCert, "--tls-key", certs.ClientKey).want(t, 0)
	kind, code := run(t, "client", "poll", "--url", url, "--format", "json", "--tls-cert", certs.ClientCert, "--tls-key", certs.ClientKey).want(t, 4).errorCode(t)
	if kind != "transport" || code != "transport_tls_failed" {
		t.Errorf("unknown CA: %s/%s", kind, code)
	}
	run(t, "client", "poll", "--url", url, "--tls-ca", certs.CA, "--tls-server-name", "rtu.example.net", "--tls-cert", certs.ClientCert, "--tls-key", certs.ClientKey).want(t, 4)
}

func TestSecureAuthentication(t *testing.T) {
	key := bytes.Repeat([]byte{0x5a}, 32)
	st := stationtest.Start(t, stationtest.Model, func(o *station.Options) { o.Auth = &secauth.User{Number: 1, Key: key} })
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "sa.key")
	_ = os.WriteFile(keyFile, []byte(hex.EncodeToString(key)+"\n"), 0o600)

	// Reads are not critical: they work without a key.
	cli(t, st, "poll").want(t, 0)
	// A control is challenged: without the key it fails, with it it is executed.
	cli(t, st, "operate", "--type", "crob", "--index", "1", "--op", "latch-on", "--mode", "direct").want(t, 6).errHas(t, "secure authentication")
	cli(t, st, "operate", "--type", "crob", "--index", "1", "--op", "latch-on", "--mode", "direct", "--sa-key-file", keyFile).want(t, 0).has(t, "SUCCESS")
	t.Setenv("DNP3CTL_SA_KEY", hex.EncodeToString(key))
	cli(t, st, "operate", "--type", "crob", "--index", "1", "--op", "latch-off", "--mode", "sbo", "--sa-mac", "hmac-sha256-16").want(t, 0)
	cli(t, st, "restart", "--type", "warm", "--sa-user", "1").want(t, 0)
	kind, code := cli(t, st, "operate", "--type", "crob", "--index", "1", "--op", "latch-on", "--mode", "direct", "--sa-key-file", keyFile, "--format", "json").want(t, 3).errorCode(t)
	if kind != "invalid_input" || code != "invalid_flag_combination" {
		t.Errorf("both key sources: %s/%s", kind, code)
	}
	t.Setenv("DNP3CTL_SA_KEY", hex.EncodeToString(bytes.Repeat([]byte{0x11}, 32)))
	kind, code = cli(t, st, "operate", "--type", "crob", "--index", "1", "--op", "latch-on", "--mode", "direct", "--format", "json").want(t, 6).errorCode(t)
	if kind != "refused" || code != "authentication_failed" {
		t.Errorf("wrong key: %s/%s", kind, code)
	}
	for _, k := range []string{"zz", "0011", ""} {
		t.Setenv("DNP3CTL_SA_KEY", k)
		if k == "" {
			cli(t, st, "poll", "--sa-mac", "hmac-sha1-10").want(t, 3).errHas(t, "--sa-mac needs a key")
			continue
		}
		kind, code := cli(t, st, "poll", "--format", "json").want(t, 3).errorCode(t)
		if code != "invalid_key" {
			t.Errorf("key %q: %s/%s", k, kind, code)
		}
	}
	t.Setenv("DNP3CTL_SA_KEY", hex.EncodeToString(key))
	cli(t, st, "poll", "--sa-mac", "md5").want(t, 3)
	cli(t, st, "poll", "--sa-key-file", "", "--sa-user", "0").want(t, 0)
	cli(t, st, "poll", "--sa-key-change-interval", "60").want(t, 3).errHas(t, "needs an update key")
}

// With an update key the session keys are negotiated by the session key change procedure.
func TestSecureAuthenticationKeyChange(t *testing.T) {
	update := bytes.Repeat([]byte{0x3c}, 32)
	st := stationtest.Start(t, stationtest.Model, func(o *station.Options) { o.Auth = &secauth.User{Number: 1, UpdateKey: update} })
	dir := t.TempDir()
	updateFile := filepath.Join(dir, "update.key")
	_ = os.WriteFile(updateFile, []byte(hex.EncodeToString(update)), 0o600)

	cli(t, st, "operate", "--type", "crob", "--index", "1", "--op", "latch-on", "--mode", "sbo", "--sa-update-key-file", updateFile, "--sa-key-change-interval", "60").
		want(t, 0).has(t, "SUCCESS")
	t.Setenv("DNP3CTL_SA_UPDATE_KEY", hex.EncodeToString(update))
	cli(t, st, "restart", "--type", "warm").want(t, 0)
	cli(t, st, "poll", "--sa-update-key-file", updateFile).want(t, 3).errHas(t, "mutually exclusive")
	t.Setenv("DNP3CTL_SA_KEY", hex.EncodeToString(update))
	cli(t, st, "poll").want(t, 3).errHas(t, "not both")
	t.Setenv("DNP3CTL_SA_KEY", "")

	// A wrong update key fails the key change, and with it every critical request.
	t.Setenv("DNP3CTL_SA_UPDATE_KEY", hex.EncodeToString(bytes.Repeat([]byte{0x11}, 32)))
	kind, code := cli(t, st, "operate", "--type", "crob", "--index", "1", "--op", "latch-off", "--mode", "direct", "--format", "json").want(t, 6).errorCode(t)
	if kind != "refused" || code != "authentication_failed" {
		t.Errorf("wrong update key: %s/%s", kind, code)
	}
	for _, k := range []string{"0011", strings.Repeat("00", 20), "zz"} {
		t.Setenv("DNP3CTL_SA_UPDATE_KEY", k)
		if _, code := cli(t, st, "poll", "--format", "json").want(t, 3).errorCode(t); code != "invalid_key" {
			t.Errorf("update key %q: %s", k, code)
		}
	}
	t.Setenv("DNP3CTL_SA_UPDATE_KEY", "")
	cli(t, st, "poll", "--sa-update-key-file", filepath.Join(dir, "missing")).want(t, 3)
}

// Broadcast, freeze at a time, attribute sets, file append and deadbands: what go-dnp3
// v1.1.0 adds and dnp3ctl exposes.
func TestBroadcastAndFreezeAtTime(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)
	db := st.Server().Database()

	t.Run("broadcast operate", func(t *testing.T) {
		var res types.OperateResult
		cli(t, st, "operate", "--type", "crob", "--index", "1", "--op", "latch-on", "--mode", "direct-no-ack", "--broadcast", "no-confirm", "--format", "json").
			want(t, 0).decode(t, &res)
		if res.Broadcast != "no-confirm" || res.Status != "not-reported" {
			t.Errorf("result %+v", res)
		}
		eventually(t, "the broadcast control to be executed", func() bool {
			v, _ := db.Get(server.BinaryOutput, 1)
			b, _ := v.(app.Binary)
			return b.Value
		})
		cli(t, st, "operate", "--type", "crob", "--index", "1", "--op", "latch-off", "--mode", "direct", "--broadcast", "confirm").
			want(t, 0).has(t, "every outstation (broadcast, confirm)", "not-reported")
		cli(t, st, "operate", "--type", "crob", "--index", "1", "--op", "latch-on", "--broadcast", "optional").want(t, 3).errHas(t, "--mode direct")
		cli(t, st, "operate", "--type", "crob", "--index", "1", "--op", "latch-on", "--mode", "direct", "--broadcast", "everyone").want(t, 3)
		// With secure authentication a broadcast cannot be authenticated: it is refused
		// before anything is sent.
		t.Setenv("DNP3CTL_SA_UPDATE_KEY", strings.Repeat("ab", 16))
		cli(t, st, "operate", "--type", "crob", "--index", "1", "--op", "latch-on", "--mode", "direct", "--broadcast", "no-confirm").want(t, 3).errHas(t, "cannot be used with secure authentication")
		cli(t, st, "freeze", "--broadcast", "no-confirm").want(t, 3).errHas(t, "cannot be used with secure authentication")
		t.Setenv("DNP3CTL_SA_UPDATE_KEY", "")
	})
	t.Run("broadcast freeze", func(t *testing.T) {
		var res types.ConfirmedResult
		cli(t, st, "freeze", "--broadcast", "optional", "--no-ack", "--format", "json").want(t, 0).decode(t, &res)
		if res.Broadcast != "optional" || res.Request != "freeze" {
			t.Errorf("result %+v", res)
		}
		eventually(t, "the broadcast freeze", func() bool {
			v, _ := db.Get(server.FrozenCounter, 0)
			c, _ := v.(app.Counter)
			return c.Value == 123456
		})
		cli(t, st, "freeze", "--broadcast", "no-confirm", "--clear").want(t, 0).has(t, "freeze-clear", "a broadcast is not answered")
		cli(t, st, "freeze", "--broadcast", "confirm", "--format", "table").want(t, 0).has(t, "sent")
	})
	t.Run("freeze at time", func(t *testing.T) {
		at := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		var res types.ConfirmedResult
		cli(t, st, "freeze", "--at", at, "--interval", "15m", "--format", "json").want(t, 0).decode(t, &res)
		if res.Request != "freeze-at-time" || res.Interval != "15m0s" || res.Time == "" {
			t.Errorf("result %+v", res)
		}
		cli(t, st, "freeze", "--object", "g30", "--at", at).want(t, 0).has(t, "freeze-at-time (g30v0 at ")
		// A time that has passed, without an interval, is refused by the outstation.
		cli(t, st, "freeze", "--at", "2020-01-01T00:00:00Z").want(t, 6)
		for _, args := range [][]string{
			{"--interval", "1h"},
			{"--at", at, "--clear"},
			{"--at", at, "--no-ack"},
			{"--at", at, "--broadcast", "confirm"},
			{"--at", "tomorrow"},
			{"--at", at, "--interval", "often"},
			{"--at", at, "--interval", "-1h"},
			{"--broadcast", "everyone"},
		} {
			cli(t, st, append([]string{"freeze"}, args...)...).want(t, 3)
		}
	})
}

func TestAttributeSetsAppendDeadbands(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)

	t.Run("read a set", func(t *testing.T) {
		var res types.AttributesResult
		cli(t, st, "attributes", "--set", "1", "--format", "json").want(t, 0).decode(t, &res)
		values := map[uint8]any{}
		for _, a := range res.Attributes {
			values[a.Variation] = a.Value
		}
		if res.Set != 1 || values[1] != "feeder" || values[2] != float64(42) || values[3] != float64(-5) || values[4] != 1.5 {
			t.Errorf("set 1: %+v", res)
		}
		cli(t, st, "attributes", "--set", "1").want(t, 0).has(t, "Device attributes (set 1)", "attribute-1", "feeder")
		cli(t, st, "attributes", "--set", "1", "--variation", "list").want(t, 0).has(t, "1(rw)", "3(r)")
		cli(t, st, "attributes", "--set", "1", "--variation", "manufacturer").want(t, 3).errHas(t, "names are those of set 0")
		cli(t, st, "attributes", "--set", "9").want(t, 6)
	})
	t.Run("write", func(t *testing.T) {
		var res types.ConfirmedResult
		cli(t, st, "write-attribute", "--set", "1", "--variation", "1", "--value", "feeder 7b", "--format", "json").want(t, 0).decode(t, &res)
		if res.Request != "write-attribute" || res.Set == nil || *res.Set != 1 || res.Attribute == nil || res.Attribute.Value != "feeder 7b" {
			t.Errorf("result %+v", res)
		}
		cli(t, st, "attributes", "--set", "1", "--variation", "1").want(t, 0).has(t, "feeder 7b")
		cli(t, st, "write-attribute", "--set", "1", "--variation", "2", "--value", "43", "--data-type", "uint").want(t, 0).has(t, "set 1 variation 2 attribute-2 = 43")
		// Variation 3 is not writable, and variation 2 takes a number.
		cli(t, st, "write-attribute", "--set", "1", "--variation", "3", "--value", "-6", "--data-type", "int").want(t, 6)
		cli(t, st, "write-attribute", "--set", "1", "--variation", "2", "--value", "text").want(t, 6)
		// In set 0 the user-assigned attributes go to the outstation's write handler.
		cli(t, st, "write-attribute", "--variation", "location", "--value", "Bay 7", "--format", "table").want(t, 0).has(t, "write-attribute", "confirmed")
		eventually(t, "the write to be logged", func() bool {
			for _, e := range st.Events() {
				if e.Event == "write" && e.Detail == "attribute set 1" {
					return true
				}
			}
			return false
		})
		for _, args := range [][]string{
			{"--variation", "all", "--value", "x"},
			{"--variation", "254", "--value", "x"},
			{"--set", "1", "--variation", "location", "--value", "x"},
			{"--variation", "245", "--value", "x", "--data-type", "blob"},
			{"--variation", "245", "--value", "x", "--data-type", "uint"},
			{"--variation", "245", "--value", "4294967296", "--data-type", "uint"},
			{"--variation", "245", "--value", "x", "--data-type", "int"},
			{"--variation", "245", "--value", "x", "--data-type", "float"},
			{"--variation", "245", "--value", ""},
		} {
			cli(t, st, append([]string{"write-attribute"}, args...)...).want(t, 3)
		}
		cli(t, st, "write-attribute", "--variation", "1", "--value", "1.5", "--data-type", "float").want(t, 6)
		run(t, "client", "write-attribute", "--ip", "127.0.0.1", "--variation", "245").want(t, 2)
	})
	t.Run("append", func(t *testing.T) {
		dir := t.TempDir()
		part := filepath.Join(dir, "part.csv")
		_ = os.WriteFile(part, []byte("line\n"), 0o600)
		cli(t, st, "file", "put", "--input", part, "--path", "/logs/x.csv").want(t, 0)
		var res types.FileTransferResult
		cli(t, st, "file", "put", "--input", part, "--path", "/logs/x.csv", "--append", "--format", "json").want(t, 0).decode(t, &res)
		if !res.Append || res.Length != 5 {
			t.Errorf("append: %+v", res)
		}
		cli(t, st, "file", "put", "--input", part, "--path", "/logs/x.csv", "--append").want(t, 0).has(t, "Appended /logs/x.csv to")
		if r := cli(t, st, "file", "get", "--path", "/logs/x.csv", "--output", "-").want(t, 0); r.stdout != "line\nline\nline\n" {
			t.Errorf("after two appends: %q", r.stdout)
		}
	})
	t.Run("deadbands", func(t *testing.T) {
		var res types.PollResult
		cli(t, st, "read", "--object", "g34v1", "--range", "0", "--format", "json").want(t, 0).decode(t, &res)
		if res.Count != 1 || res.Measurements[0].Type != "analog-deadband" || res.Measurements[0].Value != float64(1) && res.Measurements[0].Value != 0.5 {
			t.Errorf("deadbands: %+v", res)
		}
	})
}

func TestSeveralControls(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)
	db := st.Server().Database()
	on := func(i uint32) bool {
		v, _ := db.Get(server.BinaryOutput, i)
		b, _ := v.(app.Binary)
		return b.Value
	}

	var res types.OperateResult
	cli(t, st, "operate", "--type", "crob", "--index", "1,3", "--op", "latch-on", "--mode", "direct", "--format", "json").want(t, 0).decode(t, &res)
	if res.Status != "SUCCESS" || len(res.Controls) != 2 || res.Controls[1].Index != 3 || res.Controls[1].Status != "SUCCESS" || res.Error != nil {
		t.Errorf("two controls: %+v", res)
	}
	if !on(1) {
		t.Error("output 1 was not operated")
	}
	r := cli(t, st, "operate", "--type", "crob", "--index", "1,3", "--op", "latch-off", "--mode", "direct").want(t, 0)
	r.has(t, "g12v1 index 1,3: LATCH_OFF", "\n  index 1     LATCH_OFF      SUCCESS", "\n  index 3     LATCH_OFF      SUCCESS")
	if n := len(strings.Split(strings.TrimSpace(cli(t, st, "operate", "--type", "crob", "--index", "1,3", "--op", "latch-on", "--mode", "direct", "--format", "table").want(t, 0).stdout), "\n")); n != 4 {
		t.Errorf("table: %d lines, want a header, a rule and a row per control", n)
	}

	// A direct operate refused for some controls executed the others: a partial result.
	cli(t, st, "operate", "--type", "crob", "--index", "1,2", "--op", "latch-off", "--mode", "direct", "--format", "json").want(t, 7).decode(t, &res)
	if res.Status != "LOCAL" || res.Controls[0].Status != "SUCCESS" || res.Controls[1].Status != "LOCAL" || res.Error == nil || res.Error.Code != "command_refused" {
		t.Errorf("partial: %+v", res)
	}
	if on(1) {
		t.Error("the control that succeeded was not executed")
	}
	cli(t, st, "operate", "--type", "crob", "--index", "1,2", "--op", "latch-on", "--mode", "direct").want(t, 7).has(t, "error: the outstation refused the control")
	// With select-before-operate a refusal is fatal, and every status is named.
	cli(t, st, "operate", "--type", "crob", "--index", "1,2", "--op", "latch-on").want(t, 6).errHas(t, "statuses: index 1 SUCCESS, index 2 LOCAL")
	cli(t, st, "operate", "--type", "crob", "--index", "2,9", "--op", "latch-on", "--mode", "direct").want(t, 6).errHas(t, "statuses: index 2 LOCAL, index 9 NOT_SUPPORTED")

	// Set-points: one value for all, or one per index.
	cli(t, st, "operate", "--type", "analog", "--index", "0,5", "--value", "20,30", "--mode", "direct", "--format", "json").want(t, 7).decode(t, &res)
	if res.Object != "g41v1" || *res.Controls[0].Value != 20 || *res.Controls[1].Value != 30 || res.Controls[1].Status != "NOT_SUPPORTED" {
		t.Errorf("set-points: %+v", res)
	}
	cli(t, st, "operate", "--type", "analog", "--index", "0", "--value", "20.5").want(t, 0).has(t, "g41v3 index 0: 20.5")
	cli(t, st, "operate", "--type", "analog", "--index", "0,5", "--value", "1.5", "--mode", "direct").want(t, 7).has(t, "set-points", "index 5     1.5")
	cli(t, st, "operate", "--type", "crob", "--index", "1,3", "--op", "latch-on", "--mode", "direct-no-ack", "--broadcast", "confirm").want(t, 0).has(t, "index 1,3")
	for _, args := range [][]string{
		{"--type", "crob", "--index", "1,1", "--op", "latch-on"},
		{"--type", "crob", "--index", "1,x", "--op", "latch-on"},
		{"--type", "analog", "--index", "0,5", "--value", "1,2,3"},
		{"--type", "analog", "--index", "0", "--value", "1,x"},
		{"--type", "analog", "--index", "0,5", "--value", "1,1.5", "--variation", "2"},
	} {
		cli(t, st, append([]string{"operate"}, args...)...).want(t, 3)
	}
}

func TestManagementRestartAndDeadbands(t *testing.T) {
	model := strings.Replace(stationtest.Model, "  restart_delay: 1s\n", "  restart_delay: 1s\n  refused_functions: [save-configuration]\n  indications: [local-control, device-trouble]\n", 1)
	st := stationtest.Start(t, model)

	t.Run("manage", func(t *testing.T) {
		for _, f := range []string{"initialize-data", "initialize-application", "start-application", "stop-application"} {
			var res types.ConfirmedResult
			cli(t, st, "manage", "--function", f, "--format", "json").want(t, 0).decode(t, &res)
			if res.Request != f || res.Function == "" {
				t.Errorf("%s: %+v", f, res)
			}
		}
		kind, code := cli(t, st, "manage", "--function", "save-configuration", "--format", "json").want(t, 6).errorCode(t)
		if kind != "refused" || code != "no_func_code_support" {
			t.Errorf("refused function: %s/%s", kind, code)
		}
		cli(t, st, "manage", "--function", "start-application", "--format", "table").want(t, 0).has(t, "START_APPL")
		cli(t, st, "manage", "--function", "reboot").want(t, 3)
		run(t, "client", "manage", "--ip", "127.0.0.1").want(t, 2)
		managed := map[string]bool{}
		for _, e := range st.Events() {
			if e.Event == "manage" {
				managed[e.Function+" "+e.Detail] = true
			}
		}
		if !managed["SAVE_CONFIG refused"] || !managed["START_APPL "] {
			t.Errorf("manage events: %v", managed)
		}
	})
	t.Run("indications and clear-restart", func(t *testing.T) {
		var res types.PollResult
		cli(t, st, "read", "--object", "g80v1", "--format", "json").want(t, 0).decode(t, &res)
		if !strings.Contains(res.IIN, "LOCAL_CONTROL") || !strings.Contains(res.IIN, "DEVICE_TROUBLE") || !strings.Contains(res.IIN, "DEVICE_RESTART") {
			t.Errorf("IIN: %q", res.IIN)
		}
		var cr types.ConfirmedResult
		cli(t, st, "clear-restart", "--format", "json").want(t, 0).decode(t, &cr)
		if cr.Request != "clear-restart" {
			t.Errorf("clear-restart: %+v", cr)
		}
		cli(t, st, "read", "--object", "g80v1", "--format", "json").want(t, 0).decode(t, &res)
		if strings.Contains(res.IIN, "DEVICE_RESTART") {
			t.Errorf("DEVICE_RESTART was not cleared: %q", res.IIN)
		}
	})
	t.Run("deadbands", func(t *testing.T) {
		var res types.ConfirmedResult
		cli(t, st, "write-deadband", "--index", "0,1", "--value", "2.5", "--format", "json").want(t, 0).decode(t, &res)
		if res.Request != "write-deadband" || res.Objects[0] != "g34v3" || res.Range != "0,1" || res.Value == nil || *res.Value != 2.5 {
			t.Errorf("write-deadband: %+v", res)
		}
		cli(t, st, "read", "--object", "g34v3", "--range", "0-1").want(t, 0).has(t, "2.5")
		cli(t, st, "write-deadband", "--index", "0", "--value", "7").want(t, 0).has(t, "write-deadband (g34v2 0 = 7)")
		cli(t, st, "write-deadband", "--index", "0", "--value", "300", "--variation", "1").want(t, 0)
		cli(t, st, "write-deadband", "--index", "9", "--value", "1").want(t, 6)
		for _, args := range [][]string{
			{"--index", "0", "--value", "-1"},
			{"--index", "0", "--value", "x"},
			{"--index", "0", "--value", "1.5", "--variation", "2"},
			{"--index", "0", "--value", "70000", "--variation", "1"},
			{"--index", "0", "--value", "1", "--variation", "4"},
			{"--index", "0,0", "--value", "1"},
		} {
			cli(t, st, append([]string{"write-deadband"}, args...)...).want(t, 3)
		}
	})
	t.Run("debug log", func(t *testing.T) {
		cli(t, st, "--debug", "read", "--object", "g1v2").want(t, 0).errHas(t, "debug: log ", "msg=connected", "component=dnp3.client")
	})
}
