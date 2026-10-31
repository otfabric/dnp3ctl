// SPDX-License-Identifier: MIT

package types

import (
	"fmt"
	"strconv"
	"strings"
)

// PollResult is the result of `client poll` and `client read`.
type PollResult struct {
	Target     string `json:"target"`
	Outstation uint16 `json:"outstation"`
	// Request is "integrity-poll", "event-poll", "class-poll" or "read".
	Request string `json:"request"`
	// Classes are the classes a poll read; Objects the objects a read asked for, with the
	// range or count it named.
	Classes []int    `json:"classes,omitempty"`
	Objects []string `json:"objects,omitempty"`
	Range   string   `json:"range,omitempty"`
	// IIN are the internal indications of the last response fragment.
	IIN        string  `json:"iin"`
	DurationMs float64 `json:"duration_ms"`
	// Received counts what the outstation sent; Count what is listed after filtering.
	Received     int           `json:"received"`
	Count        int           `json:"count"`
	Measurements []Measurement `json:"measurements"`
	// Error is set when the outstation reported an error in its IIN while it did answer
	// with values: the result is partial (exit code 7).
	Error *ErrorInfo `json:"error,omitempty"`
}

func (r *PollResult) title() string {
	switch r.Request {
	case "integrity-poll":
		return "Integrity poll"
	case "event-poll":
		return "Event poll of class " + joinInts(r.Classes)
	case "class-poll":
		return "Poll of class " + joinInts(r.Classes)
	default:
		what := "Read of " + strings.Join(r.Objects, ",")
		if r.Range != "" {
			what += " " + r.Range
		}
		return what
	}
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *PollResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "%s of outstation %d at %s: %d objects in %.0f ms", r.title(), r.Outstation, r.Target, r.Received, r.DurationMs)
	if r.Count != r.Received {
		_, _ = fmt.Fprintf(&b, ", %d shown", r.Count)
	}
	if r.IIN != "" {
		_, _ = fmt.Fprintf(&b, "  IIN: %s", r.IIN)
	}
	b.WriteString("\n")
	if r.Error != nil {
		_, _ = fmt.Fprintf(&b, "  error: %s\n", r.Error.Message)
	}
	measurementLines(&b, r.Measurements)
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *PollResult) TableHeaders() []string { return MeasurementHeaders }

// TableRows implements format.TableMarshaler.
func (r *PollResult) TableRows() [][]string { return measurementRows(r.Measurements) }

// OperateResult is the result of `client operate`.
type OperateResult struct {
	Target     string `json:"target"`
	Outstation uint16 `json:"outstation"`
	// Mode is "sbo", "direct" or "direct-no-ack".
	Mode string `json:"mode"`
	// Type is "crob" or "analog"; Object the group and variation sent ("g12v1", "g41v3").
	Type   string `json:"type"`
	Object string `json:"object"`
	Index  uint32 `json:"index"`
	// Op, TripClose, Count, OnTimeMs and OffTimeMs describe a crob.
	Op        string `json:"op,omitempty"`
	TripClose string `json:"trip_close,omitempty"`
	Count     uint8  `json:"count,omitempty"`
	OnTimeMs  uint32 `json:"on_time_ms,omitempty"`
	OffTimeMs uint32 `json:"off_time_ms,omitempty"`
	// Value is the set-point of an analog control.
	Value *float64 `json:"value,omitempty"`
	// Broadcast is the broadcast address the control went to instead of the outstation:
	// "no-confirm", "confirm" or "optional". Omitted for a control to one outstation.
	Broadcast string `json:"broadcast,omitempty"`
	// Status is the command status the outstation echoed: "SUCCESS", or "not-reported" for
	// direct-no-ack and broadcasts. With several controls it is SUCCESS when every control
	// succeeded, otherwise the first status that is not.
	Status string `json:"status"`
	// Controls lists every control of the request (Index and Value above are those of the
	// first) with its status.
	Controls []ControlStatus `json:"controls"`
	RTTMs    float64         `json:"rtt_ms"`
	// Error is set when a direct operate of several controls was refused for some of them:
	// the others were executed, and the result is partial (exit code 7).
	Error *ErrorInfo `json:"error,omitempty"`
}

