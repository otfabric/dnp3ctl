// SPDX-License-Identifier: MIT

package runner

import (
	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/spf13/cobra"
)

// commandLinePreferrer is a configuration that drops what the environment gives a
// connection style when the command line chooses another one.
type commandLinePreferrer interface {
	PreferCommandLine(changed func(flag string) bool)
}

// RegisterCommandCfg runs [config.MustLoadFromEnv], [config.RegisterFlags] and
// [RegisterStdoutFormatCompletion] (when the command has --format), then the optional hooks
// (extra completions, required flags, etc.). When cfg has connection styles, the flags of a
// style given on the command line win over the environment variables of the others.
func RegisterCommandCfg(cmd *cobra.Command, cfg any, hooks ...func(*cobra.Command)) {
	config.MustLoadFromEnv(cfg)
	config.RegisterFlags(cmd, cfg)
	if cmd.Flags().Lookup("format") != nil {
		RegisterStdoutFormatCompletion(cmd)
	}
	if p, ok := cfg.(commandLinePreferrer); ok {
		preRun := cmd.PreRunE
		cmd.PreRunE = func(c *cobra.Command, args []string) error {
			p.PreferCommandLine(func(flag string) bool { return c.Flags().Changed(flag) })
			if preRun != nil {
				return preRun(c, args)
			}
			return nil
		}
	}
	for _, h := range hooks {
		if h != nil {
			h(cmd)
		}
	}
}
