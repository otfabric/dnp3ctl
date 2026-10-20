package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Default ports: 20000 is the IANA port of DNP3 over TCP and UDP ("dnp"), 19999 the one of
// DNP3 over TLS ("dnp-sec", IEEE 1815-2012 Annex B / IEC 62351-3).
const (
	DefaultPort    = 20000
	DefaultTLSPort = 19999
)

// URL schemes accepted by --url.
const (
	SchemeTCP    = "tcp"
	SchemeTLS    = "tls"
	SchemeUDP    = "udp"
	SchemeSerial = "serial"
)

// Target is a resolved outstation address: a host and port for tcp, tls and udp, or the
// device of a serial port. With Listen it is the local address a master waits on for the
// outstation to connect (tcp or tls; an empty host is every interface).
type Target struct {
	Scheme string
	Host   string
	Port   uint16
	Device string
	Listen bool
}

// Serial reports whether the target is a serial port.
func (t Target) Serial() bool { return t.Scheme == SchemeSerial }

// TLS reports whether the target is reached over TLS.
func (t Target) TLS() bool { return t.Scheme == SchemeTLS }

// UDP reports whether the target is reached over UDP.
func (t Target) UDP() bool { return t.Scheme == SchemeUDP }

// Addr returns host:port for dialing or listening, or the device of a serial port.
func (t Target) Addr() string {
	if t.Serial() {
		return t.Device
	}
	return net.JoinHostPort(t.Host, strconv.Itoa(int(t.Port)))
}

// String returns the target as a URL, the form used in output ("tcp://host:20000",
// "serial:///dev/ttyUSB0").
func (t Target) String() string {
	if t.Serial() {
		if strings.HasPrefix(t.Device, "/") {
			return SchemeSerial + "://" + t.Device
		}
		return SchemeSerial + ":///" + t.Device
	}
	scheme := t.Scheme
	if scheme == "" {
		scheme = SchemeTCP
	}
	return scheme + "://" + t.Addr()
}

// ParseURL parses tcp://host[:port], tls://host[:port], udp://host[:port] or
// serial:///dev/ttyUSB0 (serial://COM3 on Windows). A missing port is 20000, or 19999 for
// tls.
func ParseURL(raw string) (Target, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return Target{}, fmt.Errorf("invalid URL %q: %w", raw, err)
	}
	t := Target{Scheme: strings.ToLower(u.Scheme)}
	switch t.Scheme {
	case SchemeTCP, SchemeUDP:
		t.Port = DefaultPort
	case SchemeTLS:
		t.Port = DefaultTLSPort
	case SchemeSerial:
		if u.RawQuery != "" || u.Fragment != "" || u.User != nil || u.Port() != "" {
			return Target{}, fmt.Errorf("invalid URL %q: a serial URL names the device only (serial:///dev/ttyUSB0); set the line with --baud, --data-bits, --parity and --stop-bits", raw)
		}
		t.Device = u.Host + u.Path
		if t.Device == "" || t.Device == "/" {
			return Target{}, fmt.Errorf("invalid URL %q: missing device (use serial:///dev/ttyUSB0 or serial://COM3)", raw)
		}
		return t, nil
	case "":
		return Target{}, fmt.Errorf("invalid URL %q: missing scheme (use tcp://host:port, tls://host:port, udp://host:port or serial:///dev/ttyUSB0)", raw)
	default:
		return Target{}, fmt.Errorf("invalid URL %q: unsupported scheme %q (use tcp, tls, udp or serial)", raw, u.Scheme)
	}
	if u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return Target{}, fmt.Errorf("invalid URL %q: only scheme, host and port are allowed", raw)
	}
	t.Host = u.Hostname()
	if t.Host == "" {
		return Target{}, fmt.Errorf("invalid URL %q: missing host", raw)
	}
	if p := u.Port(); p != "" {
		n, err := strconv.ParseUint(p, 10, 16)
		if err != nil || n == 0 {
			return Target{}, fmt.Errorf("invalid URL %q: port must be 1-65535", raw)
		}
		t.Port = uint16(n)
	}
	return t, nil
}

// ResolveTarget returns the outstation a client command talks to: --url, or --ip with
// --port, or with --listen the address to wait on for the outstation to connect. The three
// styles are mutually exclusive, and one of them is required. With --ip and --listen, TLS
// is used when a TLS flag asks for it, on the port given (20000 unless --port is set; a
// port in --listen takes precedence).
func (c *ConnConfig) ResolveTarget() (Target, error) {
	url, ip, listen := strings.TrimSpace(c.URL), strings.TrimSpace(c.IP), strings.TrimSpace(c.Listen)
	switch {
	case listen != "" && (url != "" || ip != ""):
		return Target{}, fmt.Errorf("--listen is mutually exclusive with --url and --ip")
	case listen != "":
		return c.listenTarget(listen)
	case url != "" && ip != "":
		return Target{}, fmt.Errorf("--url and --ip are mutually exclusive")
	case url != "":
		return ParseURL(url)
	case ip != "":
		port := c.Port
		if port == 0 {
			port = DefaultPort
		}
		scheme := SchemeTCP
		if c.WantsTLS() {
			scheme = SchemeTLS
		}
		return Target{Scheme: scheme, Host: ip, Port: port}, nil
	default:
		return Target{}, fmt.Errorf("no outstation given: set --url (tcp://, tls://, udp://host:port or serial:///dev/ttyUSB0) or --ip")
	}
}

// listenTarget returns the target of --listen: host, host:port or :port, with the port of
// --port (20000 unless set) when it names none.
func (c *ConnConfig) listenTarget(listen string) (Target, error) {
	t := Target{Scheme: SchemeTCP, Host: strings.TrimSuffix(strings.TrimPrefix(listen, "["), "]"), Port: c.Port, Listen: true}
	if c.WantsTLS() {
		t.Scheme = SchemeTLS
	}
	if host, port, err := net.SplitHostPort(listen); err == nil {
		n, err := strconv.ParseUint(port, 10, 16)
		if err != nil || n == 0 {
			return Target{}, fmt.Errorf("invalid --listen %q: port must be 1-65535", listen)
		}
		t.Host, t.Port = host, uint16(n)
	}
	if t.Port == 0 {
		t.Port = DefaultPort
	}
	return t, nil
}
