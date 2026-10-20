# Contributing to dnp3ctl

Thank you for your interest in contributing to `dnp3ctl`! This document covers the guidelines for contributing.

## Getting Started

1. Fork the repository on GitHub
2. Clone your fork locally:
   ```sh
   git clone git@github.com:<your-username>/dnp3ctl.git
   cd dnp3ctl
   ```
3. Create a feature branch:
   ```sh
   git checkout -b feature/my-change
   ```

## Development Setup

### Prerequisites

- Go 1.25+ (see [go.mod](go.mod))
- [staticcheck](https://staticcheck.dev/), [golangci-lint](https://golangci-lint.run/) and [govulncheck](https://go.dev/doc/security/vuln/) for the checks
- No hardware: the test suite brings its own outstation (see [Testing](#testing))

### Build & Test

```sh
# Run all checks (format, lint, vet, vulnerability scan, race tests, coverage gate)
make check

# Run tests with the race detector
make test

# Run tests with whole-module coverage (fails below 85%)
make coverage

# Build for the current platform (./bin/dnp3ctl); runs `make check` first
make build
```

Run `make` (or `make help`) for the full list of targets.

## Making Changes

### Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep functions focused and small
- Use meaningful variable and function names
- Add comments only where the logic isn't self-evident; comments end with a period (enforced by the linter)
- Classify every failure: an error that reaches the user is an `*errs.Error` with a kind and a stable code (`internal/errs`, `dnp.WrapError`). A new code is part of the contract: add it to [docs/exitcodes.md](docs/exitcodes.md)
- Secrets (keys, passwords) are read from files or the environment, never from flags

### Formatting

`make fmt` formats with the `gofmt` of the Go version in `go.mod`, which is the version CI uses, not necessarily the one you have installed. `make fmt-check` (part of `make check`) fails if either that `gofmt` or your local one would change a file: `gofmt` output differs slightly between Go releases, and a file the two disagree on fails CI. If they disagree, restructure the code (for example, move trailing comments onto their own lines) rather than picking a side.

### Commit Messages

Use clear, concise commit messages:

```
component: short description

Optional longer explanation of the change, why it was made,
and any relevant context.
```

Examples:
- `monitor: poll the events of classes 1-3 at an interval`
- `operate: refuse a fractional value for a 16-bit set-point`
- `docs: document the outstation model`

### Testing

- Add tests for new functionality in the appropriate `_test.go` file
- Use [`internal/stationtest`](internal/stationtest) for anything that talks to an outstation. It serves the simulated outstation of `dnp3ctl server` on a free loopback port, records the events it logs, and makes throw-away certificates for TLS tests. Command tests run the real command tree through `cmd.Run` and assert on stdout, stderr and the exit code
- Assert behaviour (output, exit code, the outstation's database), not just that the code ran
- Tests must not write files into the repository: use `t.TempDir()`
- Do not assume timing an outstation does not promise: wait for an event with a deadline instead of sleeping
- Keep tests fast and race-clean; CI runs them with `-race`
- Whole-module statement coverage must stay at or above 85% (`make coverage`)
- If your change affects what goes on the wire, verify against a real outstation or another stack when possible (the containers of [dnp3-interop](https://github.com/otfabric/dnp3-interop) are an easy start), and say in the pull request what you tested against

### Documentation

When you change commands, flags, output or exit codes, update:

- The command's `Short` / `Long` / `Example` help text
- [README.md](README.md), including the environment variable table for new flags
- [docs/json-output.md](docs/json-output.md) and [docs/exitcodes.md](docs/exitcodes.md) — `--format json` and exit codes are a stable contract for scripts
- [docs/server-config.md](docs/server-config.md) and the commented demo model in `internal/station/demo.go` when the outstation model changes
- [RELEASE.md](RELEASE.md), calling out anything that breaks existing usage

## Submitting Changes

1. Push your branch to your fork:
   ```sh
   git push origin feature/my-change
   ```
2. Open a Pull Request against the `main` branch
3. Describe what your change does and why
4. Reference any related issues

For larger changes, please open an issue first to discuss the approach.

## Reporting Issues

- Use GitHub Issues to report bugs or suggest features
- Include the output of `dnp3ctl version` in bug reports
- Provide the exact command that triggered the issue, along with the error output; `--debug` adds every link frame and application fragment sent and received
- Mention the outstation type and vendor, the transport (TCP, TLS, UDP, serial) and its link addresses
- Report security vulnerabilities privately: see [SECURITY.md](SECURITY.md)

## Project Structure

```
cmd/                   Root command, version, completion
cmd/client/            Client commands (poll, read, monitor, operate, time-sync, restart, unsolicited, assign-class, freeze, attributes, ping, file, objects)
cmd/server/            Simulated outstation (start, generate-config)
cmd/discover/          Subnet discovery
internal/config/       Configuration types, flag and environment binding, outstation URLs
internal/dnp/          What the commands do: connections, requests, monitor, files, discovery, error classification
internal/station/      The simulated outstation: model, validation, controls, simulation, files
internal/stationtest/  The simulated outstation for tests
internal/format/       Output formatting (text, JSON, table, streaming)
internal/types/        Output data models
internal/runner/       Command wiring, formatted and streaming runs, fatal rendering
internal/errs/         Typed errors, stable codes, exit-code mapping
internal/cli/          Shared Cobra helpers
docs/                  Exit codes, JSON output contract, outstation model reference
```

## License

By contributing to `dnp3ctl`, you agree that your contributions will be licensed under the [MIT License](LICENSE).
