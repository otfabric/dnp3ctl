// Package client holds the commands of a master: `dnp3ctl client …`.
package client

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/otfabric/dnp3ctl/internal/cli"
	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/dnp"
	"github.com/otfabric/dnp3ctl/internal/format"
	"github.com/otfabric/dnp3ctl/internal/runner"
)

// NewCmd returns the `client` command with all its subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "client",
		Args:  cli.GroupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
		Short: "Act as a master: poll, read, monitor, operate, synchronize time and transfer files",
		Long: `Commands that connect to an outstation (RTU, IED, gateway) as a DNP3 master does:
open the channel, run one request and close the channel again. "client monitor" stays
connected.

Every command takes the outstation as --url (tcp://, tls://, udp://host[:port] or
serial:///dev/ttyUSB0) or as --ip with --port, and the link addresses as --master (this
master, default 1) and --outstation (default 10): they must match what the outstation is
configured with, or it does not answer. With --listen the master does not connect: it
waits for the outstation to connect to it (the dual end point), on TCP or, with the TLS
flags, as a TLS server.

Secure Authentication v5 is enabled by the update key in --sa-update-key-file (or
DNP3CTL_SA_UPDATE_KEY), or by a pre-shared session key in --sa-key-file (or
DNP3CTL_SA_KEY), as hex; keys are never accepted on the command line. --sa-aggressive
sends critical requests in aggressive mode.`,
	}
	cmd.AddCommand(
		newPollCmd(),
		newReadCmd(),
		newMonitorCmd(),
		newOperateCmd(),
		newTimeSyncCmd(),
		newRestartCmd(),
		newUnsolicitedCmd(),
		newAssignClassCmd(),
		newFreezeCmd(),
		newAttributesCmd(),
		newWriteAttributeCmd(),
		newWriteDeadbandCmd(),
		newManageCmd(),
		newClearRestartCmd(),
		newPingCmd(),
		newFileCmd(),
		newObjectsCmd(),
	)
	return cmd
}

// connDefaults are the defaults every client command starts from.
func connDefaults() config.ConnConfig {
	return config.ConnConfig{
		Port:          config.DefaultPort,
		AddressConfig: config.AddressConfig{Master: dnp.DefaultMaster, Outstation: dnp.DefaultOutstation},
		AuthConfig:    config.AuthConfig{SAUser: 1},
	}
}

// text is the default output format.
var text = string(format.FormatText)

// enum registers shell completion for a flag with a fixed set of values.
func enum(flag string, values ...string) func(*cobra.Command) {
	return func(cmd *cobra.Command) { _ = cli.RegisterEnumFlagCompletion(cmd, flag, values) }
}

// described registers shell completion for a flag whose values carry a description.
func described(flag string, values map[string]string) func(*cobra.Command) {
	return func(cmd *cobra.Command) { _ = cli.RegisterEnumFlagCompletionWithDescriptions(cmd, flag, values) }
}

// objectCompletion completes --object with the objects go-dnp3 knows.
func objectCompletion(cmd *cobra.Command) {
	if cmd.Flags().Lookup("object") == nil {
		return
	}
	var names []string
	for _, gv := range dnp.KnownObjects() {
		names = append(names, gv.String())
	}
	_ = cli.RegisterEnumFlagCompletion(cmd, "object", names)
}

// connCompletion completes the flags every client command has.
func connCompletion(cmd *cobra.Command) {
	_ = cli.RegisterEnumFlagCompletion(cmd, "parity", []string{"none", "even", "odd"})
	mac := map[string]string{}
	for k := range dnp.MACAlgorithms {
		mac[k] = "HMAC MAC of secure authentication"
	}
	_ = cli.RegisterEnumFlagCompletionWithDescriptions(cmd, "sa-mac", mac)
	for _, f := range []string{"tls-ca", "tls-cert", "tls-key"} {
		_ = cmd.MarkFlagFilename(f, "pem", "crt", "cer", "key")
	}
	_ = cmd.MarkFlagFilename("sa-key-file")
	objectCompletion(cmd)
}

// wire loads cfg from the environment, binds its flags to cmd and registers completions.
func wire(cmd *cobra.Command, cfg any, hooks ...func(*cobra.Command)) {
	runner.RegisterCommandCfg(cmd, cfg, append([]func(*cobra.Command){connCompletion}, hooks...)...)
}

// formatted runs a command with one result: validate, then collect, then write it.
func formatted(cmd *cobra.Command, outputFormat string, debug *bool, validate func() error, collect func(context.Context) (any, error), opts ...runner.FormatRunOption) error {
	*debug = cli.Debug(cmd)
	return runner.RunFormattedChecked(cmd, outputFormat, validate, collect, opts...)
}
