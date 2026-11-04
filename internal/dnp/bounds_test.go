// SPDX-License-Identifier: MIT

package dnp

import (
	"fmt"
	"testing"

	"github.com/otfabric/go-dnp3/secauth"
)

// TestParsePermissionsRunes: the rwx form compares characters, not the low octet of
// a rune: U+0172 ends in 0x72 ('r') and is no permission letter.
func TestParsePermissionsRunes(t *testing.T) {
	for _, in := range []string{"Ųxr--r--", "ŷ-xr--r-", "rwŸr--r-"} {
		if len(in) != 9 {
			t.Fatalf("%q: %d octets, want 9", in, len(in))
		}
		if p, err := ParsePermissions(in); err == nil {
			t.Errorf("ParsePermissions(%q) = %o, want an error", in, p)
		}
	}
}

// TestStatisticName: an index past 65535 is not truncated into a statistic.
func TestStatisticName(t *testing.T) {
	if got, want := statisticName(0), secauth.Statistic(0).String(); got != want {
		t.Errorf("statisticName(0) = %q, want %q", got, want)
	}
	if got := statisticName(0x10000); got != "statistic 65536" {
		t.Errorf("statisticName(0x10000) = %q, want statistic 65536", got)
	}
}

// TestExpandHostsLastOctet: a range may end at .255 and is walked to its end, once.
func TestExpandHostsLastOctet(t *testing.T) {
	for _, c := range []struct {
		spec       string
		n          int
		first, end string
	}{
		{"10.0.0.250-255", 6, "10.0.0.250", "10.0.0.255"},
		{"10.0.0.255-255", 1, "10.0.0.255", "10.0.0.255"},
		{"10.0.0.0-255", 256, "10.0.0.0", "10.0.0.255"},
	} {
		hosts, err := ExpandHosts([]string{c.spec}, 0)
		if err != nil || len(hosts) != c.n || hosts[0] != c.first || hosts[len(hosts)-1] != c.end {
			t.Errorf("ExpandHosts(%s) = %d hosts %v, %v", c.spec, len(hosts), ends(hosts), err)
		}
	}
	for _, spec := range []string{"10.0.0.5-4", "10.0.0.1-256"} {
		if _, err := ExpandHosts([]string{spec}, 0); err == nil {
			t.Errorf("ExpandHosts(%s) accepted", spec)
		}
	}
}

func ends(h []string) string {
	if len(h) == 0 {
		return "[]"
	}
	return fmt.Sprintf("[%s .. %s]", h[0], h[len(h)-1])
}
