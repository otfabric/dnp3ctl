// Package station is the simulated outstation behind `dnp3ctl server start`: a model of
// points, controls and files read from YAML or JSON, and the go-dnp3 outstation that serves
// it. The tests of the client commands run against it as well.
package station

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/link"
	"github.com/otfabric/go-dnp3/server"
	"gopkg.in/yaml.v3"

	"github.com/otfabric/dnp3ctl/internal/dnp"
)

// Duration is a time.Duration that is written as a string ("5s", "500ms") in a model.
type Duration time.Duration

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	var s string
	if err := n.Decode(&s); err != nil {
		return fmt.Errorf("line %d: a duration is written as a string such as 5s or 500ms", n.Line)
	}
	v, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil || v < 0 {
		return fmt.Errorf("line %d: invalid duration %q: use a form such as 5s or 500ms", n.Line, s)
	}
	*d = Duration(v)
	return nil
}

// MarshalYAML implements yaml.Marshaler.
func (d Duration) MarshalYAML() (any, error) { return time.Duration(d).String(), nil }

// MarshalJSON implements json.Marshaler.
func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(time.Duration(d).String()) }

// Model describes an outstation.
type Model struct {
	Outstation OutstationSpec `yaml:"outstation" json:"outstation"`
	Points     []PointSpec    `yaml:"points" json:"points"`
	Controls   []ControlSpec  `yaml:"controls,omitempty" json:"controls,omitempty"`
	Files      []FileSpec     `yaml:"files,omitempty" json:"files,omitempty"`
	Uploads    UploadSpec     `yaml:"uploads,omitempty" json:"uploads,omitempty"`

	// dir is the directory of the model file: relative paths in the model start there.
	dir string
}

// OutstationSpec holds what concerns the outstation as a whole.
type OutstationSpec struct {
	// Name is shown in the event log.
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
	// Address is the link address of the outstation, 0-65519. Default 10.
	Address *uint16 `yaml:"address,omitempty" json:"address,omitempty"`
	// Masters are the link addresses of the masters answered; empty answers any master.
	Masters []uint16 `yaml:"masters,omitempty" json:"masters,omitempty"`
	// SelfAddress makes the outstation answer frames sent to the self address 65532.
	SelfAddress bool `yaml:"self_address,omitempty" json:"self_address,omitempty"`
	// Unsolicited configures unsolicited responses.
	Unsolicited UnsolicitedSpec `yaml:"unsolicited,omitempty" json:"unsolicited,omitempty"`
	// TimeSyncInterval, when set, makes the outstation ask for time synchronization
	// (IIN1.4 NEED_TIME) at start and after this interval.
	TimeSyncInterval Duration `yaml:"time_sync_interval,omitempty" json:"time_sync_interval,omitempty"`
	// SelectTimeout is how long a SELECT stays valid. Default 10s.
	SelectTimeout Duration `yaml:"select_timeout,omitempty" json:"select_timeout,omitempty"`
	// ConfirmTimeout is the wait for an application confirmation. Default 5s.
	ConfirmTimeout Duration `yaml:"confirm_timeout,omitempty" json:"confirm_timeout,omitempty"`
	// RestartDelay is the time a master is told to wait after a cold or warm restart.
	// Without it restarts are refused.
	RestartDelay Duration `yaml:"restart_delay,omitempty" json:"restart_delay,omitempty"`
	// EventLimit is the number of events of each point type buffered per master. Default 100.
	EventLimit int `yaml:"event_limit,omitempty" json:"event_limit,omitempty"`
	// Attributes are the device attributes of set 0, by name ("manufacturer", "product",
	// ...) or variation number.
	Attributes map[string]string `yaml:"attributes,omitempty" json:"attributes,omitempty"`
}

// UnsolicitedSpec configures unsolicited responses.
type UnsolicitedSpec struct {
	Enabled bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	// Classes are enabled from the start, before any ENABLE_UNSOLICITED.
	Classes   []int    `yaml:"classes,omitempty" json:"classes,omitempty"`
	HoldTime  Duration `yaml:"hold_time,omitempty" json:"hold_time,omitempty"`
	HoldCount int      `yaml:"hold_count,omitempty" json:"hold_count,omitempty"`
	Retries   int      `yaml:"retries,omitempty" json:"retries,omitempty"`
	// RetryDelay is the pause after an unsolicited response was given up. Default 5s.
	RetryDelay Duration `yaml:"retry_delay,omitempty" json:"retry_delay,omitempty"`
}

