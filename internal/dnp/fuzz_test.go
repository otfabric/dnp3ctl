// SPDX-License-Identifier: MIT

package dnp

import (
	"reflect"
	"testing"
)

// What ParseObject accepts has a name that parses back to the same object.
func FuzzParseObject(f *testing.F) {
	for _, s := range []string{"g30v5", "30v5", "30.5", "30:5", "g30", "30", "g30v0", "", "g", "v", "g999v1", "g30v999", " G1V2 ", "1,2"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		gv, err := ParseObject(s)
		if err != nil {
			return
		}
		back, err := ParseObject(gv.String())
		if err != nil || back != gv {
			t.Fatalf("ParseObject(%q) = %v; its name parses to %v, %v", s, gv, back, err)
		}
	})
}

// A selection is what goes into a request: its rendering parses back to itself.
func FuzzParseSelection(f *testing.F) {
	for _, s := range []string{"", "0-9", "1,4,7", "5", "9-0", "1,,2", "-", "4294967295", "4294967296", "0-4294967295", " 1 - 2 "} {
		f.Add(s, uint32(0))
	}
	f.Add("", uint32(5))
	f.Add("1", uint32(5))
	f.Fuzz(func(t *testing.T, rng string, count uint32) {
		sel, err := ParseSelection(rng, count)
		if err != nil || sel.All || sel.Count != 0 {
			return
		}
		if len(sel.List) == 0 && sel.Stop < sel.Start {
			t.Fatalf("ParseSelection(%q): %+v", rng, sel)
		}
		back, err := ParseSelection(sel.String(), 0)
		if err != nil {
			t.Fatalf("ParseSelection(%q) renders as %q, which fails: %v", rng, sel.String(), err)
		}
		if len(sel.List) == 1 {
			// A list of one renders as one index.
			if back.Start != sel.List[0] || back.Stop != sel.List[0] {
				t.Fatalf("ParseSelection(%q) = %+v; back %+v", rng, sel, back)
			}
			return
		}
		if !reflect.DeepEqual(back, sel) {
			t.Fatalf("ParseSelection(%q) = %+v; back %+v", rng, sel, back)
		}
	})
}

// Every index a filter names matches it, and the bounds around a single index
// do not.
func FuzzParseIndexFilter(f *testing.F) {
	for _, s := range []string{"", "0,5-9", "7", "9-5", "a", "1-", "-1", "0-4294967295", ",,"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		filter, err := ParseIndexFilter(s)
		if err != nil {
			return
		}
		for _, r := range filter.ranges {
			if r[1] < r[0] {
				t.Fatalf("ParseIndexFilter(%q): range %v", s, r)
			}
			if !filter.Match(r[0]) || !filter.Match(r[1]) {
				t.Fatalf("ParseIndexFilter(%q): %v does not match its own bounds", s, r)
			}
		}
	})
}

// A device attribute value given on the command line, of any type.
func FuzzParseAttribute(f *testing.F) {
	for _, s := range []string{"", "1", "-1", "1.5", "text", "2026-10-31T00:00:00Z", "ff", "1e400"} {
		for _, typ := range []string{"", "string", "int", "uint", "float", "octets", "bitstring", "time", "nope"} {
			f.Add(s, typ)
		}
	}
	f.Fuzz(func(t *testing.T, value, dataType string) {
		_, _ = ParseAttribute(value, dataType)
	})
}

// What ParsePermissions accepts is at most rwxrwxrwx, and its rwx form parses back to
// the same permissions.
func FuzzParsePermissions(f *testing.F) {
	for _, s := range []string{"", "644", "0644", "777", "1000", "rw-r--r--", "rwxrwxrwx", "---------", "Ųxr--r--", "rw-r--r-", "rwxrwxrwz"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		p, err := ParsePermissions(s)
		if err != nil {
			return
		}
		if p > 0o777 {
			t.Fatalf("ParsePermissions(%q) = %o, more than 777", s, p)
		}
		text := p.String()
		back, err := ParsePermissions(text)
		if err != nil || back != p {
			t.Fatalf("ParsePermissions(%q) = %o, whose form %q parses to %o, %v", s, p, text, back, err)
		}
	})
}

// ExpandHosts never panics, and what it accepts with a limit is at most that many
// hosts, none twice.
func FuzzExpandHosts(f *testing.F) {
	for _, s := range []string{"10.0.0.1", "10.0.0.250-255", "10.0.0.0/30", "10.0.0.0/24", "10.0.0.5-4", "::1", "host", "1.2.3.4-", "0.0.0.0/0", "10.0.0.1,10.0.0.2"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		const limit = 256
		hosts, err := ExpandHosts([]string{s}, limit)
		if err != nil {
			return
		}
		if len(hosts) > limit {
			t.Fatalf("ExpandHosts(%q) gave %d hosts, limit %d", s, len(hosts), limit)
		}
		seen := map[string]bool{}
		for _, h := range hosts {
			if seen[h] {
				t.Fatalf("ExpandHosts(%q) gave %s twice", s, h)
			}
			seen[h] = true
		}
	})
}
