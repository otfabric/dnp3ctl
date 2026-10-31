// SPDX-License-Identifier: MIT

package config

import "testing"

func TestGetenvFormatSpellings(t *testing.T) {
	for _, c := range []struct{ canonical, legacy, want string }{
		{"", "", ""},
		{"json", "", "json"},
		{"", "json", "json"},
		{"table", "json", "table"},
	} {
		t.Setenv("DNP3CTL_FORMAT", c.canonical)
		t.Setenv("DNP3CTL_OUTPUT_FORMAT", c.legacy)
		if got := Getenv("DNP3CTL_FORMAT"); got != c.want {
			t.Errorf("canonical %q, legacy %q: got %q, want %q", c.canonical, c.legacy, got, c.want)
		}
	}
}
