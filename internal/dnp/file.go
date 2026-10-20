package dnp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"strings"

	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/client"

	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/errs"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// FileEntry converts a file descriptor into the entry of the results.
func FileEntry(d app.FileDescriptor) types.FileEntry {
	kind := "file"
	if d.Type == app.FileTypeDirectory {
		kind = "directory"
	}
	return types.FileEntry{Name: d.Name, Type: kind, Size: d.Size, Created: timeText(d.Created), Permissions: d.Permissions.String()}
}

// filePassword returns the password of --file-user: from --file-password-file or
// DNP3CTL_FILE_PASSWORD, never from the command line.
func filePassword(cfg config.FileAuthConfig) (string, error) {
	switch {
	case cfg.FileUser == "" && (cfg.FilePasswordFile != "" || cfg.FilePassword != ""):
		return "", fmt.Errorf("a file password needs --file-user")
	case cfg.FileUser == "":
		return "", nil
	case cfg.FilePasswordFile != "" && cfg.FilePassword != "":
		return "", fmt.Errorf("--file-password-file and DNP3CTL_FILE_PASSWORD are mutually exclusive")
	case cfg.FilePasswordFile != "":
		data, err := os.ReadFile(cfg.FilePasswordFile)
		if err != nil {
			return "", fmt.Errorf("reading the file password: %w", err)
		}
		return strings.TrimRight(string(data), "\r\n"), nil
	}
	return cfg.FilePassword, nil
}

// fileOptions authenticates when --file-user is set and returns the options of a transfer.
func fileOptions(ctx context.Context, s *Session, cfg config.FileAuthConfig) (client.FileOptions, error) {
	o := client.FileOptions{MaxBlockSize: cfg.BlockSize}
	if cfg.FileUser == "" {
		return o, nil
	}
	password, err := filePassword(cfg)
	if err != nil {
		return o, errs.WrapValidation(err)
	}
	key, err := s.AuthenticateFile(ctx, cfg.FileUser, password)
	if err != nil {
		return o, err
	}
	if key == 0 {
		return o, errs.New(errs.KindRefused, errs.CodeFileRefused, fmt.Sprintf("the outstation refused the file credentials of user %q", cfg.FileUser), nil)
	}
	o.AuthKey = key
	return o, nil
}

func checkFile(cc *config.ConnConfig, fa *config.FileAuthConfig, p string) error {
	if err := Validate(cc); err != nil {
		return err
	}
	if strings.TrimSpace(p) == "" {
		return fmt.Errorf("no file given: set --path")
	}
	if len(p) > math.MaxUint16 {
		return fmt.Errorf("--path is longer than %d octets", math.MaxUint16)
	}
	if fa != nil {
		if _, err := filePassword(*fa); err != nil {
			return err
		}
	}
	return nil
}

func checksum(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// CheckFileList validates the configuration of `client file ls`.
func CheckFileList(cfg *config.FileListConfig) error {
	return checkFile(&cfg.ConnConfig, &cfg.FileAuthConfig, cfg.Path)
}

// FileList lists a directory of the outstation.
func FileList(ctx context.Context, cfg *config.FileListConfig, trace io.Writer) (*types.FileListResult, error) {
	s, err := Dial(ctx, &cfg.ConnConfig, trace)
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.Close() }()
	var entries []app.FileDescriptor
	if _, err := s.request(ctx, func(ctx context.Context) error {
		o, err := fileOptions(ctx, s, cfg.FileAuthConfig)
		if err != nil {
			return err
		}
		entries, err = s.ListDirectory(ctx, cfg.Path, o)
		return err
	}); err != nil {
		return nil, WrapError(err)
	}
	res := &types.FileListResult{Target: s.Target.String(), Outstation: s.Outstation, Path: cfg.Path, Entries: []types.FileEntry{}}
	for _, d := range entries {
		res.Entries = append(res.Entries, FileEntry(d))
	}
	res.Count = len(res.Entries)
	return res, nil
}

// CheckFileInfo validates the configuration of `client file info`.
func CheckFileInfo(cfg *config.FileInfoConfig) error {
	return checkFile(&cfg.ConnConfig, nil, cfg.Path)
}

// FileInfo returns the descriptor of a file or directory of the outstation.
func FileInfo(ctx context.Context, cfg *config.FileInfoConfig, trace io.Writer) (*types.FileInfoResult, error) {
	s, err := Dial(ctx, &cfg.ConnConfig, trace)
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.Close() }()
	var d app.FileDescriptor
	if _, err := s.request(ctx, func(ctx context.Context) error {
		var rerr error
		d, rerr = s.FileInfo(ctx, cfg.Path)
		return rerr
	}); err != nil {
		return nil, WrapError(err)
	}
	entry := FileEntry(d)
	if entry.Name == "" {
		entry.Name = cfg.Path
	}
	return &types.FileInfoResult{Target: s.Target.String(), Outstation: s.Outstation, Path: cfg.Path, Entry: entry}, nil
}

// GetOutput returns the local file `client file get` writes: --output, or the base name of
// --path.
func GetOutput(cfg *config.FileGetConfig) string {
	if cfg.Output != "" {
		return cfg.Output
	}
	return path.Base(strings.ReplaceAll(cfg.Path, "\\", "/"))
}

