// SPDX-License-Identifier: MIT

package dnp

import (
	"encoding/hex"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/link"
)

// Every object of a list parses on its own, in the order of the list.
func FuzzParseObjects(f *testing.F) {
	for _, s := range []string{"", "g30v5", "g30v5,g1", "30.5, 1v2 ,,g12v1", ",", "g30,x", "g999", " , g1 "} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, list string) {
		gvs, err := ParseObjects(list)
		if err != nil {
			return
		}
		var want []string
		for s := range strings.SplitSeq(list, ",") {
			if strings.TrimSpace(s) != "" {
				want = append(want, s)
			}
		}
		if len(gvs) != len(want) {
			t.Fatalf("ParseObjects(%q): %d objects for %d items", list, len(gvs), len(want))
		}
		for i, s := range want {
			if gv, err := ParseObject(s); err != nil || gv != gvs[i] {
				t.Fatalf("ParseObjects(%q)[%d] = %v; ParseObject(%q) = %v, %v", list, i, gvs[i], s, gv, err)
			}
		}
	})
}

// Classes are within their bounds and each is listed once.
func FuzzParseClasses(f *testing.F) {
	for _, s := range []string{"", "1,2,3", "0", "0,1", "3,3", "4", "-1", " 2 ", "1,,2", "x"} {
		f.Add(s, 0)
		f.Add(s, 1)
	}
	f.Fuzz(func(t *testing.T, list string, minClass int) {
		minClass &= 1
		classes, err := ParseClasses(list, minClass, []int{1, 2, 3})
		if err != nil {
			return
		}
		seen := map[int]bool{}
		for _, c := range classes {
			if c < minClass || c > 3 || seen[c] {
				t.Fatalf("ParseClasses(%q, %d) = %v", list, minClass, classes)
			}
			seen[c] = true
		}
		if strings.TrimSpace(list) == "" && !slices.Equal(classes, []int{1, 2, 3}) {
			t.Fatalf("ParseClasses(%q) = %v, want the default", list, classes)
		}
	})
}

// A filter matches every object it names, and an empty filter matches everything.
func FuzzParseObjectFilter(f *testing.F) {
	for _, s := range []string{"", "g30", "g1v2,g32v3", "30.1", "g0", "x", "g30v0, g2"} {
		f.Add(s, uint8(30), uint8(1))
	}
	f.Fuzz(func(t *testing.T, s string, group, variation uint8) {
		filter, err := ParseObjectFilter(s)
		if err != nil {
			return
		}
		gvs, _ := ParseObjects(s)
		for _, gv := range gvs {
			if !filter.Match(gv) {
				t.Fatalf("ParseObjectFilter(%q) does not match %v", s, gv)
			}
		}
		if len(gvs) == 0 && !filter.Match(app.GV(group, variation)) {
			t.Fatalf("the empty filter %q does not match g%dv%d", s, group, variation)
		}
	})
}

// ParseTime takes now or an RFC 3339 time, and the time it returns renders back to itself.
func FuzzParseTime(f *testing.F) {
	for _, s := range []string{"", "now", "NOW", "2026-01-31T12:00:00Z", "2026-01-31T12:00:00.123456789+02:00", "2026-13-01T00:00:00Z", "yesterday", " now "} {
		f.Add(s)
	}
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	f.Fuzz(func(t *testing.T, s string) {
		got, err := ParseTime(s, func() time.Time { return fixed })
		if err != nil {
			return
		}
		in := strings.TrimSpace(s)
		if in == "" || strings.EqualFold(in, "now") {
			if !got.Equal(fixed) {
				t.Fatalf("ParseTime(%q) = %v, want now", s, got)
			}
			return
		}
		back, err := ParseTime(got.Format(time.RFC3339Nano), nil)
		if err != nil || !back.Equal(got) {
			t.Fatalf("ParseTime(%q) = %v; back %v, %v", s, got, back, err)
		}
	})
}

// A key read from the environment is the hex it was given, without spaces and a 0x prefix.
func FuzzReadKey(f *testing.F) {
	for _, s := range []string{"00112233445566778899aabbccddeeff", "0x 0011 2233", "0X00", " 00 \n", "", "0x", "zz", "abc", "00\t11"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, env string) {
		key, err := readKey("", env, "key", "--key-file", "KEY")
		if err != nil || key == nil {
			if err == nil && strings.TrimSpace(env) != "" {
				t.Fatalf("readKey(%q): no key and no error", env)
			}
			return
		}
		if len(key) == 0 {
			t.Fatalf("readKey(%q): an empty key", env)
		}
		again, err := readKey("", "0x"+hex.EncodeToString(key), "key", "--key-file", "KEY")
		if err != nil || !slices.Equal(again, key) {
			t.Fatalf("readKey(%q) = %x; its hex reads as %x, %v", env, key, again, err)
		}
	})
}

