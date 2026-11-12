# dnp3ctl Releases

## v0.13.1

**Date:** 2026-11-12
**Previous release:** v0.13.0

### Summary

Built on go-serial v0.3.0 (from v0.2.1). No flag, output or exit code changes.

### Changed

- go-serial v0.3.0 checks serial parameters before narrowing them: a data-bits or stop-bits value out of range is an error instead of being truncated to a valid one, and an RS-485 RTS delay above 2^32-1 ms is refused instead of wrapping around. go-serial now requires Go 1.26, which dnp3ctl already did.

---

## v0.13.0

**Date:** 2026-11-12
**Previous release:** v0.12.0

### Summary

Every gosec finding is fixed in the code; gosec runs on a maintainer's machine before each release, not in public CI. TLS always verifies the outstation: `--tls-insecure` and `DNP3CTL_TLS_INSECURE` are removed, and setting either is an error that names `--tls-ca`. Files of the outstation model and its upload directory are opened through `os.Root`. Exit codes and the JSON output are unchanged.

### Upgrade notes

- **`--tls-insecure` is removed** (breaking). TLS always verifies the outstation's certificate, against `--tls-ca` or the system roots. For an outstation with a self-signed certificate, or a certificate of a private CA, pass that certificate or CA: `dnp3ctl client poll --url tls://rtu:19999 --tls-ca rtu.pem`. A command line that still has the flag fails with exit code 2 (usage, `invalid_input`) and `error: --tls-insecure was removed in v0.13.0; TLS always verifies the outstation's certificate: pass --tls-ca with the CA certificate (or the self-signed certificate) of the outstation`.
- **`DNP3CTL_TLS_INSECURE` is removed** (breaking). Set to any value, it fails every command with exit code 3 (invalid input, `invalid_input`) and `error: DNP3CTL_TLS_INSECURE was removed in v0.13.0: unset it; TLS always verifies …`, like a malformed environment variable. Unset it. dnp3ctl has no configuration file for the client, so there is no key to remove; the outstation model never had a TLS setting.
- A relative `files[].path` of an outstation model is read through an `os.Root` of the model file's directory: `../shared/x.bin`, or a symbolic link that leads out of that directory, is now an error of the model. Use an absolute path for a file elsewhere.
- Uploads to `uploads.dir` are written and removed through an `os.Root` of that directory: a symbolic link inside it that leads out of it is no longer followed, and the write is refused.

### Changed

- Built on go-dnp3 v1.13.0 (from v1.12.0): the client's dialer gets the 10 s dial timeout in its context, as `--source-address`'s `net.Dialer` already had; no flag or output changes.
- The `random` simulation mode draws from `crypto/rand` instead of `math/rand/v2`.
- File names the operator gives (`--output`, `--input`, `--config`, key, password and certificate files) are cleaned with `filepath.Clean` before they are opened.
- `--tls-server-name` with `--listen` reports `--tls-server-name does not apply to --listen`.

### Quality

- gosec: every finding is fixed in the code. Standalone `gosec -tests ./...` (v2.30.0, the version otfabric/engineering pins) reports 0 issues, from 23 before (18 distinct: G304 ×10, G703 ×2, G115 ×3, G302 ×3, G402 ×1, G404 ×1; the rest are the test variants of the packages). There is no `nolint` or `#nosec` comment and no exclusion in the configuration; the unused `examples` formatter exclusion is gone.
- Security scanning follows otfabric/engineering ADR-0011: gosec does not run in public lint or CI (rule GO-014). A maintainer runs `cd ../engineering && make sast REPO=dnp3ctl` in a checkout of otfabric/engineering before each release. Accepted findings are kept in a private triage repository; dnp3ctl has none.
- Tests: the removed flag and environment variable, as text and as JSON, and the environment variable failing every command; model files and uploads that try to leave their directory (`..`, symbolic links); `crypto/rand` fractions. Tests use correct integer types instead of narrowing conversions; the key file fixtures are created `0600` and loosened through `os.Root`, as the warning they check is about loose permissions.
- SECURITY.md and CONTRIBUTING.md describe the static analysis and how to fix a finding.

---

## v0.12.0

**Date:** 2026-11-12
**Previous release:** v0.11.0

### Summary

Built on go-dnp3 v1.12.0. Since v1.11.0 its master runs the startup and restart procedures of IEEE 1815-2012 by itself. The one-shot commands keep doing only what they are asked; `client monitor` runs those procedures. Every go-dnp3 feature that a command can use is exposed: new flags for the master and the simulated outstation, `client manage --function activate-config`, `client time-sync --method clear-need-time` and `client file put --created`. Exit codes are unchanged; the JSON output gains one field.

### Changed

- **go-dnp3 v1.12.0** (from v1.9.1; v1.11.0 brought the changes below, v1.12.0 the dialers and redial jitter). Read its release notes; for dnp3ctl:
  - **One-shot commands send their request and nothing else.** go-dnp3 runs an integrity poll at connect (clause 5.1.1.2) and answers NEED_TIME (4.5.5) by default from v1.11; every `client` command but `monitor` turns both off, as well as the clear of IIN1.7 and the poll after a restart, as before.
  - **`client monitor`** runs the master procedures: with unsolicited classes it disables them and runs an integrity poll before it enables them (5.1.1.2 Rule 1); after the outstation reports a restart, go-dnp3 runs the restart procedure (5.1.1.1.2) and enables the classes again, which the monitor did itself before; it answers NEED_TIME (`--time-sync`, default `auto`). stderr now says `monitor: the outstation reported a restart` instead of `monitor: the outstation restarted: unsolicited responses enabled again`.
  - **The simulated outstation** (`server start`) follows go-dnp3 v1.11: it refuses a second connection from a connected master that answers a link status request (13.2.3.3) instead of replacing the first; a Secure Authentication challenge waits 2 s for its reply (was 10 s); session keys expire after 30 minutes or 2000 messages without a change; security statistics carry Association ID 0 and have the thresholds of Table 7-6; SAVE_CONFIG is answered with a delay object (g52v2), ACTIVATE_CONFIG with g91v1; frozen counter and frozen analog input events are raised at every freeze. Aggressive mode MAC follows Table A-9: it does not interoperate with dnp3ctl releases before this one.
