// SPDX-License-Identifier: MIT

package types

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
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
	// "octet-string", "time-and-interval", "security-statistic", "iin" or
	// "virtual-terminal" (g112, g113: the index is the virtual port).
	Type  string `json:"type"`
	Index uint32 `json:"index"`
	// Event is set for an object of an event group.
	Event bool `json:"event,omitempty"`
	// Value is a boolean for binary points, a string for double-bit inputs ("off", "on",
	// "intermediate", "indeterminate"), octet strings and the time of a time and interval
	// (RFC 3339 with milliseconds), and a number for everything else.
	Value any `json:"value"`
	// NonFinite names a number that JSON cannot hold, "NaN", "+Inf" or "-Inf"; Value is
	// then nil. Omitted for every other value.
	NonFinite string `json:"non_finite,omitempty"`
	// Flags are the quality flags that are set, joined with "|" ("ONLINE|RESTART"); empty
	// when none is set. Omitted for objects without flags.
	Flags string `json:"flags,omitempty"`
	// Status is the command status of a command event.
	Status string `json:"status,omitempty"`
	// Interval and IntervalUnit are the interval of a time and interval (g50v4): a count of
	// the unit ("none", "milliseconds", "seconds", "minutes", "hours", "days", "weeks",
	// "months", "months-day", "months-last", "months-same").
	Interval     *uint32 `json:"interval,omitempty"`
	IntervalUnit string  `json:"interval_unit,omitempty"`
	// Statistic names a security statistic ("authentication failures", "session key
	// changes", ...).
	Statistic string `json:"statistic,omitempty"`
	// Timestamp is the time of the event or of the freeze (RFC 3339 with milliseconds), for
	// the variations with time.
	Timestamp string `json:"timestamp,omitempty"`
}

// ValueText renders the value for text and table output.
func (m Measurement) ValueText() string {
	switch v := m.Value.(type) {
	case nil:
		if m.NonFinite != "" {
			return m.NonFinite
		}
		return "-"
	case bool:
		if v {
			return "on"
		}
		return "off"
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case string:
		return Printable(v)
	default:
		return fmt.Sprint(v)
	}
}

// Printable returns s for text and table output. A string with a character that is not
// printable, a control character such as ESC, CR or LF, or with octets that are not UTF-8,
// is written as a Go string literal without its quotes ("\x1b[2J", "a\nb"); any other
// string is returned as it is. JSON output escapes these characters by itself.
func Printable(s string) string {
	for _, r := range s {
		if r == utf8.RuneError || !unicode.IsPrint(r) {
			q := strconv.Quote(s)
			return q[1 : len(q)-1]
		}
	}
	return s
}

// PrintableLines is [Printable] for each line of s, and each part of a line between tabs:
// the line breaks and the tabs of a message laid out in lines stay.
func PrintableLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		parts := strings.Split(l, "\t")
		for j, p := range parts {
			parts[j] = Printable(p)
		}
		lines[i] = strings.Join(parts, "\t")
	}
	return strings.Join(lines, "\n")
}

// Finite returns v, or for a NaN or an infinite number, which JSON has no number for, nil
// and the name of the number: "NaN", "+Inf" or "-Inf". These names are what the text
// output prints for them.
func Finite(v any) (value any, nonFinite string) {
	var f float64
	switch n := v.(type) {
	case float64:
		f = n
	case float32:
		f = float64(n)
	default:
		return v, ""
	}
	switch {
	case math.IsNaN(f):
		return nil, "NaN"
	case math.IsInf(f, 1):
		return nil, "+Inf"
	case math.IsInf(f, -1):
		return nil, "-Inf"
	}
	return v, ""
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
	if d := m.detail(); d != "" {
		_, _ = fmt.Fprintf(&b, "  %s", d)
	}
	if m.Timestamp != "" {
		_, _ = fmt.Fprintf(&b, "  @ %s", m.Timestamp)
	}
	return strings.TrimRight(b.String(), " ")
}

// detail renders what some objects carry besides value and flags: the status of a command
// event, the interval of a time and interval, the name of a security statistic.
func (m Measurement) detail() string {
	var parts []string
	if m.Status != "" {
		parts = append(parts, m.Status)
	}
	if m.Interval != nil {
		parts = append(parts, fmt.Sprintf("every %d %s", *m.Interval, m.IntervalUnit))
	}
	if m.Statistic != "" {
		parts = append(parts, "("+m.Statistic+")")
	}
	return strings.Join(parts, " ")
}

// MeasurementHeaders are the table columns of a measurement.
var MeasurementHeaders = []string{"OBJECT", "TYPE", "INDEX", "VALUE", "FLAGS", "TIME"}

// Row renders the measurement as a table row matching [MeasurementHeaders].
func (m Measurement) Row() []string {
	flags := strings.TrimSpace(m.Flags + " " + m.detail())
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
