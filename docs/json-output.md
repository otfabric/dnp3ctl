# `--format json` field conventions

These conventions are a **stable contract** for scripting. Field names are snake_case. A breaking change to a name or shape is called out in the release notes.

## General

| Concept | JSON fields |
|--------|-------------|
| Outstation | `target`: the URL of the outstation (`tcp://host:20000`, `tls://host:19999`, `udp://host:20000`, `serial:///dev/ttyUSB0`); with `--listen` the address the outstation connected from (`tcp://10.0.0.7:53122`); `outstation`: its link address (number) |
| Object | `object` (`"g30v5"`), with `group` and `variation` (numbers) where a value is reported |
| Point index | `index` (number) |
| Internal indications | `iin`: the set IIN bits of the last response fragment joined with `\|` (`"NEED_TIME\|DEVICE_RESTART"`), `""` when none is set |
| Times | RFC 3339 in UTC with milliseconds. `timestamp` is the time the outstation stamped an event or freeze with; `received_at` is when a value arrived |
| Durations | `duration_ms`, `rtt_ms`, `delay_ms`, `processing_ms` (numbers, milliseconds with fractions) |
| Repeated rows | `measurements`, `attributes`, `entries`, `objects`, `devices`, `controls` (client operate): **arrays of objects**, empty arrays rather than null |
| A failure inside a result | `error` object (`kind`, `code`, `message`); omitted when there is none |
| Fatal error | one object `{"error": {"kind", "code", "message"}}` on stdout, nothing else; see [exitcodes.md](exitcodes.md). A usage error (exit code 2: an unknown command or flag, a missing required flag, a value a flag cannot hold) is the same object with `"kind": "usage"` and `"code": "invalid_input"`, whether JSON is asked for by `--format json` or by `DNP3CTL_FORMAT=json` |

## Measurement

One point value or event as an outstation reported it. Used by `client poll`, `read` and `monitor`.

| Field | Meaning |
|-------|---------|
| `object`, `group`, `variation` | the object the value was sent as |
| `type` | the kind of point: `binary-input`, `double-bit-input`, `binary-output`, `binary-output-command`, `counter`, `frozen-counter`, `analog-input`, `frozen-analog-input`, `analog-deadband`, `analog-output`, `analog-output-command`, `octet-string`, `time-and-interval` (g50v4), `bcd` (g101), `unsigned-integer` (g102), `security-statistic`, `iin`; `g<n>` for another group |
| `index` | the point index |
| `event` | `true` for an object of an event group (g2, g4, g11, g13, g22, g23, g32, g33, g42, g43, g111, g122); omitted otherwise |
| `value` | boolean for binary points; string for double-bit inputs (`"off"`, `"on"`, `"intermediate"`, `"indeterminate"`) and octet strings (the text when it is printable, otherwise `"0x…"` hex); the time of a time and interval (RFC 3339); number for counters, analog values, BCD (the digits read as a decimal number: `0x1234` is `1234`) and unsigned integers, and security statistics. A value sent in a single-precision variation is the shortest decimal that variation holds (`11.02`) |
| `flags` | the quality flags that are set, joined with `\|`: `ONLINE`, `RESTART`, `COMM_LOST`, `REMOTE_FORCED`, `LOCAL_FORCED`, and by type `CHATTER_FILTER` (binary and double-bit inputs), `ROLLOVER`, `DISCONTINUITY` (counters), `OVER_RANGE`, `REFERENCE_ERR` (analog points); a set bit without a name for the type is `BIT5`, `BIT6` or `BIT7`. Omitted for variations without flags (packed and "without flag" variations) and when no flag is set |
| `status` | the command status of a command event (g13, g43), for example `"SUCCESS"` |
| `interval`, `interval_unit` | a time and interval (g50v4) only: the interval (number) and its unit: `"none"`, `"milliseconds"`, `"seconds"`, `"minutes"`, `"hours"`, `"days"`, `"weeks"`, `"months"`, `"months-day"`, `"months-last"`, `"months-same"` (`"unit-<n>"` for a code the standard does not define) |
| `statistic` | a security statistic (g121, g122) only: its name by index, `"unexpected messages"`, `"authorization failures"`, `"authentication failures"`, `"reply timeouts"`, `"rekeys due to authentication failure"`, `"total messages sent"`, `"total messages received"`, `"critical messages sent"`, `"critical messages received"`, `"discarded messages"`, `"error messages sent"`, `"error messages received"`, `"successful authentications"`, `"session key changes"`, `"failed session key changes"`, `"update key changes"`, `"failed update key changes"`, `"rekeys due to restarts"` |
| `timestamp` | the time of the event or freeze, for the variations with time |
| `received_at`, `unsolicited` | streams (`client monitor`) only: when the value arrived, and `true` when it came in an unsolicited response |

## Results

