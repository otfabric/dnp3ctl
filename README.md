# dnp3ctl — DNP3 (IEEE 1815) Command-Line Tool: Poll, Monitor, Operate, Synchronize Time, Transfer Files & Simulate an Outstation

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![CI Status](https://github.com/otfabric/dnp3ctl/actions/workflows/ci.yml/badge.svg)](https://github.com/otfabric/dnp3ctl/actions/workflows/ci.yml)
[![Code Coverage](https://codecov.io/gh/otfabric/dnp3ctl/graph/badge.svg)](https://codecov.io/gh/otfabric/dnp3ctl)
[![Latest Release](https://img.shields.io/github/v/release/otfabric/dnp3ctl?label=release)](https://github.com/otfabric/dnp3ctl/releases)

**dnp3ctl is a command-line toolkit for DNP3 (IEEE 1815), the SCADA protocol between masters and outstations in electric, water and oil and gas utilities, in both roles.** As a master it polls an RTU, IED or gateway, reads any object and range, watches the unsolicited responses it sends, operates controls (select-before-operate, direct operate, no-ack; CROB and analog set-points), synchronizes its clock, restarts it, freezes counters, reads its device attributes and transfers files. As an outstation it simulates an RTU from a model of points, controls and files, so that a SCADA master or a protocol gateway has something to talk to.

It is built for commissioning and site acceptance tests, point-to-point checks, troubleshooting a link between a master and its outstations, and hardware-free test setups. Output can be plain text, a table or JSON with stable exit codes, so it works in scripts and CI. It speaks TCP, TLS, UDP and serial, and Secure Authentication v5. It is written in Go on top of [go-dnp3](https://github.com/otfabric/go-dnp3).

**Quick links:** [Quick start](#quick-start) · [Features](#features) · [Installation](#installation) · [DNP3 in two minutes](#dnp3-in-two-minutes) · [Connecting](#connecting-to-an-outstation) · [Commands](#common-commands) · [Simulated outstation](#simulate-an-outstation) · [FAQ](#faq)

> **Responsible use:** dnp3ctl sends real SCADA requests. Only use it on outstations and networks you own or are authorized to test. `client operate` operates breakers and changes set-points, `client restart`, `client time-sync`, `client freeze` and `client assign-class` change the outstation itself, and `client file put` can deliver a configuration: know what an index controls before you send to it. An outstation discards the events a master has read: polling an outstation that is in service with its master's link address takes those events away from it.

---

## Why dnp3ctl?

- **See the whole process image** — `client poll` runs an integrity poll and lists every point with its flags; `client read` reads any object (`g30v5`, `g1`, …) and range, and events with their time stamps.
- **Watch an outstation live** — `client monitor` stays connected, enables unsolicited responses and prints every event as it arrives, as text, a table or JSON Lines.
- **Operate, carefully** — `client operate` sends control relay output blocks (latch, pulse, trip, close) and analog set-points, with select-before-operate, direct operate or direct operate without acknowledgement. Everything is validated first, and a control is sent exactly once.
- **Everything else a master does** — time synchronization (LAN and non-LAN), cold and warm restarts, enable and disable unsolicited, assign class, freeze and freeze-and-clear, device attributes, delay measurement.
- **Files** — list the directories of an outstation, fetch disturbance records and logs, write and delete files (group 70).
- **Find what is there** — `discover` sweeps subnets for outstations with a single link status request, and reports the link address each answers from.
- **Simulate an RTU** — `server start` serves an outstation from a YAML model: values that change by themselves, event classes and buffers per master, unsolicited responses, controls with select-before-operate, restarts, attributes, files and uploads.
- **Every transport** — TCP, TLS with mutual authentication, UDP and serial lines; Secure Authentication v5 with a pre-shared key.
- **Script-friendly** — `--format text|json|table`, a single JSON error object on failure, stable exit codes and `DNP3CTL_*` environment variables.
- **Pre-built binaries** — Linux, macOS and Windows (x86-64 and ARM) from [GitHub Releases](https://github.com/otfabric/dnp3ctl/releases), with SHA-256 checksums; no runtime to install.

---

## Quick start

Download a binary from [GitHub Releases](https://github.com/otfabric/dnp3ctl/releases) (see [Installation](#installation) for copy-paste commands for Linux, macOS and Windows, or build from source). No outstation at hand? Start the built-in demo outstation in one terminal:

```console
dnp3ctl server start
```

and talk to it from another (replace `127.0.0.1` with your outstation, and `10` with its link address):

```console
# What does this outstation hold? (integrity poll)
dnp3ctl client poll --ip 127.0.0.1 --outstation 10

# Watch events as they arrive, until Ctrl-C
dnp3ctl client monitor --ip 127.0.0.1 --integrity

# Open a breaker with select-before-operate
dnp3ctl client operate --ip 127.0.0.1 --type crob --index 0 --op latch-off

# Change a set-point
dnp3ctl client operate --ip 127.0.0.1 --type analog --index 0 --value 11.2

# Synchronize its clock
dnp3ctl client time-sync --ip 127.0.0.1

# Which outstations are on this subnet?
dnp3ctl discover --subnets 192.168.1.0/24

# The same poll as JSON, for a script
dnp3ctl client poll --ip 127.0.0.1 --format json
```

```console
$ dnp3ctl client poll --ip 127.0.0.1
Integrity poll of outstation 10 at tcp://127.0.0.1:20000: 14 objects in 1 ms  IIN: NEED_TIME|DEVICE_RESTART
  g1v2    binary-input             0  on              [ONLINE]
  g1v2    binary-input             1  off             [ONLINE]
  g3v2    double-bit-input         0  on              [ONLINE]
  g10v2   binary-output            0  on              [ONLINE]
  g20v1   counter                  0  123456          [ONLINE]
  g30v5   analog-input             0  11.02           [ONLINE]
  g30v1   analog-input             1  212             [ONLINE]
  g40v3   analog-output            0  11              [ONLINE]
  g110v8  octet-string             0  FEEDER-7
  ...
```

Points are numbers: the **object** (`g30v5`: group 30, analog input, variation 5) says what a value is and how it is encoded, the **index** which point of that type it is. See [DNP3 in two minutes](#dnp3-in-two-minutes).

---

## How it compares

DNP3 test tools are mostly graphical: protocol test harnesses and simulators with a window per channel, many of them commercial and Windows-only. Protocol libraries are the other end of the range: they do everything, once you have written a program. dnp3ctl sits in between.

| | dnp3ctl | GUI test tools and simulators | Protocol libraries | Wireshark |
|---|:---:|:---:|:---:|:---:|
| Poll, monitor, operate without writing code | ✅ | ✅ | — | — |
| Works over SSH, on a headless gateway or a jump host | ✅ | — | ✅ | partial (tshark) |
| Scriptable: JSON output, exit codes, environment variables | ✅ | rarely | ✅ (it is code) | partial |
| Simulate an outstation from a text file | ✅ | ✅ | with code | — |
| Serial lines and UDP as well as TCP and TLS | ✅ | ✅ | most | — |
| Runs in CI, in a container, on a Raspberry Pi | ✅ | — | ✅ | ✅ |
| Decodes traffic between two other parties | — | some | — | ✅ |
| Single static binary, Linux / macOS / Windows | ✅ | — | — | — |

They combine well: use dnp3ctl to drive and check an outstation from the command line, a GUI tool when you want to click through a point list, and Wireshark when you need to see what two other devices say to each other.

---

## Use cases

- **Commissioning and site acceptance tests** — Poll the outstation, compare with the point list, operate each output with select-before-operate and watch the events arrive.
- **Point-to-point tests** — Leave `client monitor` running while someone operates the primary equipment; every event is printed with its time stamp.
- **Troubleshooting a link** — `client ping`, `discover` and `--debug` show whether the outstation answers, at which link address, how fast, and what exactly is on the wire.
- **Master and gateway development** — Point your SCADA master or protocol gateway at `server start` and get an outstation that answers polls, executes controls, reports unsolicited and refuses what you tell it to.
- **Network inventory** — Sweep subnets for DNP3 outstations and their link addresses.
- **Automation and CI** — Use `--format json`, exit codes and environment variables in scripts and pipelines; run the simulated outstation as a test fixture.
- **Collecting records** — Fetch disturbance records and logs from an outstation's file system.

---

## Table of Contents

- [Why dnp3ctl?](#why-dnp3ctl)
- [Quick start](#quick-start)
- [How it compares](#how-it-compares)
- [Use cases](#use-cases)
- [Features](#features)
- [Installation](#installation)
- [DNP3 in two minutes](#dnp3-in-two-minutes)
- [Usage](#usage)
  - [General CLI usage](#general-cli-usage)
  - [Connecting to an outstation](#connecting-to-an-outstation)
  - [Common commands](#common-commands)
  - [Simulate an outstation](#simulate-an-outstation)
- [Environment variable configuration](#environment-variable-configuration)
- [FAQ](#faq)
- [Project structure](#project-structure)
- [Development](#development)
- [Version history](#version-history)
- [Contributing](#contributing)
- [License](#license)

---

## Features

| **Feature**      | **Description**                                                                                                           |
|------------------|---------------------------------------------------------------------------------------------------------------------------|
| Poll             | Integrity poll (events of classes 1-3, then class 0), event poll of chosen classes, or a poll of exactly the classes given; filters by object and index. |
| Read             | READ of any object and variation, any variation (`g30`), ranges, index lists and counts (events).                         |
| Monitor          | Stay connected and stream every value: unsolicited responses, and optional integrity and periodic event polls. Reconnects, filters, `--count` and `--duration`. |
| Operate          | Control relay output blocks (latch on/off, pulse on/off, trip, close; count, on and off time) and analog output blocks (32-bit, 16-bit, single and double precision); select-before-operate, direct operate and direct operate without acknowledgement. |
| Time             | Time synchronization by the LAN procedure (RECORD_CURRENT_TIME), the non-LAN procedure (DELAY_MEASURE with delay compensation), or a plain write of a given time. |
| Restart          | COLD_RESTART and WARM_RESTART, with the delay the outstation reports.                                                     |
| Unsolicited      | ENABLE_UNSOLICITED and DISABLE_UNSOLICITED per class.                                                                     |
| Assign class     | ASSIGN_CLASS of objects and ranges to an event class.                                                                     |
| Freeze           | IMMED_FREEZE and FREEZE_CLEAR of counters or analog inputs, with or without acknowledgement.                              |
| Attributes       | Device attributes (group 0): all, one by name or number, or the list of supported variations.                            |
| Ping             | DELAY_MEASURE at an interval, with round trip and the outstation's processing delay.                                     |
| Files            | File transfer (group 70): list a directory, file info, fetch, write, delete; file authentication with credentials from a file or the environment. |
| Objects          | List the groups and variations dnp3ctl encodes and decodes.                                                               |
| Discover         | Find DNP3 outstations on subnets with a link status request (no application layer request) and report their link address. |
| Server           | A simulated outstation from a YAML or JSON model: every point type, event classes and deadbands, event buffers per master, unsolicited responses, controls with select-before-operate and forced refusals, restarts, time synchronization, device attributes, files and uploads, simulated values; TCP, TLS, UDP or serial; allow-list; Secure Authentication. |
| Transports       | TCP, TLS (IEEE 1815-2012 Annex B / IEC 62351-3) with server verification and client certificates, UDP and serial lines, selected with **`--url`**. See [Connecting to an outstation](#connecting-to-an-outstation). |
| Link layer       | Master and outstation link addresses, confirmed or unconfirmed user data, link timeout and retries, keep-alive.            |
| Secure Authentication | SAv5 with a pre-shared session key, read from a file or the environment (never the command line); HMAC-SHA-256 or HMAC-SHA-1. |
| Robustness       | Timeouts per fragment and per request, retries of the requests that are safe to repeat (never controls), reconnects in `monitor`. |
| `--format`       | Every command that prints a result accepts **`--format text\|json\|table`** (or **`DNP3CTL_OUTPUT_FORMAT`**); streams write JSON Lines. |
| `--debug`        | Trace every link frame and application fragment sent and received, decoded, to stderr.                                   |
| Completion       | Shell completion for bash, zsh, fish and PowerShell; flags with a fixed set of values (**`--format`**, **`--mode`**, **`--op`**, **`--object`**, …) complete to those values. |

The following tree view gives the overview of available commands.

```
dnp3ctl
├── client
│   ├── poll             Integrity, event or class poll
│   ├── read             Read objects by group, variation and range
│   ├── monitor          Print every value the outstation reports, until interrupted
│   ├── operate          CROB or analog set-point (sbo | direct | direct-no-ack)
│   ├── time-sync        Synchronize the clock (lan | non-lan | write)
│   ├── restart          COLD_RESTART or WARM_RESTART
│   ├── unsolicited
│   │   ├── enable       ENABLE_UNSOLICITED
│   │   └── disable      DISABLE_UNSOLICITED
│   ├── assign-class     ASSIGN_CLASS
│   ├── freeze           IMMED_FREEZE / FREEZE_CLEAR
│   ├── attributes       Device attributes (group 0)
│   ├── ping             DELAY_MEASURE with round-trip time
│   ├── file
│   │   ├── ls           List a directory
│   │   ├── info         File or directory descriptor
│   │   ├── get          Fetch a file
│   │   ├── put          Write a file
│   │   └── rm           Delete a file
│   └── objects          List the groups and variations dnp3ctl knows
├── server
│   ├── start            Serve a simulated outstation from a model (or the built-in demo)
│   └── generate-config  Write an example outstation model
├── discover             Find DNP3 outstations on subnets
├── completion           Generate shell completion script (bash|zsh|fish|powershell)
└── version              Print version
```

## Installation

The easiest way to install dnp3ctl is to download a pre-built binary from the [GitHub Releases page](https://github.com/otfabric/dnp3ctl/releases). No Go toolchain is required.

| Platform | CPU | Download |
|---|---|---|
| Linux | x86-64 | `dnp3ctl-<version>-linux-amd64.tar.gz` |
| Linux | 64-bit ARM (Raspberry Pi 4/5 with a 64-bit OS, AWS Graviton) | `dnp3ctl-<version>-linux-arm64.tar.gz` |
| Linux | 32-bit ARM (Raspberry Pi with a 32-bit OS) | `dnp3ctl-<version>-linux-armv7.tar.gz` |
| macOS | Apple Silicon (M1 and later) | `dnp3ctl-<version>-darwin-arm64.tar.gz` |
| macOS | Intel | `dnp3ctl-<version>-darwin-amd64.tar.gz` |
| Windows | x86-64 | `dnp3ctl-<version>-windows-amd64.zip` |
| Windows | ARM64 | `dnp3ctl-<version>-windows-arm64.zip` |

Each release also contains `dnp3ctl-<version>-checksums.txt` (SHA-256) so you can verify your download. The commands below fetch the **latest** release, verify its checksum and install it. Afterwards, check that it works:

```console
dnp3ctl version
```

### Linux

```bash
# 1. Find the latest version and your CPU architecture
VERSION=$(curl -fsSLI -o /dev/null -w '%{url_effective}' https://github.com/otfabric/dnp3ctl/releases/latest | sed 's|.*/v||')
case "$(uname -m)" in
  x86_64)         ARCH=amd64 ;;
  aarch64|arm64)  ARCH=arm64 ;;
  armv7l)         ARCH=armv7 ;;
  *) echo "Unsupported architecture: $(uname -m)" ;;
esac

# 2. Download the archive and the checksum file
FILE="dnp3ctl-${VERSION}-linux-${ARCH}.tar.gz"
BASE="https://github.com/otfabric/dnp3ctl/releases/download/v${VERSION}"
curl -fsSLO "${BASE}/${FILE}"
curl -fsSLO "${BASE}/dnp3ctl-${VERSION}-checksums.txt"

# 3. Verify the download (should print "OK")
grep "${FILE}" "dnp3ctl-${VERSION}-checksums.txt" | sha256sum -c -

# 4. Extract and install (the binary in the archive carries the version and platform in its name)
mkdir -p dnp3ctl-dl && tar -xzf "${FILE}" -C dnp3ctl-dl
sudo install -m 0755 "$(find dnp3ctl-dl -type f -name 'dnp3ctl*' | head -n1)" /usr/local/bin/dnp3ctl
```

No `sudo`? Install into your home directory instead (and make sure `~/.local/bin` is in your `PATH`):

```bash
mkdir -p ~/.local/bin
install -m 0755 "$(find dnp3ctl-dl -type f -name 'dnp3ctl*' | head -n1)" ~/.local/bin/dnp3ctl
```

To use a serial port without root, add yourself to the group that owns it (`dialout` on Debian and Ubuntu, `uucp` on Arch), then log in again.

### macOS

```bash
# 1. Find the latest version and your CPU architecture (arm64 = Apple Silicon, x86_64 = Intel)
VERSION=$(curl -fsSLI -o /dev/null -w '%{url_effective}' https://github.com/otfabric/dnp3ctl/releases/latest | sed 's|.*/v||')
case "$(uname -m)" in
  arm64)   ARCH=arm64 ;;
  x86_64)  ARCH=amd64 ;;
esac

# 2. Download the archive and the checksum file
FILE="dnp3ctl-${VERSION}-darwin-${ARCH}.tar.gz"
BASE="https://github.com/otfabric/dnp3ctl/releases/download/v${VERSION}"
curl -fsSLO "${BASE}/${FILE}"
curl -fsSLO "${BASE}/dnp3ctl-${VERSION}-checksums.txt"

# 3. Verify the download (should print "OK")
grep "${FILE}" "dnp3ctl-${VERSION}-checksums.txt" | shasum -a 256 -c -

# 4. Extract and install (the binary in the archive carries the version and platform in its name)
mkdir -p dnp3ctl-dl && tar -xzf "${FILE}" -C dnp3ctl-dl
sudo install -m 0755 "$(find dnp3ctl-dl -type f -name 'dnp3ctl*' | head -n1)" /usr/local/bin/dnp3ctl
```

If you downloaded the archive with a web browser instead of `curl` and macOS refuses to run the binary ("cannot be verified"), remove the quarantine flag: `xattr -d com.apple.quarantine /usr/local/bin/dnp3ctl`.

### Windows (PowerShell)

```powershell
# 1. Find the latest version and your CPU architecture
$version = (Invoke-RestMethod https://api.github.com/repos/otfabric/dnp3ctl/releases/latest).tag_name.TrimStart('v')
$arch    = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }

# 2. Download the archive and the checksum file
$file = "dnp3ctl-$version-windows-$arch.zip"
$base = "https://github.com/otfabric/dnp3ctl/releases/download/v$version"
Invoke-WebRequest "$base/$file" -OutFile "$env:TEMP\$file"
Invoke-WebRequest "$base/dnp3ctl-$version-checksums.txt" -OutFile "$env:TEMP\dnp3ctl-checksums.txt"

# 3. Verify the download (throws an error on mismatch)
$expected = ((Select-String -Path "$env:TEMP\dnp3ctl-checksums.txt" -Pattern $file).Line -split '\s+')[0]
$actual   = (Get-FileHash "$env:TEMP\$file" -Algorithm SHA256).Hash
if ($expected -ne $actual) { throw "Checksum mismatch for $file" }

# 4. Extract to a per-user folder and name the program dnp3ctl.exe
#    (the .exe in the archive carries the version and platform in its name)
$dir = "$env:LOCALAPPDATA\Programs\dnp3ctl"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
Expand-Archive "$env:TEMP\$file" -DestinationPath "$env:TEMP\dnp3ctl-dl" -Force
$exe = Get-ChildItem "$env:TEMP\dnp3ctl-dl" -Recurse -Filter 'dnp3ctl*.exe' | Select-Object -First 1
Copy-Item $exe.FullName "$dir\dnp3ctl.exe" -Force

# 5. Add dnp3ctl to your user PATH (then open a new terminal)
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($userPath -notlike "*$dir*") {
  [Environment]::SetEnvironmentVariable('Path', "$userPath;$dir", 'User')
}
```

If Windows SmartScreen warns about an unrecognised app, choose **More info → Run anyway**.

### Manual download

1. Open the [latest release](https://github.com/otfabric/dnp3ctl/releases/latest).
2. Download the archive for your platform from the table above.
3. Extract it, rename the binary to `dnp3ctl` (`dnp3ctl.exe` on Windows) and move it to a folder on your `PATH`.

### Update

Run the same commands again. They always fetch the latest release and overwrite the previous binary.

### Build from source

Requires Go 1.25 or later:

```console
git clone https://github.com/otfabric/dnp3ctl.git
cd dnp3ctl
make build
```

`make build` runs the checks and writes the binary to **`bin/dnp3ctl`** (see [Building](#building)). With a Go toolchain you can also install straight from the module:

```console
go install github.com/otfabric/dnp3ctl@latest
```

---

## DNP3 in two minutes

DNP3 (IEEE 1815) carries SCADA data between a **master** (the control centre, a SCADA front end: the client) and **outstations** (RTUs, IEDs, gateways: the servers), over TCP and UDP on port **20000**, over TLS on port **19999**, or over serial lines.

| Term | Meaning | In dnp3ctl |
|---|---|---|
| Link address | Every station has one, 0-65519. A frame names the station it is for and the one it is from: an outstation ignores frames for another address, and many answer only the masters they are configured for. 65532 is the self address, which some outstations accept whatever their own address | `--outstation` (default 10), `--master` (default 1) |
| Object (group and variation) | What a value is, and how it is encoded: group 1 binary inputs, 3 double-bit inputs, 10 binary outputs, 20 counters, 21 frozen counters, 30 analog inputs, 40 analog outputs, 110 octet strings; the event groups 2, 4, 11, 22, 23, 32, 42, 111. The variation selects the encoding: `g30v5` is an analog input as a single-precision float with flags, `g30v0` "any variation" | `--object`, `client objects` |
| Index | The number of a point within its type | `--index`, `--range` |
| Flags | Quality: `ONLINE`, `RESTART`, `COMM_LOST`, `REMOTE_FORCED`, `LOCAL_FORCED`, and per type `OVER_RANGE`, `CHATTER_FILTER`, … A point without `ONLINE` is not valid | shown with every value |
| Classes | Class 0 is the current value of every point (static data). Classes 1, 2 and 3 are event classes: a point assigned to one produces an event, with a time stamp, each time it changes. An outstation keeps events in a buffer per master until that master has read and confirmed them | `client poll --mode`, `client assign-class` |
| Integrity poll | A READ of classes 1, 2, 3 and 0: all pending events, then all current values. What a master does after connecting | `client poll` |
| Unsolicited responses | Events the outstation sends by itself, when the master has enabled them for their class | `client monitor`, `client unsolicited` |
| IIN | Internal indications, two octets in every response: `DEVICE_RESTART`, `NEED_TIME`, events pending per class, `EVENT_BUFFER_OVERFLOW`, and the error bits `NO_FUNC_CODE_SUPPORT`, `OBJECT_UNKNOWN`, `PARAMETER_ERROR` with which an outstation refuses a request | shown with poll results |
| Select before operate | A control in two steps: SELECT, which the outstation answers with whether it would execute it, then OPERATE with exactly the same control. A CROB (control relay output block) operates a binary output; an analog output block sets a set-point | `client operate --mode sbo` |
| Secure Authentication | SAv5: the outstation challenges critical requests (controls, writes, restarts, …) and executes them only when the master answers with a MAC over the request, made with a key both share | `--sa-key-file`, `DNP3CTL_SA_KEY` |

There are no names on the wire: an outstation does not tell you that analog input 0 is "Voltage L1". The point list of the outstation is where indexes get their meaning.

---

## Usage

### General CLI Usage

```console
dnp3ctl [command] [flags]

dnp3ctl client poll
dnp3ctl client read
dnp3ctl client monitor
dnp3ctl client operate
dnp3ctl client time-sync
dnp3ctl client restart
dnp3ctl client unsolicited enable|disable
dnp3ctl client assign-class
dnp3ctl client freeze
dnp3ctl client attributes
dnp3ctl client ping
dnp3ctl client file ls|info|get|put|rm
dnp3ctl client objects

dnp3ctl server start
dnp3ctl server generate-config

dnp3ctl discover
dnp3ctl version
```

Every command has `--help`, with a description and examples.

**Global flag:** `--debug` — trace every link frame and application fragment sent and received to stderr, decoded, and print the chain of causes after an error.

**Output format (`--format`):** every command that prints a result accepts **`--format text`** (default), **`json`** or **`table`**, or set **`DNP3CTL_OUTPUT_FORMAT`**. It controls **only** the result on **stdout**. Progress, connection state, summaries and the debug trace go to **stderr**, so stdout can be piped or parsed as it is. The JSON shapes are a stable contract: see [docs/json-output.md](docs/json-output.md).

**Streams:** `client monitor`, `client ping` and `server start` write one record as it happens: one line (**text**), one JSON object per line (**json**, JSON Lines) or one table row (**table**). They run until interrupted with Ctrl-C, which ends them normally.

**Fatal errors:** with `--format json` a command writes a **single JSON error object** to stdout on failure (fields `kind`, `code`, `message`); with **text** or **table** it writes a single **`error:`** line to stderr and leaves stdout empty. Exit codes are stable:

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage: unknown command or flag, missing required flag |
| 3 | Invalid input: nothing was sent |
| 4 | Transport: cannot connect, connection lost, TLS failure |
| 5 | Timeout: no answer |
| 6 | The outstation refused the request (IIN error, control status, file status, authentication), or a protocol error |
| 7 | Partial success: a valid result that holds failures |
| 10 | Output or internal error |

See [docs/exitcodes.md](docs/exitcodes.md) for the details and the `code` values.

**Shell completion:** `dnp3ctl completion bash` (or `zsh`, `fish`, `powershell`) prints a completion script; for example `source <(dnp3ctl completion bash)`. Flags with a fixed set of values complete to those values, **`--object`** to the objects dnp3ctl knows.

### Connecting to an outstation

Every `dnp3ctl client` command takes the outstation as a URL, or as an address and a port. The two styles are **mutually exclusive**, and one of them is required.

| Flags | Connects to |
|---|---|
| `--url tcp://host[:port]` | DNP3 over TCP; port 20000 when omitted |
| `--url tls://host[:port]` | DNP3 over TLS (IEEE 1815-2012 Annex B, IEC 62351-3); port 19999 when omitted |
| `--url udp://host[:port]` | DNP3 over UDP; port 20000 when omitted. dnp3ctl sends from any free port unless `--udp-local-port` names one, for outstations that answer only a configured master endpoint (often port 20000) |
| `--url serial:///dev/ttyUSB0` | A serial line (`serial://COM3` on Windows), with `--baud` (default 9600), `--data-bits` (8), `--parity` (`none`, `even`, `odd`) and `--stop-bits` (1) |
| `--ip host [--port n]` | TCP on port 20000 or `n`; TLS when a `--tls…` flag is given |

**Link addresses.** `--outstation` is the link address of the outstation (default 10) and `--master` the one of dnp3ctl (default 1). Both must match what the outstation is configured with: an outstation does not answer frames for another address, and many answer only the masters they know. A request that times out (exit code 5) against an outstation that accepts the connection is most often a wrong `--outstation`; `dnp3ctl discover --subnets <host> --outstations 1,2,3,4,10` finds the right one.

**Timeouts and retries.** `--timeout` bounds connecting and each request, in milliseconds (default 10000); `--response-timeout` the wait for each fragment of a response (default 5000). `--retries n` repeats a request that timed out or lost its connection up to `n` times, with backoff, but only for requests that are safe to repeat: reads, polls, time synchronization, unsolicited control, assign class and file reads. Controls, freezes, restarts and writes are never repeated.

**Link layer.** `--link-confirmed` sends user data as CONFIRMED_USER_DATA (unconfirmed is the default, as IEEE 1815-2012 recommends on TCP); `--link-timeout` and `--link-retries` govern the link layer answers; `--keep-alive` the idle time after which REQUEST_LINK_STATUS is sent (default 60 s).

**TLS**

| Flag | Meaning |
|---|---|
| `--tls` | Use TLS with `--ip` (a `tls://` URL implies it) |
| `--tls-ca` | PEM file with the CA certificates to verify the outstation with; the system roots by default |
| `--tls-cert`, `--tls-key` | PEM files with the client certificate and its key, for an outstation that authenticates its masters |
| `--tls-server-name` | The name to verify the outstation's certificate against, when it is not the host you connect to |
| `--tls-insecure` | Do not verify the outstation's certificate. For test setups only |

```console
dnp3ctl client poll --url tls://rtu.example.net --tls-ca ca.pem --tls-cert scada.pem --tls-key scada.key
```

**Secure Authentication v5.** An outstation that runs SAv5 challenges critical requests. Give dnp3ctl the pre-shared session key as hex, in a file or in the environment; it is **never** accepted on the command line, where it would end up in the shell history and the process list.

| Flag / variable | Meaning |
|---|---|
| `--sa-key-file` | File holding the key as hex (whitespace and a `0x` prefix are ignored); at least 16 octets |
| `DNP3CTL_SA_KEY` | The key as hex, instead of a file |
| `--sa-user` | User number (default 1) |
| `--sa-mac` | `hmac-sha256-16` (default), `hmac-sha256-8`, `hmac-sha1-10` or `hmac-sha1-8` |

```console
dnp3ctl client operate --ip 192.168.1.20 --type crob --index 0 --op latch-on --sa-key-file /etc/dnp3/outstation10.key
```

### Common Commands

#### Poll an outstation

```console
# Integrity poll: all pending events, then every current value
dnp3ctl client poll --ip 192.168.1.20

# The events of class 1 only
dnp3ctl client poll --ip 192.168.1.20 --mode event --class 1

# The static data, analog inputs 0-9 only, as a table
dnp3ctl client poll --ip 192.168.1.20 --mode class --class 0 --object g30 --index 0-9 --format table
```

A poll reads events, and an outstation discards what a master has read and confirmed: use a link address of your own (`--master`) if the outstation keeps a buffer per master, or poll only what you need. When the outstation answers with values and also sets an IIN error bit, the values are listed with the error and the exit code is 7.

#### Read objects

```console
# All analog inputs as single-precision floats with flags
dnp3ctl client read --ip 192.168.1.20 --object g30v5

# Binary inputs 0-15, in the variation the outstation chooses
dnp3ctl client read --ip 192.168.1.20 --object g1 --range 0-15

# Counters 0, 3 and 7 and the binary outputs, in one request
dnp3ctl client read --ip 192.168.1.20 --object g20,g10 --range 0,3,7

# At most 10 analog input events, with their time stamps
dnp3ctl client read --ip 192.168.1.20 --object g32 --count 10

# The frozen counters after a freeze
dnp3ctl client read --ip 192.168.1.20 --object g21v5
```

`dnp3ctl client objects` lists every object and variation; `--object` completes to them in the shell.

#### Monitor events

```console
# Everything the outstation reports, starting with an integrity poll, until Ctrl-C
dnp3ctl client monitor --ip 192.168.1.20 --integrity

# Analog input events only, as JSON Lines, into a file for one minute
dnp3ctl client monitor --ip 192.168.1.20 --object g32 --duration 60 --format json > events.jsonl

# An outstation without unsolicited responses: poll its events every 5 seconds
dnp3ctl client monitor --ip 192.168.1.20 --unsolicited none --poll-interval 5
```

```console
2026-10-20T09:15:17.284Z  unsol  g2v2    binary-input             1  on              [ONLINE]  @ 2026-10-20T09:15:17.281Z
2026-10-20T09:15:19.102Z  unsol  g32v7   analog-input             0  11.03           [ONLINE]  @ 2026-10-20T09:15:19.100Z
```

The monitor enables unsolicited responses for classes 1-3 (`--unsolicited`) after connecting, after every reconnect and after the outstation reports a restart (which makes it forget them), clears the outstation's restart indication, and reconnects when the connection is lost unless `--no-reconnect` is set.

An outstation takes a second connection from the same master link address for that master reconnecting, and drops the first. To run other commands while a monitor is connected, give them another `--master` (the outstation must answer it).

#### Operate an output

```console
# Close a breaker with select-before-operate (the default mode)
dnp3ctl client operate --ip 192.168.1.20 --type crob --index 0 --op latch-on

# Trip it with a direct operate and a 500 ms pulse
dnp3ctl client operate --ip 192.168.1.20 --type crob --index 0 --op trip --on-time 500 --mode direct

# Pulse an output three times: 200 ms on, 800 ms off
dnp3ctl client operate --ip 192.168.1.20 --type crob --index 4 --op pulse-on --count 3 --on-time 200 --off-time 800

# Set-points: a float, and a 16-bit integer without waiting for an answer
dnp3ctl client operate --ip 192.168.1.20 --type analog --index 0 --value 11.2
dnp3ctl client operate --ip 192.168.1.20 --type analog --index 1 --value 1200 --variation 2 --mode direct-no-ack
```

```console
Operate (sbo) g12v1 index 0: LATCH_ON on outstation 10 at tcp://192.168.1.20:20000: SUCCESS in 4.2 ms
```

| `--op` | CROB | Typical use |
|---|---|---|
| `latch-on`, `latch-off` | LATCH_ON, LATCH_OFF | an output that stays where it is put |
| `pulse-on`, `pulse-off` | PULSE_ON, PULSE_OFF with `--on-time`, `--off-time`, `--count` | a momentary output |
| `close`, `trip` | PULSE_ON with trip-close code CLOSE or TRIP | a breaker with paired close and trip outputs on one index |
| `nul` | NUL | test the path without operating anything |

A control the outstation refuses ends with exit code 6 and the status it echoed (`NO_SELECT`, `LOCAL`, `NOT_SUPPORTED`, `OUT_OF_RANGE`, …). Everything is validated before anything is sent, and a control is sent exactly once, also with `--retries`: after a timeout (exit code 5) it may or may not have been executed.

#### Time, restarts, unsolicited, classes and freezes

```console
# Synchronize the clock: LAN procedure, non-LAN procedure on a serial line, or a fixed time
dnp3ctl client time-sync --ip 192.168.1.20
dnp3ctl client time-sync --url serial:///dev/ttyUSB0 --baud 19200 --method non-lan
dnp3ctl client time-sync --ip 192.168.1.20 --method write --time 2026-01-31T12:00:00Z

# Restart the DNP3 application, or the whole outstation
dnp3ctl client restart --ip 192.168.1.20 --type warm
dnp3ctl client restart --ip 192.168.1.20 --type cold

# Enable unsolicited responses for classes 1 and 2; disable them all
dnp3ctl client unsolicited enable --ip 192.168.1.20 --class 1,2
dnp3ctl client unsolicited disable --ip 192.168.1.20

# Put analog inputs 0-7 in class 2
dnp3ctl client assign-class --ip 192.168.1.20 --class 2 --object g30 --range 0-7

# Freeze all counters; freeze and clear counters 0-3
dnp3ctl client freeze --ip 192.168.1.20
dnp3ctl client freeze --ip 192.168.1.20 --object g20 --range 0-3 --clear
```

#### Device attributes and delay

```console
# Manufacturer, product, versions, serial number, point counts, …
dnp3ctl client attributes --ip 192.168.1.20
dnp3ctl client attributes --ip 192.168.1.20 --variation serial-number

# Which attributes does it support, and which are writable?
dnp3ctl client attributes --ip 192.168.1.20 --variation list

# Four delay measurements, one per second
dnp3ctl client ping --ip 192.168.1.20
```

#### Transfer files

```console
# What does the outstation offer?
dnp3ctl client file ls --ip 192.168.1.20
dnp3ctl client file ls --ip 192.168.1.20 --path /records
dnp3ctl client file info --ip 192.168.1.20 --path /records/disturbance-0001.dat

# Fetch a file, or pipe it
dnp3ctl client file get --ip 192.168.1.20 --path /records/disturbance-0001.dat
dnp3ctl client file get --ip 192.168.1.20 --path /logs/events.csv --output - | tail

# Write and delete a file
dnp3ctl client file put --ip 192.168.1.20 --input settings.xml --path /config/settings.xml
dnp3ctl client file rm --ip 192.168.1.20 --path /config/old.xml

# An outstation that wants credentials first (the password is never a flag)
DNP3CTL_FILE_PASSWORD=… dnp3ctl client file ls --ip 192.168.1.20 --file-user engineer
```

#### Find outstations

```console
# Outstations at link address 10 (or that accept the self address) on a subnet
dnp3ctl discover --subnets 192.168.1.0/24

# Try several link addresses
dnp3ctl discover --subnets 10.0.1.0/24 --outstations 1,2,3,4,10,65532 --format table
```

The probe sends one link layer REQUEST_LINK_STATUS per link address and waits for LINK_STATUS: no application layer request is made, nothing is read and no event is taken.

### Simulate an outstation

```console
# The built-in demo outstation on port 20000
dnp3ctl server start

# Write its model, edit it, serve it
dnp3ctl server generate-config --output outstation.yaml
dnp3ctl server start --config outstation.yaml

# On another port, on localhost only, events as JSON Lines
dnp3ctl server start --config outstation.yaml --listen 127.0.0.1 --port 20001 --format json

# TLS for two masters only; UDP; a serial line
dnp3ctl server start --config outstation.yaml --tls-cert server.pem --tls-key server.key --tls-client-ca ca.pem --allow 10.0.0.5,10.0.0.6
dnp3ctl server start --config outstation.yaml --udp
dnp3ctl server start --config outstation.yaml --serial /dev/ttyUSB0 --baud 19200

# With Secure Authentication v5
dnp3ctl server start --config outstation.yaml --sa-key-file outstation.key
```

The outstation is described by a model: its link address, its points with their initial values and event classes, its controls and its files.

```yaml
outstation:
  name: Demo feeder RTU
  address: 10
  unsolicited: {enabled: true}
  restart_delay: 2s
  attributes: {manufacturer: otfabric, product: dnp3ctl demo outstation}

points:
  - {type: binary-input, index: 0, name: Breaker closed, value: true, class: 1}
  - {type: binary-output, index: 0, name: Breaker control status, value: true, class: 1}
  - type: analog-input
    index: 0
    name: Voltage L1 (kV)
    value: 11.02
    class: 2
    deadband: 0.01
    static_variation: 5
    simulate: {mode: sine, interval: 2s, min: 10.9, max: 11.1}
  - {type: counter, index: 0, name: Energy (kWh), value: 123456, class: 3}

controls:
  - {type: crob, index: 0, name: Breaker, target: {type: binary-output, index: 0}, select_required: true}
  - {type: crob, index: 2, name: Local panel, status: LOCAL}
  - {type: analog, index: 0, name: Voltage set-point, min: 10, max: 12}

files:
  - {name: /records/disturbance-0001.dat, size: 8192}

uploads:
  accept: true
```

What it does:

- **Answers** integrity, event and class polls and reads of every point type, in any variation; keeps an **event buffer per master** that survives reconnects and is confirmed by the master.
- **Reports unsolicited** the events of the classes a master enables, with confirmation and retries.
- **Executes controls** on the point named as their `target`, with select-before-operate, direct operate and direct operate without acknowledgement. `select_required` refuses a direct operate with NO_SELECT; `min`/`max` refuse a set-point with OUT_OF_RANGE; `status` answers every control with a refusal of your choice, to test a master.
- **Changes values by itself** where the model says `simulate` (`toggle`, `ramp`, `sine`, `random`, `count`), producing events of the point's class with the outstation's time.
- **Answers** time synchronization, delay measurement, cold and warm restarts (with `restart_delay`), assign class, freezes, device attributes, and file transfer with uploads.

Every event is written to stdout as it happens:

```console
2026-10-20T00:06:21.028Z  listening         tcp://127.0.0.1:20000  Demo feeder RTU, link address 10
2026-10-20T00:06:21.668Z  connected         127.0.0.1:60893
2026-10-20T00:06:21.668Z  control           SELECT  g12v1 index=0  LATCH_OFF  SUCCESS  master 1
2026-10-20T00:06:21.668Z  request           127.0.0.1:60893  SELECT  0.0 ms
2026-10-20T00:06:21.668Z  control           OPERATE  g12v1 index=0  LATCH_OFF  SUCCESS  select-before-operate, master 1, binary-output 0 = false
2026-10-20T00:06:21.668Z  request           127.0.0.1:60893  OPERATE  0.1 ms
2026-10-20T00:06:21.669Z  disconnected      127.0.0.1:60893  dnp3: connection lost: closed by peer
```

| Flag | Meaning |
|---|---|
| `--config` | The model file (YAML or JSON). Without it the built-in demo outstation is served |
| `--listen`, `--port` | Address and port to listen on. All interfaces and 20000 by default (19999 with TLS) |
| `--udp` | Serve UDP instead of TCP. The outstation answers the endpoint the last datagram came from, so it serves one master at a time, and it sends its first unsolicited response only after a master has sent it a request |
| `--serial`, `--baud`, `--data-bits`, `--parity`, `--stop-bits` | Serve on a serial port instead of the network |
| `--tls-cert`, `--tls-key` | Serve TLS with this certificate |
| `--tls-client-ca` | Require a client certificate of this CA (mutual TLS) |
| `--allow` | Only accept clients from these addresses or CIDR prefixes (TCP and TLS) |
| `--max-sessions` | Maximum number of simultaneous connections (default 16) |
| `--sa-key-file`, `--sa-user`, `--sa-mac` | Require Secure Authentication v5 with this key (or `DNP3CTL_SA_KEY`) |
| `--link-confirmed`, `--link-timeout`, `--link-retries`, `--keep-alive` | As for the client |
| `--duration` | Stop after this many seconds, for use in test pipelines |

The full reference of the model is in [docs/server-config.md](docs/server-config.md). The server listens on all interfaces and accepts any client unless told otherwise: see [SECURITY.md](SECURITY.md).

---

## Environment Variable Configuration

Every flag can also be set through an environment variable, which is convenient for automation and for flags you would otherwise repeat on every command (`DNP3CTL_URL`, `DNP3CTL_OUTSTATION`, `DNP3CTL_OUTPUT_FORMAT`). A flag on the command line wins over the environment. A value that cannot be parsed (a port that is not a number) stops dnp3ctl at startup. A flag that is **required** (`--object` of `client read`, `--type` and `--index` of `client operate`, `--type` of `client restart`, `--path` of the file commands) must still be given on the command line. The global flag `--debug` has no environment variable.

Two secrets are read **only** from the environment or a file, never from a flag: `DNP3CTL_SA_KEY` (the Secure Authentication key, as hex; or `--sa-key-file`) and `DNP3CTL_FILE_PASSWORD` (the password of `--file-user`; or `--file-password-file`).

```console
export DNP3CTL_URL=tcp://192.168.1.20:20000
export DNP3CTL_OUTSTATION=10
dnp3ctl client poll
dnp3ctl client read --object g30v5
```

<details>
<summary>All <code>DNP3CTL_*</code> environment variables</summary>

| Variable | Type | Description | Flag | Default |
|----------|------|-------------|------|---------|
| **DNP3CTL_ALLOW** | strings | **server start**: Only accept TCP and TLS clients from these addresses or CIDR prefixes (default: any) | `--allow` |  |
| **DNP3CTL_ASSIGN_CLASS** | uint8 | **client assign-class**: Class to assign, 0-3 (0 = no events) | `--class` |  |
| **DNP3CTL_ATTRIBUTE** | string | **client attributes**: Attribute to read: all, list (the list of attribute variations), a variation number 1-254 or a name such as manufacturer | `--variation` | all |
| **DNP3CTL_BAUD** | uint32 | every client command, **server start**: Serial baud rate (0 = 9600) | `--baud` |  |
| **DNP3CTL_BLOCK_SIZE** | uint16 | **client file ls**, **client file get**, **client file put**, **client file rm**: Largest block to ask for or send, in octets (0 = the outstation chooses) | `--block-size` |  |
| **DNP3CTL_CLASS** | string | **client poll**: Classes for --mode event (1-3) or class (0-3), comma-separated (default 1,2,3); **client unsolicited enable**: Event classes, comma-separated, 1-3 (default all three) | `--class` |  |
| **DNP3CTL_CONFIG** | string | **server start**: Outstation model file (YAML or JSON); without it a built-in demo outstation is served | `--config` |  |
| **DNP3CTL_CONFIG_FORMAT** | string | **server generate-config**: Format of the model: yaml or json | `--format` | yaml |
| **DNP3CTL_CONTROL_TYPE** | string | **client operate**: Control: crob (control relay output block, g12v1) or analog (analog output block, g41) | `--type` |  |
| **DNP3CTL_COUNT** | uint32 | **client read**: Read at most this many objects (events); cannot be combined with --range; **client monitor**: Stop after this many values (0 = until interrupted); **client ping**: Number of delay measurements before exiting (0 = until interrupted) | `--count` | 4 |
| **DNP3CTL_DATA_BITS** | uint8 | every client command, **server start**: Serial data bits, 5-8 (0 = 8) | `--data-bits` |  |
| **DNP3CTL_DISCOVER_FORCE_LARGE** | bool | **discover**: Allow a scan of more hosts than the built-in safety cap | `--force-large-scan` |  |
| **DNP3CTL_DURATION** | uint32 | **client monitor**, **server start**: Stop after this many seconds (0 = until interrupted) | `--duration` |  |
| **DNP3CTL_EVENTS** | bool | **client objects**: Only list event objects | `--events` |  |
| **DNP3CTL_FILE_PASSWORD_FILE** | string | **client file ls**, **client file get**, **client file put**, **client file rm**: File holding the password of --file-user | `--file-password-file` |  |
| **DNP3CTL_FILE_PATH** | string | **client file ls**: Directory to list; **client file info**: File or directory on the outstation; **client file get**: File on the outstation; **client file put**: File to create on the outstation; **client file rm**: File to delete on the outstation | `--path` | / |
| **DNP3CTL_FILE_USER** | string | **client file ls**, **client file get**, **client file put**, **client file rm**: User name to authenticate file operations with (AUTHENTICATE_FILE); the password comes from --file-password-file or DNP3CTL_FILE_PASSWORD | `--file-user` |  |
| **DNP3CTL_FORCE** | bool | **client file get**: Overwrite an existing --output file; **server generate-config**: Overwrite an existing file | `--force` |  |
| **DNP3CTL_FREEZE_CLEAR** | bool | **client freeze**: Reset the counters to zero after the freeze (FREEZE_CLEAR) | `--clear` |  |
| **DNP3CTL_GROUP** | uint8 | **client objects**: Only list the variations of this group (0 = all groups) | `--group` |  |
| **DNP3CTL_INDEX** | string | **client poll**: Only show these point indexes: list and ranges (e.g. 0,5-9); **client monitor**: Only show these point indexes: list and ranges | `--index` |  |
| **DNP3CTL_INPUT** | string | **client file put**: Local file whose content is sent | `--input` |  |
| **DNP3CTL_INTEGRITY** | bool | **client monitor**: Run an integrity poll after connecting (and after every reconnect), so the stream starts with all current values | `--integrity` |  |
| **DNP3CTL_INTERVAL** | uint32 | **client ping**: Interval between measurements in milliseconds | `--interval` | 1000 |
| **DNP3CTL_IP** | string | Outstation IP address or host name, over TCP (used when --url is not set) | `--ip` |  |
| **DNP3CTL_KEEP_ALIVE** | uint32 | every client command, **server start**: Idle seconds after which REQUEST_LINK_STATUS is sent (0 = 60) | `--keep-alive` |  |
| **DNP3CTL_LINK_CONFIRMED** | bool | every client command, **server start**: Send user data as CONFIRMED_USER_DATA (default unconfirmed, as IEEE 1815-2012 recommends on TCP) | `--link-confirmed` |  |
| **DNP3CTL_LINK_RETRIES** | uint8 | every client command, **server start**: Repeats of an unanswered primary link frame | `--link-retries` |  |
| **DNP3CTL_LINK_TIMEOUT** | uint32 | every client command, **server start**: Wait in milliseconds for a link layer answer (0 = 2000) | `--link-timeout` |  |
| **DNP3CTL_LISTEN** | string | **server start**: Address to listen on (TCP, TLS and UDP) | `--listen` |  |
| **DNP3CTL_MASTER** | uint16 | every client command: Link address of this master; **discover**: Link address the probe is sent from | `--master` | 1 |
| **DNP3CTL_MAX_SESSIONS** | uint16 | **server start**: Maximum number of simultaneous connections (0 = 16) | `--max-sessions` |  |
| **DNP3CTL_NO_ACK** | bool | **client freeze**: Send the no-acknowledgement form (IMMED_FREEZE_NR, FREEZE_CLEAR_NR): no response | `--no-ack` |  |
| **DNP3CTL_NO_RECONNECT** | bool | **client monitor**: Exit when the connection is lost instead of reconnecting | `--no-reconnect` |  |
| **DNP3CTL_OBJECT** | string | **client poll**: Only show these objects: groups or group/variations, comma-separated (e.g. g30,g1v2,32); **client read**: Objects to read, comma-separated: g30v5 (or 30.5), or g30 / g30v0 for any variation; **client monitor**: Only show these objects: groups or group/variations, comma-separated; **client assign-class**: Static objects whose points are assigned, comma-separated (e.g. g30, g1); **client freeze**: Objects to freeze, comma-separated: g20 (counters) or g30 (analog inputs) | `--object` |  |
| **DNP3CTL_OFF_TIME** | uint32 | **client operate**: Off time of a pulse in milliseconds | `--off-time` |  |
| **DNP3CTL_ON_TIME** | uint32 | **client operate**: On time of a pulse in milliseconds | `--on-time` |  |
| **DNP3CTL_OP** | string | **client operate**: Operation of a crob: latch-on, latch-off, pulse-on, pulse-off, close (pulse on, trip-close code close), trip (pulse on, trip-close code trip) or nul | `--op` |  |
| **DNP3CTL_OPERATE_INDEX** | uint32 | **client operate**: Point index of the output | `--index` |  |
| **DNP3CTL_OPERATE_MODE** | string | **client operate**: Procedure: sbo (select, then operate), direct (direct operate) or direct-no-ack (direct operate, no response) | `--mode` | sbo |
| **DNP3CTL_OP_COUNT** | uint8 | **client operate**: Number of times a crob is executed | `--count` | 1 |
| **DNP3CTL_OUTPUT** | string | **client file get**: File to write the content to (- = stdout; default: the base name of --path); **server generate-config**: File to write the model to (- = stdout) | `--output` | - |
| **DNP3CTL_OUTPUT_FORMAT** | string | every client command, **client objects**, **discover**: Output format: text, json, or table; **server start**: Format of the event log on stdout: text, json, or table | `--format` | text |
| **DNP3CTL_OUTSTATION** | uint16 | Link address of the outstation | `--outstation` | 10 |
| **DNP3CTL_OUTSTATIONS** | strings | **discover**: Link addresses to send the probe to, comma-separated; 65532 is the self address, which outstations that support it answer whatever their own address | `--outstations` | [10,65532] |
| **DNP3CTL_PARALLEL** | uint16 | **discover**: Number of concurrent probes (1-256) | `--parallel` | 32 |
| **DNP3CTL_PARITY** | string | every client command, **server start**: Serial parity: none, even or odd (default none) | `--parity` |  |
| **DNP3CTL_PERMISSIONS** | string | **client file put**: Permissions of the new file: octal (644) or rwx form (rw-r--r--) | `--permissions` |  |
| **DNP3CTL_POLL_INTERVAL** | uint32 | **client monitor**: Also poll the events of classes 1-3 every this many seconds (0 = do not poll) | `--poll-interval` |  |
| **DNP3CTL_POLL_MODE** | string | **client poll**: What to read: integrity (events of classes 1-3, then the static data of class 0), event (events of --class) or class (exactly the classes of --class, 0 included) | `--mode` | integrity |
| **DNP3CTL_PORT** | uint16 | every client command: Outstation TCP port (used when --url is not set); **server start**: Port to listen on (0 = 20000, or 19999 with TLS) | `--port` | 20000 |
| **DNP3CTL_PORTS** | strings | **discover**: TCP ports to probe | `--ports` | [20000] |
| **DNP3CTL_RANGE** | string | **client read**: Point indexes to read: a range (0-9), a list (1,4,7) or empty for all points; **client assign-class**, **client freeze**: Point indexes: a range (0-9), a list (1,4,7) or empty for all points | `--range` |  |
| **DNP3CTL_RESPONSE_TIMEOUT** | uint32 | Wait in milliseconds for each fragment of a response (0 = 5000) | `--response-timeout` |  |
| **DNP3CTL_RESTART_TYPE** | string | **client restart**: Restart: cold (full restart of the outstation) or warm (restart of the DNP3 application) | `--type` |  |
| **DNP3CTL_RETRIES** | uint8 | Retries of a request that timed out or lost its connection; only for requests that are safe to repeat (reads, time, unsolicited control, assign class, file reads), never for controls, freezes, restarts or writes | `--retries` |  |
| **DNP3CTL_SA_KEY_FILE** | string | every client command, **server start**: File holding the pre-shared session key of Secure Authentication v5, as hex (or set DNP3CTL_SA_KEY); enables secure authentication | `--sa-key-file` |  |
| **DNP3CTL_SA_MAC** | string | every client command, **server start**: Secure authentication MAC algorithm: hmac-sha256-16 (default), hmac-sha256-8, hmac-sha1-10 or hmac-sha1-8 | `--sa-mac` |  |
| **DNP3CTL_SA_USER** | uint16 | every client command, **server start**: Secure authentication user number | `--sa-user` | 1 |
| **DNP3CTL_SERIAL** | string | **server start**: Serve on this serial port (e.g. /dev/ttyUSB0 or COM3) instead of the network; line settings with --baud, --data-bits, --parity, --stop-bits | `--serial` |  |
| **DNP3CTL_STOP_BITS** | uint8 | every client command, **server start**: Serial stop bits, 1 or 2 (0 = 1) | `--stop-bits` |  |
| **DNP3CTL_SUBNETS** | strings | **discover**: Subnets, ranges or addresses to probe: CIDR (192.168.1.0/24), range (192.168.1.10-20) or a single address; comma-separated | `--subnets` |  |
| **DNP3CTL_TIME** | string | **client time-sync**: Time to write with --method write: now, or an RFC 3339 time such as 2026-01-31T12:00:00Z | `--time` |  |
| **DNP3CTL_TIMEOUT** | uint32 | every client command: Timeout in milliseconds for connecting and for each request, retries included (0 = 10000); **discover**: Timeout in milliseconds per probe (0 = 1000) | `--timeout` |  |
| **DNP3CTL_TIME_METHOD** | string | **client time-sync**: Procedure: lan (RECORD_CURRENT_TIME, then the time of that request), non-lan (DELAY_MEASURE, then the time corrected by the delay) or write (the time of --time, without correction) | `--method` | lan |
| **DNP3CTL_TLS** | bool | Use TLS with --ip (implied by a tls:// URL) | `--tls` |  |
| **DNP3CTL_TLS_CA** | string | PEM file with the CA certificates to verify the outstation with (default: system roots) | `--tls-ca` |  |
| **DNP3CTL_TLS_CERT** | string | every client command: PEM file with the own certificate (client authentication); **server start**: PEM file with the server certificate; with --tls-key it makes the server a TLS server | `--tls-cert` |  |
| **DNP3CTL_TLS_CLIENT_CA** | string | **server start**: PEM file with the CA certificates masters must present a certificate of (mutual TLS) | `--tls-client-ca` |  |
| **DNP3CTL_TLS_INSECURE** | bool | Do not verify the outstation certificate (testing only) | `--tls-insecure` |  |
| **DNP3CTL_TLS_KEY** | string | every client command, **server start**: PEM file with the private key of --tls-cert | `--tls-key` |  |
| **DNP3CTL_TLS_SERVER_NAME** | string | Name to verify the outstation certificate against (default: the host) | `--tls-server-name` |  |
| **DNP3CTL_UDP** | bool | **server start**: Serve DNP3 over UDP instead of TCP (one master at a time) | `--udp` |  |
| **DNP3CTL_UNSOLICITED** | string | **client monitor**: Classes to enable unsolicited responses for after connecting, comma-separated, or none | `--unsolicited` | 1,2,3 |
| **DNP3CTL_UDP_LOCAL_PORT** | uint16 | With udp:// URLs: send from this local UDP port, for outstations that answer only a configured master endpoint (often 20000); 0 = any free port | `--udp-local-port` |  |
| **DNP3CTL_URL** | string | Outstation URL: tcp://host[:port], tls://host[:port], udp://host[:port] or serial:///dev/ttyUSB0 (ports default to 20000, and 19999 for tls); mutually exclusive with --ip/--port | `--url` |  |
| **DNP3CTL_VALUE** | string | **client operate**: Set-point value of an analog control | `--value` |  |
| **DNP3CTL_VARIATION** | uint8 | **client operate**: Variation of an analog control: 1 (32-bit), 2 (16-bit), 3 (single-precision float), 4 (double-precision float); 0 = 1 for whole numbers that fit, 3 otherwise | `--variation` |  |
| **DNP3CTL_SA_KEY** | hex | Secure Authentication pre-shared session key (instead of `--sa-key-file`); environment only | — | |
| **DNP3CTL_FILE_PASSWORD** | string | Password of `--file-user` (instead of `--file-password-file`); environment only | — | |

</details>

---

## FAQ

### What is dnp3ctl?

dnp3ctl is a command-line tool for DNP3 (IEEE 1815), the SCADA protocol between masters and outstations. As a client (master) it polls, reads and monitors an outstation, operates controls, synchronizes its clock, restarts it and transfers files; as a server it simulates an outstation from a model file. It also finds outstations on a network.

### How do I see all the values of an RTU from the command line?

Run an integrity poll: `dnp3ctl client poll --ip 192.168.1.20 --outstation 10`. Add `--format table` or `--format json` for other output. See [Poll an outstation](#poll-an-outstation).

### How do I watch DNP3 events as they happen?

`dnp3ctl client monitor --ip 192.168.1.20 --integrity` prints the current values and then every event, with its time stamp, until you press Ctrl-C. `--format json` gives one JSON object per line. See [Monitor events](#monitor-events).

### How do I operate a breaker with select-before-operate?

`dnp3ctl client operate --ip 192.168.1.20 --type crob --index 0 --op latch-on` selects, waits for the answer and operates; select-before-operate is the default mode. See [Operate an output](#operate-an-output).

### How do I simulate a DNP3 outstation without hardware?

`dnp3ctl server start` serves a built-in demo outstation on port 20000. `dnp3ctl server generate-config --output outstation.yaml` writes its model for you to edit, and `dnp3ctl server start --config outstation.yaml` serves yours. See [Simulate an outstation](#simulate-an-outstation) and [docs/server-config.md](docs/server-config.md).

### The outstation accepts the connection but does not answer. What should I check?

- **The link addresses.** An outstation ignores frames for another link address, and many answer only the masters they are configured for: check `--outstation` and `--master`. `dnp3ctl discover --subnets <host> --outstations 1,2,3,4,10,65532` asks a list of addresses at the link layer.
- **Is it DNP3 at all?** `discover` tells in a second, without any application layer request.
- **Secure Authentication.** A control that fails with `authentication_failed` (exit code 6) needs the key: `--sa-key-file`.
- **What is on the wire.** `--debug` shows every link frame and application fragment in both directions.

### Why does a poll exit with code 7?

The outstation answered with values and set an error bit in its internal indications: PARAMETER_ERROR for indexes it does not have, OBJECT_UNKNOWN for an object it does not support. The values it did send are in the result, with the error. A request it refuses without any value exits with 6.

### Which outstations and stacks has it been tested with?

The library underneath, [go-dnp3](https://github.com/otfabric/go-dnp3), runs an interoperability suite as master and as outstation against Step Function I/O's dnp3, opendnp3 and DSC Systems' go-dnp3, in the containers of [otfabric/dnp3-interop](https://github.com/otfabric/dnp3-interop). dnp3ctl's own tests run every command against its simulated outstation over TCP, TLS, UDP and a serial stream. If you use dnp3ctl with a device it has trouble with, please [open an issue](https://github.com/otfabric/dnp3ctl/issues) with the `--debug` trace.

### Does dnp3ctl support TLS and Secure Authentication?

Yes. `--url tls://host` (port 19999 by default) with `--tls-ca`, and `--tls-cert` / `--tls-key` for an outstation that wants a client certificate; the simulated outstation serves TLS with `--tls-cert` and `--tls-key`. Secure Authentication v5 works with a pre-shared session key (`--sa-key-file` or `DNP3CTL_SA_KEY`), on both sides; the key change procedures and aggressive mode are not implemented (see [SECURITY.md](SECURITY.md)).

### Does it work on serial lines?

Yes: `--url serial:///dev/ttyUSB0 --baud 19200` (or `serial://COM3` on Windows) for the client, and `server start --serial /dev/ttyUSB0` for the outstation. On a multi-drop line every outstation has its own link address: address one with `--outstation`.

### Will polling a live outstation disturb it?

It can. Events an outstation has reported to a master are gone from its buffer: a poll or a monitor with the link address of the regular master takes those events away from it. Use a link address of your own, if the outstation keeps a buffer per master, or a read of class 0 or of static objects only. A connection with the link address of a master that is connected replaces that master's connection. An outstation may also accept few connections.

### How do I use dnp3ctl in scripts and CI?

Pass `--format json` (or set `DNP3CTL_OUTPUT_FORMAT=json`). Progress goes to stderr, the result goes to stdout, and a failure returns one JSON error object with a stable exit code. `dnp3ctl server start --config model.yaml --duration 60` gives a pipeline an outstation to test against. See [General CLI usage](#general-cli-usage), [docs/json-output.md](docs/json-output.md) and [docs/exitcodes.md](docs/exitcodes.md).

---

## Project structure

<details>
<summary>Repository layout</summary>

```console
dnp3ctl/
├── cmd/ (root command, version, completion)
│   ├── client (poll, read, monitor, operate, time-sync, restart, unsolicited, assign-class, freeze, attributes, ping, file, objects)
│   ├── discover (subnet discovery)
│   └── server (start, generate-config)
├── internal/
│   ├── config (CLI & env: configuration types, flag and environment binding, outstation URLs)
│   ├── cli (shared Cobra helpers: enum completion, required flags, debug)
│   ├── format (stdout **text/json/table** via `Write`, streaming output via `StreamWriter`, table rendering)
│   ├── runner (command wiring, formatted and streaming runs, fatal JSON/text rendering)
│   ├── errs (typed CLI errors, stable codes, exit-code mapping)
│   ├── dnp (what the commands do: connections, TLS, serial, secure authentication, requests, monitor, files, discovery, error classification)
│   ├── station (the simulated outstation: model, validation, controls, simulation, files)
│   ├── stationtest (the simulated outstation for tests, with throw-away TLS certificates)
│   └── types (DTOs for JSON/text/table output)
├── docs/ (exit codes, JSON output contract, outstation model reference)
├── Makefile (Build & test commands)
└── go.mod (Dependencies)
```

</details>

## Development

A toplevel `Makefile` is provided for convenience. Run **`make`** (or **`make help`**) for the full list. Common targets:

| Target | Purpose |
|--------|---------|
| **`make test`** | Unit and end-to-end tests with race detector (`go test -race ./...`) |
| **`make coverage`** | Tests + **`coverage.out`** across the whole module; fails below **85%** total statement coverage (`COVERAGE_MIN`) |
| **`make compile`** | Full-module typecheck (`go build ./...`) |
| **`make lint`** | `compile` then **staticcheck** |
| **`make lint-ci`** | `compile`, **go vet**, **golangci-lint** (uses `.golangci.yml`) |
| **`make vuln`** | **govulncheck** over the module and its dependencies |
| **`make fmt`** / **`fmt-check`** | Format with the gofmt of the Go version in `go.mod` (the one CI uses); `fmt-check` fails if that gofmt or the locally installed one would change any file |
| **`make check`** | `fmt`, `fmt-check`, `lint`, `lint-ci`, `vet`, `vuln`, `test`, **`coverage`** |
| **`make build`** | Runs **`check`** then writes **`bin/dnp3ctl`** |
| **`make build-all`** / **`release-all`** | Cross-compile and package archives |

### Building

`make build` writes the binary to **`bin/dnp3ctl`** (not the repo root).

```console
make build
make build-all
```

### Running Tests

```console
make test
make coverage
```

Tests need no hardware and no network beyond the loopback interface: `internal/stationtest` serves the simulated outstation of `dnp3ctl server` on a free port, and the tests of every command run the real command tree against it, over TCP, TLS and UDP; serial lines are tested over an in-memory pipe.

## Version History

Release notes for each version are maintained in **[RELEASE.md](RELEASE.md)**.

---

## Contributing

Bug reports, outstation-compatibility notes and pull requests are welcome. Please [open an issue](https://github.com/otfabric/dnp3ctl/issues) to discuss larger changes first, and run `make check` before submitting. See [CONTRIBUTING.md](CONTRIBUTING.md) for the development setup and guidelines.

Please report security vulnerabilities privately, as described in [SECURITY.md](SECURITY.md), not in a public issue.

If dnp3ctl is useful to you, a star on GitHub helps other engineers find it.

---

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE).
