// SPDX-License-Identifier: MIT

package types

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// controls reports the first octet of s below 0x20 or 0x7f that is not a line break at the
// end of a line, or -1.
func controls(s string) int {
	for i := 0; i < len(s); i++ {
		if c := s[i]; (c < 0x20 || c == 0x7f) && c != '\n' {
			return i
		}
	}
	return -1
}

func TestPrintable(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"Bay 1", "Bay 1"},
		{"naïve – ok", "naïve – ok"},
		{`C:\dir "x"`, `C:\dir "x"`},
		{"\x1b[2J", `\x1b[2J`},
		{"a\r\nb", `a\r\nb`},
		{"tab\there", `tab\there`},
		{"\x7f", `\x7f`},
		{"\u009b2J", `\u009b2J`},
		{"\xff\xfe", `\xff\xfe`},
		{`say "\x1b" and ` + "\x1b", `say \"\\x1b\" and \x1b`},
	} {
		if got := Printable(c.in); got != c.want {
			t.Errorf("Printable(%q) = %s, want %s", c.in, got, c.want)
		}
	}
	if got := PrintableLines("invalid model:\n  - a\x1bb\n  - c"); got != "invalid model:\n  - a\\x1bb\n  - c" {
		t.Errorf("PrintableLines: %q", got)
	}
	if got := PrintableLines("Did you mean this?\n\tpoll\r"); got != "Did you mean this?\n\tpoll\\r" {
		t.Errorf("PrintableLines with a tab: %q", got)
	}
}

// TestPeerStringsInText: strings a peer sent are written to text and table output without
// control characters; JSON keeps them, escaped.
func TestPeerStringsInText(t *testing.T) {
	const peer = "name\x1b[2J\r\nforged line"
	attrs := &AttributesResult{Target: "tcp://x:20000", Outstation: 10, Attributes: []Attribute{{Variation: 247, Name: "user-device-name", DataType: "VSTR", Value: peer}}}
	files := &FileListResult{Target: "tcp://x:20000", Path: "/", Entries: []FileEntry{{Name: "/" + peer, Type: "file", Permissions: "rw-r--r--"}}}
	info := &FileInfoResult{Target: "tcp://x:20000", Path: "/x", Entry: FileEntry{Name: peer, Type: "file", Permissions: "rw-r--r--"}}
	m := Measurement{Object: "g110v11", Type: "octet-string", Value: peer}
	ev := &ServerEvent{Timestamp: "t", Event: "write", Object: "g0v247", Value: peer, Detail: "file " + peer}
	for name, out := range map[string]interface{ MarshalTextOutput() (string, error) }{
		"attributes": attrs, "file ls": files, "file info": info, "measurement": m, "server event": ev,
	} {
		text, err := out.MarshalTextOutput()
		if err != nil {
			t.Fatal(err)
		}
		if i := controls(text); i >= 0 {
			t.Errorf("%s: a control character at %d of the text: %q", name, i, text)
		}
		if !strings.Contains(text, `name\x1b[2J\r\nforged line`) {
			t.Errorf("%s: the text does not show the string escaped: %q", name, text)
		}
		if t2, ok := out.(interface{ TableRows() [][]string }); ok {
			for _, row := range t2.TableRows() {
				for _, cell := range row {
					if i := controls(cell); i >= 0 {
						t.Errorf("%s: a control character in the table cell %q", name, cell)
					}
				}
			}
		}
		data, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `name\u001b[2J\r\nforged line`) {
			t.Errorf("%s: JSON does not hold the string: %s", name, data)
		}
	}
}

// TestNonFinite: NaN and the infinities have the documented JSON form, a null value and
// non_finite; every other value is unchanged.
func TestNonFinite(t *testing.T) {
	for _, c := range []struct {
		in        any
		value     any
		nonFinite string
	}{
		{math.NaN(), nil, "NaN"},
		{math.Inf(1), nil, "+Inf"},
		{math.Inf(-1), nil, "-Inf"},
		{float32(math.Inf(-1)), nil, "-Inf"},
		{1.5, 1.5, ""},
		{math.MaxFloat64, math.MaxFloat64, ""},
		{"NaN", "NaN", ""},
		{nil, nil, ""},
		{true, true, ""},
	} {
		v, nf := Finite(c.in)
		if v != c.value || nf != c.nonFinite {
			t.Errorf("Finite(%v) = %v, %q", c.in, v, nf)
		}
	}
	m := Measurement{Object: "g30v5", Type: "analog-input", Index: 3}
	m.Value, m.NonFinite = Finite(math.NaN())
	data, err := json.Marshal(m)
	if err != nil || !strings.Contains(string(data), `"value":null,"non_finite":"NaN"`) {
		t.Errorf("JSON of NaN: %s, %v", data, err)
	}
	if m.ValueText() != "NaN" || !strings.Contains(m.Line(), "NaN") {
		t.Errorf("text of NaN: %q", m.Line())
	}
	if data, _ := json.Marshal(Measurement{Value: 1.5}); strings.Contains(string(data), "non_finite") {
		t.Errorf("a finite value: %s", data)
	}
	a := Attribute{Variation: 1, Name: "attribute-1", DataType: "FLT", NonFinite: "+Inf"}
	if a.valueText() != "+Inf" {
		t.Errorf("attribute text: %q", a.valueText())
	}
	ev := &ServerEvent{Timestamp: "t", Event: "control", Function: "OPERATE", NonFinite: "-Inf", Status: "OUT_OF_RANGE"}
	if text, _ := ev.MarshalTextOutput(); !strings.Contains(text, "  -Inf  OUT_OF_RANGE") {
		t.Errorf("event text: %q", text)
	}
	if rows := ev.TableRows(); rows[0][7] != "-Inf" {
		t.Errorf("event row: %q", rows[0])
	}
	if data, err := json.Marshal(ev); err != nil || strings.Contains(string(data), `"value"`) || !strings.Contains(string(data), `"non_finite":"-Inf"`) {
		t.Errorf("event JSON: %s, %v", data, err)
	}
}
