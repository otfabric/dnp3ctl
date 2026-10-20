package dnp

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/client"

	"github.com/otfabric/dnp3ctl/internal/types"
)

// pointTypes names the kind of point of each object group.
var pointTypes = map[uint8]string{
	1: "binary-input", 2: "binary-input",
	3: "double-bit-input", 4: "double-bit-input",
	10: "binary-output", 11: "binary-output",
	13: "binary-output-command",
	20: "counter", 22: "counter",
	21: "frozen-counter", 23: "frozen-counter",
	30: "analog-input", 32: "analog-input",
	31: "frozen-analog-input", 33: "frozen-analog-input",
	34: "analog-deadband",
	40: "analog-output", 42: "analog-output",
	43:  "analog-output-command",
	80:  "iin",
	110: "octet-string", 111: "octet-string",
	121: "security-statistic", 122: "security-statistic",
}

// PointType returns the kind of point of gv, or "g<group>" for a group without a name.
func PointType(gv app.GroupVariation) string {
	if t, ok := pointTypes[gv.Group()]; ok {
		return t
	}
	return "g" + gv.String()[1:strings.IndexByte(gv.String(), 'v')]
}

// DoubleBitText renders a double-bit state as the results show it.
func DoubleBitText(d app.DoubleBit) string {
	switch d {
	case app.DoubleBitOff:
		return "off"
	case app.DoubleBitOn:
		return "on"
	case app.DoubleBitIntermediate:
		return "intermediate"
	default:
		return "indeterminate"
	}
}

// flagNames are the common flag bits; specific holds the names of bits 5 and 6 by group.
var (
	flagNames = []struct {
		bit  app.Flags
		name string
	}{
		{app.FlagOnline, "ONLINE"},
		{app.FlagRestart, "RESTART"},
		{app.FlagCommLost, "COMM_LOST"},
		{app.FlagRemoteForced, "REMOTE_FORCED"},
		{app.FlagLocalForced, "LOCAL_FORCED"},
	}
	specific = map[string][2]string{
		"binary-input":        {"CHATTER_FILTER", "BIT6"},
		"double-bit-input":    {"CHATTER_FILTER", "BIT6"},
		"counter":             {"ROLLOVER", "DISCONTINUITY"},
		"frozen-counter":      {"ROLLOVER", "DISCONTINUITY"},
		"analog-input":        {"OVER_RANGE", "REFERENCE_ERR"},
		"frozen-analog-input": {"OVER_RANGE", "REFERENCE_ERR"},
		"analog-output":       {"OVER_RANGE", "REFERENCE_ERR"},
	}
)

// FlagsText renders the flags of a point of the given kind by name, joined with "|".
func FlagsText(kind string, f app.Flags) string {
	var parts []string
	for _, n := range flagNames {
		if f&n.bit != 0 {
			parts = append(parts, n.name)
		}
	}
	names, ok := specific[kind]
	if !ok {
		names = [2]string{"BIT5", "BIT6"}
	}
	if f&0x20 != 0 {
		parts = append(parts, names[0])
	}
	if f&0x40 != 0 {
		parts = append(parts, names[1])
	}
	if f&0x80 != 0 {
		parts = append(parts, "BIT7")
	}
	return strings.Join(parts, "|")
}

// timeText renders a DNP3 time stamp, or "" for none.
func timeText(t app.Timestamp) string {
	if t == 0 {
		return ""
	}
	return t.Time().UTC().Format(timeLayout)
}

// timeLayout is RFC 3339 with milliseconds, the time format of every result.
const timeLayout = "2006-01-02T15:04:05.000Z07:00"

// Now returns the current time as the results show it.
func Now() string { return time.Now().UTC().Format(timeLayout) }

// octetText renders an octet string as text when it is printable, otherwise as 0x hex.
func octetText(b []byte) string {
	printable := len(b) > 0
	for _, r := range string(b) {
		if !unicode.IsPrint(r) && r != ' ' {
			printable = false
			break
		}
	}
	if printable {
		return string(b)
	}
	return "0x" + hex.EncodeToString(b)
}

// hasFlags reports whether gv carries a flags octet: not the packed variations, the
// variations "without flag" and the objects that have none.
func hasFlags(gv app.GroupVariation) bool {
	switch gv.Group() {
	case 13, 34, 43, 80, 110, 111, 112, 113:
		return false
	}
	name := gv.Name()
	return !strings.Contains(name, "Without Flag") && !strings.Contains(name, "Packed")
}

// ToMeasurement converts a measurement of go-dnp3 into the measurement of the results.
func ToMeasurement(m client.Measurement) types.Measurement {
	kind := PointType(m.GV)
	out := types.Measurement{
		Object:    m.GV.String(),
		Group:     m.GV.Group(),
		Variation: m.GV.Variation(),
		Type:      kind,
		Index:     m.Index,
		Event:     m.Event,
	}
	withFlags := hasFlags(m.GV)
	switch v := m.Value.(type) {
	case app.Binary:
		out.Value, out.Timestamp = v.Value, timeText(v.Time)
		if withFlags {
			out.Flags = FlagsText(kind, v.Flags)
		}
	case app.DoubleBitBinary:
		out.Value, out.Timestamp = DoubleBitText(v.Value), timeText(v.Time)
		if withFlags {
			out.Flags = FlagsText(kind, v.Flags)
		}
	case app.Counter:
		out.Value, out.Timestamp = float64(v.Value), timeText(v.Time)
		if withFlags {
			out.Flags = FlagsText(kind, v.Flags)
		}
	case app.Analog:
		out.Value, out.Timestamp = analogValue(m.GV, v.Value), timeText(v.Time)
		if withFlags {
			out.Flags = FlagsText(kind, v.Flags)
		}
	case app.AnalogDeadband:
		out.Value = v.Value
	case app.BinaryCommandEvent:
		out.Value, out.Status, out.Timestamp = v.Value, v.Status.String(), timeText(v.Time)
	case app.AnalogCommandEvent:
		out.Value, out.Status, out.Timestamp = v.Value, v.Status.String(), timeText(v.Time)
	case app.OctetString:
		out.Value = octetText(v.Data)
	case app.SecurityStatistic:
		out.Value, out.Timestamp = float64(v.Value), timeText(v.Time)
		out.Flags = FlagsText(kind, v.Flags)
	default:
		out.Value = nil
	}
	return out
}

// analogValue returns a value sent in a single-precision variation as the shortest decimal
// that variation holds (11.02, not 11.020000457763672).
func analogValue(gv app.GroupVariation, v float64) float64 {
	if !strings.Contains(gv.Name(), "Single-precision") {
		return v
	}
	f, err := strconv.ParseFloat(strconv.FormatFloat(v, 'g', -1, 32), 64)
	if err != nil {
		return v
	}
	return f
}

// CROBText renders a control relay output block without its status: "LATCH_ON",
// "PULSE_ON CLOSE count=1 on=500ms off=0ms".
func CROBText(c app.CROB) string {
	s := c.Op.String()
	if c.TripClose != app.TCCNul {
		s += " " + c.TripClose.String()
	}
	if c.Op == app.OpPulseOn || c.Op == app.OpPulseOff || c.Count > 1 {
		s += fmt.Sprintf(" count=%d on=%dms off=%dms", c.Count, c.OnTime, c.OffTime)
	}
	return s
}
