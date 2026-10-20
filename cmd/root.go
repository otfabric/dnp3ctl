// Package cmd is the command tree of dnp3ctl.
package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/otfabric/dnp3ctl/cmd/client"
	"github.com/otfabric/dnp3ctl/cmd/discover"
	"github.com/otfabric/dnp3ctl/cmd/server"
	"github.com/otfabric/dnp3ctl/internal/errs"
	"github.com/otfabric/dnp3ctl/internal/runner"
	"github.com/spf13/cobra"
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
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of dnp3ctl",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			out := cmd.OutOrStdout()
			info, _ := debug.ReadBuildInfo()
			ver, commit := versionOf(buildMeta.version, buildMeta.commit, info)
			if ver == "" {
				ver = "(unknown)"
			}
			_, _ = fmt.Fprintf(out, "dnp3ctl version: %s\n", ver)
			if s := strings.TrimSpace(buildMeta.tag); s != "" {
				_, _ = fmt.Fprintf(out, "tag:         %s\n", s)
			}
			if commit != "" {
				_, _ = fmt.Fprintf(out, "commit:      %s\n", commit)
			}
			if s := strings.TrimSpace(buildMeta.buildDate); s != "" {
				_, _ = fmt.Fprintf(out, "build date:  %s\n", s)
			}
			_, _ = fmt.Fprintf(out, "go version:  %s\n", runtime.Version())
			_, _ = fmt.Fprintf(out, "platform:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
		},
	}
}

// devVersion is the version of a binary built without the ldflags of the Makefile.
const devVersion = "dev"

// versionOf returns the version and commit to report. The ldflags of the Makefile and of
// the release set them; a binary built without them (go install, go build) reports what the
// go toolchain records: the version of its module (v0.6.3 for go install …@v0.6.3, or one
// derived from the git tags for go build in a clone, such as v0.6.3+dirty) and the git
// revision, shortened as git rev-parse --short gives it.
func versionOf(version, commit string, info *debug.BuildInfo) (string, string) {
	version, commit = strings.TrimSpace(version), strings.TrimSpace(commit)
	if info == nil {
		return version, commit
	}
	if version == devVersion || version == "" {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			version = v
		}
	}
	if commit == "" {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				commit = s.Value
				if len(commit) > 7 {
					commit = commit[:7]
				}
			}
		}
	}
	return version, commit
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
			// DNP3CTL_OUTPUT_FORMAT) all the same.
			if f, ok := runner.RequestedFormat(cmd, args); ok {
				inv.SetOutputFormat(f)
			}
		}
		return runner.RenderFatal(inv, usageError(err), stdout, stderr, trace)
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

// usageError classifies what Cobra itself rejects (an unknown command or flag, a missing
// required flag, a value a flag cannot hold) as a usage error: exit code 2.
func usageError(err error) error {
	if _, ok := errs.As(err); ok {
		return err
	}
	return errs.New(errs.KindUsage, errs.CodeInvalidInput, err.Error(), err)
}
