package dnp

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	dnp3 "github.com/otfabric/go-dnp3"
	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/client"
	"github.com/otfabric/go-dnp3/link"

	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/errs"
	"github.com/otfabric/dnp3ctl/internal/types"
)

func TestParseObject(t *testing.T) {
	for in, want := range map[string]app.GroupVariation{
		"g30v5": app.G30V5, "G30V5": app.G30V5, "30.5": app.G30V5, "30:5": app.G30V5, "30v5": app.G30V5,
		"g30": app.G30V0, "30": app.G30V0, " g1v2 ": app.G1V2, "g80v1": app.G80V1,
	} {
		if got, err := ParseObject(in); err != nil || got != want {
			t.Errorf("ParseObject(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "g", "gx", "g30vx", "g300", "g30v300", "g5", "g30v99"} {
		if got, err := ParseObject(in); err == nil {
			t.Errorf("ParseObject(%q) = %v, want an error", in, got)
		}
	}
	gvs, err := ParseObjects("g1, ,g30v5")
	if err != nil || len(gvs) != 2 || strings.Join(ObjectNames(gvs), ",") != "g1v0,g30v5" {
		t.Errorf("ParseObjects: %v, %v", gvs, err)
	}
	if _, err := ParseObjects("g1,nope"); err == nil {
		t.Error("ParseObjects accepted a bad object")
	}
}

func TestSelection(t *testing.T) {
	for _, c := range []struct {
		rng   string
		count uint32
		text  string
		qual  app.Qualifier
	}{
		{"", 0, "", app.Qual1ByteStartStop | 0x06},
		{"0-9", 0, "0-9", app.Qual1ByteStartStop},
		{"5", 0, "5", app.Qual1ByteStartStop},
		{"1,4,7", 0, "1,4,7", 0x17},
		{"", 5, "count 5", 0x07},
	} {
		s, err := ParseSelection(c.rng, c.count)
		if err != nil || s.String() != c.text {
			t.Errorf("ParseSelection(%q, %d) = %+v (%q), %v", c.rng, c.count, s, s.String(), err)
			continue
		}
		hs := s.Headers([]app.GroupVariation{app.G30V0, app.G1V0})
		if len(hs) != 2 || hs[0].GV != app.G30V0 {
			t.Errorf("headers of %q: %+v", c.rng, hs)
		}
		if _, err := hs[0].EncodedLen(); err != nil {
			t.Errorf("header of %q does not encode: %v", c.rng, err)
		}
	}
	for _, in := range []string{"x", "9-1", "1-x", "1,x", "-"} {
		if _, err := ParseSelection(in, 0); err == nil {
			t.Errorf("ParseSelection(%q) was accepted", in)
		}
	}
	if _, err := ParseSelection("1", 1); err == nil {
		t.Error("--range with --count was accepted")
	}
}

func TestFiltersClassesTimePermissions(t *testing.T) {
	f, err := ParseIndexFilter("0, 5-9")
	if err != nil || !f.Match(0) || !f.Match(7) || f.Match(3) || f.Match(10) {
		t.Errorf("index filter: %+v, %v", f, err)
	}
	if all, _ := ParseIndexFilter(""); !all.Match(12345) {
		t.Error("an empty index filter does not match everything")
	}
	for _, in := range []string{"x", "5-1", "1-y"} {
		if _, err := ParseIndexFilter(in); err == nil {
			t.Errorf("ParseIndexFilter(%q) was accepted", in)
		}
	}
	of, err := ParseObjectFilter("g30,g1v2")
	if err != nil || !of.Match(app.G30V5) || !of.Match(app.G1V2) || of.Match(app.G1V1) || of.Match(app.G32V1) {
		t.Errorf("object filter: %+v, %v", of, err)
	}
	if all, _ := ParseObjectFilter(""); !all.Match(app.G30V1) {
		t.Error("an empty object filter does not match everything")
	}
	if _, err := ParseObjectFilter("x"); err == nil {
		t.Error("a bad object filter was accepted")
	}

	if c, err := ParseClasses("3, 1,3", 1, nil); err != nil || fmt.Sprint(c) != "[3 1]" {
		t.Errorf("classes: %v, %v", c, err)
	}
	if c, _ := ParseClasses(" ", 0, []int{0}); fmt.Sprint(c) != "[0]" {
		t.Errorf("default classes: %v", c)
	}
	for _, in := range []string{"0", "4", "x"} {
		if _, err := ParseClasses(in, 1, nil); err == nil {
			t.Errorf("ParseClasses(%q) was accepted", in)
		}
	}

	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	clock := func() time.Time { return now }
	if got, err := ParseTime("now", clock); err != nil || !got.Equal(now) {
		t.Errorf("now: %v, %v", got, err)
	}
	if got, err := ParseTime("2026-01-31T12:00:00.5+01:00", clock); err != nil || got.UTC().Hour() != 11 {
		t.Errorf("RFC 3339: %v, %v", got, err)
	}
	if _, err := ParseTime("tomorrow", clock); err == nil {
		t.Error("a bad time was accepted")
	}

	for in, want := range map[string]app.Permissions{"": 0o644, "600": 0o600, "0755": 0o755, "rwxr-x---": 0o750, "---------": 0} {
		if got, err := ParsePermissions(in); err != nil || got != want {
			t.Errorf("ParsePermissions(%q) = %o, %v; want %o", in, got, err, want)
		}
	}
	for _, in := range []string{"999", "1777", "rwxrwxrwz", "rw-", "abc"} {
		if _, err := ParsePermissions(in); err == nil {
			t.Errorf("ParsePermissions(%q) was accepted", in)
		}
	}
}

func TestMeasurements(t *testing.T) {
	ts := app.TimestampOf(time.Date(2026, 1, 2, 3, 4, 5, 6e6, time.UTC))
	fifteen := uint32(15)
	for _, c := range []struct {
		m    client.Measurement
		want types.Measurement
	}{
		{client.Measurement{GV: app.G1V2, Index: 3, Value: app.Binary{Value: true, Flags: app.FlagOnline | app.FlagChatterFilter}},
			types.Measurement{Object: "g1v2", Group: 1, Variation: 2, Type: "binary-input", Index: 3, Value: true, Flags: "ONLINE|CHATTER_FILTER"}},
		{client.Measurement{GV: app.G1V1, Value: app.Binary{Value: false}},
			types.Measurement{Object: "g1v1", Group: 1, Variation: 1, Type: "binary-input", Value: false}},
		{client.Measurement{GV: app.G2V2, Event: true, Value: app.Binary{Value: true, Flags: app.FlagOnline, Time: ts}},
			types.Measurement{Object: "g2v2", Group: 2, Variation: 2, Type: "binary-input", Event: true, Value: true, Flags: "ONLINE", Timestamp: "2026-01-02T03:04:05.006Z"}},
		{client.Measurement{GV: app.G3V2, Value: app.DoubleBitBinary{Value: app.DoubleBitIntermediate, Flags: app.FlagRestart}},
			types.Measurement{Object: "g3v2", Group: 3, Variation: 2, Type: "double-bit-input", Value: "intermediate", Flags: "RESTART"}},
		{client.Measurement{GV: app.G20V1, Value: app.Counter{Value: 7, Flags: app.FlagOnline | app.FlagDiscontinuity | 0x80}},
			types.Measurement{Object: "g20v1", Group: 20, Variation: 1, Type: "counter", Value: float64(7), Flags: "ONLINE|DISCONTINUITY|BIT7"}},
		{client.Measurement{GV: app.G20V5, Value: app.Counter{Value: 8}},
			types.Measurement{Object: "g20v5", Group: 20, Variation: 5, Type: "counter", Value: float64(8)}},
		{client.Measurement{GV: app.G30V5, Value: app.Analog{Value: float64(float32(11.02)), Flags: app.FlagOnline | app.FlagOverRange | app.FlagReferenceErr}},
			types.Measurement{Object: "g30v5", Group: 30, Variation: 5, Type: "analog-input", Value: 11.02, Flags: "ONLINE|OVER_RANGE|REFERENCE_ERR"}},
		{client.Measurement{GV: app.G34V1, Value: app.AnalogDeadband{Value: 2}},
			types.Measurement{Object: "g34v1", Group: 34, Variation: 1, Type: "analog-deadband", Value: float64(2)}},
		{client.Measurement{GV: app.G13V2, Event: true, Value: app.BinaryCommandEvent{Value: true, Status: app.StatusLocal, Time: ts}},
			types.Measurement{Object: "g13v2", Group: 13, Variation: 2, Type: "binary-output-command", Event: true, Value: true, Status: "LOCAL", Timestamp: "2026-01-02T03:04:05.006Z"}},
		{client.Measurement{GV: app.G43V1, Event: true, Value: app.AnalogCommandEvent{Value: 1.5, Status: app.StatusSuccess}},
			types.Measurement{Object: "g43v1", Group: 43, Variation: 1, Type: "analog-output-command", Event: true, Value: 1.5, Status: "SUCCESS"}},
		{client.Measurement{GV: app.OctetStringGV(110, 2), Value: app.OctetString{Data: []byte{0, 1}}},
			types.Measurement{Object: "g110v2", Group: 110, Variation: 2, Type: "octet-string", Value: "0x0001"}},
		{client.Measurement{GV: app.G121V1, Value: app.SecurityStatistic{Value: 3, Flags: app.FlagOnline}},
			types.Measurement{Object: "g121v1", Group: 121, Variation: 1, Type: "security-statistic", Value: float64(3), Flags: "ONLINE", Statistic: "unexpected messages"}},
		{client.Measurement{GV: app.G50V4, Index: 2, Value: app.AbsoluteTime{Time: ts, Interval: 15, Units: app.UnitMinutes}},
			types.Measurement{Object: "g50v4", Group: 50, Variation: 4, Type: "time-and-interval", Index: 2, Value: "2026-01-02T03:04:05.006Z", Interval: &fifteen, IntervalUnit: "minutes"}},
		{client.Measurement{GV: app.GV(101, 3), Index: 2, Value: app.BCD{Value: 9999999999999999}},
			types.Measurement{Object: "g101v3", Group: 101, Variation: 3, Type: "bcd", Index: 2, Value: uint64(9999999999999999)}},
		{client.Measurement{GV: app.GV(102, 1), Value: app.UnsignedInteger{Value: 200}},
			types.Measurement{Object: "g102v1", Group: 102, Variation: 1, Type: "unsigned-integer", Value: float64(200)}},
		{client.Measurement{GV: app.G50V1, Value: app.AbsoluteTime{}},
			types.Measurement{Object: "g50v1", Group: 50, Variation: 1, Type: "g50"}},
	} {
		if got := ToMeasurement(c.m); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ToMeasurement(%v):\n got %+v\nwant %+v", c.m.GV, got, c.want)
		}
	}
	for d, want := range map[app.DoubleBit]string{app.DoubleBitOff: "off", app.DoubleBitOn: "on", app.DoubleBitIndeterminate: "indeterminate"} {
		if DoubleBitText(d) != want {
			t.Errorf("DoubleBitText(%v) = %s", d, DoubleBitText(d))
		}
	}
	if octetText(nil) != "0x" || octetText([]byte("ok go")) != "ok go" {
		t.Error("octetText")
	}
	if FlagsText("octet-string", 0x60) != "BIT5|BIT6" {
		t.Errorf("FlagsText of an unnamed kind: %s", FlagsText("x", 0x60))
	}
	if CROBText(app.CROB{Op: app.OpPulseOn, TripClose: app.TCCTrip, Count: 1, OnTime: 100}) != "PULSE_ON TRIP count=1 on=100ms off=0ms" ||
		CROBText(app.CROB{Op: app.OpLatchOn, Count: 1}) != "LATCH_ON" {
		t.Error("CROBText")
	}
}

func TestAttributes(t *testing.T) {
	for in, want := range map[string]uint8{"": app.AttrAllAttributes, "all": 254, "list": 255, "manufacturer": 252, " Product ": 250, "211": 211} {
		if got, err := ParseAttributeVariation(in); err != nil || got != want {
			t.Errorf("ParseAttributeVariation(%q) = %d, %v", in, got, err)
		}
	}
	for _, in := range []string{"0", "256", "colour"} {
		if _, err := ParseAttributeVariation(in); err == nil {
			t.Errorf("ParseAttributeVariation(%q) was accepted", in)
		}
	}
	if AttributeName(252) != "manufacturer" || AttributeName(5) != "attribute-5" {
		t.Error("AttributeName")
	}
	ts := app.Timestamp(1767225600000)
	for _, c := range []struct {
		a    app.Attribute
		want string
	}{
		{app.StringAttribute("otfabric"), "otfabric"},
		{app.UintAttribute(70000), "70000"},
		{app.IntAttribute(-3), "-3"},
		{app.FloatAttribute(1.5), "1.5"},
		{app.TimeAttribute(ts), "2026-01-01T00:00:00.000Z"},
		{app.Attribute{Type: app.AttrOctetString, Data: []byte{1, 2}}, "0x0102"},
		{app.Attribute{Type: app.AttrUnsignedInt, Data: []byte{1, 2, 3}}, "0x010203"},
		{app.Attribute{Type: app.AttrTime, Data: []byte{1}}, "0x01"},
		{app.ListAttribute(app.AttributeListEntry{Variation: 252, Writable: true}), "[{252 manufacturer true}]"},
	} {
		if got := fmt.Sprint(attributeValue(c.a)); got != c.want {
			t.Errorf("attributeValue(%v) = %s, want %s", c.a.Type, got, c.want)
		}
	}
}

func TestWrapError(t *testing.T) {
	for _, c := range []struct {
		err        error
		kind, code string
	}{
		{&dnp3.CommandError{GV: app.G12V1, Index: 1, Status: app.StatusLocal}, "refused", errs.CodeCommandRefused},
		{&dnp3.IINError{Function: app.FuncRead, IIN: app.IINObjectUnknown}, "refused", errs.CodeObjectUnknown},
		{&dnp3.IINError{Function: app.FuncRead, IIN: app.IINNoFuncCodeSupport}, "refused", errs.CodeNoFuncCodeSupport},
		{&dnp3.IINError{Function: app.FuncRead, IIN: app.IINParameterError}, "refused", errs.CodeParameterError},
		{&dnp3.IINError{Function: app.FuncOperate, IIN: app.IINAlreadyExecuting}, "refused", errs.CodeAlreadyExecuting},
		{&dnp3.FileError{Op: "open", Status: app.FileNotFound, Text: "gone"}, "refused", errs.CodeFileRefused},
		{&dnp3.AuthError{Code: app.AuthErrAuthenticationFailed, User: 1}, "refused", errs.CodeAuthenticationFailed},
		{fmt.Errorf("x: %w", dnp3.ErrAuthentication), "refused", errs.CodeAuthenticationFailed},
		{context.Canceled, "interrupted", errs.CodeContextCanceled},
		{&tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}, "transport", errs.CodeTransportTLSFailed},
		{fmt.Errorf("x: %w", dnp3.ErrConnectFailed), "transport", errs.CodeTransportConnectFailed},
		{fmt.Errorf("x: %w", dnp3.ErrTimeout), "timeout", errs.CodeRequestTimeout},
		{context.DeadlineExceeded, "timeout", errs.CodeRequestTimeout},
		{fmt.Errorf("%w: %w", dnp3.ErrConnectionLost, dnp3.ErrTimeout), "transport", errs.CodeConnectionLost},
		{dnp3.ErrNotConnected, "transport", errs.CodeConnectionLost},
		{dnp3.ErrClosed, "transport", errs.CodeConnectionLost},
		{fmt.Errorf("x: %w", dnp3.ErrProtocol), "protocol", errs.CodeProtocolViolation},
		{fmt.Errorf("x: %w", dnp3.ErrBusy), "protocol", errs.CodeRequestBusy},
		{fmt.Errorf("x: %w", dnp3.ErrInvalidOption), "invalid_input", errs.CodeInvalidInput},
		{fmt.Errorf("x: %w", app.ErrInvalidValue), "invalid_input", errs.CodeInvalidInput},
		{&net.OpError{Op: "read", Err: errors.New("reset")}, "transport", errs.CodeConnectionLost},
		{errors.New("odd"), "internal", errs.CodeInternalUnexpected},
	} {
		e, ok := errs.As(WrapError(c.err))
		if !ok || string(e.Kind) != c.kind || e.Code != c.code || !errors.Is(e, c.err) {
			t.Errorf("WrapError(%v) = %+v, want %s/%s", c.err, e, c.kind, c.code)
		}
	}
	typed := errs.New(errs.KindUsage, "x", "y", nil)
	if WrapError(nil) != nil || WrapError(typed) != typed {
		t.Error("WrapError must pass nil and typed errors through")
	}
	if EmbeddedErrorInfo(nil) != nil || EmbeddedErrorInfo(dnp3.ErrClosed).Code != errs.CodeConnectionLost {
		t.Error("EmbeddedErrorInfo")
	}
	for err, fatal := range map[error]bool{
		nil: false, &dnp3.IINError{}: false, dnp3.ErrTimeout: false, context.DeadlineExceeded: false,
		context.Canceled: true, dnp3.ErrConnectionLost: true, fmt.Errorf("%w: %w", dnp3.ErrConnectionLost, dnp3.ErrTimeout): true, errors.New("x"): true,
	} {
		if Fatal(err) != fatal {
			t.Errorf("Fatal(%v) = %v", err, !fatal)
		}
	}
}

