package client

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/otfabric/dnp3ctl/internal/cli"
	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/dnp"
)

func newFileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "file",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
		Short: "Transfer files: list, info, get, put, delete (group 70)",
		Long: `File transfer of DNP3 (group 70): list the directories of an outstation, read a
file's descriptor, fetch a file, write one and delete one.

Outstations that protect their files take credentials first (AUTHENTICATE_FILE): give the
user with --file-user and the password in --file-password-file or the environment
variable DNP3CTL_FILE_PASSWORD; the password is never accepted on the command line.`,
	}
	cmd.AddCommand(newFileListCmd(), newFileInfoCmd(), newFileGetCmd(), newFilePutCmd(), newFileDeleteCmd())
	return cmd
}

func fileCompletion(cmd *cobra.Command) {
	_ = cmd.MarkFlagFilename("file-password-file")
}

func newFileListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list", "dir"},
		Short:   "List a directory of the outstation",
		Long: `Open the directory --path for reading and list its entries: name, type, size, time
of creation and permissions.`,
		Example: `
  dnp3ctl client file ls --ip 192.168.1.20
  dnp3ctl client file ls --ip 192.168.1.20 --path /records --format table
`,
	}
	cfg := config.FileListConfig{ConnConfig: connDefaults(), Path: "/", OutputFormat: text}
	wire(cmd, &cfg, fileCompletion)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return formatted(cmd, cfg.OutputFormat, &cfg.Debug,
			func() error { return dnp.CheckFileList(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.FileList(ctx, &cfg, cmd.ErrOrStderr()) })
	}
	return cmd
}

func newFileInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "info",
		Aliases: []string{"stat"},
		Short:   "Show the descriptor of a file or directory (GET_FILE_INFO)",
		Long:    `Ask the outstation for the descriptor of --path: type, size, time of creation and permissions.`,
		Example: `
  dnp3ctl client file info --ip 192.168.1.20 --path /logs/events.csv
`,
	}
	cfg := config.FileInfoConfig{ConnConfig: connDefaults(), OutputFormat: text}
	wire(cmd, &cfg)
	cli.MustMarkFlagRequired(cmd, "path")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return formatted(cmd, cfg.OutputFormat, &cfg.Debug,
			func() error { return dnp.CheckFileInfo(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.FileInfo(ctx, &cfg, cmd.ErrOrStderr()) })
	}
	return cmd
}

func newFileGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get",
		Aliases: []string{"download"},
		Short:   "Fetch a file from the outstation",
		Long: `Read the file --path from the outstation and write it to --output (default: its
base name in the current directory). An existing file is not overwritten without --force.

With --output - the content goes to stdout and the result (length, SHA-256) to stderr, so
that the content can be piped.`,
		Example: `
  dnp3ctl client file get --ip 192.168.1.20 --path /records/disturbance-0001.dat
  dnp3ctl client file get --ip 192.168.1.20 --path /logs/events.csv --output - | head
`,
	}
	cfg := config.FileGetConfig{ConnConfig: connDefaults(), OutputFormat: text}
	wire(cmd, &cfg, fileCompletion, func(c *cobra.Command) { _ = c.MarkFlagFilename("output") })
	cli.MustMarkFlagRequired(cmd, "path")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		content := cmd.OutOrStdout()
		if cfg.Output == "-" {
			// The content takes stdout; the result goes to stderr.
			cmd.SetOut(cmd.ErrOrStderr())
		}
		return formatted(cmd, cfg.OutputFormat, &cfg.Debug,
			func() error { return dnp.CheckFileGet(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.FileGet(ctx, &cfg, cmd.ErrOrStderr(), content) })
	}
	return cmd
}

func newFilePutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "put",
		Aliases: []string{"upload"},
		Short:   "Write a file to the outstation",
		Long: `Send the local file --input to the outstation as --path, with the permissions of
--permissions (default rw-r--r--). With --append the content is added to the end of the
file instead (OPEN_FILE in APPEND mode), for logs and other files that grow.

THIS CHANGES THE OUTSTATION: an existing file of that name is replaced (or added to), and
the file may be a configuration the outstation acts on. The transfer is not repeated.`,
		Example: `
  dnp3ctl client file put --ip 192.168.1.20 --input settings.xml --path /config/settings.xml
  dnp3ctl client file put --ip 192.168.1.20 --input fw.bin --path /fw.bin --permissions 600 --block-size 1024
  dnp3ctl client file put --ip 192.168.1.20 --input today.csv --path /logs/events.csv --append
`,
	}
	cfg := config.FilePutConfig{ConnConfig: connDefaults(), OutputFormat: text}
	wire(cmd, &cfg, fileCompletion, func(c *cobra.Command) { _ = c.MarkFlagFilename("input") })
	for _, f := range []string{"path", "input"} {
		cli.MustMarkFlagRequired(cmd, f)
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return formatted(cmd, cfg.OutputFormat, &cfg.Debug,
			func() error { return dnp.CheckFilePut(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.FilePut(ctx, &cfg, cmd.ErrOrStderr()) })
	}
	return cmd
}

func newFileDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rm",
		Aliases: []string{"delete", "del"},
		Short:   "Delete a file of the outstation",
		Long:    `Delete the file --path on the outstation. THIS CHANGES THE OUTSTATION, and cannot be undone.`,
		Example: `
  dnp3ctl client file rm --ip 192.168.1.20 --path /logs/old.csv
`,
	}
	cfg := config.FileDeleteConfig{ConnConfig: connDefaults(), OutputFormat: text}
	wire(cmd, &cfg, fileCompletion)
	cli.MustMarkFlagRequired(cmd, "path")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return formatted(cmd, cfg.OutputFormat, &cfg.Debug,
			func() error { return dnp.CheckFileDelete(&cfg) },
			func(ctx context.Context) (any, error) { return dnp.FileDelete(ctx, &cfg, cmd.ErrOrStderr()) })
	}
	return cmd
}