// CheckFileGet validates the configuration of `client file get`.
func CheckFileGet(cfg *config.FileGetConfig) error {
	if err := checkFile(&cfg.ConnConfig, &cfg.FileAuthConfig, cfg.Path); err != nil {
		return err
	}
	out := GetOutput(cfg)
	switch out {
	case "-":
		return nil
	case "", ".", "/":
		return fmt.Errorf("cannot name the local file after --path %q: set --output", cfg.Path)
	}
	if _, err := os.Stat(out); err == nil && !cfg.Force {
		return errs.InvalidInput(errs.CodeOutputFileCreateFailed, fmt.Sprintf("%s exists: pass --force to overwrite it", out), nil)
	}
	return nil
}

// FileGet reads a file of the outstation into the local file of cfg, or into content for
// --output -.
func FileGet(ctx context.Context, cfg *config.FileGetConfig, trace, content io.Writer) (*types.FileTransferResult, error) {
	out := GetOutput(cfg)
	s, err := Dial(ctx, &cfg.ConnConfig, trace)
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.Close() }()
	var buf bytes.Buffer
	took, err := s.request(ctx, func(ctx context.Context) error {
		o, err := fileOptions(ctx, s, cfg.FileAuthConfig)
		if err != nil {
			return err
		}
		_, err = s.ReadFile(ctx, cfg.Path, &buf, o)
		return err
	})
	if err != nil {
		return nil, WrapError(err)
	}
	data := buf.Bytes()
	res := &types.FileTransferResult{
		Target: s.Target.String(), Outstation: s.Outstation, Direction: "download", Path: cfg.Path,
		Length: int64(len(data)), SHA256: checksum(data), DurationMs: ms(took),
	}
	if out == "-" {
		if _, err := content.Write(data); err != nil {
			return nil, errs.Output(errs.CodeOutputWriteFailed, err)
		}
		return res, nil
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if cfg.Force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(out, flags, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, errs.InvalidInput(errs.CodeOutputFileCreateFailed, fmt.Sprintf("%s exists: pass --force to overwrite it", out), err)
		}
		return nil, errs.Output(errs.CodeOutputFileCreateFailed, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return nil, errs.Output(errs.CodeOutputFileWriteFailed, err)
	}
	if err := f.Close(); err != nil {
		return nil, errs.Output(errs.CodeOutputFileWriteFailed, err)
	}
	res.Local = out
	return res, nil
}

// CheckFilePut validates the configuration of `client file put`, the input file included.
func CheckFilePut(cfg *config.FilePutConfig) error {
	if err := checkFile(&cfg.ConnConfig, &cfg.FileAuthConfig, cfg.Path); err != nil {
		return err
	}
	if _, err := ParsePermissions(cfg.Permissions); err != nil {
		return err
	}
	if cfg.Input == "" {
		return fmt.Errorf("no input given: set --input to the file to send")
	}
	info, err := os.Stat(cfg.Input)
	switch {
	case err != nil:
		return errs.InvalidInput(errs.CodeInputFileOpenFailed, fmt.Sprintf("input file: %v", err), err)
	case info.IsDir():
		return fmt.Errorf("input file %s is a directory", cfg.Input)
	case info.Size() > math.MaxUint32:
		return fmt.Errorf("input file %s has %d octets: the maximum is %d", cfg.Input, info.Size(), uint32(math.MaxUint32))
	}
	return nil
}

// FilePut writes a local file to the outstation.
func FilePut(ctx context.Context, cfg *config.FilePutConfig, trace io.Writer) (*types.FileTransferResult, error) {
	perm, err := ParsePermissions(cfg.Permissions)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(cfg.Input)
	if err != nil {
		return nil, errs.InvalidInput(errs.CodeInputFileOpenFailed, fmt.Sprintf("input file: %v", err), err)
	}
	s, err := Dial(ctx, &cfg.ConnConfig, trace)
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.Close() }()
	var n int64
	took, err := s.request(ctx, func(ctx context.Context) error {
		o, err := fileOptions(ctx, s, cfg.FileAuthConfig)
		if err != nil {
			return err
		}
		o.Permissions, o.Append = perm, cfg.Append
		n, err = s.WriteFile(ctx, cfg.Path, bytes.NewReader(data), uint32(len(data)), o)
		return err
	})
	if err != nil {
		return nil, WrapError(err)
	}
	return &types.FileTransferResult{
		Target: s.Target.String(), Outstation: s.Outstation, Direction: "upload", Path: cfg.Path, Local: cfg.Input, Append: cfg.Append,
		Length: n, SHA256: checksum(data), DurationMs: ms(took),
	}, nil
}

// CheckFileDelete validates the configuration of `client file rm`.
func CheckFileDelete(cfg *config.FileDeleteConfig) error {
	return checkFile(&cfg.ConnConfig, &cfg.FileAuthConfig, cfg.Path)
}

// FileDelete deletes a file of the outstation.
func FileDelete(ctx context.Context, cfg *config.FileDeleteConfig, trace io.Writer) (*types.ConfirmedResult, error) {
	res := &types.ConfirmedResult{Request: "delete-file", Path: cfg.Path}
	return confirmed(ctx, &cfg.ConnConfig, trace, res, func(ctx context.Context, s *Session) error {
		o, err := fileOptions(ctx, s, cfg.FileAuthConfig)
		if err != nil {
			return err
		}
		return s.DeleteFile(ctx, cfg.Path, o)
	})
}