| Command | Shape |
|---------|-------|
| `client poll`, `client read` | `target`, `outstation`, `request` (`"integrity-poll"`, `"event-poll"`, `"class-poll"` or `"read"`), `classes` (polls), `objects` and `range` (read; `range` is `"0-9"`, `"1,4,7"`, `"count 5"` or omitted for all points), `iin`, `duration_ms`, `received` (values the outstation sent), `count` (values listed after `--object` / `--index`), `measurements`, and `error` when the outstation reported an IIN error while it did answer with values (exit code 7) |
| `client monitor` (stream) | one **Measurement** per line (JSON Lines), written as it arrives. Connection state and the summary go to stderr as text |
| `client operate` | `target`, `outstation`, `mode` (`"sbo"`, `"direct"`, `"direct-no-ack"`), `type` (`"crob"` or `"analog"`), `object` (`"g12v1"`, `"g41v1"` … `"g41v4"`), `index`; for a crob `op` (`"LATCH_ON"`, `"PULSE_ON"`, …), `trip_close` (`"NUL"`, `"CLOSE"`, `"TRIP"`), `count`, `on_time_ms`, `off_time_ms`; for an analog control `value`; `broadcast` (`"no-confirm"`, `"confirm"` or `"optional"`; omitted for a control to one outstation), `status` (the echoed command status, `"SUCCESS"`, or `"not-reported"` for direct-no-ack and broadcasts; with several controls `"SUCCESS"` when all succeeded, otherwise the first status that did not), `controls` (every control of the request: `index`, `value` for set-points, `status`; `index` and `value` above are those of the first), `rtt_ms`, and `error` when a direct operate of several controls was refused for some of them while the others were executed (exit code 7) |
| `client time-sync`, `restart`, `unsolicited enable\|disable`, `assign-class`, `freeze`, `write-attribute`, `write-deadband`, `manage`, `clear-restart`, `file rm` | `target`, `outstation`, `request` (`"time-sync"`, `"cold-restart"`, `"warm-restart"`, `"enable-unsolicited"`, `"disable-unsolicited"`, `"assign-class"`, `"freeze"`, `"freeze-clear"`, `"freeze-at-time"`, `"write-attribute"`, `"write-deadband"`, `"clear-restart"`, `"delete-file"`, or the management function of `manage`: `"initialize-data"`, `"initialize-application"`, `"start-application"`, `"stop-application"`, `"save-configuration"`), and where they apply `method` and `time` (time-sync), `delay_ms` (restart: when the outstation can be polled again), `classes` (unsolicited), `class`, `objects`, `range` (assign-class, freeze), `no_ack` (freeze), `time` and `interval` (freeze-at-time: the time of the freeze and its repeat interval, `"1h0m0s"`), `broadcast` (freeze sent to every outstation: `"no-confirm"`, `"confirm"` or `"optional"`), `function` (manage: the function code, `"SAVE_CONFIG"`), `objects`, `range` and `value` (write-deadband: the g34 variation sent, the indexes and the deadband), `set` and `attribute` (write-attribute: the set, and the attribute written, as an attribute of `client attributes`), `path` (file rm); `rtt_ms`. A broadcast is not answered: its `rtt_ms` is the time to send it |
| `client attributes` | `target`, `outstation`, `set` (the attribute set read; 0 is the standard set), `count`, `attributes`: `variation`, `name` (`"manufacturer"`, `"product"`, … in set 0, or `"attribute-<n>"`), `data_type` (`"VSTR"`, `"UINT"`, `"INT"`, `"FLT"`, `"OSTR"`, `"BSTR"`, `"TIME"`, `"LIST"`), `value` (a string, a number, an RFC 3339 time, `"0x…"` for octet and bit strings, or for the list of variations an array of `variation`, `name`, `writable`) |
| `client ping` (stream) | one object per measurement (JSON Lines): `seq` (1-based), `timestamp`, `rtt_ms`, `processing_ms` (the delay the outstation reported), and `error` on a failed measurement. The summary goes to stderr as text |
| `client file ls` | `target`, `outstation`, `path`, `count`, `entries`: `name`, `type` (`"file"` or `"directory"`), `size`, `created`, `permissions` (`"rw-r--r--"`) |
| `client file info` | `target`, `outstation`, `path`, `entry` (as an entry of `file ls`) |
| `client file get`, `client file put` | `target`, `outstation`, `direction` (`"download"` or `"upload"`), `path` (on the outstation), `local` (the local file; omitted for stdout), `append` (an upload added to the end of the file; omitted otherwise), `length`, `sha256` (of the content), `duration_ms` |
| `client objects` | `count`, `objects`: `object`, `group`, `variation`, `name`, `event`, `size` (octets of one object, 0 when not fixed) |
| `discover` | `subnets`, `ports`, `hosts` (addresses probed), `found`, `duration_ms`, `devices`: `ip`, `port`, `status` (`"dnp3"` or `"open"`), `outstation` and `master` (the link addresses of the answer; `"dnp3"` only), `rtt_ms` |
| `server start` (stream) | one event per line (JSON Lines): `timestamp`, `event`, and where they apply `outstation` (the link address of the outstation the event is about, when several are served), `remote`, `function` (`"READ"`, `"SELECT"`, `"OPERATE"`, …), `object`, `index`, `value`, `status`, `detail`. Events: `listening` (or `dialing` with `--dial`, naming the master in `remote`), `connected` (over UDP when the first datagram arrives, naming its sender, or at once with `--udp-peer`), `disconnected` (over UDP only after a `connected`), `refused`, `request`, `control`, `manage`, `unsolicited`, `time-sync`, `restart`, `write`, `file`, `events-discarded`, `error`, `stopping` |

## Streams and stderr

- A streaming command writes **compact** JSON, one object per line; every other command writes **one** indented JSON document.
- Progress, connection state, summaries and the `--debug` frame trace go to **stderr**, as text, in every format. stdout holds nothing but the result.
- `client file get --output -` is the exception that proves the rule: the file content takes stdout, and the result goes to stderr.
