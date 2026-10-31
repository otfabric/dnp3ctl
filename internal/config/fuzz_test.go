// SPDX-License-Identifier: MIT

package config

import "testing"

// A target comes from --url, a flag or a script. What ParseURL accepts must be
// usable: a device for a serial port, a host and a port otherwise.
func FuzzParseURL(f *testing.F) {
	for _, s := range []string{
		"tcp://10.0.0.1:20000", "tls://host", "udp://[::1]:20000", "serial:///dev/ttyUSB0", "serial://COM3",
		"TCP://host:1", "tcp://", "tcp://host:0", "tcp://host:65536", "tcp://u@host", "tcp://host/path",
		"serial:///", "serial:///dev/tty?baud=1", "ftp://host", "", " tcp://host ", "%zz",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		tgt, err := ParseURL(raw)
		if err != nil {
			if tgt != (Target{}) {
				t.Fatalf("ParseURL(%q): %+v with error %v", raw, tgt, err)
			}
			return
		}
		switch tgt.Scheme {
		case SchemeSerial:
			if tgt.Device == "" || tgt.Addr() != tgt.Device {
				t.Fatalf("ParseURL(%q): %+v", raw, tgt)
			}
		case SchemeTCP, SchemeTLS, SchemeUDP:
			if tgt.Host == "" || tgt.Port == 0 {
				t.Fatalf("ParseURL(%q): %+v", raw, tgt)
			}
		default:
			t.Fatalf("ParseURL(%q): scheme %q", raw, tgt.Scheme)
		}
	})
}