// PointSpec is one point of the database.
type PointSpec struct {
	// Type is binary-input, double-bit-input, binary-output, counter, frozen-counter,
	// analog-input, frozen-analog-input, analog-output or octet-string.
	Type  string `yaml:"type" json:"type"`
	Index uint32 `yaml:"index" json:"index"`
	// Name describes the point. It is not sent: DNP3 carries no names.
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
	// Value is the initial value: true/false (binary), off/on/intermediate/indeterminate
	// (double-bit), a number, or text (octet string).
	Value any `yaml:"value" json:"value"`
	// Flags lists the quality flags that are set. Default [ONLINE].
	Flags []string `yaml:"flags,omitempty" json:"flags,omitempty"`
	// Class is the event class 0-3 (0: no events). Default 0.
	Class uint8 `yaml:"class,omitempty" json:"class,omitempty"`
	// StaticVariation and EventVariation select the variations reported; 0 is the default
	// of the point type.
	StaticVariation uint8 `yaml:"static_variation,omitempty" json:"static_variation,omitempty"`
	EventVariation  uint8 `yaml:"event_variation,omitempty" json:"event_variation,omitempty"`
	// Deadband applies to counters and analog points.
	Deadband float64 `yaml:"deadband,omitempty" json:"deadband,omitempty"`
	// ExcludeFromClass0 leaves the point out of class 0 responses.
	ExcludeFromClass0 bool `yaml:"exclude_from_class0,omitempty" json:"exclude_from_class0,omitempty"`
	// Simulate makes the value change by itself.
	Simulate *SimSpec `yaml:"simulate,omitempty" json:"simulate,omitempty"`
}

// SimSpec describes how a value changes by itself.
type SimSpec struct {
	// Mode is toggle (binary and double-bit), ramp (up to max, then back to min), sine,
	// random (uniform between min and max) or count (adds step, for counters).
	Mode     string   `yaml:"mode" json:"mode"`
	Interval Duration `yaml:"interval" json:"interval"`
	Min      float64  `yaml:"min,omitempty" json:"min,omitempty"`
	Max      float64  `yaml:"max,omitempty" json:"max,omitempty"`
	// Step is the increment of ramp and count (default 1), and the period of sine in
	// intervals (default 60).
	Step float64 `yaml:"step,omitempty" json:"step,omitempty"`
}

// ControlSpec is one output masters can operate.
type ControlSpec struct {
	// Type is crob (control relay output block, g12) or analog (analog output block, g41).
	Type  string `yaml:"type" json:"type"`
	Index uint32 `yaml:"index" json:"index"`
	Name  string `yaml:"name,omitempty" json:"name,omitempty"`
	// Target is the point that takes the commanded value: a binary output or binary input
	// for a crob, an analog output or analog input for an analog control. Default: the
	// binary output (crob) or analog output (analog) of the same index, when the model has
	// one.
	Target *TargetSpec `yaml:"target,omitempty" json:"target,omitempty"`
	// SelectRequired refuses a direct operate (NO_SELECT).
	SelectRequired bool `yaml:"select_required,omitempty" json:"select_required,omitempty"`
	// Min and Max bound the set-point of an analog control (OUT_OF_RANGE outside).
	Min *float64 `yaml:"min,omitempty" json:"min,omitempty"`
	Max *float64 `yaml:"max,omitempty" json:"max,omitempty"`
	// Status, when set, is the status every control of this output is answered with, for
	// example LOCAL or HARDWARE_ERROR, to test how a master handles a refusal.
	Status string `yaml:"status,omitempty" json:"status,omitempty"`
}

// TargetSpec names a point of the model.
type TargetSpec struct {
	Type  string `yaml:"type" json:"type"`
	Index uint32 `yaml:"index" json:"index"`
}

// FileSpec is one file the outstation offers.
type FileSpec struct {
	// Name is the file name masters use, for example /logs/events.csv.
	Name string `yaml:"name" json:"name"`
	// One of Path (a file, relative to the model file), Content (text) or Size (that many
	// octets of a repeating pattern).
	Path    string `yaml:"path,omitempty" json:"path,omitempty"`
	Content string `yaml:"content,omitempty" json:"content,omitempty"`
	Size    int    `yaml:"size,omitempty" json:"size,omitempty"`
	// Permissions of the file, octal or rwx form. Default rw-r--r--.
	Permissions string `yaml:"permissions,omitempty" json:"permissions,omitempty"`
}