func TestOptionsAndSecureAuth(t *testing.T) {
	dir := t.TempDir()
	base := func() *config.ConnConfig {
		return &config.ConnConfig{IP: "127.0.0.1", AddressConfig: config.AddressConfig{Master: 1, Outstation: 10}}
	}
	if err := Validate(base()); err != nil {
		t.Fatal(err)
	}
	c := base()
	c.Outstation = uint16(link.SelfAddress)
	c.ResponseTimeout, c.Retries, c.Debug = 100, 2, true
	if _, opts, _, err := options(c, &bytes.Buffer{}); err != nil || len(opts) < 7 {
		t.Errorf("options: %d, %v", len(opts), err)
	}

	keyFile := filepath.Join(dir, "key")
	_ = os.WriteFile(keyFile, []byte("0x 00112233 44556677 8899aabb ccddeeff\n"), 0o600)
	u, err := SecureAuthUser(config.AuthConfig{SAKeyFile: keyFile, SAMAC: "hmac-sha1-10"})
	if err != nil || u.Number != 1 || len(u.Key) != 16 || u.MAC != app.MACHMACSHA1Trunc10 {
		t.Errorf("key file: %+v, %v", u, err)
	}
	if u, err := SecureAuthUser(config.AuthConfig{}); u != nil || err != nil {
		t.Errorf("no key: %v, %v", u, err)
	}
	for _, c := range []config.AuthConfig{
		{SAKeyFile: filepath.Join(dir, "missing")},
		{SAKeyFile: keyFile, SAKey: "00"},
		{SAKey: "xyz"},
		{SAKey: "0011"},
		{SAKey: strings.Repeat("00", 16), SAMAC: "crc"},
		{SAMAC: "hmac-sha256-8"},
	} {
		if _, err := SecureAuthUser(c); err == nil {
			t.Errorf("%+v was accepted", c)
		}
	}

	if _, err := ClientTLS(config.TLSConfig{CACert: keyFile}); err == nil {
		t.Error("a CA file without certificates was accepted")
	}
	if _, err := ClientTLS(config.TLSConfig{Cert: keyFile, Key: keyFile}); err == nil {
		t.Error("a bad key pair was accepted")
	}
	if _, err := SerialSettings("/dev/x", config.SerialConfig{Baud: 19200, Parity: "even", DataBits: 7, StopBits: 2}); err != nil {
		t.Error(err)
	}
	if _, err := SerialSettings("/dev/x", config.SerialConfig{Parity: "odd", DataBits: 9}); err == nil {
		t.Error("9 data bits were accepted")
	}
	if _, _, err := UDPOpener("256.0.0.1:20000", 1)(context.Background()); err == nil {
		t.Error("UDPOpener dialed an address that does not exist")
	}
	if Timeout(0) != DefaultTimeout || Timeout(5) != 5*time.Millisecond {
		t.Error("Timeout")
	}
}

