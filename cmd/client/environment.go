// SPDX-License-Identifier: MIT

package client

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/errs"
)

// controlEnv are the environment variables of the parameters that decide what a control or
// a freeze does and where it goes.
var controlEnv = map[string]string{
	"broadcast": "DNP3CTL_BROADCAST",
	"op":        "DNP3CTL_OP",
	"mode":      "DNP3CTL_OPERATE_MODE",
	"value":     "DNP3CTL_VALUE",
}

// controlFromEnvironment applies the rules for the destination of a control or a freeze
// and says on stderr which of its parameters (flags) came from the environment:
//
//   - --broadcast and --outstation both on the command line are refused: a broadcast goes
//     to every outstation, --outstation names one;
//   - --outstation on the command line wins over DNP3CTL_BROADCAST: the request goes to
//     that outstation, and stderr says that DNP3CTL_BROADCAST was not used;
//   - every other parameter taken from the environment is named on stderr, with its value.
//
// broadcast is the --broadcast of the configuration; it is cleared when it is not used.
func controlFromEnvironment(cmd *cobra.Command, broadcast *string, flags ...string) error {
	changed := cmd.Flags().Changed
	stderr := cmd.ErrOrStderr()
	explicit := changed("outstation")
	if explicit && changed("broadcast") {
		return errs.InvalidInput(errs.CodeInvalidFlagCombination,
			"--broadcast and --outstation cannot be combined: a broadcast goes to every outstation on the channel; give --outstation for one outstation, or --broadcast without it for all", nil)
	}
	for _, flag := range append([]string{"broadcast"}, flags...) {
		env := controlEnv[flag]
		value := config.Getenv(env)
		if changed(flag) || value == "" {
			continue
		}
		if flag == "broadcast" && explicit {
			_, _ = fmt.Fprintf(stderr, "notice: %s=%s is not used: --outstation %s is on the command line, and the request goes to that outstation only\n",
				env, value, cmd.Flags().Lookup("outstation").Value)
			*broadcast = ""
			continue
		}
		_, _ = fmt.Fprintf(stderr, "notice: --%s %s is taken from %s\n", flag, value, env)
	}
	return nil
}