// ControlStatus is one control of a request and the status the outstation echoed.
type ControlStatus struct {
	Index  uint32   `json:"index"`
	Value  *float64 `json:"value,omitempty"`
	Status string   `json:"status"`
}

// control describes the control, without its index: "LATCH_ON CLOSE count=1 on=…" for a
// crob, the value of an analog control.
func (r *OperateResult) control(value *float64) string {
	if r.Type == "crob" {
		s := r.Op
		if r.TripClose != "" && r.TripClose != "NUL" {
			s += " " + r.TripClose
		}
		if r.OnTimeMs != 0 || r.OffTimeMs != 0 || r.Count > 1 {
			s += fmt.Sprintf(" count=%d on=%dms off=%dms", r.Count, r.OnTimeMs, r.OffTimeMs)
		}
		return s
	}
	v := 0.0
	if value != nil {
		v = *value
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// indexes renders the indexes of the controls ("3", "0,1,4").
func (r *OperateResult) indexes() string {
	if len(r.Controls) == 0 {
		return strconv.FormatUint(uint64(r.Index), 10)
	}
	parts := make([]string, 0, len(r.Controls))
	for _, c := range r.Controls {
		parts = append(parts, strconv.FormatUint(uint64(c.Index), 10))
	}
	return strings.Join(parts, ",")
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *OperateResult) MarshalTextOutput() (string, error) {
	what := r.control(r.Value)
	if r.Type != "crob" && len(r.Controls) > 1 {
		what = "set-points"
	}
	s := fmt.Sprintf("Operate (%s) %s index %s: %s on %s at %s: %s in %.1f ms", r.Mode, r.Object, r.indexes(), what, recipient(r.Outstation, r.Broadcast), r.Target, r.Status, r.RTTMs)
	if len(r.Controls) > 1 {
		for _, c := range r.Controls {
			s += fmt.Sprintf("\n  index %-5d %-14s %s", c.Index, r.control(c.Value), c.Status)
		}
	}
	if r.Error != nil {
		s += "\n  error: " + r.Error.Message
	}
	return s, nil
}

// recipient names who a request went to: the outstation, or every outstation of a
// broadcast.
func recipient(outstation uint16, broadcast string) string {
	if broadcast != "" {
		return "every outstation (broadcast, " + broadcast + ")"
	}
	return "outstation " + strconv.Itoa(int(outstation))
}

// TableHeaders implements format.TableMarshaler.
func (r *OperateResult) TableHeaders() []string {
	return []string{"OBJECT", "INDEX", "CONTROL", "MODE", "STATUS", "RTT_MS"}
}

// TableRows implements format.TableMarshaler: a row per control.
func (r *OperateResult) TableRows() [][]string {
	controls := r.Controls
	if len(controls) == 0 {
		controls = []ControlStatus{{Index: r.Index, Value: r.Value, Status: r.Status}}
	}
	rows := make([][]string, 0, len(controls))
	for _, c := range controls {
		rows = append(rows, []string{r.Object, strconv.FormatUint(uint64(c.Index), 10), r.control(c.Value), r.Mode, c.Status, fmt.Sprintf("%.1f", r.RTTMs)})
	}
	return rows
}

// ConfirmedResult is the result of a request that is answered by a null response alone:
// `client time-sync`, `restart`, `unsolicited`, `assign-class`, `freeze` and `file rm`.
type ConfirmedResult struct {
	Target     string `json:"target"`
	Outstation uint16 `json:"outstation"`
	// Request is "time-sync", "cold-restart", "warm-restart", "enable-unsolicited",
	// "disable-unsolicited", "assign-class", "freeze", "freeze-clear", "freeze-at-time",
	// "write-attribute", "write-deadband", "clear-restart", "delete-file", or the
	// management function of `client manage` ("save-configuration", ...).
	Request string `json:"request"`
	// Method is the time synchronization procedure: "lan", "non-lan" or "write".
	Method string `json:"method,omitempty"`
	// Time is the time a time synchronization wrote, as this computer saw it, or the time of
	// a freeze-at-time; Interval the interval it repeats at.
	Time     string `json:"time,omitempty"`
	Interval string `json:"interval,omitempty"`
	// DelayMs is the time after which an outstation that restarts can be polled again.
	DelayMs *float64 `json:"delay_ms,omitempty"`
	// Classes are the classes unsolicited responses were enabled or disabled for; Class is
	// the class points were assigned to.
	Classes []int `json:"classes,omitempty"`
	Class   *int  `json:"class,omitempty"`
	// Objects and Range name the points of assign-class and freeze.
	Objects []string `json:"objects,omitempty"`
	Range   string   `json:"range,omitempty"`
	// NoAck is set when the request was sent in its no-acknowledgement form.
	NoAck bool `json:"no_ack,omitempty"`
	// Broadcast is the broadcast address the request went to: "no-confirm", "confirm" or
	// "optional". Nobody answers a broadcast.
	Broadcast string `json:"broadcast,omitempty"`
	// Function is the function code of a management function ("SAVE_CONFIG").
	Function string `json:"function,omitempty"`
	// Value is the deadband of write-deadband.
	Value *float64 `json:"value,omitempty"`
	// Set and Attribute are the attribute set and the attribute of write-attribute.
	Set       *int       `json:"set,omitempty"`
	Attribute *Attribute `json:"attribute,omitempty"`
	// Path is the file of delete-file.
	Path  string  `json:"path,omitempty"`
	RTTMs float64 `json:"rtt_ms"`
}

func (r *ConfirmedResult) detail() string {
	switch r.Request {
	case "time-sync":
		return r.Method + " " + r.Time
	case "cold-restart", "warm-restart":
		if r.DelayMs != nil {
			return fmt.Sprintf("poll again after %.0f ms", *r.DelayMs)
		}
	case "enable-unsolicited", "disable-unsolicited":
		return "class " + joinInts(r.Classes)
	case "assign-class":
		c := 0
		if r.Class != nil {
			c = *r.Class
		}
		return fmt.Sprintf("%s%s to class %d", strings.Join(r.Objects, ","), rangeSuffix(r.Range), c)
	case "freeze", "freeze-clear":
		return strings.Join(r.Objects, ",") + rangeSuffix(r.Range)
	case "freeze-at-time":
		s := strings.Join(r.Objects, ",") + rangeSuffix(r.Range) + " at " + r.Time
		if r.Interval != "" {
			s += " every " + r.Interval
		}
		return s
	case "write-attribute":
		if r.Set != nil && r.Attribute != nil {
			return fmt.Sprintf("set %d variation %d %s = %s", *r.Set, r.Attribute.Variation, r.Attribute.Name, r.Attribute.valueText())
		}
	case "delete-file":
		return r.Path
	case "write-deadband":
		v := 0.0
		if r.Value != nil {
			v = *r.Value
		}
		return fmt.Sprintf("%s%s = %s", strings.Join(r.Objects, ","), rangeSuffix(r.Range), strconv.FormatFloat(v, 'g', -1, 64))
	}
	if r.Function != "" {
		return r.Function
	}
	return ""
}

func rangeSuffix(rng string) string {
	if rng == "" {
		return ""
	}
	return " " + rng
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *ConfirmedResult) MarshalTextOutput() (string, error) {
	verdict := fmt.Sprintf("confirmed in %.1f ms", r.RTTMs)
	switch {
	case r.Broadcast != "":
		verdict = "sent; a broadcast is not answered"
	case r.NoAck:
		verdict = "sent without acknowledgement"
	}
	return fmt.Sprintf("%s (%s) of %s at %s: %s", r.Request, r.detail(), recipient(r.Outstation, r.Broadcast), r.Target, verdict), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *ConfirmedResult) TableHeaders() []string {
	return []string{"OUTSTATION", "REQUEST", "DETAIL", "RESULT", "RTT_MS"}
}

// TableRows implements format.TableMarshaler.
func (r *ConfirmedResult) TableRows() [][]string {
	result := "confirmed"
	if r.NoAck || r.Broadcast != "" {
		result = "sent"
	}
	return [][]string{{strconv.Itoa(int(r.Outstation)), r.Request, r.detail(), result, fmt.Sprintf("%.1f", r.RTTMs)}}
}

// PingSample is one delay measurement of `client ping`, written as it completes.
type PingSample struct {
	Seq       uint32  `json:"seq"`
	Timestamp string  `json:"timestamp"`
	RTTMs     float64 `json:"rtt_ms"`
	// ProcessingMs is the processing delay the outstation reported (g52v2).
	ProcessingMs float64    `json:"processing_ms"`
	Error        *ErrorInfo `json:"error,omitempty"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (s *PingSample) MarshalTextOutput() (string, error) {
	if s.Error != nil {
		return fmt.Sprintf("%s  seq=%d  error: %s", s.Timestamp, s.Seq, s.Error.Message), nil
	}
	return fmt.Sprintf("%s  seq=%d  rtt=%.2f ms  processing=%.0f ms", s.Timestamp, s.Seq, s.RTTMs, s.ProcessingMs), nil
}

// TableHeaders implements format.TableMarshaler.
func (s *PingSample) TableHeaders() []string {
	return []string{"TIMESTAMP", "SEQ", "RTT_MS", "PROCESSING_MS", "ERROR"}
}

// TableRows implements format.TableMarshaler.
func (s *PingSample) TableRows() [][]string {
	rtt, proc := fmt.Sprintf("%.2f", s.RTTMs), fmt.Sprintf("%.0f", s.ProcessingMs)
	if s.Error != nil {
		rtt, proc = "-", "-"
	}
	return [][]string{{s.Timestamp, strconv.FormatUint(uint64(s.Seq), 10), rtt, proc, ErrorMessage(s.Error)}}
}

// PingSummary is printed to stderr when `client ping` ends.
type PingSummary struct {
	Sent, Failed  int
	Min, Avg, Max float64
	Target        string
}

func (s PingSummary) String() string {
	if s.Sent == s.Failed {
		return fmt.Sprintf("--- %s delay measurement: %d sent, %d failed", s.Target, s.Sent, s.Failed)
	}
	return fmt.Sprintf("--- %s delay measurement: %d sent, %d failed, rtt min/avg/max = %.2f/%.2f/%.2f ms", s.Target, s.Sent, s.Failed, s.Min, s.Avg, s.Max)
}

// MonitorSummary is printed to stderr when `client monitor` ends.
type MonitorSummary struct {
	Target      string
	Values      int
	Unsolicited int
	Seconds     float64
}

func (s MonitorSummary) String() string {
	return fmt.Sprintf("--- %s: %d values (%d unsolicited) in %.0f s", s.Target, s.Values, s.Unsolicited, s.Seconds)
}

// Attribute is one device attribute.
type Attribute struct {
	Variation uint8  `json:"variation"`
	Name      string `json:"name"`
	// DataType is the data type code of the standard: "VSTR", "UINT", "INT", "FLT", "OSTR",
	// "BSTR", "TIME" or "LIST".
	DataType string `json:"data_type"`
	// Value is a string, a number or, for the list of attribute variations, an array of
	// {variation, name, writable}.
	Value any `json:"value"`
}

// AttributeListEntry is one entry of the list of attribute variations.
type AttributeListEntry struct {
	Variation uint8  `json:"variation"`
	Name      string `json:"name"`
	Writable  bool   `json:"writable"`
}

// AttributesResult is the result of `client attributes`.
type AttributesResult struct {
	Target     string `json:"target"`
	Outstation uint16 `json:"outstation"`
	// Set is the device attribute set read; 0 is the standard set.
	Set        uint16      `json:"set"`
	Count      int         `json:"count"`
	Attributes []Attribute `json:"attributes"`
}

func (a Attribute) valueText() string {
	switch v := a.Value.(type) {
	case []AttributeListEntry:
		parts := make([]string, 0, len(v))
		for _, e := range v {
			mode := "r"
			if e.Writable {
				mode = "rw"
			}
			parts = append(parts, fmt.Sprintf("%d(%s)", e.Variation, mode))
		}
		return strings.Join(parts, " ")
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *AttributesResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	set := ""
	if r.Set != 0 {
		set = fmt.Sprintf(" (set %d)", r.Set)
	}
	_, _ = fmt.Fprintf(&b, "Device attributes%s of outstation %d at %s: %d\n", set, r.Outstation, r.Target, r.Count)
	for _, a := range r.Attributes {
		_, _ = fmt.Fprintf(&b, "  %3d  %-34s %-5s %s\n", a.Variation, a.Name, a.DataType, a.valueText())
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *AttributesResult) TableHeaders() []string {
	return []string{"VARIATION", "NAME", "TYPE", "VALUE"}
}

// TableRows implements format.TableMarshaler.
func (r *AttributesResult) TableRows() [][]string {
	rows := make([][]string, 0, len(r.Attributes))
	for _, a := range r.Attributes {
		rows = append(rows, []string{strconv.Itoa(int(a.Variation)), a.Name, a.DataType, a.valueText()})
	}
	return rows
}

// FileEntry is one entry of a directory of the outstation, or the answer to file info.
type FileEntry struct {
	Name string `json:"name"`
	// Type is "file" or "directory".
	Type        string `json:"type"`
	Size        uint32 `json:"size"`
	Created     string `json:"created,omitempty"`
	Permissions string `json:"permissions"`
}

func (e FileEntry) line() string {
	return fmt.Sprintf("%s %-9s %10d  %-24s  %s", e.Permissions, e.Type, e.Size, orDash(e.Created), e.Name)
}

func (e FileEntry) row() []string {
	return []string{e.Name, e.Type, strconv.FormatUint(uint64(e.Size), 10), orDash(e.Created), e.Permissions}
}

var fileHeaders = []string{"NAME", "TYPE", "SIZE", "CREATED", "PERMISSIONS"}

// FileListResult is the result of `client file ls`.
type FileListResult struct {
	Target     string      `json:"target"`
	Outstation uint16      `json:"outstation"`
	Path       string      `json:"path"`
	Count      int         `json:"count"`
	Entries    []FileEntry `json:"entries"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *FileListResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Directory %s of outstation %d at %s: %d entries\n", r.Path, r.Outstation, r.Target, r.Count)
	for _, e := range r.Entries {
		b.WriteString("  " + e.line() + "\n")
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *FileListResult) TableHeaders() []string { return fileHeaders }

// TableRows implements format.TableMarshaler.
func (r *FileListResult) TableRows() [][]string {
	rows := make([][]string, 0, len(r.Entries))
	for _, e := range r.Entries {
		rows = append(rows, e.row())
	}
	return rows
}

// FileInfoResult is the result of `client file info`.
type FileInfoResult struct {
	Target     string    `json:"target"`
	Outstation uint16    `json:"outstation"`
	Path       string    `json:"path"`
	Entry      FileEntry `json:"entry"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *FileInfoResult) MarshalTextOutput() (string, error) {
	return fmt.Sprintf("File %s of outstation %d at %s:\n  %s", r.Path, r.Outstation, r.Target, r.Entry.line()), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *FileInfoResult) TableHeaders() []string { return fileHeaders }

// TableRows implements format.TableMarshaler.
func (r *FileInfoResult) TableRows() [][]string { return [][]string{r.Entry.row()} }

// FileTransferResult is the result of `client file get` and `client file put`.
type FileTransferResult struct {
	Target     string `json:"target"`
	Outstation uint16 `json:"outstation"`
	// Direction is "download" or "upload".
	Direction string `json:"direction"`
	// Path is the file on the outstation; Local the local file (empty for stdout).
	Path  string `json:"path"`
	Local string `json:"local,omitempty"`
	// Append is set for an upload added to the end of an existing file.
	Append     bool    `json:"append,omitempty"`
	Length     int64   `json:"length"`
	SHA256     string  `json:"sha256"`
	DurationMs float64 `json:"duration_ms"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *FileTransferResult) MarshalTextOutput() (string, error) {
	verb, where := "Fetched", "from"
	switch {
	case r.Append:
		verb, where = "Appended", "to"
	case r.Direction == "upload":
		verb, where = "Delivered", "to"
	}
	s := fmt.Sprintf("%s %s %s outstation %d at %s: %d octets in %.0f ms\n  sha256: %s", verb, r.Path, where, r.Outstation, r.Target, r.Length, r.DurationMs, r.SHA256)
	if r.Local != "" {
		s += "\n  local:  " + r.Local
	}
	return s, nil
}

// TableHeaders implements format.TableMarshaler.
func (r *FileTransferResult) TableHeaders() []string {
	return []string{"DIRECTION", "PATH", "LENGTH", "SHA256", "LOCAL"}
}

// TableRows implements format.TableMarshaler.
func (r *FileTransferResult) TableRows() [][]string {
	return [][]string{{r.Direction, r.Path, strconv.FormatInt(r.Length, 10), r.SHA256, orDash(r.Local)}}
}

// ObjectInfo describes one group and variation.
type ObjectInfo struct {
	Object    string `json:"object"`
	Group     uint8  `json:"group"`
	Variation uint8  `json:"variation"`
	Name      string `json:"name"`
	Event     bool   `json:"event"`
	// Size is the size of one object in octets; 0 when it is not fixed.
	Size int `json:"size"`
}

// ObjectsResult is the result of `client objects`.
type ObjectsResult struct {
	Count   int          `json:"count"`
	Objects []ObjectInfo `json:"objects"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *ObjectsResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	for _, o := range r.Objects {
		_, _ = fmt.Fprintf(&b, "%-8s %s\n", o.Object, o.Name)
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *ObjectsResult) TableHeaders() []string {
	return []string{"OBJECT", "GROUP", "VARIATION", "EVENT", "SIZE", "NAME"}
}

// TableRows implements format.TableMarshaler.
func (r *ObjectsResult) TableRows() [][]string {
	rows := make([][]string, 0, len(r.Objects))
	for _, o := range r.Objects {
		size := "-"
		if o.Size > 0 {
			size = strconv.Itoa(o.Size)
		}
		rows = append(rows, []string{o.Object, strconv.Itoa(int(o.Group)), strconv.Itoa(int(o.Variation)), strconv.FormatBool(o.Event), size, o.Name})
	}
	return rows
}

// DiscoveredDevice is one address and port that answered `discover`.
type DiscoveredDevice struct {
	IP   string `json:"ip"`
	Port uint16 `json:"port"`
	// Status is "dnp3" (answered the link status request) or "open" (the port accepts
	// connections but did not answer as a DNP3 outstation).
	Status string `json:"status"`
	// Outstation is the link address the outstation answered from; Master the address it
	// answered to. Set for "dnp3".
	Outstation *uint16 `json:"outstation,omitempty"`
	Master     *uint16 `json:"master,omitempty"`
	RTTMs      float64 `json:"rtt_ms"`
}

// DiscoverResult is the result of `discover`.
type DiscoverResult struct {
	Subnets    []string           `json:"subnets"`
	Ports      []uint16           `json:"ports"`
	Hosts      int                `json:"hosts"`
	Found      int                `json:"found"`
	DurationMs float64            `json:"duration_ms"`
	Devices    []DiscoveredDevice `json:"devices"`
}

func (d DiscoveredDevice) addressText() string {
	if d.Outstation == nil {
		return "-"
	}
	return strconv.Itoa(int(*d.Outstation))
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *DiscoverResult) MarshalTextOutput() (string, error) {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Discovery: %d DNP3 outstations among %d hosts in %.0f ms\n", r.Found, r.Hosts, r.DurationMs)
	for _, d := range r.Devices {
		_, _ = fmt.Fprintf(&b, "  %-15s port %-5d  %-5s  outstation %-5s  %.1f ms\n", d.IP, d.Port, d.Status, d.addressText(), d.RTTMs)
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (r *DiscoverResult) TableHeaders() []string {
	return []string{"IP", "PORT", "STATUS", "OUTSTATION", "RTT_MS"}
}

// TableRows implements format.TableMarshaler.
func (r *DiscoverResult) TableRows() [][]string {
	rows := make([][]string, 0, len(r.Devices))
	for _, d := range r.Devices {
		rows = append(rows, []string{d.IP, strconv.Itoa(int(d.Port)), d.Status, d.addressText(), fmt.Sprintf("%.1f", d.RTTMs)})
	}
	return rows
}

// ServerEvent is one line of the event log of `server start`.
type ServerEvent struct {
	Timestamp string `json:"timestamp"`
	// Event is "listening", "connected", "disconnected", "refused", "request", "control",
	// "manage", "unsolicited", "time-sync", "restart", "write", "file", "events-discarded",
	// "stopping" or "error".
	Event string `json:"event"`
	// Outstation is the link address of the outstation an event is about, when several
	// outstations share the connections.
	Outstation *uint16 `json:"outstation,omitempty"`
	Remote     string  `json:"remote,omitempty"`
	// Function is the function code of a request ("READ", "SELECT", "OPERATE", ...).
	Function string `json:"function,omitempty"`
	// Object and Index name the point an event is about.
	Object string  `json:"object,omitempty"`
	Index  *uint32 `json:"index,omitempty"`
	Value  any     `json:"value,omitempty"`
	// Status is the command status a control was answered with.
	Status string `json:"status,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (e *ServerEvent) MarshalTextOutput() (string, error) {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "%s  %-16s", e.Timestamp, e.Event)
	if e.Outstation != nil {
		_, _ = fmt.Fprintf(&b, "  [%d]", *e.Outstation)
	}
	if e.Remote != "" {
		_, _ = fmt.Fprintf(&b, "  %s", e.Remote)
	}
	if e.Function != "" {
		_, _ = fmt.Fprintf(&b, "  %s", e.Function)
	}
	if e.Object != "" {
		_, _ = fmt.Fprintf(&b, "  %s", e.Object)
	}
	if e.Index != nil {
		_, _ = fmt.Fprintf(&b, " index=%d", *e.Index)
	}
	if e.Value != nil {
		_, _ = fmt.Fprintf(&b, "  %s", valueText(e.Value))
	}
	if e.Status != "" {
		_, _ = fmt.Fprintf(&b, "  %s", e.Status)
	}
	if e.Detail != "" {
		_, _ = fmt.Fprintf(&b, "  %s", e.Detail)
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (e *ServerEvent) TableHeaders() []string {
	return []string{"TIMESTAMP", "EVENT", "OUTSTATION", "REMOTE", "FUNCTION", "OBJECT", "INDEX", "VALUE", "STATUS", "DETAIL"}
}

// TableRows implements format.TableMarshaler.
func (e *ServerEvent) TableRows() [][]string {
	outstation, index, value := "", "", ""
	if e.Outstation != nil {
		outstation = strconv.Itoa(int(*e.Outstation))
	}
	if e.Index != nil {
		index = strconv.FormatUint(uint64(*e.Index), 10)
	}
	if e.Value != nil {
		value = valueText(e.Value)
	}
	return [][]string{{e.Timestamp, e.Event, outstation, e.Remote, e.Function, e.Object, index, value, e.Status, e.Detail}}
}

func valueText(v any) string {
	return Measurement{Value: v}.ValueText()
}

func joinInts(ns []int) string {
	parts := make([]string, 0, len(ns))
	for _, n := range ns {
		parts = append(parts, strconv.Itoa(n))
	}
	return strings.Join(parts, ",")
}