- UDP connections are opened with go-dnp3's `client.UDPWithDialer`, binding `--udp-local-port` and `--source-address`.
- `--sa-reply-timeout` defaults to 2000 ms and `--sa-statistics-threshold` to the thresholds of Table 7-6, go-dnp3's new defaults; `--sa-statistics-threshold` above 65535 is refused.
- The model's `refused_functions` takes `activate-config`.
- The simulated outstation gives an uploaded file the time of creation the master sends.
- `gopkg.in/yaml.v3` is replaced by its maintained successor `go.yaml.in/yaml/v3` v3.0.6; spf13/pflag v1.1.0.

### Added

- **Client, every command:** `--source-address` (connect from this local IP address over TCP, TLS and UDP; `client.WithDialer`, `client.UDPWithDialer`), `--retry-jitter` (randomized retry backoff, `Retry.Jitter`), `--max-fragment` and `--max-request-fragment` (`WithMaxFragment`, `WithMaxRequestFragment`), `--no-solicited-confirm` (`WithSolicitedConfirm`), `--sa-challenge-macs` (`WithChallengeMACs`), `--sa-key-change-count` (`WithKeyChangeCount`).
- **`client monitor`:** `--time-sync auto|lan|non-lan|clear-need-time|none` (`WithTimeSync`), `--reconnect-jitter` (`Reconnect.Jitter`), `--auto-event-poll` (`WithAutoEventPoll`), `--queue-limit` (`WithQueueLimit`), `--no-unsolicited-confirm` (`WithUnsolicitedConfirm`), `--dual-endpoint` and `--dual-endpoint-allow` (`WithDualEndpoint`); with secure authentication the master's own security statistics (`Client.SecurityStatistics`) go to stderr at the end.
- **`client manage --function activate-config`** with `--config-name` (g70v8) and `--config-string` (g110): ACTIVATE_CONFIG (`Client.ActivateConfig`); the result lists the outstation's status of requested operation.
- **`client time-sync --method clear-need-time`** (`Client.ClearNeedTime`).
- **`client file put --created`** (`FileOptions.Created`).
- **`server start`:** `--redial-min-delay`, `--redial-max-delay` and `--redial-jitter` (the backoff of `--dial`, `server.Redial`), `--dial-source-address` (`server.WithDialer`), `--master-ips` (`WithMasterIPs`), `--udp-fixed` (`ServeUDPFixed`), `--max-controls` (`WithMaxControls`), `--max-tx-fragment` and `--max-rx-fragment` (`WithMaxFragment`), `--deadband-integer-only` (`WithDeadbandIntegerOnly`), `--file-timeout` and `--file-timeout-class` (`WithFileTimeout`), `--sa-threshold`, `--sa-no-aggressive`, `--sa-expected-key-change-interval`, `--sa-expected-key-change-count` and `--sa-max-key-status-requests` (the new `SecureAuth` fields).
- **JSON output:** `activation` in the result of `client manage --function activate-config` (`delay_ms`, `results` with `code` and `text`); `method` of `client time-sync` may be `"clear-need-time"`, which has no `time`.
- **Key and password files:** on Unix, a warning on stderr when a key file (`--sa-key-file`, `--sa-update-key-file`) or `--file-password-file` can be accessed by its group or others. The file is used all the same.
- README: which go-dnp3 features are not exposed and why (dialers other than a source address bind, `ServeByMasterIP`, a key change interval of 0).

### Upgrade notes

- A script that waited for `unsolicited responses enabled again` on the stderr of `client monitor` should look for `the outstation reported a restart`.
- A master that relied on the simulated outstation replacing a connection of the same master address with a new one now has the new connection refused while the first one answers: give the second client another `--master`.
- Secure Authentication aggressive mode between this release and an older dnp3ctl or go-dnp3 fails: upgrade both ends.

### Quality

- Fuzz targets `FuzzParseObjects`, `FuzzParseClasses`, `FuzzParseObjectFilter`, `FuzzParseTime`, `FuzzReadKey`, `FuzzParseIndexListAndValues`, `FuzzParseBroadcast`, `FuzzParsePortsAndAddresses`, `FuzzParseAttributeVariation`, `FuzzParsePrefixes`, `FuzzParseChallengeMACs`, `FuzzParseAllow`, `FuzzParseThresholds` and `FuzzLoadFromEnv` (`config.LoadFromEnv` now reads through a lookup function, so the environment can be fuzzed); each ran 30 s without a finding.
- Chaos tests: an outstation that drips its responses one octet at a time (within and beyond the response timeout), and one that stops in the middle of a response without closing the connection.
- End-to-end tests of the built binary against an outstation it serves itself (`server start`): read, operate, file get, and the indications a one-shot command leaves alone.
- Command tests of every new flag against the simulated outstation, including the time synchronization modes of the monitor and the one-shot command that sends nothing but its request.
- CI: otfabric/.github reusable workflows v3.6.0, golangci-lint v2.15.0 as `make check` runs it, engineering policy 1.5; a CodeQL workflow (Go) on pushes and pull requests to `main` and weekly.

---

## v0.11.0

**Date:** 2026-11-08
**Previous release:** v0.10.1

### Changed

- Built on go-dnp3 v1.9.1, whose decoders, master and outstation were checked against 182 recorded DNP3 captures (ITI, Zeek, igbe; software-generated or of unknown origin, no hardware claim).
- Outstation (`station`): a READ of object group 0 variation 0 is answered with IIN2.1 (object unknown) alone; an outstation without attribute sets used to answer IIN2.2 as well.

---

## v0.10.1

**Date:** 2026-11-08
**Previous release:** v0.10.0

### Changed

- Release binaries are built with the latest Go 1.26 patch release; v0.10.0 was built with go1.26.0.

---

## v0.10.0

**Date:** 2026-11-07
**Previous release:** v0.9.0

### Summary

`version` prints one line, the same in every OT Fabric CLI, and `version --json` one JSON object; shell completion now covers every flag and argument.

### Changed

- **`version` output:** one line, `dnp3ctl 0.10.0 (tag v0.10.0, commit <full sha>, built <date>, go-dnp3 1.9.0, go-serial 0.2.1, go1.26.x, linux/amd64)`; a build without a tag leaves out the tag part, and a build without injected metadata (`go build`, `go install`) reports the module version of a release or `dev`, and the VCS revision (with `-dirty`) the toolchain recorded. Was a multi-line `dnp3ctl version: …` block.
- **`version --json`** (new): one JSON line with the keys `name`, `version`, `tag`, `commit`, `built`, `libraries` (`name`, `version` of each OT Fabric library), `go`, `platform`, in that order and always present (docs/json-output.md). `DNP3CTL_FORMAT=json` prints the same.
- **Shell completion** (bash, zsh, fish, PowerShell) of every flag that takes a value: new value completion for `server start --sa-mac`, and `--data-bits` and `--stop-bits` of the client and server commands; files of their type for the file flags, and no file names for addresses, numbers and other free values. The arguments of commands that take none complete no file names either.
- **Makefile:** injects the version and tag only when HEAD is exactly on a tag (any other build reports `dev`), the version without the leading `v`, and the full commit SHA, as the release workflow does.

