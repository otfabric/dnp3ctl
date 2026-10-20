package types

import (
	"strings"
	"testing"
)

type marshaler interface {
	MarshalTextOutput() (string, error)
	TableHeaders() []string
	TableRows() [][]string
}

func ptr[T any](v T) *T { return &v }

func TestRendering(t *testing.T) {
	m := Measurement{ReceivedAt: "2026-01-01T00:00:00.000Z", Unsolicited: true, Object: "g32v7", Group: 32, Variation: 7, Type: "analog-input",
		Index: 3, Event: true, Value: 1.5, Flags: "ONLINE", Timestamp: "2026-01-01T00:00:00.000Z"}
	cmd := Measurement{Object: "g13v2", Type: "binary-output-command", Value: true, Status: "LOCAL"}
	errInfo := &ErrorInfo{Kind: "refused", Code: "parameter_error", Message: "the outstation refused READ"}
	for _, c := range []struct {
		v    marshaler
		text []string
	}{
		{m, []string{"unsol", "g32v7", "1.5", "[ONLINE]", "@ 2026"}},
		{Measurement{ReceivedAt: "x", Object: "g1v2", Value: false}, []string{"resp", "off"}},
		{&PollResult{Request: "integrity-poll", IIN: "DEVICE_RESTART", Received: 2, Count: 1, Measurements: []Measurement{m}, Error: errInfo},
			[]string{"Integrity poll", "2 objects", "1 shown", "IIN: DEVICE_RESTART", "error: the outstation refused READ"}},
		{&PollResult{Request: "event-poll", Classes: []int{1, 2}}, []string{"Event poll of class 1,2"}},
		{&PollResult{Request: "class-poll", Classes: []int{0}}, []string{"Poll of class 0"}},
		{&PollResult{Request: "read", Objects: []string{"g30v5"}, Range: "0-9", Measurements: []Measurement{cmd}}, []string{"Read of g30v5 0-9", "LOCAL"}},
		{&OperateResult{Mode: "sbo", Type: "crob", Object: "g12v1", Op: "PULSE_ON", TripClose: "CLOSE", Count: 2, OnTimeMs: 100, Status: "SUCCESS"},
			[]string{"Operate (sbo) g12v1 index 0: PULSE_ON CLOSE count=2 on=100ms off=0ms", "SUCCESS"}},
		{&OperateResult{Mode: "direct", Type: "analog", Object: "g41v3", Value: ptr(1.25), Status: "SUCCESS"}, []string{"g41v3 index 0: 1.25"}},
		{&OperateResult{Type: "analog", Object: "g41v1"}, []string{"g41v1 index 0: 0"}},
		{&ConfirmedResult{Request: "time-sync", Method: "lan", Time: "T"}, []string{"time-sync (lan T)", "confirmed in"}},
		{&ConfirmedResult{Request: "cold-restart", DelayMs: ptr(2000.0)}, []string{"poll again after 2000 ms"}},
		{&ConfirmedResult{Request: "warm-restart"}, []string{"warm-restart ()"}},
		{&ConfirmedResult{Request: "enable-unsolicited", Classes: []int{1, 3}}, []string{"class 1,3"}},
		{&ConfirmedResult{Request: "assign-class", Class: ptr(2), Objects: []string{"g30v0"}, Range: "1"}, []string{"g30v0 1 to class 2"}},
		{&ConfirmedResult{Request: "assign-class", Objects: []string{"g1v0"}}, []string{"g1v0 to class 0"}},
		{&ConfirmedResult{Request: "freeze", Objects: []string{"g20v0"}, NoAck: true}, []string{"g20v0", "sent without acknowledgement"}},
		{&ConfirmedResult{Request: "delete-file", Path: "/x"}, []string{"delete-file (/x)"}},
		{&ConfirmedResult{Request: "other"}, []string{"other ()"}},
		{&PingSample{Seq: 1, RTTMs: 1.5, ProcessingMs: 2}, []string{"seq=1", "rtt=1.50 ms", "processing=2 ms"}},
		{&PingSample{Seq: 2, Error: errInfo}, []string{"seq=2", "error: the outstation refused READ"}},
		{&AttributesResult{Count: 3, Attributes: []Attribute{
			{Variation: 252, Name: "manufacturer", DataType: "VSTR", Value: "otfabric"},
			{Variation: 203, Name: "device-altitude", DataType: "FLT", Value: 12.5},
			{Variation: 255, Name: "list", DataType: "LIST", Value: []AttributeListEntry{{Variation: 252, Writable: true}, {Variation: 250}}},
		}}, []string{"Device attributes", "otfabric", "12.5", "252(rw) 250(r)"}},
		{&FileListResult{Path: "/", Count: 1, Entries: []FileEntry{{Name: "a", Type: "file", Size: 3, Permissions: "rw-r--r--"}}}, []string{"Directory / ", "rw-r--r-- file", " a"}},
		{&FileInfoResult{Path: "/a", Entry: FileEntry{Name: "a", Type: "file", Created: "T", Permissions: "rw-------"}}, []string{"File /a", "rw-------"}},
		{&FileTransferResult{Direction: "download", Path: "/a", Local: "a", Length: 3, SHA256: "ab"}, []string{"Fetched /a from", "sha256: ab", "local:  a"}},
		{&FileTransferResult{Direction: "upload", Path: "/a", Length: 3}, []string{"Delivered /a to"}},
		{&ObjectsResult{Count: 2, Objects: []ObjectInfo{{Object: "g30v5", Name: "Analog Input", Size: 5}, {Object: "g70v5", Name: "File"}}}, []string{"g30v5    Analog Input"}},
		{&DiscoverResult{Hosts: 2, Found: 1, Devices: []DiscoveredDevice{{IP: "10.0.0.1", Port: 20000, Status: "dnp3", Outstation: ptr(uint16(10)), Master: ptr(uint16(1))}, {IP: "10.0.0.2", Port: 20000, Status: "open"}}},
			[]string{"1 DNP3 outstations among 2 hosts", "outstation 10", "outstation -"}},
		{&ServerEvent{Timestamp: "T", Event: "control", Remote: "r", Function: "OPERATE", Object: "g12v1", Index: ptr(uint32(0)), Value: true, Status: "SUCCESS", Detail: "d"},
			[]string{"control", "OPERATE", "g12v1 index=0", "on", "SUCCESS", "d"}},
		{&ServerEvent{Timestamp: "T", Event: "listening"}, []string{"listening"}},
	} {
		text, err := c.v.MarshalTextOutput()
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range c.text {
			if !strings.Contains(text, w) {
				t.Errorf("%T text lacks %q:\n%s", c.v, w, text)
			}
		}
		h := c.v.TableHeaders()
		for _, row := range c.v.TableRows() {
			if len(row) != len(h) {
				t.Errorf("%T: a row of %d cells under %d headers", c.v, len(row), len(h))
			}
		}
	}
	if ErrorMessage(nil) != "" || ErrorMessage(errInfo) != errInfo.Message {
		t.Error("ErrorMessage")
	}
	if (Measurement{}).ValueText() != "-" || (Measurement{Value: uint8(3)}).ValueText() != "3" || valueText("x") != "x" {
		t.Error("ValueText")
	}
	if s := (PingSummary{Sent: 2, Failed: 2, Target: "t"}).String(); !strings.Contains(s, "2 sent, 2 failed") || strings.Contains(s, "rtt") {
		t.Errorf("summary: %s", s)
	}
	if s := (PingSummary{Sent: 2, Min: 1, Avg: 2, Max: 3, Target: "t"}).String(); !strings.Contains(s, "1.00/2.00/3.00") {
		t.Errorf("summary: %s", s)
	}
	if s := (MonitorSummary{Target: "t", Values: 3, Unsolicited: 1, Seconds: 2}).String(); s != "--- t: 3 values (1 unsolicited) in 2 s" {
		t.Errorf("monitor summary: %s", s)
	}
}
