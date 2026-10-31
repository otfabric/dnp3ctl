// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/otfabric/dnp3ctl/internal/cli"
	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/dnp"
	"github.com/otfabric/dnp3ctl/internal/errs"
	"github.com/otfabric/dnp3ctl/internal/format"
	"github.com/otfabric/dnp3ctl/internal/runner"
	"github.com/otfabric/dnp3ctl/internal/types"
)

func newMonitorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Stay connected and print every value the outstation reports",
		Long: `Connect, enable unsolicited responses and print each value as it arrives: the events
the outstation reports by itself, and the answers to the polls of this command. This is
what a master sees between its requests.

After connecting (and after every reconnect) the master enables unsolicited responses for
the classes of --unsolicited (default 1,2,3; "none" to leave them as they are) and, with
--integrity, runs an integrity poll first so that the stream starts with all current
values; it is repeated when the outstation reports a restart or a lost event. With
--poll-interval the events of classes 1-3 are also polled at that interval, for
outstations that do not report unsolicited. When the outstation reports a restart, the
master clears its restart indication and enables unsolicited responses again, since an
outstation that restarted has forgotten them.

Monitoring runs until interrupted (Ctrl-C), or until --duration seconds have passed or
--count values were printed. A lost connection is established again (turn that off with
--no-reconnect).

Each value goes to stdout as it arrives: one line with --format text, one JSON object per
line with --format json, or one table row with --format table. Connection state changes
and a summary go to stderr.

Events an outstation has reported to this master are gone from its buffer: monitoring
with the link address of the regular master takes its events away. An outstation also
takes a second connection from the same master address for that master reconnecting, and
drops the first: run other commands next to a monitor with another --master.`,
		Example: `
  # Watch everything outstation 10 reports, starting with an integrity poll
  dnp3ctl client monitor --ip 192.168.1.20 --integrity

  # Analog input events only, as JSON Lines
  dnp3ctl client monitor --ip 192.168.1.20 --object g32 --format json

  # An outstation without unsolicited responses: poll its events every 5 seconds
  dnp3ctl client monitor --ip 192.168.1.20 --unsolicited none --poll-interval 5

  # Capture one minute into a file
  dnp3ctl client monitor --ip 192.168.1.20 --duration 60 --format json > events.jsonl

  # An outstation that connects to the master over TLS; wait for it again after a loss
  dnp3ctl client monitor --listen 10.0.0.1:19999 --tls-cert scada.pem --tls-key scada.key --tls-ca ca.pem --timeout 60000
`,
	}
	cfg := config.MonitorConfig{ConnConfig: connDefaults(), Unsolicited: "1,2,3", OutputFormat: text}
	wire(cmd, &cfg, enum("unsolicited", "1,2,3", "none"))
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		cfg.Debug = cli.Debug(cmd)
		return runner.RunStream(cmd, cfg.OutputFormat,
			func() error { return dnp.CheckMonitor(&cfg) },
			func(ctx context.Context, out *format.StreamWriter) (int, error) {
				summary, err := dnp.Monitor(ctx, &cfg, cmd.ErrOrStderr(), func(m *types.Measurement) error { return out.Write(m) })
				if summary.Target != "" {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), summary)
				}
				if err != nil {
					return 0, err
				}
				return errs.ExitOK, nil
			})
	}
	return cmd
}

func newPingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ping",
		Short: "Measure the delay to an outstation (DELAY_MEASURE)",
		Long: `Connect to an outstation and send DELAY_MEASURE at an interval, printing the round
trip time and the processing delay the outstation reports. DELAY_MEASURE changes nothing
on the outstation: it is the request of the non-LAN time synchronization.

Measuring runs until --count measurements were made, or until interrupted (Ctrl-C) when
--count is 0. A measurement that fails is reported and measuring goes on while the
connection is up. Exit code 7 (partial success) means at least one failed.`,
		Example: `
  # Four measurements, one per second
  dnp3ctl client ping --ip 192.168.1.20

  # Every 500 ms until Ctrl-C, over UDP
  dnp3ctl client ping --url udp://192.168.1.20 --count 0 --interval 500
`,
	}
	cfg := config.PingConfig{ConnConfig: connDefaults(), Count: 4, Interval: 1000, OutputFormat: text}
	wire(cmd, &cfg)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		cfg.Debug = cli.Debug(cmd)
		return runner.RunStream(cmd, cfg.OutputFormat,
			func() error { return dnp.CheckPing(&cfg) },
			func(ctx context.Context, out *format.StreamWriter) (int, error) {
				emit := func(s *types.PingSample) error {
					if err := out.Write(s); err != nil {
						return errs.Output(errs.CodeOutputWriteFailed, err)
					}
					return nil
				}
				summary, err := dnp.Ping(ctx, &cfg, cmd.ErrOrStderr(), emit)
				if summary.Sent > 0 {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), summary)
				}
				if err != nil {
					return 0, err
				}
				if summary.Failed > 0 {
					return errs.ExitPartial, nil
				}
				return errs.ExitOK, nil
			})
	}
	return cmd
}
