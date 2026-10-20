// Package dnp holds what the commands of dnp3ctl do with go-dnp3: connecting to an
// outstation, the requests of every client command, the conversion of what the outstation
// answers into the results of package types, and the classification of errors.
package dnp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	dnp3 "github.com/otfabric/go-dnp3"
	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/client"
	"github.com/otfabric/go-dnp3/link"
	"github.com/otfabric/go-dnp3/secauth"
	serial "github.com/otfabric/go-serial"

	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/errs"
)

// DefaultTimeout bounds connecting and each request when --timeout is 0.
const DefaultTimeout = 10 * time.Second

// Default link addresses: the ones go-dnp3 and most outstations ship with.
const (
	DefaultMaster     = 1
	DefaultOutstation = 10
)

// Timeout returns the request timeout of cfg.
func Timeout(ms uint32) time.Duration {
	if ms == 0 {
		return DefaultTimeout
	}
	return time.Duration(ms) * time.Millisecond
}

// ms converts a duration to milliseconds with fractions, the unit of every result.
func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// LinkSettings returns the link layer settings of cfg as go-dnp3 takes them.
func LinkSettings(cfg config.LinkConfig) (confirmed bool, timeout time.Duration, retries int, keepAlive time.Duration) {
	return cfg.LinkConfirmed, time.Duration(cfg.LinkTimeout) * time.Millisecond, int(cfg.LinkRetries), time.Duration(cfg.KeepAlive) * time.Second
}

func clientLink(cfg config.LinkConfig) client.Link {
	c, t, r, k := LinkSettings(cfg)
	return client.Link{Confirmed: c, Timeout: t, Retries: r, KeepAlive: k}
}

// MACAlgorithms maps the values of --sa-mac to the algorithms of IEEE 1815-2012 Table A-12.
var MACAlgorithms = map[string]app.MACAlgorithm{
	"hmac-sha256-16": app.MACHMACSHA256Trunc16,
	"hmac-sha256-8":  app.MACHMACSHA256Trunc8,
	"hmac-sha1-10":   app.MACHMACSHA1Trunc10,
	"hmac-sha1-8":    app.MACHMACSHA1Trunc8,
}

// readKey returns the key of a file or an environment variable, as hex; name describes it
// in errors, and the two sources are named by fileFlag and envVar. An empty result means
// neither is set.
func readKey(file, env, name, fileFlag, envVar string) ([]byte, error) {
	raw := strings.TrimSpace(env)
	switch {
	case file != "" && raw != "":
		return nil, errs.InvalidInput(errs.CodeInvalidFlagCombination, fmt.Sprintf("%s and %s are mutually exclusive", fileFlag, envVar), nil)
	case file != "":
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, errs.InvalidInput(errs.CodeInputFileOpenFailed, fmt.Sprintf("reading the %s: %v", name, err), err)
		}
		raw = strings.TrimSpace(string(data))
	case raw == "":
		return nil, nil
	}
	key, err := hex.DecodeString(strings.Join(strings.Fields(strings.TrimPrefix(strings.TrimPrefix(raw, "0x"), "0X")), ""))
	if err != nil || len(key) == 0 {
		return nil, errs.InvalidInput(errs.CodeInvalidKey, fmt.Sprintf("the %s must be hex, for example 32 hex digits for a 16-octet key", name), err)
	}
	return key, nil
}

