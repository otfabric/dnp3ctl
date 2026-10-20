package station

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/client"
	"github.com/otfabric/go-dnp3/link"
	"github.com/otfabric/go-dnp3/server"
	serial "github.com/otfabric/go-serial"

	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/dnp"
	"github.com/otfabric/dnp3ctl/internal/errs"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// eventLog collects the events of a station.
type eventLog struct {
	mu     sync.Mutex
	events []types.ServerEvent
}

func (l *eventLog) log(e types.ServerEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, e)
}

func (l *eventLog) has(event, detail string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.events {
		if e.Event == event && strings.Contains(e.Detail, detail) {
			return true
		}
	}
	return false
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestDemoAndEncode(t *testing.T) {
	m := Demo()
	if m.Address() != 10 || len(m.Points) == 0 || len(m.Controls) == 0 || len(m.Files) == 0 {
		t.Fatalf("demo: %+v", m.Outstation)
	}
	for _, asJSON := range []bool{false, true} {
		data, err := m.Encode(asJSON)
		if err != nil {
			t.Fatal(err)
		}
		back, err := Parse(data)
		if err != nil || back.Validate() != nil || len(back.Points) != len(m.Points) || back.Outstation.SelectTimeout != m.Outstation.SelectTimeout {
			t.Errorf("round trip (json=%v): %v\n%s", asJSON, err, data)
		}
	}
	if !strings.Contains(strings.Join(PointTypeNames(), ","), "analog-input") || typeName(server.PointType(99)) == "" {
		t.Error("PointTypeNames")
	}
	if _, err := Parse(nil); err == nil {
		t.Error("an empty model was accepted")
	}
	if _, err := Parse([]byte("outstation: {link_address: 3}\n")); err == nil {
		t.Error("an unknown field was accepted")
	}
	if _, err := Parse([]byte("outstation: {select_timeout: 5}\n")); err == nil {
		t.Error("a duration without unit was accepted")
	}
	if _, err := Parse([]byte("outstation: {select_timeout: [1]}\n")); err == nil {
		t.Error("a duration that is no string was accepted")
	}
}

func TestValidate(t *testing.T) {
	for _, c := range []struct {
		model string
		want  []string
	}{
		{"outstation: {address: 65520, masters: [10, 65530]}\npoints: []\n", []string{"address 65520", "masters: 65530", "at least one point"}},
		{"outstation: {masters: [10]}\npoints: [{type: binary-input, index: 0, value: true}]\n", []string{"masters: 10"}},
		{"outstation: {indications: [local-control, on-fire], refused_functions: [save-configuration, reboot]}\npoints: [{type: binary-input, index: 0, value: true}]\n",
			[]string{`indications: "on-fire"`, `refused_functions: "reboot"`}},
		{`outstation:
  attribute_sets:
    - {set: 0, attributes: {1: a}}
    - {set: 2}
    - {set: 3, attributes: {0: a, 254: b, x: c, 1: "", 2: [1], 3: 5000000000, 4: -3000000000, 5: true}}
    - {set: 3, attributes: {1: a}, writable: [2, 300]}
points: [{type: binary-input, index: 0, value: true}]
`, []string{"set 0 is the standard set", "(set 2): a set needs at least one attribute", `variation "0": use 1-253`, `variation "254"`, `variation "x"`,
			"variation 1: a string holds 1-255 octets", "variation 2: value [1]", "variation 3: value 5000000000", "variation 4: value -3000000000",
			"variation 5: value true", "the set is given twice", "writable variation 2 is not an attribute", "writable variation 300"}},
		{"outstation: {unsolicited: {classes: [0, 4]}}\npoints: [{type: binary-input, index: 0, value: true}]\n",
			[]string{"classes: 0", "classes: 4", "unsolicited.enabled is not"}},
		{"outstation: {unsolicited: {enabled: true, hold_count: -1}, time_sync_interval: 10ms, attributes: {colour: red, manufacturer: \"\"}}\npoints: [{type: binary-input, index: 0, value: true}]\n",
			[]string{"cannot be negative", "time_sync_interval", "\"colour\" is not a device attribute", "attributes.manufacturer: the value is empty"}},
		{`outstation: {}
points:
  - {type: relay, index: 0, value: 1}
  - {type: binary-input, index: 0, value: maybe}
  - {type: binary-input, index: 0, value: true, class: 4}
  - {type: double-bit-input, index: 0, value: half}
  - {type: counter, index: 0, value: -1}
  - {type: counter, index: 1, value: 1.5, deadband: -1}
  - {type: analog-input, index: 0, value: x, static_variation: 9}
  - {type: analog-input, index: 1, value: 1, event_variation: 99, flags: [SHINY]}
  - {type: octet-string, index: 0, value: ""}
  - {type: binary-input, index: 5, value: true, simulate: {mode: sine, interval: 1s, min: 0, max: 1}}
  - {type: analog-input, index: 5, value: 1, simulate: {mode: toggle, interval: 1s}}
  - {type: analog-input, index: 6, value: 1, simulate: {mode: ramp, interval: 1s, min: 2, max: 1}}
  - {type: analog-input, index: 7, value: 1, simulate: {mode: walk, interval: 1s}}
  - {type: analog-input, index: 8, value: 1, simulate: {mode: count, interval: 1ms}}
  - {type: octet-string, index: 5, value: x, simulate: {mode: count, interval: 1s}}
  - {type: analog-input, index: 9, value: 1, simulate: {mode: count, interval: 1s, step: -1}}
`, []string{`type "relay"`, "not a binary state", "used twice", "class 4", "double-bit state", "whole number 0-", "deadband cannot", "is not a number",
			"static_variation 9", "event_variation 99", `flag "SHINY"`, "1-255 octets", "mode sine is for counters", "mode toggle is for binary",
			"needs max above min", `mode "walk"`, "use at least 50ms", "mode count is for counters", "step cannot be negative"}},
		{`outstation: {}
points:
  - {type: time-and-interval, index: 0, value: 2026-01-04T02:00:00Z, class: 1, static_variation: 1, simulate: {mode: count, interval: 1s}}
  - {type: time-and-interval, index: 1, value: "next week"}
  - {type: time-and-interval, index: 2, value: 5}
  - {type: time-and-interval, index: 3, value: "2026-01-04T02:00:00Z", interval: 1, interval_unit: fortnights}
  - {type: analog-input, index: 0, value: 1, interval: 5, command_event_class: 1}
  - {type: binary-output, index: 0, value: true, command_event_class: 4}
  - {type: analog-output, index: 0, value: 1, command_event_variation: 9}
`, []string{"time-and-interval 0): a time and interval has no events", "static_variation 1: a time and interval is g50v4", "is not simulated",
			`value "next week" is not a time`, "value 5 is not a time", `interval_unit "fortnights": use none, milliseconds`,
			"interval and interval_unit are for a time-and-interval", "command_event_class and command_event_variation are for a binary-output",
			"command_event_class 4: use 1-3", "command_event_variation 9 is not a variation of g43"}},
		{`outstation: {}
points:
  - {type: binary-output, index: 0, value: true}
  - {type: analog-input, index: 0, value: 1}
controls:
  - {type: relay, index: 0}
  - {type: crob, index: 0, status: MAYBE, min: 1}
  - {type: crob, index: 0}
  - {type: analog, index: 0, min: 5, max: 1, target: {type: binary-output, index: 0}}
  - {type: analog, index: 1, target: {type: analog-output, index: 7}}
  - {type: analog, index: 2, target: {type: pump, index: 0}}
`, []string{`type "relay"`, `status "MAYBE"`, "min and max are for analog", "used twice", "max is below min", "a analog control sets", "analog-output 7 is not a point", `target type "pump"`}},
		{`outstation: {}
points: [{type: binary-input, index: 0, value: true}]
files:
  - {name: "", content: x}
  - {name: /a, content: x, size: 3}
  - {name: /b, size: -1}
  - {name: /c, content: x, permissions: rwz}
  - {name: /d, path: missing.bin}
  - {name: /e, content: x}
  - {name: /e/f, content: y}
  - {name: /g, content: x}
  - {name: /g, content: x}
uploads: {dir: up}
`, []string{`name ""`, "exactly one of", "size cannot be negative", "invalid --permissions", "missing.bin", "/e is a file and a directory", "used twice", "uploads.accept is not"}},
	} {
		m, err := Parse([]byte(c.model))
		if err != nil {
			t.Fatalf("parse: %v\n%s", err, c.model)
		}
		err = m.Validate()
		if err == nil {
			t.Errorf("model accepted:\n%s", c.model)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("the error lacks %q:\n%v", w, err)
			}
		}
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rec.bin"), []byte{1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	model := filepath.Join(dir, "model.yaml")
	_ = os.WriteFile(model, []byte("outstation: {address: 4}\npoints: [{type: binary-input, index: 0, value: on}]\nfiles: [{name: rec.bin, path: rec.bin}]\nuploads: {accept: true, dir: up}\n"), 0o600)
	m, err := Load(model)
	if err != nil || m.Address() != 4 {
		t.Fatalf("load: %v", err)
	}
	if m.resolve("") != "" || m.resolve("/abs") != "/abs" || m.resolve("up") != filepath.Join(dir, "up") {
		t.Error("resolve")
	}
	if _, err := Load(filepath.Join(dir, "missing.yaml")); err == nil {
		t.Error("a missing model was loaded")
	}
	_ = os.WriteFile(model, []byte("points: [{type: binary-input}]\n"), 0o600)
	if _, err := Load(model); err == nil || !strings.Contains(err.Error(), model) {
		t.Errorf("an invalid model was loaded: %v", err)
	}
	_ = os.WriteFile(model, []byte("{"), 0o600)
	if _, err := Load(model); err == nil {
		t.Error("a malformed model was loaded")
	}
}

func TestValues(t *testing.T) {
	on := app.FlagOnline
	for _, c := range []struct {
		t    server.PointType
		v    any
		want app.Value
	}{
		{server.BinaryInput, "on", app.Binary{Value: true, Flags: on}},
		{server.BinaryInput, 0, app.Binary{Flags: on}},
		{server.BinaryOutput, "FALSE", app.Binary{Flags: on}},
		{server.DoubleBitInput, true, app.DoubleBitBinary{Value: app.DoubleBitOn, Flags: on}},
		{server.DoubleBitInput, false, app.DoubleBitBinary{Value: app.DoubleBitOff, Flags: on}},
		{server.DoubleBitInput, "indeterminate", app.DoubleBitBinary{Value: app.DoubleBitIndeterminate, Flags: on}},
		{server.DoubleBitInput, "intermediate", app.DoubleBitBinary{Value: app.DoubleBitIntermediate, Flags: on}},
		{server.DoubleBitInput, 2, app.DoubleBitBinary{Value: app.DoubleBitOn, Flags: on}},
		{server.Counter, "42", app.Counter{Value: 42, Flags: on}},
		{server.FrozenCounter, int64(7), app.Counter{Value: 7, Flags: on}},
		{server.AnalogInput, uint64(3), app.Analog{Value: 3, Flags: on}},
		{server.AnalogOutput, 1.25, app.Analog{Value: 1.25, Flags: on}},
		{server.OctetString, "abc", app.OctetString{Data: []byte("abc")}},
	} {
		got, err := initialValue(c.t, PointSpec{Value: c.v})
		if err != nil || fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("initialValue(%v, %v) = %v, %v; want %v", c.t, c.v, got, err, c.want)
		}
	}
	for _, c := range []struct {
		t server.PointType
		v any
	}{
		{server.BinaryInput, 2}, {server.BinaryInput, 1.5}, {server.DoubleBitInput, 4}, {server.DoubleBitInput, 1.5},
		{server.Counter, "x"}, {server.AnalogInput, []int{1}}, {server.OctetString, nil}, {server.OctetString, strings.Repeat("x", 256)},
	} {
		if _, err := initialValue(c.t, PointSpec{Value: c.v}); err == nil {
			t.Errorf("initialValue(%v, %v) was accepted", c.t, c.v)
		}
	}
	if f, err := (PointSpec{Flags: []string{"online", "comm_lost", "local_forced", "remote_forced", "restart"}}).flags(); err != nil || f != 0x1f {
		t.Errorf("flags: %v, %v", f, err)
	}

	s := &SimSpec{Mode: "ramp", Min: 0, Max: 10}
	if simValue(s, 9.5, 1, 1) != 0 || simValue(s, 2, 1, 1) != 3 {
		t.Error("ramp")
	}
	s = &SimSpec{Mode: "sine", Min: 0, Max: 2}
	if v := simValue(s, 0, 15, 1); v < 1.99 || v > 2 {
		t.Errorf("sine at a quarter period: %v", v)
	}
	s = &SimSpec{Mode: "random", Min: 5, Max: 6}
	for range 10 {
		if v := simValue(s, 0, 1, 1); v < 5 || v > 6 {
			t.Errorf("random: %v", v)
		}
	}
	if simValue(&SimSpec{Mode: "count"}, 1, 1, 2) != 3 {
		t.Error("count")
	}
}