// UploadSpec says whether the outstation takes and deletes files.
type UploadSpec struct {
	// Accept makes the outstation take the files masters write, and delete files.
	Accept bool `yaml:"accept,omitempty" json:"accept,omitempty"`
	// Dir, when set, is where written files are stored (relative to the model file), under
	// their name. Without it they are kept in memory.
	Dir string `yaml:"dir,omitempty" json:"dir,omitempty"`
}

// pointTypes are the point types a model may use.
var pointTypes = map[string]server.PointType{
	"binary-input":        server.BinaryInput,
	"double-bit-input":    server.DoubleBitInput,
	"binary-output":       server.BinaryOutput,
	"counter":             server.Counter,
	"frozen-counter":      server.FrozenCounter,
	"analog-input":        server.AnalogInput,
	"frozen-analog-input": server.FrozenAnalogInput,
	"analog-output":       server.AnalogOutput,
	"octet-string":        server.OctetString,
}

// PointTypeNames lists the point types of a model.
func PointTypeNames() []string {
	out := make([]string, 0, len(pointTypes))
	for n := range pointTypes {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// typeName returns the model name of a point type.
func typeName(t server.PointType) string {
	for n, pt := range pointTypes {
		if pt == t {
			return n
		}
	}
	return t.String()
}

// controlTargets lists, per control type, the point types that can take its value.
var controlTargets = map[string][]server.PointType{
	"crob":   {server.BinaryOutput, server.BinaryInput},
	"analog": {server.AnalogOutput, server.AnalogInput},
}

var flagBits = map[string]app.Flags{
	"ONLINE":         app.FlagOnline,
	"RESTART":        app.FlagRestart,
	"COMM_LOST":      app.FlagCommLost,
	"REMOTE_FORCED":  app.FlagRemoteForced,
	"LOCAL_FORCED":   app.FlagLocalForced,
	"CHATTER_FILTER": app.FlagChatterFilter,
	"ROLLOVER":       app.FlagRollover,
	"OVER_RANGE":     app.FlagOverRange,
	"DISCONTINUITY":  app.FlagDiscontinuity,
	"REFERENCE_ERR":  app.FlagReferenceErr,
}

// commandStatuses maps the names of command statuses to their codes.
var commandStatuses = func() map[string]app.CommandStatus {
	out := map[string]app.CommandStatus{}
	for c := 0; c < 128; c++ {
		s := app.CommandStatus(c)
		if name := s.String(); name != "" && !strings.HasPrefix(name, "STATUS(") && !strings.Contains(name, "(") {
			out[name] = s
		}
	}
	return out
}()

// Load reads a model from a YAML or JSON file and validates it.
func Load(file string) (*Model, error) {
	data, err := os.ReadFile(file) //nolint:gosec // the operator's own model file
	if err != nil {
		return nil, err
	}
	m, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	m.dir = filepath.Dir(file)
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return m, nil
}

// Parse decodes a model from YAML; JSON is a subset of YAML and is read the same way. A
// field the model does not know is an error, so that a typo does not go unnoticed.
func Parse(data []byte) (*Model, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var m Model
	if err := dec.Decode(&m); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("the model is empty")
		}
		return nil, err
	}
	return &m, nil
}

func (m *Model) resolve(p string) string {
	if p == "" || filepath.IsAbs(p) || m.dir == "" {
		return p
	}
	return filepath.Join(m.dir, p)
}

// Address returns the link address of the outstation.
func (m *Model) Address() uint16 {
	if m.Outstation.Address == nil {
		return dnp.DefaultOutstation
	}
	return *m.Outstation.Address
}

type pointKey struct {
	t     server.PointType
	index uint32
}

