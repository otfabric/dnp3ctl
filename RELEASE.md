# dnp3ctl Releases

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

`client` against an outstation that answers `NOT_SUPPORTED` with IIN2.2 (go-dnp3 v1.4, opendnp3, Step Function I/O's dnp3) reports the echoed statuses, as for any refused control: exit code 6, or 7 when other controls of a direct operate were executed. With go-dnp3 v1.4.0 it would have reported only the IIN error.

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