func TestTracer(t *testing.T) {
	var b bytes.Buffer
	tr := &Tracer{W: &b}
	addr := serialAddr("/dev/ttyS0")
	tr.OnConnect(addr)
	tr.OnFrameSent(addr, &link.Frame{Function: link.PrimaryRequestLinkStatus, Dir: true, Dest: 10, Src: 1}, 10)
	tr.OnFrameReceived(nil, &link.Frame{Function: link.SecondaryLinkStatus, Dest: 1, Src: 10}, 10)
	tr.OnFramingError(addr, link.ErrCRC)
	tr.OnFragmentSent(addr, &app.Fragment{Function: app.FuncRead}, 2)
	tr.OnFragmentReceived(addr, &app.Fragment{Function: app.FuncResponse}, 4)
	tr.OnDecodeError(addr, app.ErrMalformed)
	tr.OnDisconnect(addr, errors.New("gone"))
	tr.OnDisconnect(addr, nil)
	for _, want := range []string{"-- /dev/ttyS0 connected", "-> /dev/ttyS0 link PRI REQUEST_LINK_STATUS", "<-  link SEC LINK_STATUS", "framing error", "app  ", "undecodable", "disconnected: gone"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("trace lacks %q:\n%s", want, b.String())
		}
	}
	if addr.Network() != "serial" {
		t.Error("serialAddr.Network")
	}
}

