// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"strings"
	"testing"
)

func TestRemovedOptions(t *testing.T) {
	none := func(string) string { return "" }
	if err := CheckRemovedEnv(none); err != nil {
		t.Errorf("no removed variable set: %v", err)
	}
	for _, v := range []string{"1", "true", "false", "0"} {
		err := CheckRemovedEnv(func(k string) string { return map[string]string{"DNP3CTL_TLS_INSECURE": v}[k] })
		if err == nil || !strings.Contains(err.Error(), "DNP3CTL_TLS_INSECURE was removed in v0.13.0") || !strings.Contains(err.Error(), "--tls-ca") {
			t.Errorf("DNP3CTL_TLS_INSECURE=%s: %v", v, err)
		}
	}

	for _, msg := range []string{"unknown flag: --tls-insecure", "unknown flag: --tls-insecure=true"} {
		if err := RemovedFlag(errors.New(msg)); err == nil || !strings.Contains(err.Error(), "--tls-insecure was removed in v0.13.0") || !strings.Contains(err.Error(), "--tls-ca") {
			t.Errorf("%s: %v", msg, err)
		}
	}
	for _, err := range []error{nil, errors.New("unknown flag: --nope"), errors.New("unknown shorthand flag: 'x' in -x"), errors.New("unknown flag: --tls-insecurex")} {
		if got := RemovedFlag(err); got != nil {
			t.Errorf("%v: %v", err, got)
		}
	}
}
