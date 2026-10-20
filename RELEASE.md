# dnp3ctl Releases

## Unreleased

- **`--udp-local-port`:** with `udp://` URLs, send from a fixed local port, for outstations that answer only a configured master endpoint.
- **`server start --udp`:** the event log names the master the outstation answers as `remote`, not the outstation's own socket.
- **Docs:** the UDP behaviour of the simulated outstation, and how Secure Authentication has been verified.

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
