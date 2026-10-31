# AGENTS.md — dnp3ctl

## What this is

Command-line toolkit for DNP3 (IEEE 1815) built on [go-dnp3](https://github.com/otfabric/go-dnp3): master commands (poll, operate, time, files) and a simulated outstation. Pure Go, one binary. Profile: **cli**.

## Organisation standards

This repository follows **otfabric/engineering policy 1.2**, profile **cli** (`.otfabric/policy.yaml`, mode `enforce`). Where this file and the organisation standards disagree, this file wins here — say so in your report.

## Where the truth lives

| Question | Source |
|---|---|
| Exit codes (script-facing API) | docs/exitcodes.md |
| JSON output contract | docs/json-output.md |
| Commands and usage | README.md |
| How to contribute | CONTRIBUTING.md |
| Protocol behaviour | go-dnp3 |
| Release history | RELEASE.md (`## vX.Y.Z` … `---`) |

## Hard rules

1. Exit codes and JSON output are a public contract: a change is called out in RELEASE.md and documented in docs/exitcodes.md or docs/json-output.md.
2. Never lower `COVERAGE_MIN` or the CI `coverage-min`, add lint suppressions without a reason, or skip or delete tests to make CI pass.
3. Never invent protocol semantics or claim interoperability or conformance beyond recorded evidence; protocol questions go to go-dnp3.
4. `github.com/otfabric/*` dependencies are pinned to released tags; no committed `replace` directives.
5. Secrets (keys, passwords) are read from files or the environment, never from flags.
6. An error that reaches the user is an `*errs.Error` with a kind and a stable code (`internal/errs`, `dnp.WrapError`); a new code goes into docs/exitcodes.md.

## Commands

| When you change… | Run |
|---|---|
| anything | `make check` |
| the command line, exit codes or `main` | `make e2e` (builds the binary and runs it) |
| 32-bit relevant code (linux/arm/v7 is a release target) | `make cross` |

## Definition of Done

`make check` green; tests for new commands, error paths included; exit codes and JSON documented; RELEASE.md section for user-visible changes.

## Git and releases

Branches and PRs. Do not push to `main`, tag or run the Release workflow unless the maintainer asks in the session.