// Validate checks the model, files on disk included, and reports everything that is wrong
// with it at once.
func (m *Model) Validate() error {
	var problems []string
	bad := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	o := m.Outstation
	if a := m.Address(); link.Address(a) > link.MaxStationAddress {
		bad("outstation.address %d: use 0-65519", a)
	}
	for _, a := range o.Masters {
		if link.Address(a) > link.MaxStationAddress || a == m.Address() {
			bad("outstation.masters: %d: use 0-65519, other than the outstation's own address", a)
		}
	}
	for _, c := range o.Unsolicited.Classes {
		if c < 1 || c > 3 {
			bad("outstation.unsolicited.classes: %d: use 1-3", c)
		}
	}
	if len(o.Unsolicited.Classes) > 0 && !o.Unsolicited.Enabled {
		bad("outstation.unsolicited.classes is set but unsolicited.enabled is not")
	}
	if o.Unsolicited.HoldCount < 0 || o.Unsolicited.Retries < 0 || o.EventLimit < 0 {
		bad("outstation: hold_count, retries and event_limit cannot be negative")
	}
	if d := time.Duration(o.TimeSyncInterval); d != 0 && d < time.Second {
		bad("outstation.time_sync_interval %s: use at least 1s", d)
	}
	for k, v := range o.Attributes {
		if _, err := attributeVariation(k); err != nil {
			bad("outstation.attributes: %v", err)
			continue
		}
		if v == "" {
			bad("outstation.attributes.%s: the value is empty", k)
		}
	}
	if len(m.Points) == 0 {
		bad("points: an outstation needs at least one point")
	}

	points := map[pointKey]bool{}
	for i, p := range m.Points {
		what := fmt.Sprintf("points[%d]", i)
		t, ok := pointTypes[strings.ToLower(strings.TrimSpace(p.Type))]
		if !ok {
			bad("%s: type %q: use one of %s", what, p.Type, strings.Join(PointTypeNames(), ", "))
			continue
		}
		what = fmt.Sprintf("%s (%s %d)", what, typeName(t), p.Index)
		k := pointKey{t, p.Index}
		if points[k] {
			bad("%s: the index is used twice", what)
		}
		points[k] = true
		if _, err := initialValue(t, p); err != nil {
			bad("%s: %v", what, err)
		}
		if p.Class > 3 {
			bad("%s: class %d: use 0-3", what, p.Class)
		}
		if p.Deadband < 0 {
			bad("%s: deadband cannot be negative", what)
		}
		if p.StaticVariation != 0 && !staticGroup(t, p.StaticVariation).Known() {
			bad("%s: static_variation %d is not a variation of %s", what, p.StaticVariation, typeName(t))
		}
		if p.EventVariation != 0 && !eventGroup(t, p.EventVariation).Known() {
			bad("%s: event_variation %d is not an event variation of %s", what, p.EventVariation, typeName(t))
		}
		if s := p.Simulate; s != nil {
			if err := s.validate(t); err != nil {
				bad("%s: simulate: %v", what, err)
			}
		}
	}

	controls := map[pointKey]bool{}
	for i, c := range m.Controls {
		what := fmt.Sprintf("controls[%d]", i)
		typ := strings.ToLower(strings.TrimSpace(c.Type))
		targets, ok := controlTargets[typ]
		if !ok {
			bad("%s: type %q: use crob or analog", what, c.Type)
			continue
		}
		what = fmt.Sprintf("%s (%s %d)", what, typ, c.Index)
		k := pointKey{targets[0], c.Index}
		if controls[k] {
			bad("%s: the index is used twice", what)
		}
		controls[k] = true
		if c.Status != "" {
			if _, ok := commandStatuses[strings.ToUpper(strings.TrimSpace(c.Status))]; !ok {
				bad("%s: status %q: use a command status such as LOCAL, NOT_SUPPORTED or HARDWARE_ERROR", what, c.Status)
			}
		}
		if typ == "crob" && (c.Min != nil || c.Max != nil) {
			bad("%s: min and max are for analog controls", what)
		}
		if c.Min != nil && c.Max != nil && *c.Max < *c.Min {
			bad("%s: max is below min", what)
		}
		if c.Target == nil {
			continue
		}
		tt, ok := pointTypes[strings.ToLower(strings.TrimSpace(c.Target.Type))]
		if !ok {
			bad("%s: target type %q: use one of %s", what, c.Target.Type, strings.Join(PointTypeNames(), ", "))
			continue
		}
		fits := false
		for _, t := range targets {
			fits = fits || t == tt
		}
		switch {
		case !fits:
			bad("%s: target %s %d: a %s control sets a %s or a %s", what, typeName(tt), c.Target.Index, typ, typeName(targets[0]), typeName(targets[1]))
		case !points[pointKey{tt, c.Target.Index}]:
			bad("%s: target %s %d is not a point of the model", what, typeName(tt), c.Target.Index)
		}
	}

	names := map[string]bool{}
	for i, f := range m.Files {
		what := fmt.Sprintf("files[%d]", i)
		name, err := cleanName(f.Name)
		if err != nil || name == "/" {
			bad("%s: name %q: use a path such as /logs/events.csv", what, f.Name)
			continue
		}
		what = fmt.Sprintf("%s (%s)", what, name)
		if names[name] {
			bad("%s: the name is used twice", what)
		}
		names[name] = true
		sources := 0
		for _, set := range []bool{f.Path != "", f.Content != "", f.Size != 0} {
			if set {
				sources++
			}
		}
		if sources != 1 {
			bad("%s: give exactly one of path, content and size", what)
			continue
		}
		if f.Size < 0 {
			bad("%s: size cannot be negative", what)
			continue
		}
		if _, err := dnp.ParsePermissions(f.Permissions); err != nil {
			bad("%s: %v", what, err)
		}
		data, err := m.fileContent(f)
		switch {
		case err != nil:
			bad("%s: %v", what, err)
		case int64(len(data)) > math.MaxUint32:
			bad("%s: %d octets: the maximum is %d", what, len(data), uint32(math.MaxUint32))
		}
	}
	for n := range names {
		for p := path.Dir(n); p != "/"; p = path.Dir(p) {
			if names[p] {
				bad("files: %s is a file and a directory of %s", p, n)
			}
		}
	}
	if m.Uploads.Dir != "" && !m.Uploads.Accept {
		bad("uploads.dir is set but uploads.accept is not")
	}

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("invalid model:\n  - %s", strings.Join(problems, "\n  - "))
}