func TestKnownObjects(t *testing.T) {
	if res := Objects(&config.ObjectsConfig{Group: 30}); res.Count == 0 || res.Objects[0].Group != 30 {
		t.Errorf("objects of group 30: %+v", res)
	}
	if PointType(app.G50V1) != "g50" || PointType(app.G30V1) != "analog-input" {
		t.Error("PointType")
	}
}

func TestIntervalUnits(t *testing.T) {
	if IntervalUnitText(app.UnitWeeks) != "weeks" || IntervalUnitText(app.IntervalUnit(42)) != "unit-42" {
		t.Error("IntervalUnitText")
	}
	if u, ok := ParseIntervalUnit(" Months-Last "); !ok || u != app.UnitMonthsLast {
		t.Errorf("ParseIntervalUnit: %v, %v", u, ok)
	}
	if _, ok := ParseIntervalUnit("fortnights"); ok {
		t.Error("an unknown unit was accepted")
	}
	if names := IntervalUnitNames(); len(names) != 11 || names[0] != "none" || names[10] != "months-same" {
		t.Errorf("IntervalUnitNames: %v", names)
	}
}

func TestListenOptions(t *testing.T) {
	dir := t.TempDir()
	junk := filepath.Join(dir, "junk.pem")
	_ = os.WriteFile(junk, []byte("junk"), 0o600)
	for _, c := range []config.TLSConfig{
		{TLS: true},
		{Cert: junk},
		{Cert: junk, Key: junk, Insecure: true},
		{Cert: junk, Key: junk, ServerName: "rtu"},
		{Cert: junk, Key: junk},
	} {
		if _, err := ListenTLS(c); err == nil {
			t.Errorf("%+v was accepted", c)
		}
	}
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = free.Close()
	cfg := &config.ConnConfig{Listen: free.Addr().String(), AddressConfig: config.AddressConfig{Master: 1, Outstation: 10}}
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
	// An option the client refuses closes the listener again.
	if _, err := Open(cfg, nil, client.WithResponseTimeout(-1)); err == nil {
		t.Error("an invalid option was accepted")
	}
	s, err := Open(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	addr := s.acceptor.Addr().String()
	if err := s.Close(); err != nil {
		t.Error(err)
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("the listener of a closed session is still open: %v", err)
	}
	_ = l.Close()
}

func TestBroadcastWithSecureAuthentication(t *testing.T) {
	for _, a := range []config.AuthConfig{{SAKeyFile: "k"}, {SAUpdateKeyFile: "k"}, {SAKey: "00"}, {SAUpdateKey: "00"}} {
		if _, _, err := checkBroadcast("confirm", &config.ConnConfig{AuthConfig: a}); err == nil {
			t.Errorf("a broadcast with %+v was accepted", a)
		}
	}
	if _, name, err := checkBroadcast("confirm", &config.ConnConfig{}); err != nil || name != "confirm" {
		t.Errorf("a broadcast without secure authentication: %q, %v", name, err)
	}
	if _, name, err := checkBroadcast("", &config.ConnConfig{AuthConfig: config.AuthConfig{SAKey: "00"}}); err != nil || name != "" {
		t.Errorf("no broadcast with secure authentication: %q, %v", name, err)
	}
}

func TestFreezeFunctionsAndPlans(t *testing.T) {
	for _, c := range []struct {
		clear, noAck bool
		want         app.FunctionCode
	}{
		{false, false, app.FuncImmediateFreeze},
		{false, true, app.FuncImmediateFreezeNoAck},
		{true, false, app.FuncFreezeClear},
		{true, true, app.FuncFreezeClearNoAck},
	} {
		if got := freezeFunction(c.clear, c.noAck); got != c.want {
			t.Errorf("freezeFunction(%v, %v) = %v, want %v", c.clear, c.noAck, got, c.want)
		}
	}
	// The request functions check their input themselves, also when called without the
	// check of the command.
	ctx := context.Background()
	if _, err := WriteAttribute(ctx, &config.WriteAttributeConfig{Variation: "all", Value: "x"}, nil); err == nil {
		t.Error("WriteAttribute accepted variation all")
	}
	if _, err := Freeze(ctx, &config.FreezeConfig{Interval: "1h"}, nil); err == nil {
		t.Error("Freeze accepted --interval without --at")
	}
	if err := CheckWriteAttribute(&config.WriteAttributeConfig{}); err == nil {
		t.Error("CheckWriteAttribute accepted no outstation")
	}
}

// failingWriter fails every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("pipe closed") }

func TestDebugLogger(t *testing.T) {
	var b bytes.Buffer
	DebugLogger(&b).Infof("hello %d", 7)
	if !strings.HasPrefix(b.String(), "debug: log time=") || !strings.Contains(b.String(), "hello 7") {
		t.Errorf("log line: %q", b.String())
	}
	if _, err := (&linePrefixer{w: failingWriter{}}).Write([]byte("x\n")); err == nil {
		t.Error("a failing writer was not reported")
	}
}
