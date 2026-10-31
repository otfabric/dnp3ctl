// SPDX-License-Identifier: MIT

package runner

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/otfabric/dnp3ctl/internal/cli"
	"github.com/otfabric/dnp3ctl/internal/format"
)

// stdoutFormatAnnotation marks the --format flag that selects the format of stdout (text,
// json, table), as opposed to a --format of something else (the model of
// "server generate-config").
const stdoutFormatAnnotation = "dnp3ctl_stdout_format"

// RegisterStdoutFormatCompletion registers shell completion for --format (text, json, table) on client stdout commands,
// and marks the flag as the format of stdout.
func RegisterStdoutFormatCompletion(cmd *cobra.Command) {
	_ = cmd.Flags().SetAnnotation("format", stdoutFormatAnnotation, []string{"true"})
	_ = cli.RegisterEnumFlagCompletionWithDescriptions(cmd, "format", format.ValueDescriptions())
}

// isStdoutFormat reports whether f is the --format of stdout.
func isStdoutFormat(f *pflag.Flag) bool {
	_, ok := f.Annotations[stdoutFormatAnnotation]
	return ok
}

// RequestedFormat returns the format of stdout an invocation asks for when Cobra rejected
// it before the command could record one (an unknown command or flag, a missing required
// flag, a value a flag cannot hold): the last --format in args, or else the value of the
// --format of cmd (the command Cobra failed on), which is DNP3CTL_FORMAT when that
// is set. It reports false when nothing asks for a valid format, and when the --format of
// cmd is not the format of stdout.
func RequestedFormat(cmd *cobra.Command, args []string) (format.OutputFormat, bool) {
	raw, found := formatArg(args)
	if cmd != nil {
		if f := cmd.Flags().Lookup("format"); f != nil {
			if !isStdoutFormat(f) {
				return "", false
			}
			if !found {
				raw, found = f.Value.String(), true
			}
		}
	}
	if !found {
		return "", false
	}
	f, err := format.Parse(raw)
	if err != nil {
		return "", false
	}
	return f, true
}

// formatArg returns the value of the last --format in args (--format json or
// --format=json), up to a "--" that ends the flags.
func formatArg(args []string) (string, bool) {
	var value string
	var found bool
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return value, found
		case a == "--format" && i+1 < len(args):
			value, found = args[i+1], true
			i++
		case strings.HasPrefix(a, "--format="):
			value, found = strings.TrimPrefix(a, "--format="), true
		}
	}
	return value, found
}