// staticGroup and eventGroup return the object of variation v of a point type.
func staticGroup(t server.PointType, v uint8) app.GroupVariation {
	groups := map[server.PointType]uint8{
		server.BinaryInput: 1, server.DoubleBitInput: 3, server.BinaryOutput: 10, server.Counter: 20,
		server.FrozenCounter: 21, server.AnalogInput: 30, server.FrozenAnalogInput: 31, server.AnalogOutput: 40, server.OctetString: 110,
	}
	return app.GV(groups[t], v)
}

func eventGroup(t server.PointType, v uint8) app.GroupVariation {
	groups := map[server.PointType]uint8{
		server.BinaryInput: 2, server.DoubleBitInput: 4, server.BinaryOutput: 11, server.Counter: 22,
		server.FrozenCounter: 23, server.AnalogInput: 32, server.FrozenAnalogInput: 33, server.AnalogOutput: 42, server.OctetString: 111,
	}
	return app.GV(groups[t], v)
}

// attributeVariation parses the key of outstation.attributes: a name or a number.
func attributeVariation(key string) (uint8, error) {
	v, err := dnp.ParseAttributeVariation(key)
	if err != nil || v >= 254 {
		return 0, fmt.Errorf("%q is not a device attribute: use a name such as manufacturer or a variation 1-253", key)
	}
	return v, nil
}

// cleanName normalizes a file name to an absolute slash-separated path.
func cleanName(name string) (string, error) {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	if name == "" {
		return "", errors.New("empty file name")
	}
	return path.Clean("/" + name), nil
}

// fileContent returns the octets of a file of the model.
func (m *Model) fileContent(f FileSpec) ([]byte, error) {
	switch {
	case f.Path != "":
		return os.ReadFile(m.resolve(f.Path))
	case f.Content != "":
		return []byte(f.Content), nil
	}
	data := make([]byte, f.Size)
	for i := range data {
		data[i] = byte('A' + i%26)
	}
	return data, nil
}

func numeric(t server.PointType) bool {
	switch t {
	case server.Counter, server.FrozenCounter, server.AnalogInput, server.FrozenAnalogInput, server.AnalogOutput:
		return true
	default:
		return false
	}
}

