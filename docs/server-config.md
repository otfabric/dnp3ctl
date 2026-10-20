# Outstation model (`server start --config`)

`dnp3ctl server start` serves the outstation a model file describes. The file is YAML, or JSON (which is YAML). `dnp3ctl server generate-config` writes a commented example; this page is the reference.

A field the model does not know is an error, and everything that is wrong with a model is reported at once, before the server listens.

`--config` can be given several times (or with comma-separated files): every model is then an outstation of its own, and all of them are served behind the same port, serial line or UDP socket, as on a multi-drop line. Each answers at the `address` of its model, which must differ between the models, and keeps its own points, event buffers, controls, files and secure authentication state; a broadcast reaches every one. Options of the command (`--allow`, `--max-sessions`, secure authentication keys, link settings) apply to all of them. `--dial` and `--udp-peer` take a single model.

```yaml
outstation:
  name: Feeder 7 RTU          # shown in the event log; not sent
  address: 10                 # link address, 0-65519 (default 10)
  masters: [1]                # optional: answer only these masters
  unsolicited: {enabled: true}
  time_sync_interval: 1h      # optional: ask for time synchronization
  restart_delay: 2s           # optional: accept cold and warm restarts
  attributes: {manufacturer: ACME, product: RTU-500}

points:                       # the point database
  - {type: binary-input, index: 0, name: Breaker closed, value: true, class: 1}
  - type: analog-input
    index: 0
    value: 11.02
    class: 2
    deadband: 0.01
    static_variation: 5
    simulate: {mode: sine, interval: 2s, min: 10.9, max: 11.1}

controls:                     # outputs masters can operate
  - {type: crob, index: 0, target: {type: binary-output, index: 0}, select_required: true}
  - {type: analog, index: 0, min: 10, max: 12}

files:
  - {name: /logs/events.csv, path: ./events.csv}

uploads:
  accept: true
  dir: ./uploads
```

## `outstation`

| Field | Meaning |
|-------|---------|
| `address` | Link address of the outstation, 0-65519. Default 10. A master must use it as `--outstation` |
| `name` | A label for the event log |
| `masters` | Link addresses of the masters answered. Without it any master is answered, and its association (with its event buffer) is created on first contact. With it the associations exist from the start, so events are kept while a master is away |
| `self_address` | Also answer frames sent to the self address 65532 |
| `unsolicited` | Unsolicited responses; see below |
| `time_sync_interval` | Set IIN1.4 (NEED_TIME) at start and again after this interval (at least `1s`), so that masters synchronize the clock |
| `select_timeout` | How long a SELECT stays valid for its OPERATE. Default `10s` |
| `confirm_timeout` | How long the outstation waits for an application confirmation. Default `5s` |
| `restart_delay` | The time a master is told to wait after a cold or warm restart. Without it restarts are refused (IIN2.0). A cold restart also puts every point back to its value in the model |
| `event_limit` | Events buffered per point type and master. Default 100; when a buffer is full the oldest event is dropped and IIN2.3 (EVENT_BUFFER_OVERFLOW) is set |
| `attributes` | Device attributes of set 0 (group 0) as strings, by name or variation number: `manufacturer`, `product`, `software-version`, `hardware-version`, `serial-number`, `device-name`, `location`, `owner`, `user-id`, … (the names `dnp3ctl client attributes` shows). Of these, the user-assigned ones a master may write: `owner`, `location`, `user-id` and `device-name` |
| `attribute_sets` | Further sets of device attributes; see below |
| `indications` | Internal indications the outstation reports from the start: `local-control`, `device-trouble`, `config-corrupt` |
| `refused_functions` | Management functions the outstation refuses with NO_FUNC_CODE_SUPPORT: `initialize-data`, `initialize-application`, `start-application`, `stop-application`, `save-configuration`. It accepts the others, and logs each as a `manage` event |

Durations are strings such as `500ms`, `5s` or `1h`.

### `unsolicited`

