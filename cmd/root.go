// SPDX-License-Identifier: MIT

// Package cmd is the command tree of dnp3ctl.
package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/otfabric/dnp3ctl/cmd/client"
	"github.com/otfabric/dnp3ctl/cmd/discover"
	"github.com/otfabric/dnp3ctl/cmd/server"
	"github.com/otfabric/dnp3ctl/internal/buildinfo"
	clihelp "github.com/otfabric/dnp3ctl/internal/cli"
	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/errs"
	"github.com/otfabric/dnp3ctl/internal/runner"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// buildMeta holds link-time metadata set from main via SetBuildMeta.
var buildMeta struct {
	version, tag, commit, buildDate string
}

// SetBuildMeta is called from main before Execute; values come from ldflags -X main.*.
func SetBuildMeta(version, tag, commit, buildDate string) {
	buildMeta.version = version
	buildMeta.tag = tag
	buildMeta.commit = commit
	buildMeta.buildDate = buildDate
}

// NewRootCmd builds the command tree. Every call returns a fresh tree, with the flags of
// every command at the defaults the environment gives them.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "dnp3ctl",
		Short: "DNP3 (IEEE 1815) command-line master and outstation simulator",
		Long: `dnp3ctl speaks DNP3 (IEEE 1815), the SCADA protocol between masters and outstations
in electric, water and oil and gas utilities, in both roles.

As a master ("dnp3ctl client") it polls an outstation, reads objects, monitors unsolicited
responses, operates controls, synchronizes its clock, restarts it, freezes counters and
transfers files. As an outstation ("dnp3ctl server") it simulates an RTU from a model of
points, controls and files. "dnp3ctl discover" finds outstations on a network.

Results go to stdout as text, a table or JSON (--format); progress and diagnostics go to
stderr. Exit codes are stable: see docs/exitcodes.md.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().Bool("debug", false, "Trace every link frame and application fragment sent and received, with go-dnp3's own log entries, to stderr, and show the causes of an error")
	root.AddCommand(
		client.NewCmd(),
		server.NewCmd(),
		discover.NewCmd(),
		newCompletionCmd(),
		newVersionCmd(),
	)
	// Every other flag and argument completes no file names, except these files.
	clihelp.CompleteRest(root, map[string][]string{
		"config": {"yaml", "yml", "json"}, "input": nil, "output": nil, "serial": nil,
		"file-password-file": nil, "sa-key-file": nil, "sa-update-key-file": nil,
		"tls-ca": pemFiles, "tls-cert": pemFiles, "tls-key": pemFiles, "tls-client-ca": pemFiles,
	}, nil)
	return root
}

// pemFiles are the extensions of the certificate and key files the TLS flags take.
var pemFiles = []string{"pem", "crt", "cer", "key"}

func newVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version of dnp3ctl",
		Long: `Print the version of dnp3ctl on one line: the version, the git tag and commit it
was built from, the build date, the Go version and the platform. --json (or
DNP3CTL_FORMAT=json) prints the same as one JSON object.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := buildinfo.Resolve("dnp3ctl", buildMeta.version, buildMeta.tag, buildMeta.commit, buildMeta.buildDate)
			line := info.String()
			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON || strings.EqualFold(strings.TrimSpace(config.Getenv("DNP3CTL_FORMAT")), "json") {
				line = info.JSON()
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), line)
			return err
		},
	}
	cmd.Flags().Bool("json", false, "Print the version as one JSON object")
	return cmd
}

// Execute runs the root command and returns the process exit code (main should call os.Exit).
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
}

// Run executes dnp3ctl with args, writing to stdout and stderr, and returns the exit
// code. It is Execute without the process: what the tests drive.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	root, err := buildRoot()
	if err != nil {
		return runner.RenderFatal(nil, err, stdout, stderr, false)
	}
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	inv := &runner.Invocation{}
	ctx = runner.WithInvocation(ctx, inv)
	cmd, err := root.ExecuteContextC(ctx)
	trace, _ := root.PersistentFlags().GetBool("debug")
	if err != nil {
		if !inv.HasOutputFormat {
			// Cobra failed before the command recorded its format: honour --format (or
			// DNP3CTL_FORMAT) all the same.
			if f, ok := runner.RequestedFormat(cmd, args); ok {
				inv.SetOutputFormat(f)
			}
		}
		return runner.RenderFatal(inv, usageError(unknownSubcommand(cmd, args, err)), stdout, stderr, trace)
	}
	if inv.SuccessExitSet {
		return inv.SuccessExitCode
	}
	return errs.ExitOK
}

// buildRoot is NewRootCmd with a malformed environment variable reported as invalid input
// instead of a panic: the configuration of every command is read when the tree is built.
func buildRoot() (root *cobra.Command, err error) {
	defer func() {
		if r := recover(); r != nil {
			msg := strings.TrimPrefix(fmt.Sprint(r), "dnp3ctl: ")
			err = errs.InvalidInput(errs.CodeInvalidInput, msg, nil)
		}
	}()
	return NewRootCmd(), nil
}

// unknownSubcommand returns the error for an unknown subcommand of a group command
// ("dnp3ctl client nope --format json") in place of the error about the first flag:
// Cobra stops at the group, which has none of its subcommands' flags, and reports that
// flag. Any other error is returned as it is.
func unknownSubcommand(cmd *cobra.Command, args []string, err error) error {
	if cmd == nil || !cmd.HasSubCommands() || !strings.HasPrefix(err.Error(), "unknown flag") {
		return err
	}
	depth := len(strings.Fields(cmd.CommandPath())) - 1 // the words after "dnp3ctl"
	words := 0
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "-") {
			if takesValue(cmd, a) {
				i++ // its value is the next argument
			}
			continue
		}
		words++
		if words <= depth {
			continue
		}
		for _, sub := range cmd.Commands() {
			if sub.Name() == a || sub.HasAlias(a) {
				return err
			}
		}
		return clihelp.UnknownCommand(cmd, a)
	}
	return err
}

// takesValue reports whether the flag arg ("--format", "-o", not "--format=json") names a
// flag of cmd or of one of its subcommands that takes a value, so that the next argument
// is that value rather than a command.
func takesValue(cmd *cobra.Command, arg string) bool {
	if strings.Contains(arg, "=") {
		return false
	}
	name := strings.TrimLeft(arg, "-")
	if !strings.HasPrefix(arg, "--") && len(name) != 1 {
		return false // a group of short flags, or a negative number
	}
	lookup := func(c *cobra.Command) *pflag.Flag {
		if strings.HasPrefix(arg, "--") {
			return c.Flags().Lookup(name)
		}
		return c.Flags().ShorthandLookup(name)
	}
	for _, c := range append([]*cobra.Command{cmd}, cmd.Commands()...) {
		if f := lookup(c); f != nil {
			return f.NoOptDefVal == ""
		}
	}
	return false
}

// usageError classifies what Cobra itself rejects (an unknown command or flag, a missing
// required flag, a value a flag cannot hold) as a usage error: exit code 2.
func usageError(err error) error {
	if _, ok := errs.As(err); ok {
		return err
	}
	return errs.New(errs.KindUsage, errs.CodeInvalidInput, err.Error(), err)
}