### Upgrade notes

- A script that parsed the old multi-line `version` output should read `version --json` instead.

### Tests

- Version line with and without a tag, development fallback, JSON key order and values, `--json` and `DNP3CTL_FORMAT`.
- `__complete` for the four shells and for flags of every command group; a walk of the command tree fails for a flag that takes a value without completion, and for one whose usage lists values that completes none.

---

## v0.9.0

**Date:** 2026-11-04
**Previous release:** v0.8.0

### Summary

Hardening from a low/low static analysis scan. The simulated outstation no longer takes uploads past 4 GiB, the rwx form of `--permissions` no longer accepts some non-ASCII characters, and new output files are created owner-only (0600). New chaos, end-to-end and fuzz tests. Exit codes and JSON output are unchanged.

### Changed

- Built on go-dnp3 v1.9.0. File transfer with fragments over 65547 octets now works, `FSFileServer` refuses files of 4 GiB or more, and a few out-of-range inputs that the library sent wrapped are refused.
- `client file get` and `server generate-config` create their output file readable by its owner only (mode 0600, was 0644 before the umask). A downloaded file or an outstation model may hold configuration that is not for every user of the host. An existing file overwritten with `--force` keeps its mode.

### Fixed

- **Uploads past 4 GiB.** The simulated outstation took a file a master wrote, or appended to, of any length, and announced its size modulo 2^32. It now answers a write past 4 294 967 295 octets with FILE_FATAL and keeps nothing of that file.
- **`--permissions` in the rwx form** compared the low octet of each character, so `Ųxr--r--` (U+0172 ends in 0x72, `r`) was accepted as `rwxr--r--`. Characters other than `r`, `w`, `x` and `-` are refused.
- **Security statistics with an index over 65535** (g121, g122) were named after the index modulo 2^16; they are now shown as `statistic <index>`.
- `client file put` checks the size of the data it read, not only of the file it looked at before: a file that grew past 4 GiB in between, or a pipe, is refused with `invalid_input` instead of its size being sent modulo 2^32.
- A model's writable attribute variation over 255 is refused without first being looked up modulo 256.

### Quality

- Low/low static analysis (gosec, opengrep): every narrowing integer conversion is range-checked where the scanner can see it. The findings that remain (files the operator names, `--tls-insecure`, math/rand in the random simulation mode) are reviewed and recorded.
- Chaos tests: `client file get` and `client poll` through a connection cut mid-transfer, against a peer that sends noise and one that hangs up. Each ends within its timeout with the documented exit code and error kind, and leaves no partial output file.
- An end-to-end test of an upload past the outstation's limit over DNP3, and unit tests of the size limit, the rwx parser, statistic names, the last octet of host ranges and the mode of new output files.
- Fuzz targets `FuzzParsePermissions` and `FuzzExpandHosts`.
- CI uses the shared workflows v3.3.1.

---

## v0.8.0

**Date:** 2026-11-03
**Previous release:** v0.7.7

### Changed

- Built on go-dnp3 v1.8.0, which requires Go 1.26 and has been modernised with `go fix`. The wire behaviour is unchanged.
- Go 1.26 is the minimum Go version for building dnp3ctl (was 1.25). CI tests Go 1.26 and 1.27.
- The code base has been modernised with `go fix ./...`: `sync.WaitGroup.Go`, `errors.AsType`, `new(expr)`, `strings.SplitSeq`, `slices`, `maps` and range-over-int loops. The commands, flags and output are unchanged.

---

## v0.7.7

**Date:** 2026-11-03
**Previous release:** v0.7.6

### Changed

- Built on go-dnp3 v1.7.0. The outstation answers an event READ with a count of 0 with no events, and reports the first change of a point that ASSIGN_CLASS moved out of class 0. The master accepts an AUTHENTICATE_FILE answer with an empty user name and password at offset 0, which cB-Sigma dnp3 sends. `dnp3ctl` code is unchanged.
- README: go-dnp3 runs dnp3-interop v0.11.0, with cB-Sigma dnp3 and nfm-dnp3 added to its reference stacks.

---

## v0.7.6

**Date:** 2026-11-02
**Previous release:** v0.7.5

### Changed

- Built on go-dnp3 v1.6.0. The library adds ACTIVATE_CONFIG with its status answer (g91v1) at both ends, decodes g91v1 under a count qualifier as DSC go-dnp3 sends it, and adds two client options that withhold application confirmations for tests. `dnp3ctl` code is unchanged.

---

## v0.7.5

**Date:** 2026-11-02
**Previous release:** v0.7.4

### Changed

- Removed interop test functionality against this stack, as requested by the maintainer of this stack: the Step Function I/O dnp3 stack. README, SECURITY and these notes no longer list it; go-dnp3's interop suite now runs against opendnp3, DSC go-dnp3 and dnp3py (dnp3-interop v0.9.0).
- Built on go-dnp3 v1.5.4, which has no change in the library. `dnp3ctl` code is unchanged.

---

## v0.7.4

**Date:** 2026-11-01
**Previous release:** v0.7.3

### Fixed

- Through go-dnp3: one reconnect at a time with secure authentication, and a data race in the request queue. `dnp3ctl` code is unchanged.
- Built on go-dnp3 v1.5.3, go-serial v0.2.1.

---

## v0.7.3

**Date:** 2026-11-01
**Previous release:** v0.7.2

### Changed

- Dependency update, no other change.
- Built on go-dnp3 v1.5.2, go-serial v0.2.1.

---

## v0.7.2

**Date:** 2026-10-31
**Previous release:** v0.7.1

### Changed

- Policy 1.2: build output in `bin/` and `release/`, a per-package coverage floor, and more tests and CI. No change in behaviour.
- Built on go-dnp3 v1.5.1, go-serial v0.2.1.

---

## v0.7.1

**Date:** 2026-10-30
**Previous release:** v0.7.0

### Dependencies

- `go-serial` → v0.2.0: on macOS and BSD the baud constant no longer ends up in `Cflag`, where it set parity and stop-bit flags on the port.