| Field | Meaning |
|-------|---------|
| `enabled` | Allow unsolicited responses. The outstation sends a null unsolicited response when a master connects, and reports the events of the classes the master enables with ENABLE_UNSOLICITED |
| `classes` | Classes 1-3 enabled from the start, before any ENABLE_UNSOLICITED ("unsolicited allowed at startup") |
| `hold_time` | Collect new events this long before sending them. Default: at once |
| `hold_count` | Send at once when this many events are pending |
| `retries` | Repeats of an unconfirmed unsolicited response before it is given up (its events are reported later). Default 3 |
| `retry_delay` | Pause after an unsolicited response was given up. Default `5s` |

### `attribute_sets`

Sets of device attributes other than set 0 (IEEE 1815-2012 clause 11.9.1), which a master reads with `client attributes --set` and writes with `client write-attribute --set`.

```yaml
outstation:
  attribute_sets:
    - set: 1
      attributes: {1: Feeder 7, 2: 11000, 3: 0.95}
      writable: [1]
```

| Field | Meaning |
|-------|---------|
| `set` | The set number, 1-65535 (set 0 is `attributes`). Required, unique |
| `attributes` | Variation numbers 1-253 and their values: a string (VSTR, 1-255 octets), a whole number (UINT, or INT when negative) or a number with a fraction (FLT). At least one |
| `writable` | The variations a master may write, with a value of the same type; the outstation keeps the written value. Other writes are refused with PARAMETER_ERROR |

## `points`

| Field | Meaning |
|-------|---------|
| `type` | The point type; see the table below. Required |
| `index` | The point index. Unique per type. Required |
| `value` | The initial value. Required |
| `name` | A description. DNP3 carries no names: it is for you |
| `flags` | Quality flags that are set. Default `[ONLINE]`; `[]` sets none (the point is offline). `ONLINE`, `RESTART`, `COMM_LOST`, `REMOTE_FORCED`, `LOCAL_FORCED`, and `CHATTER_FILTER` (binary), `ROLLOVER`, `DISCONTINUITY` (counters), `OVER_RANGE`, `REFERENCE_ERR` (analog) |
| `class` | Event class 1-3: a change of the point is an event of that class. 0 (default): no events; the point is still read in class 0 |
| `static_variation` | The variation reported in class 0 and for "any variation" reads. Default as below |
| `event_variation` | The variation of its events. Default as below |
| `deadband` | Counters and analog points: a change of the value is an event only when it exceeds the deadband. A change of the flags always is |
| `exclude_from_class0` | Leave the point out of class 0 responses; it can still be read by object |
| `command_event_class` | `binary-output` and `analog-output` only: report every executed control of this output's index (OPERATE, DIRECT_OPERATE, DIRECT_OPERATE_NR) as a command event of this class 1-3, with the commanded state or value and the status it was answered with. Default 0: none |
| `command_event_variation` | The variation of the command events: g13v1 or g13v2 for a binary output, g43v1 … g43v8 for an analog output. Default g13v2 or g43v3 (with time) |
| `interval`, `interval_unit` | `time-and-interval` only: the interval, a count of the unit; the unit is `none` (default), `milliseconds`, `seconds`, `minutes`, `hours`, `days`, `weeks`, `months`, `months-day`, `months-last` or `months-same` (IEEE 1815-2012 Table 11-23) |
| `simulate` | Makes the value change by itself; see below |

| `type` | Objects | Value | Default variations (static, event) |
|--------|---------|-------|------------------------------------|
| `binary-input` | g1, g2 | `true` / `false` (or `on` / `off`) | g1v2, g2v2 |
| `double-bit-input` | g3, g4 | `off`, `on`, `intermediate`, `indeterminate` (quote `"on"` and `"off"` in YAML) | g3v2, g4v2 |
| `binary-output` | g10, g11 | `true` / `false` | g10v2, g11v2 |
| `counter` | g20, g22 | whole number 0 … 4 294 967 295 | g20v1, g22v5 |
| `frozen-counter` | g21, g23 | whole number; a freeze copies the counter of the same index into it | g21v1, g23v5 |
| `analog-input` | g30, g32 | number | g30v1, g32v3 |
| `frozen-analog-input` | g31, g33 | number; a freeze copies the analog input of the same index | g31v1, g33v3 |
| `analog-output` | g40, g42 | number | g40v1, g42v3 |
| `octet-string` | g110, g111 | text of 1-255 octets | g110 with the length as variation |
| `time-and-interval` | g50v4 | an RFC 3339 time (`2026-01-04T02:00:00Z`), with `interval` and `interval_unit` | g50v4; no events |

