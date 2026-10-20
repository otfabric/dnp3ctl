# Outstation model (`server start --config`)

`dnp3ctl server start` serves the outstation a model file describes. The file is YAML, or JSON (which is YAML). `dnp3ctl server generate-config` writes a commented example; this page is the reference.

A field the model does not know is an error, and everything that is wrong with a model is reported at once, before the server listens.

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
| `attributes` | Device attributes of set 0 (group 0) as strings, by name or variation number: `manufacturer`, `product`, `software-version`, `hardware-version`, `serial-number`, `device-name`, `location`, `owner`, `user-id`, … (the names `dnp3ctl client attributes` shows) |

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
| IMMED_FREEZE, FREEZE_CLEAR (and their `_NR` forms) | copies counters into frozen counters and analog inputs into frozen analog inputs; FREEZE_CLEAR resets the counters |
| READ / WRITE g0 | the attributes of `outstation.attributes`, and their list (g0v255) |
| File transfer (g70) | serves `files`, lists directories, takes uploads |
| Anything else | IIN2.0 (NO_FUNC_CODE_SUPPORT) |

The outstation serves any number of masters (16 connections by default, `--max-sessions`), each with its own association and event buffer. With `--sa-key-file` (or `DNP3CTL_SA_KEY`) critical requests (controls, writes, restarts, freezes, assign class, opening and deleting files, among others) are challenged by Secure Authentication v5 and executed only when the master answers with the right MAC.
