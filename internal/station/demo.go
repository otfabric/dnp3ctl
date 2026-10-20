package station

// DemoYAML is the model `dnp3ctl server start` serves when it is given none, and what
// `dnp3ctl server generate-config` writes: a small feeder RTU with a point of every type,
// controls for its outputs and a few files. It is kept as text so that the comments reach
// the generated file.
const DemoYAML = `# dnp3ctl outstation model.
# Serve it with:  dnp3ctl server start --config outstation.yaml
# Reference:      docs/server-config.md

outstation:
  name: Demo feeder RTU
  address: 10                 # link address of the outstation
  # masters: [1]              # answer only these masters (default: any)
  unsolicited:
    enabled: true             # masters enable classes with ENABLE_UNSOLICITED
  time_sync_interval: 1h      # ask for time synchronization (NEED_TIME) at start and hourly
  select_timeout: 10s
  restart_delay: 2s           # accept cold and warm restarts; masters wait this long
  attributes:
    manufacturer: otfabric
    product: dnp3ctl demo outstation
    software-version: "1.0"
    device-name: Feeder 7 RTU
    location: Substation North
  # Further attribute sets, defined by the outstation (read with --set): variation numbers
  # to values; "writable" variations can be written by a master.
  attribute_sets:
    - set: 1
      attributes: {1: Feeder 7, 2: 11000, 3: 0.95}
      writable: [1]

# The point database. "class" 1-3 makes a point produce events of that class when it
# changes (0: no events; it is still read in class 0). "simulate" makes a value change by
# itself. "flags" default to [ONLINE].
points:
  - {type: binary-input, index: 0, name: Breaker closed, value: true, class: 1}
  - type: binary-input
    index: 1
    name: Door open
    value: false
    class: 1
    simulate: {mode: toggle, interval: 15s}
  - {type: binary-input, index: 2, name: Protection trip, value: false, class: 1}
  - {type: double-bit-input, index: 0, name: Disconnector, value: "on", class: 1}
  # "command_event_class" reports every executed control of an output as a command event
  # (g13 for binary outputs, g43 for analog outputs) with the commanded value and status.
  - {type: binary-output, index: 0, name: Breaker control status, value: true, class: 1, command_event_class: 1}
  - {type: binary-output, index: 1, name: Reclose blocked, value: false, class: 1}
  - type: counter
    index: 0
    name: Energy (kWh)
    value: 123456
    class: 3
    simulate: {mode: count, interval: 10s, step: 1}
  - {type: frozen-counter, index: 0, name: Energy at freeze (kWh), value: 0}
  - type: analog-input
    index: 0
    name: Voltage L1 (kV)
    value: 11.02
    class: 2
    deadband: 0.01
    static_variation: 5       # g30v5: single-precision float with flag
    event_variation: 7        # g32v7: single-precision float with time
    simulate: {mode: sine, interval: 2s, min: 10.9, max: 11.1}
  - type: analog-input
    index: 1
    name: Current L1 (A)
    value: 212
    class: 2
    deadband: 1
    simulate: {mode: random, interval: 5s, min: 190, max: 230}
  - type: analog-input
    index: 2
    name: Active power (kW)
    value: 3900
    class: 2
    deadband: 10
    simulate: {mode: ramp, interval: 5s, min: 3500, max: 4500, step: 25}
  - {type: frozen-analog-input, index: 0, name: Active power at freeze (kW), value: 0}
  - type: analog-output
    index: 0
    name: Voltage set-point (kV)
    value: 11
    class: 2
    static_variation: 3
    command_event_class: 2
  - {type: octet-string, index: 0, name: Feeder name, value: FEEDER-7}
  # A time and interval (g50v4): a time with a repeat interval, such as a schedule. It has no
  # events and is not in class 0: read it with "client read --object g50v4".
  - type: time-and-interval
    index: 0
    name: Maintenance window
    value: 2026-01-04T02:00:00Z
    interval: 1
    interval_unit: weeks

# Outputs masters can operate. "target" is the point that takes the commanded value (by
# default the output status of the same index); "select_required" refuses a direct operate.
controls:
  - {type: crob, index: 0, name: Breaker, target: {type: binary-output, index: 0}, select_required: true}
  - {type: crob, index: 1, name: Reclose block}
  - {type: crob, index: 2, name: Local panel, status: LOCAL}
  - {type: analog, index: 0, name: Voltage set-point, min: 10, max: 12}

# Files masters can read (and list by directory). Give one of "path", "content" or "size".
files:
  - name: /readme.txt
    content: |
      dnp3ctl demo outstation
      This file is served by the file transfer of DNP3 (group 70).
  - {name: /logs/events.csv, content: "time,event\n2026-01-01T00:00:00Z,started\n"}
  - {name: /records/disturbance-0001.dat, size: 8192}

# Take the files masters write, and let them delete files. Add "dir: ./uploads" to keep them
# on disk.
uploads:
  accept: true
`

// Demo returns the built-in demo model.
func Demo() *Model {
	m, err := Parse([]byte(DemoYAML))
	if err == nil {
		err = m.Validate()
	}
	if err != nil {
		panic("dnp3ctl: the built-in demo model is invalid: " + err.Error())
	}
	return m
}
