package client

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/otfabric/dnp3ctl/internal/cli"
	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/dnp"
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
`,
	}
	cfg := config.OperateConfig{ConnConfig: connDefaults(), Mode: dnp.ModeSBO, Count: 1, OutputFormat: text}
	wire(cmd, &cfg,
		described("type", dnp.ControlTypes()),
		described("mode", dnp.OperateModes()),
		enum("op", dnp.CROBOps()...),
		enum("variation", "1", "2", "3", "4"))
	for _, f := range []string{"type", "index"} {
		cli.MustMarkFlagRequired(cmd, f)
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return formatted(cmd, cfg.OutputFormat, &cfg.Debug,
			func() error { return dnp.CheckOperate(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.Operate(ctx, &cfg, cmd.ErrOrStderr()) })
	}
	return cmd
}
