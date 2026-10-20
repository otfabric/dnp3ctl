package types

import (
	"fmt"
	"strconv"
	"strings"
)

// Measurement is one point value or event as an outstation reported it.
type Measurement struct {
	// ReceivedAt is when the value arrived (RFC 3339, UTC, milliseconds). Streams only.
	ReceivedAt string `json:"received_at,omitempty"`
	// Unsolicited is set for a value of an unsolicited response. Streams only.
	Unsolicited bool `json:"unsolicited,omitempty"`
	// Object is the group and variation the value was sent as ("g30v5"); Group and
	// Variation are its numbers.
	Object    string `json:"object"`
	Group     uint8  `json:"group"`
	Variation uint8  `json:"variation"`
	// Type is the kind of point: "binary-input", "double-bit-input", "binary-output",
	// "binary-output-command", "counter", "frozen-counter", "analog-input",
	// "frozen-analog-input", "analog-deadband", "analog-output", "analog-output-command",
	// "octet-string", "security-statistic" or "iin".
	Type  string `json:"type"`
	Index uint32 `json:"index"`
	// Event is set for an object of an event group.
	Event bool `json:"event,omitempty"`
	// Value is a boolean for binary points, a string for double-bit inputs ("off", "on",
	// "intermediate", "indeterminate") and octet strings, and a number for everything else.
	Value any `json:"value"`
	// Flags are the quality flags that are set, joined with "|" ("ONLINE|RESTART"); empty
	// when none is set. Omitted for objects without flags.
	Flags string `json:"flags,omitempty"`
	// Status is the command status of a command event.
	Status string `json:"status,omitempty"`
	// Timestamp is the time of the event or of the freeze (RFC 3339 with milliseconds), for
	// the variations with time.
	Timestamp string `json:"timestamp,omitempty"`
}

// ValueText renders the value for text and table output.
func (m Measurement) ValueText() string {
	switch v := m.Value.(type) {
	case nil:
		return "-"
	case bool:
		if v {
			return "on"
		}
		return "off"
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// Line renders the measurement as one line of text.
func (m Measurement) Line() string {
	var b strings.Builder
	if m.ReceivedAt != "" {
		b.WriteString(m.ReceivedAt)
		b.WriteString("  ")
		if m.Unsolicited {
			b.WriteString("unsol  ")
		} else {
			b.WriteString("resp   ")
		}
	}
	_, _ = fmt.Fprintf(&b, "%-7s %-20s %5d  %-14s", m.Object, m.Type, m.Index, m.ValueText())
	if m.Flags != "" {
		_, _ = fmt.Fprintf(&b, "  [%s]", m.Flags)
	}
	if m.Status != "" {
		_, _ = fmt.Fprintf(&b, "  %s", m.Status)
	}
	if m.Timestamp != "" {
		_, _ = fmt.Fprintf(&b, "  @ %s", m.Timestamp)
	}
	return strings.TrimRight(b.String(), " ")
}

// MeasurementHeaders are the table columns of a measurement.
var MeasurementHeaders = []string{"OBJECT", "TYPE", "INDEX", "VALUE", "FLAGS", "TIME"}

// Row renders the measurement as a table row matching [MeasurementHeaders].
func (m Measurement) Row() []string {
	flags := m.Flags
	if m.Status != "" {
		flags = strings.TrimPrefix(flags+" "+m.Status, " ")
	}
	return []string{m.Object, m.Type, strconv.FormatUint(uint64(m.Index), 10), m.ValueText(), orDash(flags), orDash(m.Timestamp)}
}

// MarshalTextOutput implements format.TextMarshaler for a measurement in a stream.
func (m Measurement) MarshalTextOutput() (string, error) { return m.Line(), nil }

// TableHeaders implements format.TableMarshaler for a measurement in a stream.
func (m Measurement) TableHeaders() []string {
	return append([]string{"RECEIVED", "SOURCE"}, MeasurementHeaders...)
}

// TableRows implements format.TableMarshaler for a measurement in a stream.
func (m Measurement) TableRows() [][]string {
	source := "response"
	if m.Unsolicited {
		source = "unsolicited"
	}
	return [][]string{append([]string{m.ReceivedAt, source}, m.Row()...)}
}

func measurementLines(b *strings.Builder, ms []Measurement) {
	for _, m := range ms {
		b.WriteString("  ")
		b.WriteString(m.Line())
		b.WriteString("\n")
	}
}

func measurementRows(ms []Measurement) [][]string {
	rows := make([][]string, 0, len(ms))
	for _, m := range ms {
		rows = append(rows, m.Row())
	}
	return rows
}