func (s *SimSpec) validate(t server.PointType) error {
	if time.Duration(s.Interval) < 50*time.Millisecond {
		return fmt.Errorf("interval %s: use at least 50ms", time.Duration(s.Interval))
	}
	switch strings.ToLower(s.Mode) {
	case "toggle":
		if t != server.BinaryInput && t != server.DoubleBitInput && t != server.BinaryOutput {
			return fmt.Errorf("mode toggle is for binary and double-bit points; a %s takes ramp, sine, random or count", typeName(t))
		}
	case "ramp", "sine", "random":
		if !numeric(t) {
			return fmt.Errorf("mode %s is for counters and analog points; a %s takes toggle", s.Mode, typeName(t))
		}
		if s.Max <= s.Min {
			return fmt.Errorf("mode %s needs max above min", s.Mode)
		}
	case "count":
		if !numeric(t) {
			return fmt.Errorf("mode count is for counters and analog points; a %s takes toggle", typeName(t))
		}
	default:
		return fmt.Errorf("mode %q: use toggle, ramp, sine, random or count", s.Mode)
	}
	if s.Step < 0 {
		return errors.New("step cannot be negative")
	}
	return nil
}

// flags returns the flags of a point: ONLINE unless the model lists them.
func (p PointSpec) flags() (app.Flags, error) {
	if p.Flags == nil {
		return app.FlagOnline, nil
	}
	var f app.Flags
	for _, name := range p.Flags {
		b, ok := flagBits[strings.ToUpper(strings.TrimSpace(name))]
		if !ok {
			return 0, fmt.Errorf("flag %q: use ONLINE, RESTART, COMM_LOST, REMOTE_FORCED, LOCAL_FORCED, CHATTER_FILTER, OVER_RANGE, DISCONTINUITY, ROLLOVER or REFERENCE_ERR", name)
		}
		f |= b
	}
	return f, nil
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	}
	return 0, false
}

// initialValue converts the value of a point of the model to the value of the database.
func initialValue(t server.PointType, p PointSpec) (app.Value, error) {
	flags, err := p.flags()
	if err != nil {
		return nil, err
	}
	switch t {
	case server.BinaryInput, server.BinaryOutput:
		b, ok := boolValue(p.Value)
		if !ok {
			return nil, fmt.Errorf("value %v is not a binary state: use true or false (on or off)", p.Value)
		}
		return app.Binary{Value: b, Flags: flags}, nil
	case server.DoubleBitInput:
		d, ok := doubleBitValue(p.Value)
		if !ok {
			return nil, fmt.Errorf("value %v is not a double-bit state: use off, on, intermediate or indeterminate", p.Value)
		}
		return app.DoubleBitBinary{Value: d, Flags: flags}, nil
	case server.OctetString:
		s := fmt.Sprint(p.Value)
		if p.Value == nil || s == "" || len(s) > 255 {
			return nil, fmt.Errorf("value: an octet string holds 1-255 octets of text")
		}
		return app.OctetString{Data: []byte(s)}, nil
	case server.Counter, server.FrozenCounter:
		f, ok := toFloat(p.Value)
		if !ok || f < 0 || f > math.MaxUint32 || f != math.Trunc(f) {
			return nil, fmt.Errorf("value %v: a counter holds a whole number 0-%d", p.Value, uint32(math.MaxUint32))
		}
		return app.Counter{Value: uint32(f), Flags: flags}, nil
	default:
		f, ok := toFloat(p.Value)
		if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("value %v is not a number", p.Value)
		}
		return app.Analog{Value: f, Flags: flags}, nil
	}
}

func boolValue(v any) (bool, bool) {
	switch b := v.(type) {
	case bool:
		return b, true
	case string:
		switch strings.ToLower(strings.TrimSpace(b)) {
		case "on", "true", "1":
			return true, true
		case "off", "false", "0":
			return false, true
		}
	case int:
		if b == 0 || b == 1 {
			return b == 1, true
		}
	}
	return false, false
}

func doubleBitValue(v any) (app.DoubleBit, bool) {
	switch s := v.(type) {
	case bool:
		if s {
			return app.DoubleBitOn, true
		}
		return app.DoubleBitOff, true
	case string:
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "off":
			return app.DoubleBitOff, true
		case "on":
			return app.DoubleBitOn, true
		case "intermediate":
			return app.DoubleBitIntermediate, true
		case "indeterminate":
			return app.DoubleBitIndeterminate, true
		}
	case int:
		if s >= 0 && s <= 3 {
			return app.DoubleBit(s), true
		}
	}
	return 0, false
}

// Encode writes the model as YAML, or as JSON when asJSON is set.
func (m *Model) Encode(asJSON bool) ([]byte, error) {
	if asJSON {
		b, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(b, '\n'), nil
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
