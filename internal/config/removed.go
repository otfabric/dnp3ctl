// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"fmt"
	"strings"
)

// RemovedOption is an option dnp3ctl no longer has. Its flag and its environment variable
// are rejected with an error that names what replaces them, so that a script or an
// environment that still sets it fails instead of silently getting other behaviour.
type RemovedOption struct {
	Flag    string // without the leading --
	Env     string
	Version string // the release that removed it
	Instead string // what to do instead
}

// removed lists the options of [RemovedOption].
var removed = []RemovedOption{
	{
		Flag: "tls-insecure", Env: "DNP3CTL_TLS_INSECURE", Version: "v0.13.0",
		Instead: "TLS always verifies the outstation's certificate: pass --tls-ca with the CA certificate (or the self-signed certificate) of the outstation",
	},
}

// CheckRemovedEnv returns an error for every removed environment variable that getenv
// reports as set, nil when none is.
func CheckRemovedEnv(getenv func(string) string) error {
	var out []error
	for _, r := range removed {
		if r.Env != "" && getenv(r.Env) != "" {
			out = append(out, fmt.Errorf("%s was removed in %s: unset it; %s", r.Env, r.Version, r.Instead))
		}
	}
	return errors.Join(out...)
}

// RemovedFlag returns the error for the removed flag that Cobra reports as unknown in err
// ("unknown flag: --tls-insecure"), and nil when err is about another flag or no flag.
func RemovedFlag(err error) error {
	if err == nil {
		return nil
	}
	name, ok := strings.CutPrefix(err.Error(), "unknown flag: --")
	if !ok {
		return nil
	}
	name, _, _ = strings.Cut(name, "=")
	for _, r := range removed {
		if r.Flag != "" && name == r.Flag {
			return fmt.Errorf("--%s was removed in %s; %s", r.Flag, r.Version, r.Instead)
		}
	}
	return nil
}