---

## v0.7.0

**Date:** 2026-10-30
**Previous release:** v0.6.8

### Summary

**Breaking for scripts:** the exit codes follow the OT Fabric organisation table (ADR-0006 of otfabric/engineering). Nothing else changes in the commands.

### Exit codes

| Case | Old | New | JSON error |
|---|---|---|---|
| `context_canceled` (interrupted before a result) | 10 | 130 | kind `interrupted`, was `internal` |

Every other exit code is unchanged. See [docs/exitcodes.md](docs/exitcodes.md).

### Environment

`DNP3CTL_FORMAT` is now the canonical name of the format variable; `DNP3CTL_OUTPUT_FORMAT` stays accepted, and `DNP3CTL_FORMAT` wins when both are set.

---

## v0.6.8

**Date:** 2026-10-30
**Previous release:** v0.6.7

### Summary

Built on go-dnp3 v1.5.0, which hardens both ends against hostile or unlucky peers. No command or flag changes.

### Changed (from go-dnp3 v1.5.0)

- **`server start`** (the simulated outstation): a READ or ASSIGN CLASS that names every point many times over, or a range of 2^32 indexes, is answered with IIN2.2 instead of holding the outstation for seconds or minutes. A TLS connection must finish its handshake within 10 s. The listener keeps accepting after running out of file descriptors, a UDP outstation survives ICMP errors, and a panic loses one request, not the process. A WRITE of a NaN or negative deadband is answered with IIN2.2.
- **`server start`**: frozen analog inputs read without flag (g31v5, g31v6) keep their flags when they are not ONLINE alone, and analog values that a variation without flag cannot carry are reported with OVER_RANGE. Binary and double-bit events recorded while the time was unsynchronized are reported with relative time under an unsynchronized CTO.
- **`client`**: connecting synchronizes the time before the startup integrity poll when the outstation asks for it, and a startup step the outstation rejects (IIN2.0 to DISABLE UNSOLICITED from an outstation without unsolicited responses, for one) is logged instead of failing the connection. TCP connects and TLS handshakes time out after 10 s, and `udp://` connections survive an ICMP port unreachable.

### Docs

- FAQ: go-dnp3 is tested against dnp3-interop v0.6.0 (with DSC go-dnp3 v0.8.2).

---

## v0.6.7

**Date:** 2026-10-29
**Previous release:** v0.6.6

### Summary

Dependency update for serial (`serial://`) connections: go-serial v0.1.8 fixes two POSIX serial I/O bugs. No command or flag changes.

### Dependencies

- go-serial v0.1.6 → v0.1.8.

### Fixes (from go-serial v0.1.8)

- `Write` no longer loses data when the serial port's output buffer is
  full. It used to return a short count with a nil error (or `n == -1`
  with `EAGAIN`); it now blocks until every byte is written.
- `Read` works when the port's file descriptor is 1024 or higher. It used
  to return a spurious `io.EOF` at once.

---

## v0.6.6

**Date:** 2026-10-21  
**Previous release:** v0.6.5

### Summary

Built on go-dnp3 v1.4.1. The simulated outstation (`server start`) no longer crashes on a malformed request, and it answers as the DNP Users Group's IED certification procedures require; go-dnp3 now runs those in CI.

### Security

- **`server start` no longer crashes on a malformed READ:** go-dnp3 v1.3.2 and earlier took the whole outstation process down on a READ of g70v5 (file transport) whose header carries indices but no objects. Any master that can reach a `server start` could send it. Fixed in go-dnp3 v1.4.0: such a request is answered with IIN2.2.

### Changed

The simulated outstation follows go-dnp3 v1.4.x:

- An OPERATE after the select timeout is answered TIMEOUT, not NO_SELECT. A control it does not support (`NOT_SUPPORTED`) also sets IIN2.2. A control whose request carries a status is refused with FORMAT_ERROR.
- A read of a point type the model has no points of is answered with IIN2.1.
- Relative-time events carry an unsynchronized CTO (g51v2) until the time is synchronized.
- A variation without flags reports a point with abnormal flags in the variation with flags.
- The response that reports a broadcast to `confirm` or `optional` asks for confirmation.
- A READ sent while an unsolicited response waits for its confirmation is answered after it.

`client` against an outstation that answers `NOT_SUPPORTED` with IIN2.2 (go-dnp3 v1.4, opendnp3) reports the echoed statuses, as for any refused control: exit code 6, or 7 when other controls of a direct operate were executed. With go-dnp3 v1.4.0 it would have reported only the IIN error.

### Docs

- FAQ: go-dnp3 is tested against dnp3-interop v0.5.1 and with the IED certification procedures (dnp3c).
- SECURITY: use 0.6.6 or later.

### Upgrade notes

- No breaking changes in the command line or the JSON output.

---

## v0.6.5

**Date:** 2026-10-21  
**Previous release:** v0.6.4

### Summary

A misspelt subcommand of `client` or `server` gets the "Did you mean this?" suggestion with flags and without, for a typo as well as an abbreviation.

### Fixed