// SecureAuthUser returns the Secure Authentication user of cfg, or nil when secure
// authentication is not configured. The keys come from files or environment variables, as
// hex; they are never accepted on the command line.
func SecureAuthUser(cfg config.AuthConfig) (*secauth.User, error) {
	key, err := readKey(cfg.SAKeyFile, cfg.SAKey, "secure authentication key", "--sa-key-file", "DNP3CTL_SA_KEY")
	if err != nil {
		return nil, err
	}
	update, err := readKey(cfg.SAUpdateKeyFile, cfg.SAUpdateKey, "secure authentication update key", "--sa-update-key-file", "DNP3CTL_SA_UPDATE_KEY")
	if err != nil {
		return nil, err
	}
	switch {
	case key != nil && update != nil:
		return nil, errs.InvalidInput(errs.CodeInvalidFlagCombination, "give a pre-shared session key or an update key, not both", nil)
	case key == nil && update == nil:
		if cfg.SAMAC != "" {
			return nil, errs.InvalidInput(errs.CodeInvalidFlagCombination, "--sa-mac needs a key: set --sa-update-key-file or --sa-key-file (or their environment variables)", nil)
		}
		return nil, nil
	case key != nil && len(key) < 16:
		return nil, errs.InvalidInput(errs.CodeInvalidKey, fmt.Sprintf("the secure authentication key has %d octets: use at least 16", len(key)), nil)
	case update != nil && len(update) != 16 && len(update) != 32:
		return nil, errs.InvalidInput(errs.CodeInvalidKey, fmt.Sprintf("the secure authentication update key has %d octets: use 16 (AES-128) or 32 (AES-256)", len(update)), nil)
	}
	user := secauth.User{Number: cfg.SAUser, Key: key, UpdateKey: update}
	if user.Number == 0 {
		user.Number = 1
	}
	if name := strings.ToLower(strings.TrimSpace(cfg.SAMAC)); name != "" {
		alg, ok := MACAlgorithms[name]
		if !ok {
			return nil, errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("invalid --sa-mac %q: use hmac-sha256-16, hmac-sha256-8, hmac-sha1-10 or hmac-sha1-8", cfg.SAMAC), nil)
		}
		user.MAC = alg
	}
	if err := user.Validate(); err != nil {
		return nil, errs.InvalidInput(errs.CodeInvalidKey, fmt.Sprintf("secure authentication: %v", err), err)
	}
	return &user, nil
}

// ClientTLS builds the TLS configuration of a client connection.
func ClientTLS(cfg config.TLSConfig) (*tls.Config, error) {
	out := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         cfg.ServerName,
		InsecureSkipVerify: cfg.Insecure,
	}
	if cfg.CACert != "" {
		pool, err := CertPool(cfg.CACert)
		if err != nil {
			return nil, err
		}
		out.RootCAs = pool
	}
	if (cfg.Cert == "") != (cfg.Key == "") {
		return nil, fmt.Errorf("--tls-cert and --tls-key must be given together")
	}
	if cfg.Cert != "" {
		pair, err := tls.LoadX509KeyPair(cfg.Cert, cfg.Key)
		if err != nil {
			return nil, fmt.Errorf("loading the certificate and key: %w", err)
		}
		out.Certificates = []tls.Certificate{pair}
	}
	return out, nil
}

// CertPool reads PEM certificates from path.
func CertPool(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading CA certificates: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no certificates found in %s", path)
	}
	return pool, nil
}

// SerialSettings returns the serial port configuration of cfg for device.
func SerialSettings(device string, cfg config.SerialConfig) (serial.Config, error) {
	out := serial.Config{Address: device, BaudRate: serial.BaudRate(cfg.Baud), DataBits: serial.DataBits(cfg.DataBits), StopBits: serial.StopBits(cfg.StopBits)}
	switch strings.ToLower(strings.TrimSpace(cfg.Parity)) {
	case "", "none", "n":
		out.Parity = serial.ParityNone
	case "even", "e":
		out.Parity = serial.ParityEven
	case "odd", "o":
		out.Parity = serial.ParityOdd
	default:
		return out, fmt.Errorf("invalid --parity %q: use none, even or odd", cfg.Parity)
	}
	if err := out.Validate(); err != nil {
		return out, fmt.Errorf("serial line: %w", err)
	}
	return out, nil
}

// openSerial is how a serial port is opened; tests replace it.
var openSerial = func(c *serial.Config) (io.ReadWriteCloser, error) { return serial.Open(c) }

// serialAddr names a serial port in logs and the debug trace.
type serialAddr string

func (a serialAddr) Network() string { return "serial" }
func (a serialAddr) String() string  { return string(a) }

// SerialOpener returns a stream opener for the serial port of sc.
func SerialOpener(sc serial.Config) client.StreamOpener {
	return func(context.Context) (io.ReadWriteCloser, net.Addr, error) {
		p, err := openSerial(&sc)
		if err != nil {
			return nil, nil, err
		}
		return p, serialAddr(sc.Address), nil
	}
}

