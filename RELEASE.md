# dnp3ctl Releases

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

### Fixed

- **The linux/armv7 build:** a range check of the outstation model compared an `int` with 4294967295, which does not compile where `int` has 32 bits; whole numbers that do not fit an `int` on such platforms are now accepted as well. `make check` now typechecks for 32-bit targets (`make cross`).

### Upgrade notes

- No breaking changes to commands, flags or JSON output; the JSON gains `broadcast`, `interval`, `set`, `attribute`, `append`, `function` and `value` fields where they apply, and `client operate` gains `controls` (and `error` for a partial result).
- `--index` of `client operate` is now a list; a single index works as before.

### Quality

- Every changed line is covered by tests, end to end against the simulated outstation: broadcast controls and freezes, scheduled freezes (also the refusal of a time that has passed), attribute sets read and written (writable and not), set-0 writes, append, deadbands read and written, several controls (all succeeded, partly refused, refused at select), every management function and a refused one, indications and clearing the restart indication, the debug log of both roles.

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
