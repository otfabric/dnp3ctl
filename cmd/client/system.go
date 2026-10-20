package client

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/otfabric/dnp3ctl/internal/cli"
	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/dnp"
)

func newTimeSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "time-sync",
		Aliases: []string{"time"},
		Short:   "Synchronize the clock of an outstation (LAN, non-LAN or a plain write)",
		Long: `Set the clock of the outstation, which time-stamps its events, and clear its
NEED_TIME indication.

  --method lan      RECORD_CURRENT_TIME, then WRITE g50v3 with the time that request was
                    sent: the procedure for networks, where the delay is small (default)
  --method non-lan  DELAY_MEASURE, then WRITE g50v1 with the current time corrected by half
                    the round trip minus the outstation's processing delay: the procedure
                    for serial links
  --method write    WRITE g50v1 with --time ("now" or an RFC 3339 time), uncorrected

DNP3 time is UTC. THIS CHANGES THE OUTSTATION: the time stamps of its events follow its
clock. The request is repeated with --retries, as it is safe to repeat.`,
		Example: `
  # Synchronize with the LAN procedure
  dnp3ctl client time-sync --ip 192.168.1.20

  # Over a serial line, with delay compensation
  dnp3ctl client time-sync --url serial:///dev/ttyUSB0 --method non-lan

  # Set a given time, for a test
  dnp3ctl client time-sync --ip 192.168.1.20 --method write --time 2026-01-31T12:00:00Z
`,
	}
	cfg := config.TimeSyncConfig{ConnConfig: connDefaults(), Method: dnp.TimeLAN, OutputFormat: text}
	wire(cmd, &cfg, described("method", dnp.TimeMethods()))
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return formatted(cmd, cfg.OutputFormat, &cfg.Debug,
			func() error { return dnp.CheckTimeSync(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.TimeSync(ctx, &cfg, cmd.ErrOrStderr()) })
	}
	return cmd
}

func newRestartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart an outstation (COLD_RESTART or WARM_RESTART)",
		Long: `Ask the outstation for a restart and report the time after which it can be polled
again.

  --type cold  COLD_RESTART: the whole outstation restarts, as after a power cycle
  --type warm  WARM_RESTART: the DNP3 application restarts

THIS CHANGES THE OUTSTATION: it stops reporting while it restarts, and loses the events it
has not reported. --type has no default for that reason. The request is sent once.`,
		Example: `
  dnp3ctl client restart --ip 192.168.1.20 --type warm
  dnp3ctl client restart --ip 192.168.1.20 --type cold --format json
`,
	}
	cfg := config.RestartConfig{ConnConfig: connDefaults(), OutputFormat: text}
	wire(cmd, &cfg, described("type", dnp.RestartTypes()))
	cli.MustMarkFlagRequired(cmd, "type")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return formatted(cmd, cfg.OutputFormat, &cfg.Debug,
			func() error { return dnp.CheckRestart(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.Restart(ctx, &cfg, cmd.ErrOrStderr()) })
	}
	return cmd
}

func newUnsolicitedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unsolicited",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
		Short: "Enable or disable unsolicited responses",
		Long: `Enable or disable the unsolicited responses of an outstation for event classes
(ENABLE_UNSOLICITED, DISABLE_UNSOLICITED). The setting belongs to this master's
association: other masters are not affected. "client monitor" enables them by itself.`,
	}
	for _, enable := range []bool{true, false} {
		cmd.AddCommand(newUnsolicitedSubCmd(enable))
	}
	return cmd
}

func newUnsolicitedSubCmd(enable bool) *cobra.Command {
	use, short := "disable", "Disable unsolicited responses (DISABLE_UNSOLICITED)"
	if enable {
		use, short = "enable", "Enable unsolicited responses (ENABLE_UNSOLICITED)"
	}
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long: short + `.

--class names the event classes, comma-separated (default 1,2,3).`,
		Example: `
  dnp3ctl client unsolicited ` + use + ` --ip 192.168.1.20
  dnp3ctl client unsolicited ` + use + ` --ip 192.168.1.20 --class 1,2
`,
	}
	cfg := config.UnsolicitedConfig{ConnConfig: connDefaults(), OutputFormat: text}
	wire(cmd, &cfg, enum("class", "1", "2", "3"))
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return formatted(cmd, cfg.OutputFormat, &cfg.Debug,
			func() error { return dnp.CheckUnsolicited(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.Unsolicited(ctx, &cfg, enable, cmd.ErrOrStderr()) })
	}
	return cmd
}

func newAssignClassCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "assign-class",
		Short: "Assign points to an event class (ASSIGN_CLASS)",
		Long: `Assign the points --object and --range name to the event class --class: 1-3 makes
them produce events of that class, 0 makes them produce none. The points are still read
in class 0 polls.

--object takes static objects in their "any variation" form, for example g1 (binary
inputs), g20 (counters) or g30 (analog inputs). THIS CHANGES THE OUTSTATION: which events
it reports to every master changes.`,
		Example: `
  # All analog inputs to class 2
  dnp3ctl client assign-class --ip 192.168.1.20 --class 2 --object g30

  # Binary inputs 0-7 to class 1
  dnp3ctl client assign-class --ip 192.168.1.20 --class 1 --object g1 --range 0-7
`,
	}
	cfg := config.AssignClassConfig{ConnConfig: connDefaults(), OutputFormat: text}
	wire(cmd, &cfg, enum("class", "0", "1", "2", "3"))
	for _, f := range []string{"class", "object"} {
		cli.MustMarkFlagRequired(cmd, f)
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return formatted(cmd, cfg.OutputFormat, &cfg.Debug,
			func() error { return dnp.CheckAssignClass(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.AssignClass(ctx, &cfg, cmd.ErrOrStderr()) })
	}
	return cmd
}

func newFreezeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "freeze",
		Short: "Freeze counters or analog inputs, now or at a time (IMMED_FREEZE, FREEZE_CLEAR, FREEZE_AT_TIME)",
		Long: `Freeze the points --object and --range name: the outstation copies their current
values into the frozen counters (g21) or frozen analog inputs (g31), with the time of the
freeze. Read them afterwards with "client read --object g21".

--object defaults to g20 (all counters). --clear resets the counters to zero after the
freeze (FREEZE_CLEAR); --no-ack sends the no-acknowledgement form, which the outstation
does not answer.

--at schedules the freeze for a time to come instead (FREEZE_AT_TIME), and --interval
repeats it from then on, for example every 15m. The outstation keeps a limited number of
scheduled freezes, and refuses a time that has passed when there is no interval.

--broadcast sends the freeze to every outstation on the channel at once, for example to
freeze all the meters of a feeder at the same moment. Nobody answers a broadcast: each
outstation sets IIN1.0 BROADCAST in its next response. A broadcast cannot be combined
with secure authentication (see "client operate --help").

THIS CHANGES THE OUTSTATION: frozen values are overwritten, and with --clear the counters
start again from zero. The request is sent once.`,
		Example: `
  # Freeze every counter
  dnp3ctl client freeze --ip 192.168.1.20

  # Freeze and clear counters 0-3
  dnp3ctl client freeze --ip 192.168.1.20 --object g20 --range 0-3 --clear

  # Freeze the analog inputs
  dnp3ctl client freeze --ip 192.168.1.20 --object g30

  # Freeze every counter at midnight, and every hour from then on
  dnp3ctl client freeze --ip 192.168.1.20 --at 2026-02-01T00:00:00Z --interval 1h

  # Freeze the counters of every outstation on a serial line at once
  dnp3ctl client freeze --url serial:///dev/ttyUSB0 --broadcast no-confirm --no-ack
`,
	}
	cfg := config.FreezeConfig{ConnConfig: connDefaults(), OutputFormat: text}
	wire(cmd, &cfg, described("broadcast", dnp.BroadcastModes()))
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return formatted(cmd, cfg.OutputFormat, &cfg.Debug,
			func() error { return dnp.CheckFreeze(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.Freeze(ctx, &cfg, cmd.ErrOrStderr()) })
	}
	return cmd
}

// attributeNames lists the names of the attributes of set 0, for completion.
func attributeNames() []string {
	names := make([]string, 0, len(dnp.AttributeNames))
	for _, n := range dnp.AttributeNames {
		names = append(names, n)
	}
	return names
}

func newAttributesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "attributes",
		Aliases: []string{"attrs", "info"},
		Short:   "Read the device attributes of an outstation (group 0)",
		Long: `Read device attributes. Set 0, the default, is the standard set: manufacturer,
product, software and hardware version, serial number, point counts and the other
attributes of IEEE 1815-2012 Table 11-11 the outstation supports. --set reads another set,
one the outstation defines (clause 11.9.1).

--variation selects what is read: all (default), list (the list of the attribute
variations the set has, with whether each is writable), a variation number or, in set 0,
a name such as manufacturer, product, software-version or serial-number.`,
		Example: `
  dnp3ctl client attributes --ip 192.168.1.20
  dnp3ctl client attributes --ip 192.168.1.20 --variation list
  dnp3ctl client attributes --ip 192.168.1.20 --variation manufacturer --format json

  # Attribute set 1, as the outstation defines it
  dnp3ctl client attributes --ip 192.168.1.20 --set 1
`,
	}
	cfg := config.AttributesConfig{ConnConfig: connDefaults(), Variation: "all", OutputFormat: text}
	wire(cmd, &cfg, enum("variation", attributeNames()...))
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return formatted(cmd, cfg.OutputFormat, &cfg.Debug,
			func() error { return dnp.CheckAttributes(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.Attributes(ctx, &cfg, cmd.ErrOrStderr()) })
	}
	return cmd
}

func newWriteAttributeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "write-attribute",
		Short: "Write a device attribute of an outstation (WRITE of group 0)",
		Long: `Write the device attribute --variation of the set --set (0, the standard set, by
default) with --value, of the data type --data-type: vstr (a visible string, the
default), uint, int or float.

Only the attributes the outstation marks as writable can be written: "client attributes
--variation list" shows which. In set 0 these are typically the user-assigned ones, such
as location (245), device-name (247), owner (244) and user-id (246).

THIS CHANGES THE OUTSTATION. The request is sent once.`,
		Example: `
  # Name the location of the outstation
  dnp3ctl client write-attribute --ip 192.168.1.20 --variation location --value "Substation North, bay 7"

  # A numeric attribute of attribute set 1
  dnp3ctl client write-attribute --ip 192.168.1.20 --set 1 --variation 2 --value 42 --data-type uint
`,
	}
	cfg := config.WriteAttributeConfig{ConnConfig: connDefaults(), DataType: "vstr", OutputFormat: text}
	wire(cmd, &cfg, enum("variation", attributeNames()...), described("data-type", dnp.AttributeDataTypes()))
	for _, f := range []string{"variation", "value"} {
		cli.MustMarkFlagRequired(cmd, f)
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return formatted(cmd, cfg.OutputFormat, &cfg.Debug,
			func() error { return dnp.CheckWriteAttribute(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.WriteAttribute(ctx, &cfg, cmd.ErrOrStderr()) })
	}
	return cmd
}

func newManageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manage",
		Short: "Send a management function (INITIALIZE_DATA, START_APPL, SAVE_CONFIG, …)",
		Long: `Send one of the management functions of IEEE 1815-2012 clause 4.4.8 and report
whether the outstation accepted it:

  initialize-data         INITIALIZE_DATA (15)
  initialize-application  INITIALIZE_APPL (16), for every application (g90v1)
  start-application       START_APPL (17), for every application
  stop-application        STOP_APPL (18), for every application
  save-configuration      SAVE_CONFIG (19)

An outstation that does not implement a function refuses it with NO_FUNC_CODE_SUPPORT
(exit code 6).

THIS CHANGES THE OUTSTATION: stopping its applications stops what it does, and
initializing data sets its values back. The request is sent once.`,
		Example: `
  dnp3ctl client manage --ip 192.168.1.20 --function save-configuration
  dnp3ctl client manage --ip 192.168.1.20 --function stop-application --format json
`,
	}
	cfg := config.ManageConfig{ConnConfig: connDefaults(), OutputFormat: text}
	wire(cmd, &cfg, described("function", dnp.ManagementFunctions()))
	cli.MustMarkFlagRequired(cmd, "function")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return formatted(cmd, cfg.OutputFormat, &cfg.Debug,
			func() error { return dnp.CheckManage(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.Manage(ctx, &cfg, cmd.ErrOrStderr()) })
	}
	return cmd
}

func newClearRestartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clear-restart",
		Short: "Clear the DEVICE_RESTART indication of an outstation (WRITE g80v1 index 7)",
		Long: `Clear IIN1.7 DEVICE_RESTART, which an outstation sets when it restarts and keeps set
until a master clears it. The one-shot commands of dnp3ctl leave it set on purpose, so
that they change nothing they were not asked to; "client monitor" clears it, as a
long-running master does.`,
		Example: `
  dnp3ctl client clear-restart --ip 192.168.1.20
`,
	}
	cfg := config.ClearRestartConfig{ConnConfig: connDefaults(), OutputFormat: text}
	wire(cmd, &cfg)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return formatted(cmd, cfg.OutputFormat, &cfg.Debug,
			func() error { return dnp.CheckClearRestart(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.ClearRestart(ctx, &cfg, cmd.ErrOrStderr()) })
	}
	return cmd
}

func newWriteDeadbandCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "write-deadband",
		Short: "Write the deadband of analog inputs (WRITE g34)",
		Long: `Write the deadband of the analog inputs --index: how far a value must change before
the outstation reports an event. Read the deadbands with "client read --object g34".

--variation picks the encoding: 1 (16-bit), 2 (32-bit) or 3 (single-precision float); by
default 2 for whole numbers and 3 otherwise.

THIS CHANGES THE OUTSTATION: a larger deadband reports fewer events, a smaller one more.`,
		Example: `
  dnp3ctl client write-deadband --ip 192.168.1.20 --index 0 --value 0.5
  dnp3ctl client write-deadband --ip 192.168.1.20 --index 0,1,2 --value 10 --variation 1
`,
	}
	cfg := config.WriteDeadbandConfig{ConnConfig: connDefaults(), OutputFormat: text}
	wire(cmd, &cfg, enum("variation", "1", "2", "3"))
	for _, f := range []string{"index", "value"} {
		cli.MustMarkFlagRequired(cmd, f)
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return formatted(cmd, cfg.OutputFormat, &cfg.Debug,
			func() error { return dnp.CheckWriteDeadband(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.WriteDeadband(ctx, &cfg, cmd.ErrOrStderr()) })
	}
	return cmd
}
