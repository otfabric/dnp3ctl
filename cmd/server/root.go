// Package server holds the commands of a controlled station: `dnp3ctl server …`.
package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/otfabric/dnp3ctl/internal/cli"
	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/errs"
	"github.com/otfabric/dnp3ctl/internal/format"
	"github.com/otfabric/dnp3ctl/internal/runner"
	"github.com/otfabric/dnp3ctl/internal/station"
	"github.com/otfabric/dnp3ctl/internal/types"
	"github.com/spf13/cobra"
)

// NewCmd returns the `server` command with all its subcommands.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Args:  cli.GroupArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
		Short: "Act as an outstation: simulate an RTU from a model of points, controls and files",
		Long: `Commands that run a simulated outstation: a stand-in for an RTU, IED or gateway that
a SCADA master, a protocol gateway or "dnp3ctl client" can connect to.

The outstation is described by a model file (YAML or JSON): its link address, its points
with their initial values and event classes, its controls and its files. "server
generate-config" writes a commented example to start from.`,
	}
	cmd.AddCommand(newStartCmd(), newGenerateConfigCmd())
	return cmd
}

func newStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Serve a simulated outstation",
		Long: `Listen for masters and serve the outstation the model --config describes. Without
--config a built-in demo outstation is served (see "server generate-config").

The outstation answers integrity, event and class polls and reads of any object and
range, keeps an event buffer per master that survives reconnects, sends unsolicited
responses when masters enable them, executes controls (select-before-operate, direct
operate and direct operate without acknowledgement) on the points the model names as
their targets, answers time synchronization, delay measurement, restarts, freezes, assign
class and device attributes, and serves files. Values with "simulate" in the model change
by themselves and produce events.

It serves TCP by default (port 20000), TLS with --tls-cert and --tls-key (port 19999), UDP
with --udp, or a serial port with --serial. With --dial it does not listen but connects to
its master (the dual end point), tcp:// or tls://, and connects again whenever the
connection ends. With --sa-update-key-file or --sa-key-file (or DNP3CTL_SA_UPDATE_KEY,
DNP3CTL_SA_KEY) it requires Secure Authentication v5: critical requests are challenged,
or accepted in aggressive mode, and executed only with the right MAC; the security
statistics of every master are kept (g121, events g122 of --sa-statistics-class every
--sa-statistics-threshold increments), and a challenge waits --sa-reply-timeout for its
reply.

Every event (connections, requests, controls, management functions, unsolicited responses,
file transfers) is written to stdout as it happens: one line with --format text, one JSON
object per line with --format json, or one table row with --format table. With --debug
every frame is traced to stderr, together with go-dnp3's own log entries.

The server runs until interrupted (Ctrl-C), or for --duration seconds. It listens on all
interfaces by default and accepts any client: use --listen, --allow and the masters of the
model to restrict it.`,
		Example: `
  # Serve the built-in demo outstation on port 20000
  dnp3ctl server start

  # Serve your own model on another port, on localhost only
  dnp3ctl server start --config outstation.yaml --listen 127.0.0.1 --port 20001

  # TLS, masters must present a certificate of this CA
  dnp3ctl server start --config outstation.yaml --tls-cert server.pem --tls-key server.key --tls-client-ca ca.pem

  # Secure Authentication v5 with a pre-shared key, events as JSON Lines
  dnp3ctl server start --config outstation.yaml --sa-key-file sa.key --format json

  # On a serial port, 19200 baud
  dnp3ctl server start --serial /dev/ttyUSB0 --baud 19200

  # Connect to the master instead of listening, over TLS
  dnp3ctl server start --config outstation.yaml --dial tls://scada.example.net --tls-ca ca.pem --tls-cert rtu.pem --tls-key rtu.key

  # In a test pipeline: serve for 60 seconds, then exit
  dnp3ctl server start --config outstation.yaml --duration 60
`,
	}
	cfg := config.ServerConfig{AuthConfig: config.AuthConfig{SAUser: 1}, OutputFormat: string(format.FormatText)}
	runner.RegisterCommandCfg(cmd, &cfg, func(c *cobra.Command) {
		_ = c.MarkFlagFilename("config", "yaml", "yml", "json")
		_ = cli.RegisterEnumFlagCompletion(c, "parity", []string{"none", "even", "odd"})
		for _, f := range []string{"tls-cert", "tls-key", "tls-client-ca", "tls-ca"} {
			_ = c.MarkFlagFilename(f, "pem", "crt", "cer", "key")
		}
		_ = c.MarkFlagFilename("sa-key-file")
		_ = cli.RegisterEnumFlagCompletion(c, "sa-statistics-class", []string{"1", "2", "3"})
	})
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		cfg.Debug = cli.Debug(cmd)
		return runner.RunStream(cmd, cfg.OutputFormat,
			func() error { return station.Check(&cfg) },
			func(ctx context.Context, out *format.StreamWriter) (int, error) {
				var writeErr error
				log := func(e types.ServerEvent) {
					if err := out.Write(&e); err != nil && writeErr == nil {
						writeErr = err
					}
				}
				ready := func(net.Addr) {}
				if err := station.Run(ctx, &cfg, log, cmd.ErrOrStderr(), ready); err != nil {
					return 0, err
				}
				if writeErr != nil {
					return 0, errs.Output(errs.CodeOutputWriteFailed, writeErr)
				}
				return errs.ExitOK, nil
			})
	}
	return cmd
}

func newGenerateConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate-config",
		Short: "Write an example outstation model to start from",
		Long: `Write the model of the built-in demo outstation: a small feeder RTU with a point of
every type, controls for its outputs and a few files. Edit it and serve it with
"dnp3ctl server start --config <file>".

The YAML form carries comments that explain every section; --format json writes the same
model as JSON. An existing file is not overwritten without --force.`,
		Example: `
  dnp3ctl server generate-config --output outstation.yaml
  dnp3ctl server generate-config --format json --output outstation.json
  dnp3ctl server generate-config            # to stdout
`,
	}
	cfg := config.GenerateConfigConfig{Output: "-", Format: "yaml"}
	// Not runner.RegisterCommandCfg: --format is the format of the model here, not of stdout.
	config.MustLoadFromEnv(&cfg)
	config.RegisterFlags(cmd, &cfg)
	_ = cli.RegisterEnumFlagCompletion(cmd, "format", []string{"yaml", "json"})
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		data, err := generate(cfg)
		if err != nil {
			return err
		}
		if cfg.Output == "-" || cfg.Output == "" {
			if _, err := cmd.OutOrStdout().Write(data); err != nil {
				return errs.Output(errs.CodeOutputWriteFailed, err)
			}
			return nil
		}
		flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
		if cfg.Force {
			flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		}
		f, err := os.OpenFile(cfg.Output, flags, 0o644)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				return errs.InvalidInput(errs.CodeOutputFileCreateFailed, fmt.Sprintf("%s exists: pass --force to overwrite it", cfg.Output), err)
			}
			return errs.Output(errs.CodeOutputFileCreateFailed, err)
		}
		if _, err := f.Write(data); err != nil {
			_ = f.Close()
			return errs.Output(errs.CodeOutputFileWriteFailed, err)
		}
		if err := f.Close(); err != nil {
			return errs.Output(errs.CodeOutputFileWriteFailed, err)
		}
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "wrote %s\n", cfg.Output)
		return nil
	}
	return cmd
}

func generate(cfg config.GenerateConfigConfig) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Format)) {
	case "", "yaml", "yml":
		return []byte(station.DemoYAML), nil
	case "json":
		data, err := station.Demo().Encode(true)
		if err != nil {
			return nil, errs.Output(errs.CodeJSONEncodeFailed, err)
		}
		return data, nil
	}
	return nil, errs.InvalidInput(errs.CodeInvalidFormat, fmt.Sprintf("invalid --format %q (expected: yaml, json)", cfg.Format), nil)
}
