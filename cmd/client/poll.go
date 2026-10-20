package client

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/otfabric/dnp3ctl/internal/cli"
	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/dnp"
	"github.com/otfabric/dnp3ctl/internal/errs"
	"github.com/otfabric/dnp3ctl/internal/runner"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// partialExit is the exit code of a poll or read: 7 when the outstation reported an error
// in its IIN while it did answer with values.
func partialExit(v any) int {
	if r, ok := v.(*types.PollResult); ok && r.Error != nil {
		return errs.ExitPartial
	}
	return errs.ExitOK
}

func newPollCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "poll",
		Aliases: []string{"integrity"},
		Short:   "Poll an outstation: integrity poll, or the events of classes",
		Long: `Read the data of an outstation by class and list every value with its flags and,
for events, its time.

  --mode integrity  READ of the events of classes 1, 2 and 3, then of the static data of
                    class 0: the whole process image, as a master reads it after
                    connecting. The default.
  --mode event      READ of the events of the classes of --class (default 1,2,3).
  --mode class      READ of exactly the classes of --class, 0 (static data) included.

A poll reads events, and an outstation discards the events a master has read: polling an
outstation that is in service takes those events away from its regular master, unless
the outstation keeps a buffer per master (one per link address).

Use --object and --index to narrow what is shown; the outstation is still asked for
everything. When the outstation answers with values and also reports an error in its
internal indications, the values are listed and the exit code is 7.`,
		Example: `
  # Integrity poll of outstation 10 at 192.168.1.20
  dnp3ctl client poll --ip 192.168.1.20 --outstation 10

  # The same as a table, or as JSON for a script
  dnp3ctl client poll --ip 192.168.1.20 --format table
  dnp3ctl client poll --ip 192.168.1.20 --format json

  # The events of class 1 only
  dnp3ctl client poll --ip 192.168.1.20 --mode event --class 1

  # Static data only, analog inputs 0-9
  dnp3ctl client poll --ip 192.168.1.20 --mode class --class 0 --object g30 --index 0-9

  # Over a serial line
  dnp3ctl client poll --url serial:///dev/ttyUSB0 --baud 19200 --outstation 4

  # An outstation that connects to the master: wait for it on port 20000
  dnp3ctl client poll --listen :20000
`,
	}
	cfg := config.PollConfig{ConnConfig: connDefaults(), Mode: dnp.PollIntegrity, OutputFormat: text}
	wire(cmd, &cfg, described("mode", dnp.PollModes()), enum("class", "0", "1", "2", "3"))
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return formatted(cmd, cfg.OutputFormat, &cfg.Debug,
			func() error { return dnp.CheckPoll(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.Poll(ctx, &cfg, cmd.ErrOrStderr()) },
			runner.WithSuccessExit(partialExit))
	}
	return cmd
}

func newReadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "read",
		Short: "Read objects by group, variation and index range",
		Long: `Send a READ for the objects --object names and list what the outstation answers.

--object takes groups and variations: g30v5 (or 30.5) for one variation, g30 (or g30v0)
for "any variation", which the outstation answers in the variation it is configured with.
Several objects, comma-separated, are read in one request with the same --range.

--range selects the points: a range of indexes (0-9), a list (1,4,7) or one index. Without
it all points of the object are read. --count reads at most that many objects, which is
how a master reads a limited number of events (for example --object g2 --count 10).

Reading event objects (g2, g32, …) or classes takes the events away from the outstation's
buffer, as a poll does. "dnp3ctl client objects" lists the objects dnp3ctl knows.`,
		Example: `
  # All analog inputs, as single-precision floats with flags
  dnp3ctl client read --ip 192.168.1.20 --object g30v5

  # Binary inputs 0-15 in the variation the outstation chooses
  dnp3ctl client read --ip 192.168.1.20 --object g1 --range 0-15

  # Three counters and the binary outputs, in one request
  dnp3ctl client read --ip 192.168.1.20 --object g20,g10 --range 0,3,7

  # At most 10 analog input events
  dnp3ctl client read --ip 192.168.1.20 --object g32 --count 10

  # The internal indications (g80v1)
  dnp3ctl client read --ip 192.168.1.20 --object g80v1

  # Times and intervals (g50v4), and the security statistics of Secure Authentication
  dnp3ctl client read --ip 192.168.1.20 --object g50v4
  dnp3ctl client read --ip 192.168.1.20 --object g121v1
`,
	}
	cfg := config.ReadConfig{ConnConfig: connDefaults(), OutputFormat: text}
	wire(cmd, &cfg)
	cli.MustMarkFlagRequired(cmd, "object")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return formatted(cmd, cfg.OutputFormat, &cfg.Debug,
			func() error { return dnp.CheckRead(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.Read(ctx, &cfg, cmd.ErrOrStderr()) },
			runner.WithSuccessExit(partialExit))
	}
	return cmd
}

func newObjectsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "objects",
		Short: "List the objects (groups and variations) dnp3ctl knows",
		Long: `List every group and variation of IEEE 1815-2012 that dnp3ctl encodes and decodes,
with its name and the size of one object. These are the values --object takes.

Nothing is sent: this command needs no outstation.`,
		Example: `
  dnp3ctl client objects
  dnp3ctl client objects --group 30
  dnp3ctl client objects --events --format table
`,
		Args: cobra.NoArgs,
	}
	cfg := config.ObjectsConfig{OutputFormat: text}
	runner.RegisterCommandCfg(cmd, &cfg)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return runner.RunFormattedChecked(cmd, cfg.OutputFormat,
			func() error { return nil },
			func(context.Context) (any, error) { return dnp.Objects(&cfg), nil })
	}
	return cmd
}
