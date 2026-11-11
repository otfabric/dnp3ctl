// SPDX-License-Identifier: MIT

package config

import (
	"strings"
	"testing"
)

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

// Environment variables are text anyone can set. LoadFromEnv sets a field only from a
// value of its type, reports every other value, and never panics.
func FuzzLoadFromEnv(f *testing.F) {
	for _, s := range []string{"", "1", "0", "255", "256", "-1", "65535", "4294967296", "true", "maybe", "a,b , c", "10000", " 5 "} {
		f.Add("DNP3CTL_TIMEOUT", s)
		f.Add("DNP3CTL_RETRIES", s)
		f.Add("DNP3CTL_LINK_CONFIRMED", s)
		f.Add("DNP3CTL_ALLOW", s)
	}
	f.Add("DNP3CTL_OUTPUT_FORMAT", "json")
	f.Fuzz(func(t *testing.T, name, value string) {
		env := map[string]string{name: value}
		getenv := func(k string) string { return env[k] }
		var conn MonitorConfig
		err := loadFromEnvWith(&conn, getenv)
		var srv ServerConfig
		serr := loadFromEnvWith(&srv, getenv)
		if value == "" && (err != nil || serr != nil) {
			t.Fatalf("an empty %s failed: %v, %v", name, err, serr)
		}
		if name == "DNP3CTL_TIMEOUT" && err == nil && value != "" && conn.Timeout == 0 && strings.Trim(value, "0") != "" {
			t.Fatalf("DNP3CTL_TIMEOUT=%q accepted as 0", value)
		}
		if name == "DNP3CTL_ALLOW" && serr == nil && value != "" && len(srv.Allow) != len(strings.Split(value, ",")) {
			t.Fatalf("DNP3CTL_ALLOW=%q: %v", value, srv.Allow)
		}
	})
}
