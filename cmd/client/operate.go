// SPDX-License-Identifier: MIT

package client

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/otfabric/dnp3ctl/internal/cli"
	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/dnp"
	"github.com/otfabric/dnp3ctl/internal/runner"
)

func newOperateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "operate",
		Aliases: []string{"control"},
		Short:   "Operate an output: CROB (latch, pulse, trip, close) or analog set-point",
		Long: `Send a control to the output --index and report the status the outstation echoes.

  --type crob    control relay output block (g12v1), with --op:
                   latch-on, latch-off   latch the output
                   pulse-on, pulse-off   pulse it for --on-time / --off-time ms, --count times
                   close, trip           pulse on with the trip-close code close or trip
                                         (a breaker with paired close and trip outputs)
                   nul                   no operation
  --type analog  analog output block (g41), with --value; --variation picks the encoding:
                   1 (32-bit), 2 (16-bit), 3 (single-precision), 4 (double-precision).
                   By default 1 for whole numbers that fit, 3 otherwise.

--mode chooses the procedure:

  sbo            select before operate: SELECT, and when the outstation accepts it,
                 OPERATE with the same control (default)
  direct         DIRECT_OPERATE
  direct-no-ack  DIRECT_OPERATE_NR: no response, so no status is reported

THIS CHANGES THE PROCESS. On real equipment a control opens a breaker, starts a pump or
changes a set-point. Know what an index controls before you send to it. Everything is
validated before anything is sent, and a control is sent exactly once: it is never
repeated, also not with --retries and also not when its response does not arrive.

--index takes several indexes, comma-separated, to send several controls in one request:
the same --op to every output, or with --type analog one --value for all or one per index.
The outstation executes them together and echoes a status for each. When a direct operate
is refused for some of them, the others were executed: the result lists every status and
the exit code is 7. With select-before-operate a refused SELECT ends the operation and
nothing is executed.

--broadcast sends the control to every outstation on the channel at once, at one of the
broadcast addresses, with --mode direct or direct-no-ack. Nobody answers it, so no status
is reported: each outstation sets IIN1.0 BROADCAST in its next response. A broadcast cannot
be combined with secure authentication: an outstation running SAv5 executes a broadcast
control only with an aggressive mode MAC over an earlier challenge of the same connection,
and every dnp3ctl command opens a new connection.

A control the outstation refuses (a status other than SUCCESS, such as NO_SELECT, LOCAL or
NOT_SUPPORTED) ends with exit code 6; a missing response with 5, after which the control
may or may not have been executed.`,
		Example: `
  # Close a breaker with select-before-operate
  dnp3ctl client operate --ip 192.168.1.20 --type crob --index 0 --op latch-on

  # Trip it with a direct operate and a 500 ms pulse
  dnp3ctl client operate --ip 192.168.1.20 --type crob --index 0 --op trip --on-time 500 --mode direct

  # Set-point of 11.2 as single-precision float, result as JSON
  dnp3ctl client operate --ip 192.168.1.20 --type analog --index 0 --value 11.2 --format json

  # A 16-bit set-point, without waiting for a response
  dnp3ctl client operate --ip 192.168.1.20 --type analog --index 1 --value 1200 --variation 2 --mode direct-no-ack

  # Secure Authentication with session key change; the OPERATE in aggressive mode
  dnp3ctl client operate --ip 192.168.1.20 --type crob --index 0 --op latch-on --sa-update-key-file rtu.key --sa-aggressive

  # Latch three outputs on in one request
  dnp3ctl client operate --ip 192.168.1.20 --type crob --index 0,1,4 --op latch-on --mode direct

  # Two set-points in one request
  dnp3ctl client operate --ip 192.168.1.20 --type analog --index 0,1 --value 11.2,230

  # Open the breaker at index 3 of every outstation on the line, without answers
  dnp3ctl client operate --url serial:///dev/ttyUSB0 --type crob --index 3 --op trip --mode direct-no-ack --broadcast no-confirm
`,
	}
	cfg := config.OperateConfig{ConnConfig: connDefaults(), Mode: dnp.ModeSBO, Count: 1, OutputFormat: text}
	wire(cmd, &cfg,
		described("type", dnp.ControlTypes()),
		described("mode", dnp.OperateModes()),
		enum("op", dnp.CROBOps()...),
		enum("variation", "1", "2", "3", "4"),
		described("broadcast", dnp.BroadcastModes()))
	for _, f := range []string{"type", "index"} {
		cli.MustMarkFlagRequired(cmd, f)
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return formatted(cmd, cfg.OutputFormat, &cfg.Debug,
			func() error { return dnp.CheckOperate(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.Operate(ctx, &cfg, cmd.ErrOrStderr()) },
			runner.WithSuccessExit(partialExit))
	}
	return cmd
}