// OpenSerial opens the serial port of sc, for the server side.
func OpenSerial(sc serial.Config) (io.ReadWriteCloser, net.Addr, error) {
	return SerialOpener(sc)(context.Background())
}

// UDPOpener returns a stream opener for DNP3 over UDP to addr, sending from localPort (0
// for any free port). Every link frame travels in a datagram, as with client.UDP.
func UDPOpener(addr string, localPort uint16) client.StreamOpener {
	if localPort == 0 {
		return client.UDP(addr)
	}
	return func(ctx context.Context) (io.ReadWriteCloser, net.Addr, error) {
		d := net.Dialer{LocalAddr: &net.UDPAddr{Port: int(localPort)}}
		conn, err := d.DialContext(ctx, "udp", addr)
		if err != nil {
			return nil, nil, err
		}
		return conn, conn.RemoteAddr(), nil
	}
}

// Session is a client of an outstation, prepared from the configuration of a command.
type Session struct {
	*client.Client
	Target     config.Target
	Outstation uint16
	Timeout    time.Duration
}

// Validate checks everything about cfg that can be checked without connecting.
func Validate(cfg *config.ConnConfig) error {
	_, _, err := options(cfg, nil)
	return err
}

// options returns the target of cfg and the client options that reach it.
func options(cfg *config.ConnConfig, trace io.Writer) (config.Target, []client.Option, error) {
	target, err := cfg.ResolveTarget()
	if err != nil {
		return target, nil, errs.InvalidInput(errs.CodeInvalidURL, err.Error(), err)
	}
	if cfg.Master == cfg.Outstation {
		return target, nil, errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("--master and --outstation are both %d: the link addresses must differ", cfg.Master), nil)
	}
	for name, a := range map[string]uint16{"--master": cfg.Master, "--outstation": cfg.Outstation} {
		if link.Address(a) > link.MaxStationAddress && link.Address(a) != link.SelfAddress {
			return target, nil, errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("%s %d: use 0-65519, or 65532 (the self address) for the outstation", name, a), nil)
		}
	}
	if link.Address(cfg.Master) == link.SelfAddress {
		return target, nil, errs.InvalidInput(errs.CodeInvalidInput, "--master cannot be the self address 65532", nil)
	}
	opts := []client.Option{
		client.WithAddresses(link.Address(cfg.Master), link.Address(cfg.Outstation)),
		client.WithLink(clientLink(cfg.LinkConfig)),
		// One-shot commands do exactly what they are asked: no integrity poll and no write
		// of IIN1.7 behind the operator's back.
		client.WithAutoIntegrity(false),
		client.WithAutoClearRestart(false),
	}
	if cfg.ResponseTimeout != 0 {
		opts = append(opts, client.WithResponseTimeout(time.Duration(cfg.ResponseTimeout)*time.Millisecond))
	}
	if cfg.Retries != 0 {
		opts = append(opts, client.WithRetry(client.Retry{Attempts: int(cfg.Retries) + 1}))
	}
	switch target.Scheme {
	case config.SchemeTLS:
		tc, err := ClientTLS(cfg.TLSConfig)
		if err != nil {
			return target, nil, errs.WrapValidation(err)
		}
		opts = append(opts, client.WithTLS(tc))
	case config.SchemeUDP:
		opts = append(opts, client.WithStream(UDPOpener(target.Addr(), cfg.UDPLocalPort)))
	case config.SchemeSerial:
		sc, err := SerialSettings(target.Device, cfg.SerialConfig)
		if err != nil {
			return target, nil, errs.WrapValidation(err)
		}
		opts = append(opts, client.WithStream(SerialOpener(sc)))
	}
	if target.Scheme != config.SchemeUDP && cfg.UDPLocalPort != 0 {
		return target, nil, errs.InvalidInput(errs.CodeInvalidFlagCombination, fmt.Sprintf("--udp-local-port needs a udp:// URL; the target is %s", target), nil)
	}
	if target.Scheme != config.SchemeTLS && cfg.WantsTLS() {
		return target, nil, errs.InvalidInput(errs.CodeInvalidFlagCombination, fmt.Sprintf("TLS flags need a tls:// URL or --ip; the target is %s", target), nil)
	}
	user, err := SecureAuthUser(cfg.AuthConfig)
	if err != nil {
		return target, nil, err
	}
	if cfg.SAKeyChange != 0 && (user == nil || user.UpdateKey == nil) {
		return target, nil, errs.InvalidInput(errs.CodeInvalidFlagCombination, "--sa-key-change-interval needs an update key: set --sa-update-key-file or DNP3CTL_SA_UPDATE_KEY", nil)
	}
	if user != nil {
		opts = append(opts, client.WithSecureAuthentication(*user))
	}
	if cfg.SAKeyChange != 0 {
		opts = append(opts, client.WithKeyChangeInterval(time.Duration(cfg.SAKeyChange)*time.Second))
	}
	if cfg.Debug && trace != nil {
		opts = append(opts, client.WithMetrics(&Tracer{W: trace}))
	}
	return target, opts, nil
}

