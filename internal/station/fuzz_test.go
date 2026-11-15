// SPDX-License-Identifier: MIT

package station

import (
	"strings"
	"testing"

	"github.com/otfabric/go-dnp3/secauth"
)

// FuzzParse feeds model files: the operator's, or one handed to them. A model
// that parses and validates must survive its own encoding.
func FuzzParse(f *testing.F) {
	data, err := Demo().Encode(false)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	if data, err = Demo().Encode(true); err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	for _, s := range []string{"", "{}", "null", "- a\n- b\n", "outstation: {select_timeout: 5}\n", "outstation: {link_address: 3}\n", "points: [{}]\n", "\xff\xfe", "a: &a [*a]\n",
		"points: [{type: binary-input, index: 0, value: true}]\nfiles: [{name: /a, size: 4294967295}, {name: /b, size: 4294967296}, {name: /c, content: x}]\n",
		"points: [{type: binary-input, index: 0, value: true}]\nfiles: [{name: /a, size: 9223372036854775807}]\nuploads: {accept: true, max_files: 1, max_bytes: 1}\n"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := Parse(data)
		if err != nil {
			return
		}
		// Validate opens the files a model names by path: no input gets to pick them here.
		// Files given by content or size stay: Validate checks them without making them.
		files := m.Files[:0]
		for _, file := range m.Files {
			if file.Path == "" {
				files = append(files, file)
			}
		}
		m.Files = files
		if m.Validate() != nil {
			return
		}
		out, err := m.Encode(false)
		if err != nil {
			t.Fatalf("a valid model does not encode: %v\n%s", err, data)
		}
		back, err := Parse(out)
		if err != nil {
			t.Fatalf("a valid model does not parse back: %v\n%s", err, out)
		}
		if err := back.Validate(); err != nil {
			t.Fatalf("a valid model does not validate back: %v\n%s", err, out)
		}
	})
}

// --allow takes addresses and prefixes; every prefix it returns is valid and masked.
func FuzzParseAllow(f *testing.F) {
	for _, s := range []string{"127.0.0.1", "10.0.0.0/8", "::1,192.168.0.0/16", "fe80::1%lo0", "1.2.3.4/40", "", "host"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		ps, err := parseAllow(strings.Split(s, ","))
		if err != nil {
			return
		}
		for _, p := range ps {
			if !p.IsValid() || p != p.Masked() {
				t.Fatalf("parseAllow(%q) = %v", s, ps)
			}
		}
	})
}

// --sa-threshold takes index=threshold pairs of the statistics of Table 7-6.
func FuzzParseThresholds(f *testing.F) {
	for _, s := range []string{"3=10", "0=1,17=65535", "18=1", "3=0", "x=1", "3", "", " 2 = 5 "} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		m, err := parseThresholds(strings.Split(s, ","))
		if err != nil {
			return
		}
		for k, v := range m {
			if k >= secauth.NumStatistics || v == 0 {
				t.Fatalf("parseThresholds(%q) = %v", s, m)
			}
		}
	})
}