- **Suggestions for subcommands:** `dnp3ctl client pol` said only `unknown command "pol" for "dnp3ctl client"`; it now adds "Did you mean this? poll", the same message as with flags. Both paths also suggest by edit distance, not only by prefix: `dnp3ctl server stat` suggests `start` (Cobra sets that distance only for the root's own commands). One function builds the message for both paths.

### Upgrade notes

- No breaking changes.

---

## v0.6.4

**Date:** 2026-10-21  
**Previous release:** v0.6.3

### Summary

An unknown subcommand of `client` or `server` given together with a flag is reported as the unknown command it is, with suggestions, instead of as an unknown flag.

### Fixed

- **Unknown subcommand with a flag:** `dnp3ctl client nope --format json` (or any flag after an unknown subcommand) reported `unknown flag: --format`: Cobra stops at the `client` group, which has none of its subcommands' flags. It now reports `unknown command "nope" for "dnp3ctl client"`, with "Did you mean this?" suggestions for a misspelt one (`client pol` → `poll`), as text or as the JSON error object. Without flags the message was already right. Exit code 2 as before.

### Upgrade notes

- No breaking changes.

---

## v0.6.3

**Date:** 2026-10-21  
**Previous release:** v0.6.2

### Summary

Four behaviours of the command line and of the simulated outstation follow what the documentation promises: usage errors honour `--format json`, a binary built with `go install` reports its version, a connection style on the command line wins over another one in the environment, and over UDP the event log says `connected` once a master is known. Commands, flags and the JSON shapes are unchanged.

### Changed

- **Usage errors and `--format json`:** an unknown command or flag, a missing required flag or a value a flag cannot hold is now the JSON error object `{"error": {"kind": "usage", "code": "invalid_input", "message": ...}}` on stdout when the invocation asks for JSON, by `--format json` (also `--format=json`, wherever it stands on the command line) or by `DNP3CTL_OUTPUT_FORMAT=json` for a command that has `--format`. It was always the text `error:` line on stderr. The exit code stays 2, and text stays as it was. `server generate-config --format` is the format of the model file it writes, not of a result, so its errors stay text.
- **Connection styles, environment and command line:** a flag of one connection style on the command line now wins over the environment variables of the others, as for every other flag: `client … --ip` works with `DNP3CTL_URL` set (likewise `--url`, `--listen` with `DNP3CTL_IP`, `DNP3CTL_MASTER_LISTEN`), and `server start --dial` with `DNP3CTL_PORT` or `DNP3CTL_UDP` set (likewise `--serial`, and `--listen`, `--port`, `--udp`, `--udp-peer`, `--allow` with `DNP3CTL_DIAL` or `DNP3CTL_SERIAL`). Two styles both on the command line, or both in the environment, are still refused as mutually exclusive.
- **go-dnp3 v1.3.2**, which runs its interop suite against otfabric/dnp3-interop v0.4.3 (the FAQ says so); no change of the library.
- Docs: README.md, docs/json-output.md, docs/exitcodes.md and docs/server-config.md describe the new behaviour, and say why `server generate-config --format` takes `yaml` or `json`.

### Fixed

- **Version of `go install` binaries:** a binary built without the ldflags of the release (`go install …@v0.6.3`, `go build`) reported version `dev`. It now reports the version the go toolchain records, such as `v0.6.3` (with `go build` in a clone the one it derives from the git tags, such as `v0.6.3+dirty`), and the git commit it records, when there is one; `dev` remains when nothing is recorded, as in a copy without git.
- **UDP `connected`:** over UDP (`server start --udp`, with one model or several) the event log said `connected` with an empty `remote` as soon as the socket was served, before any master had sent anything. It now says `connected` when the first datagram arrives, naming its sender as `remote` (at once, naming the master, with `--udp-peer`); a socket no master sent to logs neither `connected` nor `disconnected`.
- **`server start --dial` with `--duration`:** the goroutine that dials the master read the context that `--duration` replaces, a data race the race detector reports; it now gets its context before it starts.

### Upgrade notes

- No breaking changes. A script that read usage errors from stderr while passing `--format json` now finds them on stdout, as every other error.

---

## v0.6.2

**Date:** 2026-10-20  
**Previous release:** v0.6.1

### Summary

dnp3ctl moves to [go-dnp3](https://github.com/otfabric/go-dnp3) v1.3.1, a documentation-only release, and its documentation has been reviewed claim by claim against the code before the repository goes public. Two behaviours of the simulated outstation are fixed: simulated counters follow `ramp`, `sine` and `random`, and with several outstations over UDP the event log names the master. Commands, flags and JSON output are unchanged.

### Changed

- go-dnp3 v1.3.1 (documentation only, no API change).
- Docs, reviewed for going public:
  - **README.md:** the install commands unpack into an emptied folder and install the binary by its exact name, so that an update cannot install an older binary left behind, and stop on an unsupported architecture (also on macOS); building from source names the tools `make build` needs and the plain `go build`; the required flags of every command, the connection styles staying exclusive across environment and flags, the defaults per command in the environment table, the command aliases and the undocumented flags (`client objects --group`/`--events`, the file commands' `--permissions`, `--block-size`, `--file-password-file`, `file get --force`/`--output`, `server generate-config --force`/`--format`, `server start --format`); the BCD and unsigned integer read example (`--object g101v1,g102v1`), a FREEZE_AT_TIME example in the future, the commands that change an outstation, the comparison table worded as typical per category, the interop stacks (with dnp3py) and the project structure.
  - **docs/server-config.md:** the main example model validates (it lacked binary output 0, and its `events.csv` must exist); what reads of unsupported objects and of point types without points answer; which attribute writes are refused; the Secure Authentication statistics and reply timeout flags; `--max-sessions`; double-bit values in YAML need no quotes.
  - **docs/json-output.md, docs/exitcodes.md:** usage errors (exit code 2) are always a text `error:` line on stderr, also with `--format json`, with code `invalid_input`; the IIN order (`NEED_TIME|DEVICE_RESTART`); `CHATTER_FILTER` for double-bit inputs and `BIT5`-`BIT7` for unnamed flags; `controls` among the arrays.
  - **SECURITY.md:** supported versions (0.6.x); the security statistic flags; DSC Systems' go-dnp3 linked. **CONTRIBUTING.md:** the release notes rule and the full list of client commands. **RELEASE.md:** the linux/armv7 compile error of v0.5.0 never shipped, and is listed under Quality.

### Fixed

- **Simulated counters:** a `counter` or `frozen-counter` with `simulate` mode `ramp`, `sine` or `random` counted up by `step` like `count`; it now follows its mode, rounded to a whole number the counter can hold (0 … 4 294 967 295).
- **Several outstations over UDP:** with more than one `--config` and `--udp`, the event log named the server's own socket as `remote` of every event; it now names the master the last datagram came from, as with one outstation.

### Upgrade notes

- No breaking changes. A model that simulates a counter with `ramp`, `sine` or `random` now gets values between `min` and `max` instead of a count.

---

## v0.6.1

**Date:** 2026-10-20  
**Previous release:** v0.6.0

### Summary

dnp3ctl moves to [go-dnp3](https://github.com/otfabric/go-dnp3) v1.3.0, which generates its object table and the objects it reports as measurements from one catalog. Commands, flags and output are unchanged, except for the size `client objects` gives g86v2.

### Changed

- **`client objects`:** g86v2 (Data Set Descriptor - Characteristics) has size 1, the one octet IEEE 1815-2012 defines; it was listed as free-format (size 0). dnp3ctl does not read or write data sets, so nothing else changes.
- go-dnp3 v1.3.0.

### Upgrade notes

- No breaking changes.

---

## v0.6.0

**Date:** 2026-10-20  
**Previous release:** v0.5.0

### Summary

dnp3ctl moves to [go-dnp3](https://github.com/otfabric/go-dnp3) v1.2.0: `server start` serves several outstations behind one port, serial line or UDP socket, and the client commands report BCD (g101) and unsigned integer (g102) points. Nothing that worked before changes.

### Highlights

- **Several outstations:** `server start --config a.yaml --config b.yaml` (or comma-separated files, or `DNP3CTL_CONFIG`) serves every model as an outstation of its own, behind the same port, serial line or UDP socket, as on a multi-drop line. Each answers at the link address of its model with its own points, event buffers and controls; a broadcast reaches all of them. Two models with the same link address are refused; `--dial` and `--udp-peer` take a single model.
- **The event log names the outstation** when several are served: `[11]` after the event in text, `outstation` in JSON, an OUTSTATION column in the table. `listening` lists every outstation with its link address.
- **BCD and unsigned integers:** `client poll`, `read` and `monitor` report g101 (BCD, `type` `bcd`, the decimal number) and g102 (`type` `unsigned-integer`), which go-dnp3 now decodes.

### Changed

- dnp3ctl reads times and intervals (g50v4) through go-dnp3's own measurement list, which now carries them, instead of its own decoding; the output is the same.

### Fixed

- go-dnp3 v1.2.0 fixes a panic of the outstation when a response fragment had almost no room left, which could stop the simulated outstation while it answered a large poll.

### Upgrade notes

- No breaking changes to commands, flags or JSON output. Server events gain `outstation` when several outstations are served, and the table output of `server start` gains an OUTSTATION column (empty with one outstation).

---

## v0.5.0

**Date:** 2026-10-20  
**Previous release:** v0.4.0

### Summary

dnp3ctl exposes what [go-dnp3](https://github.com/otfabric/go-dnp3) v1.1.0 added for dnp3-interop's contract 1.3 (broadcast, scheduled freezes, device attribute sets, file append, deadbands) and the rest of go-dnp3's API that a command line can use: several controls in one request, the management functions, clearing the restart indication, outstation indications, go-dnp3's log under --debug and the Secure Authentication options of the outstation. New flags and commands; nothing that worked before changes.

### Highlights

- **`--broadcast no-confirm|confirm|optional`** on `client operate` (with `--mode direct` or `direct-no-ack`) and `client freeze`: the request goes to every outstation on the channel at a broadcast address. Nobody answers; each outstation sets IIN1.0 BROADCAST in its next response. Refused with secure authentication: outstations running SAv5 accept a critical broadcast only with an aggressive mode MAC over an earlier challenge of the same connection, which a dnp3ctl command cannot have.
- **`client freeze --at TIME [--interval DURATION]`:** FREEZE_AT_TIME, a freeze at a time to come, repeated at the interval.
- **Device attribute sets:** `client attributes --set N` reads a set other than the standard set 0.
- **`client write-attribute`** (new): writes a device attribute of any set, with `--value` of `--data-type vstr|uint|int|float`.
- **`client file put --append`:** adds to the end of a file (OPEN_FILE in APPEND mode).
- **`client read --object g34`:** the deadbands of the analog inputs, which outstations now answer; **`client write-deadband`** (new) writes them.
- **Several controls in one request:** `client operate --index 0,1,4`, with `--value` once or per index for set-points. The result lists a status per control (`controls` in JSON); a direct operate refused for some controls reports the others as executed, with exit code 7.
- **`client manage`** (new): INITIALIZE_DATA, INITIALIZE_APPL, START_APPL, STOP_APPL and SAVE_CONFIG.
- **`client clear-restart`** (new): clears DEVICE_RESTART (IIN1.7), which the one-shot commands leave set.
- **`--debug`** also shows go-dnp3's own log entries (connections, reconnects, warnings, authentication), next to the frame trace.
- **`server start --sa-reply-timeout`, `--sa-statistics-class`, `--sa-statistics-threshold`:** the challenge reply timeout and the security statistic events of the outstation.
- **Simulated outstation:** `attribute_sets` in the model, with writable variations; set 0 accepts writes of the user-assigned attributes the model gives (owner, location, user-id, device-name) and refuses the others; it answers broadcasts, scheduled freezes, reads and writes of deadbands and the management functions (`refused_functions` in the model refuses some), and reports the `indications` of the model (local-control, device-trouble, config-corrupt).

### Changed

- The simulated outstation refuses writes of device attributes of set 0 other than the user-assigned ones the model gives; it accepted every write before.
- Docs name the DSC Systems library "DSC go-dnp3", as dnp3-interop does, and the interop note refers to dnp3-interop v0.4.1 (contract 1.3).

### Upgrade notes

- No breaking changes to commands, flags or JSON output; the JSON gains `broadcast`, `interval`, `set`, `attribute`, `append`, `function` and `value` fields where they apply, and `client operate` gains `controls` (and `error` for a partial result).
- `--index` of `client operate` is now a list; a single index works as before.

### Quality

- Every changed line is covered by tests, end to end against the simulated outstation: broadcast controls and freezes, scheduled freezes (also the refusal of a time that has passed), attribute sets read and written (writable and not), set-0 writes, append, deadbands read and written, several controls (all succeeded, partly refused, refused at select), every management function and a refused one, indications and clearing the restart indication, the debug log of both roles.
- `make check` now typechecks for 32-bit targets (`make cross`).

---

## v0.4.0

**Date:** 2026-10-20  
**Previous release:** v0.3.2

### Summary

dnp3ctl exposes what [go-dnp3](https://github.com/otfabric/go-dnp3) v1.0.0 added: the dual end point, where the outstation connects to its master, in both roles; Secure Authentication aggressive mode and security statistics; and command events and times and intervals in the simulated outstation. New flags only: nothing that worked before changes.

### Highlights

- **Dual end point** (IEEE 1815-2012 annex A.3), for outstations behind a firewall or on a dial-out link:
  - **`client … --listen host[:port]`** (`DNP3CTL_MASTER_LISTEN`) makes every client command wait for the outstation to connect instead of connecting to it, within `--timeout`; `client monitor` waits for it again after a lost connection. With `--tls-cert` and `--tls-key` dnp3ctl is the TLS server of that connection, and `--tls-ca` makes the outstation present a certificate of that CA. The result's `target` is the address the outstation connected from. Mutually exclusive with `--url` and `--ip`.
  - **`server start --dial tcp://host[:port]`** or **`tls://host[:port]`** (`DNP3CTL_DIAL`) makes the simulated outstation connect to its master, and again whenever the connection ends (after 1 s, doubling up to 1 min). Over TLS it is the TLS client: the new **`--tls-ca`** (`DNP3CTL_TLS_CA`) verifies the master, `--tls-cert` and `--tls-key` are presented to it. The event log starts with a `dialing` event.
- **Secure Authentication aggressive mode**: **`--sa-aggressive`** (`DNP3CTL_SA_AGGRESSIVE`) sends critical requests with their MAC once the outstation has challenged one, saving a round trip; in `client operate --mode sbo` the OPERATE goes in aggressive mode after the challenged SELECT. It needs a key. The simulated outstation accepts it.
- **Security statistics**: the simulated outstation keeps the 18 statistics of IEEE 1815-2012 clause 7.6 per master when secure authentication is on, and reports g122 events in class 1. **`client read --object g121v1`** shows them with their name (`statistic` in JSON: `"session key changes"`, `"authentication failures"`, …).
- **Command events** in the outstation model: **`command_event_class`** (and `command_event_variation`) on a `binary-output` or `analog-output` reports every executed control as a g13 or g43 event with the commanded value and the status. The demo outstation has them on binary output 0 and analog output 0.
- **Times and intervals** (g50v4): the point type **`time-and-interval`** with an RFC 3339 `value`, `interval` and `interval_unit`, read with `client read --object g50v4`, which shows the time and the interval (`interval` and `interval_unit` in JSON, type `time-and-interval`). Like in the reference outstations it is not in class 0. The demo outstation has one.

### Changed

- `client` results: a measurement of a time and interval has the new fields `interval` and `interval_unit`, a security statistic the new field `statistic`; text and table output show them after the value. g50v4 objects used to be left out of results.
- The client's `--tls-ca`, `--tls-cert` and `--tls-key` also apply to `--listen`, and `server start` has a `--tls-ca` for `--dial`; `--tls-ca` without a `tls://` `--dial` is refused.
- An outstation that does not connect to `--listen` in time, and a `--listen` address that cannot be opened, exit with code 4 (`transport_connect_failed`, `listen_failed`).
- docs: [SECURITY.md](SECURITY.md) on aggressive mode, the security statistics and the dual end point; the interoperability note now names DSC Systems' go-dnp3, against which go-dnp3 verifies Secure Authentication.

### Upgrade notes

- No breaking changes. Scripts that read `--format json` see three new optional fields (`interval`, `interval_unit`, `statistic`) and the `dialing` event.
- `DNP3CTL_MASTER_LISTEN` is the client's variable for `--listen`; `server start --listen` keeps `DNP3CTL_LISTEN`.

---

## v0.3.2

**Date:** 2026-10-20  
**Previous release:** v0.3.1

### Summary

dnp3ctl now builds on [go-dnp3](https://github.com/otfabric/go-dnp3) v1.0.0, its first production release, whose API follows semantic versioning from here on. No change to commands, flags or output.

### Changed

- **go-dnp3 v1.0.0.** Of its behaviour changes, none affects dnp3ctl: a client whose startup sequence fails is now closed by go-dnp3 as well, which dnp3ctl already did; the new point types (security statistics, command events, time and interval) are not served by the simulated outstation's model; aggressive mode is not used.

### Upgrade notes

- No breaking changes.

---

## v0.3.1

**Date:** 2026-10-20  
**Previous release:** v0.3.0

### Summary

Secure Authentication interoperability fixes from [go-dnp3](https://github.com/otfabric/go-dnp3) v0.2.1, found by testing against DSC Systems' go-dnp3. No change to commands, flags or output.

### Fixed

- **Challenge replies:** the master's reply to a challenge keeps the sequence number of the critical request it answers, as outstations expect.
- **Session key change:** the key change data carries the key status without its MAC, so a second key change within one connection (`--sa-key-change-interval`) is accepted.
- **Simulated outstation:** sends 32-octet challenges.

### Upgrade notes

- No breaking changes. Recommended for everyone who uses Secure Authentication with an update key (`--sa-update-key-file`, `DNP3CTL_SA_UPDATE_KEY`).

---

## v0.3.0

**Date:** 2026-10-20  
**Previous release:** v0.2.0

### Summary

Secure Authentication with session key change, and a UDP outstation that can reach its master before the master speaks. Built on [go-dnp3](https://github.com/otfabric/go-dnp3) v0.2.0.

### Highlights

- **Secure Authentication session key change** (IEEE 1815-2012 clause 7.5.2.3): give dnp3ctl the outstation's **update key** with **`--sa-update-key-file`** or **`DNP3CTL_SA_UPDATE_KEY`** (16 octets for AES-128 key wrap, 32 for AES-256; hex; never accepted as a flag). On every connection the master negotiates fresh session keys wrapped with it, and again every **`--sa-key-change-interval`** seconds (default 900). `server start` accepts the same update key and verifies the key change. The pre-shared session key (`--sa-key-file`, `DNP3CTL_SA_KEY`) still works for outstations without key change; give one kind of key or the other.
- **`server start --udp-peer host:port`** (`DNP3CTL_UDP_PEER`): the master's address, so that unsolicited responses reach it before it has sent anything. The event log names it from the start.

### Fixed

- **The UDP outstation stopped before any master spoke** when the model configured its masters: the first unsolicited write had no peer and ended the server (go-dnp3 v0.2.0).

### Upgrade notes

- No breaking changes. Existing pre-shared key configurations behave as before.

### Quality

- Whole-module statement coverage of **94.8%**; every changed line is covered. New end-to-end tests: SAv5 key change against the simulated outstation and through `server start` (right and wrong update key, conflicting key sources, invalid key lengths), and `--udp-peer` delivering the null unsolicited response before the master sends anything.

---

## v0.2.0

**Date:** 2026-10-20  
**Previous release:** v0.1.0

### Summary

UDP improvements found while testing against other DNP3 stacks, and corrected security documentation. Built on [go-dnp3](https://github.com/otfabric/go-dnp3) v0.1.1.

### Highlights

- **`--udp-local-port`** (`DNP3CTL_UDP_LOCAL_PORT`): with `udp://` URLs, send from a fixed local port, for outstations that answer only a configured master endpoint (opendnp3, for example).
- **`server start --udp`:** the event log now names the master the outstation answers as `remote`; it showed the outstation's own socket.
- **`server start`:** an error the transport reports while the server stops is logged as an `error` event instead of being dropped.

### Documentation

- **SECURITY.md:** the security contact is security@otfabric.io (it said otfabric.com). It now also explains how Secure Authentication has been verified.
- **docs/server-config.md, README.md:** how the simulated outstation behaves over UDP: it answers whoever sent the last datagram, serves one master at a time, and sends unsolicited responses only after a master's first request.

### Quality

- Whole-module statement coverage of **94.6%**; the changes of this release are covered completely.
- Every `nolint` comment is gone: the code passes golangci-lint and staticcheck as configured.
- Fixed two races in the tests that made the coverage job fail on main.

---

## v0.1.0

**Date:** 2026-10-20  
**Previous release:** none

### Summary

First release. dnp3ctl is a command-line tool for **DNP3 (IEEE 1815)** in both roles: a **master** that polls, reads, monitors and operates an outstation, synchronizes its time, restarts it, freezes counters, reads its attributes and transfers files, and a **simulated outstation** served from a model file. It also finds outstations on a network. Built on [go-dnp3](https://github.com/otfabric/go-dnp3) v0.1.1.

### Highlights

- **`client poll`:** Integrity poll (classes 1, 2, 3, then 0), event poll of chosen classes, or a poll of exactly the classes given (**`--mode integrity|event|class`**, **`--class`**). Every value with its object, flags and, for events, time stamp, and the IIN of the response; **`--object`** and **`--index`** filter what is shown.
- **`client read`:** READ of any object and variation, any variation (`g30`), a range, an index list or a count (**`--object`**, **`--range`**, **`--count`**). An answer with values and an IIN error is listed with the error (exit code **7**).
- **`client monitor`:** Stays connected, enables unsolicited responses (**`--unsolicited`**), optionally polls integrity (**`--integrity`**) and events (**`--poll-interval`**), and prints every value as it arrives; enables unsolicited responses again after an outstation restart; **`--object`**, **`--index`**, **`--count`**, **`--duration`**; reconnects unless **`--no-reconnect`**.
- **`client operate`:** Control relay output blocks (**`--op latch-on|latch-off|pulse-on|pulse-off|close|trip|nul`**, **`--count`**, **`--on-time`**, **`--off-time`**) and analog set-points (**`--value`**, **`--variation 1-4`**) with **`--mode sbo|direct|direct-no-ack`**. Everything is validated before anything is sent, and a control is sent exactly once.
- **`client time-sync`:** LAN (RECORD_CURRENT_TIME), non-LAN (DELAY_MEASURE with compensation) or a plain write of **`--time`**.
- **`client restart`**, **`client unsolicited enable|disable`**, **`client assign-class`**, **`client freeze`** (**`--clear`**, **`--no-ack`**), **`client attributes`** (all, list, by name or number) and **`client ping`** (DELAY_MEASURE with round trip and processing delay).
- **`client file ls|info|get|put|rm`:** File transfer (group 70), with **`--block-size`** and file authentication (**`--file-user`**, password from a file or the environment); SHA-256 of the content in the result.
- **`client objects`:** The groups and variations dnp3ctl encodes and decodes.
- **`discover`:** Sweeps subnets with one link layer REQUEST_LINK_STATUS per link address (**`--outstations`**, default 10 and the self address) and reports the link address each outstation answers from. Safety cap of 4096 hosts (**`--force-large-scan`**).
- **`server start`:** A simulated outstation from a YAML or JSON model, or the built-in demo: every point type with event classes, variations and deadbands, event buffers per master, unsolicited responses, controls with select-before-operate, **`select_required`**, set-point limits and forced statuses, restarts, time synchronization, device attributes, files with directories and uploads, simulated values (**`toggle`**, **`ramp`**, **`sine`**, **`random`**, **`count`**). TCP, TLS, UDP (**`--udp`**) or a serial port (**`--serial`**); **`--allow`**, **`--max-sessions`**, **`--duration`**; Secure Authentication. Every event is logged to stdout.
- **`server generate-config`:** Writes the commented demo model, as YAML or JSON.
- **Transports:** TCP, TLS (**`--tls-ca`**, **`--tls-cert`**, **`--tls-key`**, **`--tls-server-name`**, **`--tls-insecure`**), UDP and serial lines (**`--baud`**, **`--data-bits`**, **`--parity`**, **`--stop-bits`**, via [go-serial](https://github.com/otfabric/go-serial)), with **`--url tcp://|tls://|udp://|serial://`** or **`--ip`** / **`--port`**.
- **Link and application layer:** **`--master`**, **`--outstation`**, **`--link-confirmed`**, **`--link-timeout`**, **`--link-retries`**, **`--keep-alive`**, **`--timeout`**, **`--response-timeout`**, and **`--retries`** for the requests that are safe to repeat (never controls, freezes, restarts or writes).
- **Secure Authentication v5:** Pre-shared session key from **`--sa-key-file`** or **`DNP3CTL_SA_KEY`** (never a flag), **`--sa-user`**, **`--sa-mac`**; on the client and the simulated outstation.
- **Output:** **`--format text|json|table`** on every command; streams write JSON Lines. A single JSON error object on failure with `--format json`, and stable exit codes (0, 2, 3, 4, 5, 6, 7, 10). The JSON shapes and exit codes are documented in **`docs/json-output.md`** and **`docs/exitcodes.md`**.
- **Environment:** Every flag has a **`DNP3CTL_*`** variable; a flag wins over the environment.
- **`--debug`:** Traces every link frame and application fragment sent and received, decoded, and shows the causes of an error.
- **Completion:** bash, zsh, fish and PowerShell; flags with a fixed set of values complete to those values, **`--object`** to the known objects.

### Limitations

- Secure Authentication: pre-shared session keys only; no key change procedures, no aggressive mode (as go-dnp3).
- `client operate` sends one control per request; pattern control blocks (g12v2) are not offered, and the simulated outstation answers them with NOT_SUPPORTED.
- FREEZE_AT_TIME and writes of attributes and octet strings are not offered by the client.
- The simulated outstation serves one link address per instance, and over UDP one master at a time.

### Quality

- **Tests:** Whole-module statement coverage of **94.6%**, with a floor of 85% enforced by **`make coverage`** (`COVERAGE_MIN`). Every command is tested through the real command tree against an in-process simulated outstation (**`internal/stationtest`**), over TCP, TLS and UDP and over a serial stream, in all three output formats, with its exit codes; Secure Authentication with the right and the wrong key.
- **Checks:** `make check` runs gofmt, staticcheck, go vet, golangci-lint, govulncheck and the race-enabled tests.
- **Hardware-free:** The test suite needs no outstation and no network beyond the loopback interface.

---
