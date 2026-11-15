// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"
	"testing"

	"github.com/otfabric/dnp3ctl/internal/stationtest"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// controlOctet returns the first octet of s below 0x20, or 0x7f, other than a line break,
// or -1.
func controlOctet(s string) int {
	for i := 0; i < len(s); i++ {
		if c := s[i]; (c < 0x20 || c == 0x7f) && c != '\n' {
			return i
		}
	}
	return -1
}

// TestPeerStringsEscaped: a device attribute and a file name an outstation sends with ESC,
// CR and LF in them are escaped in text and table output, one line each; JSON has them as
// they are.
func TestPeerStringsEscaped(t *testing.T) {
	model := strings.NewReplacer(
		"product: test outstation", `product: "test\e[2J\r\nforged"`,
		`{name: /readme.txt, content: "hello from the outstation\n"}`, `{name: "/read\e[1mme\r\n.txt", content: x}`,
	).Replace(stationtest.Model)
	if model == stationtest.Model {
		t.Fatal("the test model has changed")
	}
	st := stationtest.Start(t, model)
	for _, c := range []struct {
		args    []string
		escaped string
	}{
		{[]string{"attributes", "--variation", "product"}, `test\x1b[2J\r\nforged`},
		{[]string{"file", "ls"}, `read\x1b[1mme\r\n.txt`},
	} {
		for _, format := range []string{"text", "table"} {
			r := cli(t, st, append(c.args, "--format", format)...).want(t, 0)
			if i := controlOctet(r.stdout); i >= 0 {
				t.Errorf("%v %s: a control character at %d: %q", c.args, format, i, r.stdout)
			}
			if !strings.Contains(r.stdout, c.escaped) {
				t.Errorf("%v %s: the string is not shown escaped:\n%s", c.args, format, r.stdout)
			}
			if strings.Contains(r.stdout, "\nforged") || strings.Contains(r.stdout, "\n.txt") {
				t.Errorf("%v %s: a line of its own from the outstation:\n%s", c.args, format, r.stdout)
			}
		}
	}
	var attrs types.AttributesResult
	cli(t, st, "attributes", "--variation", "product", "--format", "json").want(t, 0).decode(t, &attrs)
	if len(attrs.Attributes) != 1 || attrs.Attributes[0].Value != "test\x1b[2J\r\nforged" {
		t.Errorf("JSON: %+v", attrs)
	}
}