// Open prepares a client for the outstation of cfg without connecting. trace receives the
// frame trace of --debug. extra options are applied last.
func Open(cfg *config.ConnConfig, trace io.Writer, extra ...client.Option) (*Session, error) {
	target, opts, err := options(cfg, trace)
	if err != nil {
		return nil, err
	}
	c, err := client.New(target.Addr(), append(opts, extra...)...)
	if err != nil {
		return nil, errs.WrapValidation(err)
	}
	return &Session{Client: c, Target: target, Outstation: cfg.Outstation, Timeout: Timeout(cfg.Timeout)}, nil
}

// Dial is [Open] followed by connecting, bounded by the timeout of cfg.
func Dial(ctx context.Context, cfg *config.ConnConfig, trace io.Writer, extra ...client.Option) (*Session, error) {
	s, err := Open(cfg, trace, extra...)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	if err := s.Connect(cctx); err != nil {
		_ = s.Close()
		return nil, WrapError(err)
	}
	return s, nil
}

// request runs fn on a connected session with the timeout of the command and returns the
// time it took.
func (s *Session) request(ctx context.Context, fn func(context.Context) error) (time.Duration, error) {
	rctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	begin := time.Now()
	err := fn(rctx)
	return time.Since(begin), err
}

// Tracer prints every link frame and application fragment sent and received, for
// --debug. It is a go-dnp3 Metrics.
type Tracer struct {
	dnp3.NopMetrics
	W  io.Writer
	mu sync.Mutex
}

// OnConnect implements dnp3.Metrics.
func (t *Tracer) OnConnect(remote net.Addr) { t.line("--", remote, "connected") }

// OnDisconnect implements dnp3.Metrics.
func (t *Tracer) OnDisconnect(remote net.Addr, err error) {
	if err != nil {
		t.line("--", remote, "disconnected: "+err.Error())
		return
	}
	t.line("--", remote, "disconnected")
}

// OnFrameSent implements dnp3.Metrics.
func (t *Tracer) OnFrameSent(remote net.Addr, f *link.Frame, _ int) {
	t.line("->", remote, "link "+f.String())
}

// OnFrameReceived implements dnp3.Metrics.
func (t *Tracer) OnFrameReceived(remote net.Addr, f *link.Frame, _ int) {
	t.line("<-", remote, "link "+f.String())
}

// OnFramingError implements dnp3.Metrics.
func (t *Tracer) OnFramingError(remote net.Addr, err error) {
	t.line("<-", remote, "framing error: "+err.Error())
}

// OnFragmentSent implements dnp3.Metrics.
func (t *Tracer) OnFragmentSent(remote net.Addr, f *app.Fragment, _ int) {
	t.line("->", remote, "app  "+f.String())
}

// OnFragmentReceived implements dnp3.Metrics.
func (t *Tracer) OnFragmentReceived(remote net.Addr, f *app.Fragment, _ int) {
	t.line("<-", remote, "app  "+f.String())
}

// OnDecodeError implements dnp3.Metrics.
func (t *Tracer) OnDecodeError(remote net.Addr, err error) {
	t.line("<-", remote, "undecodable: "+err.Error())
}

func (t *Tracer) line(dir string, remote net.Addr, text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	addr := ""
	if remote != nil {
		addr = remote.String()
	}
	_, _ = fmt.Fprintf(t.W, "debug: %s %s %s %s\n", time.Now().UTC().Format("15:04:05.000"), dir, addr, text)
}