A `time-and-interval` is a time with a repeat interval, such as a schedule. Like in the reference outstations it has no class and no events and is not part of class 0: masters read it by object (`client read --object g50v4`). `class`, `event_variation`, `deadband` and `simulate` do not apply to it.

A value is encoded in the variation a master asks for: an integer variation rounds it, and a value the variation cannot hold is sent as the nearest one with `OVER_RANGE` set.

### `simulate`

| Field | Meaning |
|-------|---------|
| `mode` | `toggle`, `ramp`, `sine`, `random` or `count` |
| `interval` | How often the value changes, at least `50ms` |
| `min`, `max` | Bounds of `ramp`, `sine` and `random` |
| `step` | Increment of `ramp` and `count` (default 1); period of `sine`, in intervals (default 60) |

| Mode | For | Behaviour |
|------|-----|-----------|
| `toggle` | binary inputs and outputs, double-bit inputs | flips between off and on |
| `ramp` | counters and analog points | adds `step` each interval; above `max` it starts again at `min` |
| `sine` | counters and analog points | a sine between `min` and `max` |
| `random` | counters and analog points | a uniformly distributed value between `min` and `max` |
| `count` | counters and analog points | adds `step`; a counter that passes 4 294 967 295 starts again at 0 |

A change produces an event of the point's class (when it has one and the change exceeds the deadband), stamped with the outstation's clock. Events are buffered per master until that master has read and confirmed them, also while it is not connected, and are reported in event polls and, when enabled, unsolicited.

## `controls`

| Field | Meaning |
|-------|---------|
| `type` | `crob` (control relay output block, g12v1) or `analog` (analog output block, g41v1-v4). Required |
| `index` | The index of the output. Unique per type. Required |
| `target` | The point that takes the commanded value: `{type, index}`. A crob sets a `binary-output` or `binary-input`, an analog control an `analog-output` or `analog-input`. Default: the `binary-output` (crob) or `analog-output` (analog) of the same index, when the model has one. Without a target the control is answered and nothing changes |
| `select_required` | Refuse a direct operate with NO_SELECT: the output only operates with select-before-operate |
| `min`, `max` | Analog controls: a set-point outside is refused with OUT_OF_RANGE |
| `status` | Answer every control of this output with this status, without executing it: `LOCAL`, `NOT_SUPPORTED`, `HARDWARE_ERROR`, `BLOCKED`, … (the names of IEEE 1815-2012 Table 11-4). For testing how a master handles a refusal |
| `name` | A description |

How the outstation handles a control:

- **SELECT**: answered with the status the control would get (SUCCESS, or the refusal), and remembered for `select_timeout`. The OPERATE must follow from the same master with the same control; otherwise it is answered with NO_SELECT.
- **OPERATE**, **DIRECT_OPERATE**: executed and answered with the status. A crob with `LATCH_ON`, `PULSE_ON` or trip-close code `CLOSE` sets its target on, `LATCH_OFF`, `PULSE_OFF` or `TRIP` off; `NUL` changes nothing. An analog control sets its target to the value. The change of the target produces an event like any other change.
- **DIRECT_OPERATE_NR**: executed, not answered.
- An index without a control is answered with NOT_SUPPORTED; a pattern control block (g12v2) as well.
- **Command events**: when the output of the control's index has a `command_event_class`, every OPERATE, DIRECT_OPERATE and DIRECT_OPERATE_NR is reported as a command event (g13 or g43) with the commanded state or value and the status it was answered with, also when the model refuses it.

## `files`

| Field | Meaning |
|-------|---------|
| `name` | The name masters use, a path such as `/logs/events.csv`. Required |
| `path` | A file on disk, relative to the model file |
| `content` | The content, as text |
| `size` | That many octets of a repeating pattern |
| `permissions` | Octal (`644`) or rwx form (`rw-r--r--`). Default `rw-r--r--` |

Give exactly one of `path`, `content` and `size`. Directories are implied by the names: `/logs/events.csv` makes `/logs` a directory, which `client file ls --path /logs` lists. The root `/` lists the top level. Files are listed with the time the server started. The content of `path` files is read when the server starts.