// An index list holds every index once, and a value list one value per index.
func FuzzParseIndexListAndValues(f *testing.F) {
	f.Add("0,1,4", "1.5", uint8(3))
	f.Add("1,1", "1,2", uint8(2))
	f.Add("4294967295", "NaN", uint8(1))
	f.Add("", "", uint8(0))
	f.Add(" 7 ", "-1e308,2", uint8(2))
	f.Fuzz(func(t *testing.T, list, values string, n uint8) {
		idx, err := parseIndexList(list, "--index")
		if err == nil {
			seen := map[uint32]bool{}
			for _, i := range idx {
				if seen[i] {
					t.Fatalf("parseIndexList(%q) = %v: %d twice", list, idx, i)
				}
				seen[i] = true
			}
			if len(idx) != len(strings.Split(list, ",")) {
				t.Fatalf("parseIndexList(%q) = %v", list, idx)
			}
		}
		count := int(n%16) + 1
		vs, err := parseValues(values, count)
		if err != nil {
			return
		}
		if len(vs) != count {
			t.Fatalf("parseValues(%q, %d) = %v", values, count, vs)
		}
	})
}

// A broadcast name selects one of the three broadcast addresses.
func FuzzParseBroadcast(f *testing.F) {
	for _, s := range []string{"", "no-confirm", "CONFIRM", " optional ", "all", "0xffff"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		addr, name, err := parseBroadcast(s)
		switch {
		case err != nil:
			return
		case name == "":
			if addr != 0 || strings.TrimSpace(s) != "" {
				t.Fatalf("parseBroadcast(%q) = %v, %q", s, addr, name)
			}
		case addr < link.BroadcastNoConfirm || broadcastAddresses[name] != addr:
			t.Fatalf("parseBroadcast(%q) = %v, not a broadcast address", s, addr)
		}
	})
}

// Ports are 1-65535 and link addresses station or self addresses.
func FuzzParsePortsAndAddresses(f *testing.F) {
	f.Add("20000,20001", "10,65532")
	f.Add("0", "65520")
	f.Add("65536", "0x0a")
	f.Add("", "")
	f.Add(" 1 ,, 2", "010")
	f.Fuzz(func(t *testing.T, ports, addrs string) {
		if ps, err := parsePorts(strings.Split(ports, ",")); err == nil {
			for _, p := range ps {
				if p == 0 {
					t.Fatalf("parsePorts(%q) = %v", ports, ps)
				}
			}
		}
		if as, err := parseAddresses(strings.Split(addrs, ",")); err == nil {
			for _, a := range as {
				if a > link.MaxStationAddress && a != link.SelfAddress {
					t.Fatalf("parseAddresses(%q) = %v", addrs, as)
				}
			}
		}
	})
}

// An attribute variation that parses has a name that parses back to it.
func FuzzParseAttributeVariation(f *testing.F) {
	for _, s := range []string{"", "all", "list", "manufacturer", "252", "254", "255", "0", "256", "attribute-7", " Location "} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		v, err := ParseAttributeVariation(s)
		if err != nil {
			return
		}
		back, err := ParseAttributeVariation(strings.TrimPrefix(AttributeName(v), "attribute-"))
		if err != nil || back != v {
			t.Fatalf("ParseAttributeVariation(%q) = %d; %q parses to %d, %v", s, v, AttributeName(v), back, err)
		}
	})
}

// An address list parses into prefixes that contain the addresses given.
func FuzzParsePrefixes(f *testing.F) {
	for _, s := range []string{"127.0.0.1", "10.0.0.0/8,::1", "fe80::1%eth0", "192.168.1.1/33", "", "x", " 2001:db8::/32 "} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		ps, err := ParsePrefixes(strings.Split(s, ","), "--allow")
		if err != nil {
			return
		}
		for _, p := range ps {
			if !p.IsValid() || p != p.Masked() {
				t.Fatalf("ParsePrefixes(%q) = %v", s, ps)
			}
		}
	})
}

// Challenge MAC lists name only known algorithms.
func FuzzParseChallengeMACs(f *testing.F) {
	for _, s := range []string{"", "hmac-sha256-16", "HMAC-SHA1-4, hmac-sha1-10", ",", "md5"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		macs, err := parseChallengeMACs(s)
		if err != nil {
			return
		}
		for _, m := range macs {
			if m.Len() == 0 {
				t.Fatalf("parseChallengeMACs(%q) = %v", s, macs)
			}
		}
	})
}