const simModel = `
outstation: {}
points:
  - {type: binary-input, index: 0, value: false, simulate: {mode: toggle, interval: 50ms}}
  - {type: double-bit-input, index: 0, value: "off", simulate: {mode: toggle, interval: 50ms}}
  - {type: counter, index: 0, value: 0, simulate: {mode: count, interval: 50ms, step: 2}}
  - {type: analog-input, index: 0, value: 0, simulate: {mode: ramp, interval: 50ms, min: 0, max: 100}}
  - {type: analog-input, index: 1, value: 0, simulate: {mode: sine, interval: 50ms, min: 0, max: 100, step: 4}}
  - {type: analog-input, index: 2, value: 0, simulate: {mode: random, interval: 50ms, min: 10, max: 20}}
`

func newStation(t *testing.T, model string, edit ...func(*Options)) (*Station, *eventLog) {
	t.Helper()
	m, err := Parse([]byte(model))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	l := &eventLog{}
	o := Options{Log: l.log}
	for _, f := range edit {
		f(&o)
	}
	st, err := New(m, o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st, l
}

func TestSimulation(t *testing.T) {
	st, _ := newStation(t, simModel)
	db := st.Server().Database()
	eventually(t, "every simulated value to change", func() bool {
		b, _ := db.Get(server.BinaryInput, 0)
		d, _ := db.Get(server.DoubleBitInput, 0)
		c, _ := db.Get(server.Counter, 0)
		a0, _ := db.Get(server.AnalogInput, 0)
		a1, _ := db.Get(server.AnalogInput, 1)
		a2, _ := db.Get(server.AnalogInput, 2)
		return b.(app.Binary).Value && d.(app.DoubleBitBinary).Value == app.DoubleBitOn && c.(app.Counter).Value >= 2 &&
			a0.(app.Analog).Value >= 1 && a1.(app.Analog).Value != 0 && a2.(app.Analog).Value >= 10
	})
	eventually(t, "the double-bit input to toggle back", func() bool {
		d, _ := db.Get(server.DoubleBitInput, 0)
		return d.(app.DoubleBitBinary).Value == app.DoubleBitOff
	})
	if st.Model().Address() != 10 {
		t.Error("Model")
	}
	_ = st.Close()
	_ = st.Close() // a second Close is harmless
}

func TestFileServer(t *testing.T) {
	dir := t.TempDir()
	m, _ := Parse([]byte(`outstation: {}
points: [{type: binary-input, index: 0, value: true}]
files: [{name: /a.txt, content: hello}, {name: /d/b.bin, size: 3, permissions: "600"}]
uploads: {accept: true, dir: up}
`))
	m.dir = dir
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	l := &eventLog{}
	now := func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	fs, err := newFileServer(m, l.log, now)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	read := func(name string) ([]byte, app.FileStatus) {
		f, st := fs.Open(ctx, server.FileRequest{Name: name, Mode: app.FileModeRead})
		if st != app.FileSuccess {
			return nil, st
		}
		b, _ := io.ReadAll(f)
		if f.Size() != uint32(len(b)) {
			t.Errorf("%s: size %d, read %d", name, f.Size(), len(b))
		}
		_ = f.Close(false)
		return b, st
	}
	if b, _ := read("a.txt"); string(b) != "hello" {
		t.Errorf("a.txt: %q", b)
	}
	b, _ := read("/")
	entries, err := app.DecodeFileDescriptors(b)
	if err != nil || len(entries) != 2 || entries[0].Name != "a.txt" || entries[1].Type != app.FileTypeDirectory {
		t.Errorf("root: %+v, %v", entries, err)
	}
	b, _ = read("/d")
	entries, _ = app.DecodeFileDescriptors(b)
	if len(entries) != 1 || entries[0].Permissions != 0o600 || entries[0].Size != 3 {
		t.Errorf("/d: %+v", entries)
	}
	if _, st := read("/nope"); st != app.FileNotFound {
		t.Errorf("missing file: %v", st)
	}
	if _, st := fs.Open(ctx, server.FileRequest{Name: " ", Mode: app.FileModeRead}); st != app.FileNotFound {
		t.Errorf("empty name: %v", st)
	}
	if _, st := fs.Open(ctx, server.FileRequest{Name: "/x", Mode: app.FileMode(9)}); st != app.FileInvalidMode {
		t.Errorf("bad mode: %v", st)
	}

	// Writing, appending, and the upload directory on disk.
	w, st := fs.Open(ctx, server.FileRequest{Name: "/up/new.txt", Mode: app.FileModeWrite})
	if st != app.FileSuccess {
		t.Fatal(st)
	}
	_, _ = w.Write([]byte("abc"))
	if n, _ := w.Read(make([]byte, 1)); n != 0 || w.Size() != 3 {
		t.Error("a file opened for writing reads")
	}
	if err := w.Close(false); err != nil {
		t.Fatal(err)
	}
	a, _ := fs.Open(ctx, server.FileRequest{Name: "/up/new.txt", Mode: app.FileModeAppend, Permissions: 0o640})
	_, _ = a.Write([]byte("def"))
	_ = a.Close(false)
	if b, _ := read("/up/new.txt"); string(b) != "abcdef" {
		t.Errorf("appended: %q", b)
	}
	if disk, err := os.ReadFile(filepath.Join(dir, "up", "up", "new.txt")); err != nil || string(disk) != "abcdef" {
		t.Errorf("on disk: %q, %v", disk, err)
	}
	ab, _ := fs.Open(ctx, server.FileRequest{Name: "/aborted", Mode: app.FileModeWrite})
	_ = ab.Close(true)
	if _, st := read("/aborted"); st != app.FileNotFound {
		t.Error("an aborted write was kept")
	}
	if _, st := fs.Open(ctx, server.FileRequest{Name: "/d", Mode: app.FileModeWrite}); st != app.FilePermissionDenied {
		t.Errorf("writing a directory: %v", st)
	}
	rf, _ := fs.Open(ctx, server.FileRequest{Name: "/a.txt", Mode: app.FileModeRead})
	if _, err := rf.Write([]byte("x")); err == nil {
		t.Error("a file opened for reading takes writes")
	}
	_ = rf.Close(true)

	if d, st := fs.Info(ctx, "/d/b.bin"); st != app.FileSuccess || d.Size != 3 {
		t.Errorf("info: %+v %v", d, st)
	}
	if d, st := fs.Info(ctx, "/d"); st != app.FileSuccess || d.Type != app.FileTypeDirectory {
		t.Errorf("info of a directory: %+v %v", d, st)
	}
	if _, st := fs.Info(ctx, "/nope"); st != app.FileNotFound {
		t.Error("info of a missing file")
	}
	if _, st := fs.Info(ctx, ""); st != app.FileNotFound {
		t.Error("info of an empty name")
	}
	if fs.Delete(ctx, "/up/new.txt", 0) != app.FileSuccess || fs.Delete(ctx, "/up/new.txt", 0) != app.FileNotFound || fs.Delete(ctx, "", 0) != app.FileNotFound {
		t.Error("delete")
	}
	if _, err := os.Stat(filepath.Join(dir, "up", "up", "new.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a deleted upload stayed on disk")
	}
	for _, want := range []string{"sent /a.txt (5 octets)", "directory /", "received /up/new.txt", "write of /aborted aborted", "read of /a.txt aborted", "deleted /up/new.txt"} {
		if !l.has("file", want) {
			t.Errorf("no file event %q", want)
		}
	}

	fs.accept = false
	if _, st := fs.Open(ctx, server.FileRequest{Name: "/x", Mode: app.FileModeWrite}); st != app.FilePermissionDenied {
		t.Error("a read-only outstation took a file")
	}
	if fs.Delete(ctx, "/a.txt", 0) != app.FilePermissionDenied {
		t.Error("a read-only outstation deleted a file")
	}
	fs.dir = filepath.Join(dir, "a.txt-is-a-file")
	_ = os.WriteFile(fs.dir, nil, 0o600)
	if err := fs.store("/x/y", []byte("z"), 0o600); err == nil {
		t.Error("storing below a file worked")
	}
}

// New builds only from a valid model; an invalid attribute set is refused even when the
// model was not validated first.
func TestNewRefusesInvalidAttributeSets(t *testing.T) {
	m, err := Parse([]byte("outstation: {attribute_sets: [{set: 1, attributes: {0: a}}]}\npoints: [{type: binary-input, index: 0, value: true}]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if st, err := New(m, Options{}); err == nil {
		_ = st.Close()
		t.Error("New accepted an invalid attribute set")
	}
}

func TestHandler(t *testing.T) {
	var trace bytes.Buffer
	st, l := newStation(t, simModel+"controls: [{type: crob, index: 0, target: {type: binary-input, index: 0}}]\n", func(o *Options) {
		o.Metrics = &dnp.Tracer{W: &trace}
	})
	h := &handler{st: st}
	ctx := context.Background()
	// The manufacturer is no user-assigned attribute, and the location is not in the model.
	for _, v := range []uint8{app.AttrManufacturerName, app.AttrUserLocation} {
		if err := h.WriteAttribute(ctx, v, app.StringAttribute("x")); err == nil {
			t.Errorf("WriteAttribute(%d) was accepted", v)
		}
	}
	if !l.has("write", "manufacturer: refused") {
		t.Error("the refused write was not logged")
	}
	if err := h.WriteOctetString(ctx, 0, []byte("y")); err != nil {
		t.Error("WriteOctetString")
	}
	if s := h.Select(ctx, server.Command{GV: app.G12V2, Index: 0, Value: app.CROB{Op: app.OpLatchOn}}); s != app.StatusNotSupported {
		t.Errorf("pattern control block: %v", s)
	}
	if s := h.Operate(ctx, server.Command{GV: app.G12V1, Index: 0, Value: app.CROB{Op: app.OpNul}}, server.DirectOperate); s != app.StatusSuccess {
		t.Errorf("NUL: %v", s)
	}
	m := &eventMetrics{st: st, trace: st.opts.Metrics}
	addr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}
	m.OnFramingError(addr, link.ErrCRC)
	m.OnDecodeError(nil, app.ErrMalformed)
	m.OnEventsDiscarded(2, 5)
	m.OnUnsolicited(addr, 3, nil)
	m.OnUnsolicited(addr, 3, errors.New("no confirm"))
	for _, c := range [][2]string{{"error", "undecodable"}, {"events-discarded", "5 events of class 2"}, {"unsolicited", "3 events confirmed"}, {"unsolicited", "not confirmed"}} {
		if !l.has(c[0], c[1]) {
			t.Errorf("no %s event %q", c[0], c[1])
		}
	}
	if !strings.Contains(trace.String(), "framing error") {
		t.Error("the trace did not receive the callbacks")
	}
}

// pipeAddr names the master end of a pipe.
func TestRun(t *testing.T) {
	t.Run("serial", func(t *testing.T) {
		master, outstation := net.Pipe()
		var opened serial.Config
		openSerial = func(sc serial.Config) (io.ReadWriteCloser, net.Addr, error) {
			opened = sc
			return outstation, nil, nil
		}
		t.Cleanup(func() { openSerial = dnp.OpenSerial })
		l := &eventLog{}
		cfg := &config.ServerConfig{Serial: "/dev/ttyFAKE", SerialConfig: config.SerialConfig{Baud: 9600}}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- Run(ctx, cfg, l.log, nil, func(net.Addr) {}) }()

		c, err := client.Dial(ctx, "serial", client.WithStream(func(context.Context) (io.ReadWriteCloser, net.Addr, error) { return master, nil, nil }))
		if err != nil {
			t.Fatal(err)
		}
		res, err := c.Read(ctx, app.AllObjects(app.G1V0))
		if err != nil || len(res.Measurements()) != 3 {
			t.Errorf("read over the serial pipe: %v, %v", res, err)
		}
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
		_ = c.Close()
		if opened.Address != "/dev/ttyFAKE" || !l.has("listening", "Demo feeder RTU") || !l.has("stopping", "") {
			t.Errorf("serial: %+v", opened)
		}

		openSerial = func(serial.Config) (io.ReadWriteCloser, net.Addr, error) { return nil, nil, errors.New("busy") }
		err = Run(context.Background(), cfg, l.log, nil, nil)
		if e, ok := errs.As(err); !ok || e.Code != errs.CodeListenFailed {
			t.Errorf("a port that does not open: %v", err)
		}
	})
	t.Run("serve ends", func(t *testing.T) {
		master, outstation := net.Pipe()
		openSerial = func(serial.Config) (io.ReadWriteCloser, net.Addr, error) { return outstation, nil, nil }
		t.Cleanup(func() { openSerial = dnp.OpenSerial })
		done := make(chan error, 1)
		go func() { done <- Run(context.Background(), &config.ServerConfig{Serial: "/dev/x"}, nil, nil, nil) }()
		time.Sleep(50 * time.Millisecond)
		_ = master.Close()
		if err := <-done; err == nil {
			// The stream ended: ServeConn returns the reason, and Run reports it.
			t.Log("ServeConn returned nil after the stream ended")
		}
	})
	t.Run("an error while stopping is logged", func(t *testing.T) {
		st, l := newStation(t, simModel)
		served := make(chan error, 1)
		served <- errors.New("socket gone")
		st.shutdown(served)
		if !l.has("stopping", "") || !l.has("error", "stopping: socket gone") {
			t.Errorf("events: %+v", l.events)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		err := Run(context.Background(), &config.ServerConfig{Config: "/nonexistent.yaml"}, nil, nil, nil)
		if e, ok := errs.As(err); !ok || e.Code != errs.CodeInvalidConfig {
			t.Errorf("missing model: %v", err)
		}
		if listenAddr(&config.ServerConfig{Listen: "::1"}, true) != "[::1]:19999" || listenAddr(&config.ServerConfig{Port: 7}, false) != ":7" {
			t.Error("listenAddr")
		}
	})
}
