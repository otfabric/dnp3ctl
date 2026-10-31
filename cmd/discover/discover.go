// SPDX-License-Identifier: MIT

// Package discover holds `dnp3ctl discover`.
package discover

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/otfabric/dnp3ctl/internal/cli"
	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/dnp"
	"github.com/otfabric/dnp3ctl/internal/format"
	"github.com/otfabric/dnp3ctl/internal/runner"
)

// NewCmd returns the `discover` command.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Find DNP3 outstations on subnets",
		Long: `Probe every address of --subnets on --ports and list the ones that answer as a DNP3
outstation, with the link address they answer from.

The probe opens a TCP connection and sends one link layer REQUEST_LINK_STATUS from
--master to each address of --outstations, and waits for LINK_STATUS. That is the least a
master can say: no application layer request is made, nothing is read and no event is
taken. An outstation answers only frames sent to its own link address, or to the self
address 65532 when it supports that: add the addresses your outstations use to
--outstations (default 10 and 65532). A port that accepts the connection but does not
answer is listed as "open".

--subnets takes CIDR prefixes (192.168.1.0/24), ranges on the last octet (192.168.1.10-20)
and single addresses or host names. More than 4096 hosts need --force-large-scan.

An outstation accepts a limited number of connections, often one per master it knows: a
probe can be turned away by an outstation that is in service, and it may show up in its
logs. Scan only networks you are responsible for.`,
		Example: `
  # Sweep a subnet on the standard port
  dnp3ctl discover --subnets 192.168.1.0/24

  # Outstations at link addresses 1-4 and 10, 64 probes at a time, as JSON
  dnp3ctl discover --subnets 10.0.1.0/24 --outstations 1,2,3,4,10,65532 --parallel 64 --format json

  # A handful of addresses with a short timeout
  dnp3ctl discover --subnets 192.168.1.10-20 --timeout 300
`,
	}
	cfg := config.DiscoverConfig{Ports: []string{"20000"}, Outstations: []string{"10", "65532"}, Master: dnp.DefaultMaster, Parallel: 32, OutputFormat: string(format.FormatText)}
	runner.RegisterCommandCfg(cmd, &cfg)
	cli.MustMarkFlagRequired(cmd, "subnets")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		cfg.Debug = cli.Debug(cmd)
		return runner.RunFormattedChecked(cmd, cfg.OutputFormat,
			func() error { return dnp.CheckDiscover(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.Discover(ctx, &cfg, cmd.ErrOrStderr()) })
	}
	return cmd
}