## `uploads`

| Field | Meaning |
|-------|---------|
| `accept` | Take the files masters write (`client file put`), and let them delete files (`client file rm`). Without it both are refused with PERMISSION_DENIED |
| `dir` | Also write files masters write to this directory (relative to the model file, created if missing), under their name; delete removes them again. Without it they are kept in memory |

A written file appears in its directory and can be read back.

## What the outstation answers

| Request | Answer |
|---------|--------|
| READ of classes 0-3 (integrity, event and class polls) | the static data of class 0; the buffered events of classes 1-3, confirmed by the master and then removed |
| READ of objects and ranges | the points, in the variation asked for or the static variation; PARAMETER_ERROR for indexes that do not exist, OBJECT_UNKNOWN for objects it does not have |
| SELECT, OPERATE, DIRECT_OPERATE, DIRECT_OPERATE_NR | as described under `controls` |
| WRITE g50v1, RECORD_CURRENT_TIME + WRITE g50v3, DELAY_MEASURE | sets the outstation's clock (logged as `time-sync`), clears NEED_TIME |
| WRITE g80v1 index 7 | clears DEVICE_RESTART |
| COLD_RESTART, WARM_RESTART | the delay of `restart_delay`, then the restart: DEVICE_RESTART is set again and the event buffers are cleared; or IIN2.0 without `restart_delay` |
| ENABLE_UNSOLICITED, DISABLE_UNSOLICITED | per master |
| ASSIGN_CLASS | changes the event class of the points |
| READ of g34 | the deadbands of the analog inputs (their `deadband`) |
| WRITE of g34 | sets the deadband of the analog inputs; PARAMETER_ERROR for an index that is not one |
| INITIALIZE_DATA, INITIALIZE_APPL, START_APPL, STOP_APPL, SAVE_CONFIG, ACTIVATE_CONFIG | accepted and logged (`manage` event), unless `refused_functions` lists them |
| FREEZE_AT_TIME | a freeze at a time to come, repeated at the interval when one is given; time 0 freezes at once. Up to 16 wait at once; a time that has passed, without an interval, is refused |
| A broadcast (to 0xFFFD, 0xFFFE or 0xFFFF) | executed like the same request to the outstation, not answered; IIN1.0 BROADCAST is set in the next response. With secure authentication a critical broadcast is executed only in aggressive mode with a valid MAC, otherwise discarded |
| IMMED_FREEZE, FREEZE_CLEAR (and their `_NR` forms) | copies counters into frozen counters and analog inputs into frozen analog inputs; FREEZE_CLEAR resets the counters |
| READ / WRITE g0 | the attributes of `outstation.attributes` and of `attribute_sets`, and their lists (g0v255); writes as described under `attributes` and `attribute_sets` |
| File transfer (g70) | serves `files`, lists directories, takes uploads |
| Anything else | IIN2.0 (NO_FUNC_CODE_SUPPORT) |

The outstation serves any number of masters (16 connections by default, `--max-sessions`), each with its own association and event buffer. Over UDP (`--udp`) there are no connections: the outstation answers the endpoint the last datagram came from, which the event log shows as `remote`, so one master is served at a time, and unsolicited responses start once a master has sent a request, unless `--udp-peer` names the master. With `--sa-update-key-file` (session key change) or `--sa-key-file` (a pre-shared session key), or their environment variables, critical requests (controls, writes, restarts, freezes, assign class, opening and deleting files, among others) are challenged by Secure Authentication v5 and executed only when the master answers with the right MAC, or when they carry it themselves in aggressive mode. The outstation then keeps the security statistics of each master (g121v1, read with `client read --object g121v1`) and reports a g122 event in class 1 every 10 increments of a statistic.

With `--dial tcp://host[:port]` or `--dial tls://host[:port]` the outstation does not listen: it connects to its master (the dual end point of IEEE 1815-2012 annex A.3), serves that connection like an accepted one, and connects again whenever it ends, after 1 second and doubling up to 1 minute between failed attempts. Over TLS it is the TLS client: `--tls-ca` verifies the master (the system roots by default), and `--tls-cert` and `--tls-key`, when given, are presented to it. A master that waits for it is `dnp3ctl client … --listen`.
