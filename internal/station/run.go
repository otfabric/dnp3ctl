package station

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/otfabric/go-dnp3/server"

	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/dnp"
	"github.com/otfabric/dnp3ctl/internal/errs"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// serverTLS builds the TLS configuration of the listener, or nil without a certificate.
func serverTLS(cfg *config.ServerConfig) (*tls.Config, error) {
	if (cfg.Cert == "") != (cfg.Key == "") {
		return nil, fmt.Errorf("--tls-cert and --tls-key must be given together")
	}
	if cfg.Cert == "" {
		if cfg.ClientCA != "" {
			return nil, fmt.Errorf("--tls-client-ca needs --tls-cert and --tls-key")
		}
		return nil, nil
	}
	pair, err := tls.LoadX509KeyPair(cfg.Cert, cfg.Key)
	if err != nil {
		return nil, fmt.Errorf("loading the certificate and key: %w", err)
	}
	out := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair}}
	if cfg.ClientCA != "" {
		pool, err := dnp.CertPool(cfg.ClientCA)
		if err != nil {
			return nil, err
		}
		out.ClientCAs, out.ClientAuth = pool, tls.RequireAndVerifyClientCert
	}
	return out, nil
}

func parseAllow(list []string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, s := range list {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p.Masked())
			continue
		}
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("invalid --allow %q: use an address or a CIDR prefix", s)
		}
		out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out, nil
}

// LoadModel returns the model of cfg: the file --config names, or the built-in demo.
func LoadModel(cfg *config.ServerConfig) (*Model, error) {
	if cfg.Config == "" {
		return Demo(), nil
	}
	return Load(cfg.Config)
}

// plan is a validated configuration of `server start`.
type plan struct {
	model *Model
	opts  Options
	allow []netip.Prefix
}

func check(cfg *config.ServerConfig) (*plan, error) {
	m, err := LoadModel(cfg)
	if err != nil {
		return nil, err
	}
	tc, err := serverTLS(cfg)
	if err != nil {
		return nil, err
	}
	allow, err := parseAllow(cfg.Allow)
	if err != nil {
		return nil, err
	}
	switch {
	case cfg.Serial != "" && (cfg.UDP || tc != nil || len(allow) > 0 || cfg.Listen != "" || cfg.Port != 0):
		return nil, fmt.Errorf("--serial cannot be combined with --udp, --listen, --port, --allow or TLS")
	case cfg.UDP && (tc != nil || len(allow) > 0):
		return nil, fmt.Errorf("--udp cannot be combined with TLS or --allow")
	case cfg.Serial != "":
		if _, err := dnp.SerialSettings(cfg.Serial, cfg.SerialConfig); err != nil {
			return nil, err
		}
	}
	user, err := dnp.SecureAuthUser(cfg.AuthConfig)
	if err != nil {
		return nil, err
	}
	c, t, r, k := dnp.LinkSettings(cfg.LinkConfig)
	return &plan{model: m, allow: allow, opts: Options{
		Link: server.Link{Confirmed: c, Timeout: t, Retries: r, KeepAlive: k}, Auth: user, TLS: tc, MaxConnections: int(cfg.MaxSessions),
	}}, nil
}

// Check validates the configuration of `server start`, the model included.
func Check(cfg *config.ServerConfig) error {
	_, err := check(cfg)
	return err
}

// listenAddr returns the address `server start` listens on.
func listenAddr(cfg *config.ServerConfig, withTLS bool) string {
	port := cfg.Port
	if port == 0 {
		port = config.DefaultPort
		if withTLS {
			port = config.DefaultTLSPort
		}
	}
	return net.JoinHostPort(strings.TrimSpace(cfg.Listen), strconv.Itoa(int(port)))
}

// allowListener closes the connections of clients --allow does not name.
type allowListener struct {
	net.Listener
	allow []netip.Prefix
	log   func(types.ServerEvent)
}

func (l *allowListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		ap, perr := netip.ParseAddrPort(c.RemoteAddr().String())
		if perr == nil {
			for _, p := range l.allow {
				if p.Contains(ap.Addr().Unmap()) {
					return c, nil
				}
			}
		}
		l.log(types.ServerEvent{Event: "refused", Remote: c.RemoteAddr().String(), Detail: "not in --allow"})
		_ = c.Close()
	}
}

// openSerial opens the serial port of `server start --serial`; tests replace it.
var openSerial = dnp.OpenSerial

// Run serves the outstation of cfg until ctx ends or the duration of cfg has passed. Every
// event goes to log and, with --debug, every frame to trace; ready, when not nil, is called
// with the address once the outstation serves.
func Run(ctx context.Context, cfg *config.ServerConfig, log func(types.ServerEvent), trace io.Writer, ready func(net.Addr)) error {
	p, err := check(cfg)
	if err != nil {
		return errs.InvalidInput(errs.CodeInvalidConfig, err.Error(), err)
	}
	p.opts.Log = log
	if cfg.Debug && trace != nil {
		p.opts.Metrics = &dnp.Tracer{W: trace}
	}
	st, err := New(p.model, p.opts)
	if err != nil {
		return errs.InvalidInput(errs.CodeInvalidConfig, err.Error(), err)
	}
	defer func() { _ = st.Close() }()

	name := p.model.Outstation.Name
	if name == "" {
		name = "outstation"
	}
	detail := fmt.Sprintf("%s, link address %d", name, p.model.Address())
	served := make(chan error, 1)
	var addr net.Addr
	switch {
	case cfg.Serial != "":
		sc, _ := dnp.SerialSettings(cfg.Serial, cfg.SerialConfig)
		port, remote, err := openSerial(sc)
		if err != nil {
			return errs.New(errs.KindTransport, errs.CodeListenFailed, fmt.Sprintf("cannot open %s: %v", cfg.Serial, err), err)
		}
		addr = remote
		st.event(types.ServerEvent{Event: "listening", Remote: "serial://" + cfg.Serial, Detail: detail})
		go func() { served <- st.ServeConn(port, remote) }()
	case cfg.UDP:
		pc, err := net.ListenPacket("udp", listenAddr(cfg, false))
		if err != nil {
			return errs.New(errs.KindTransport, errs.CodeListenFailed, fmt.Sprintf("cannot listen: %v", err), err)
		}
		addr = pc.LocalAddr()
		st.event(types.ServerEvent{Event: "listening", Remote: "udp://" + addr.String(), Detail: detail})
		go func() { served <- st.ServeUDP(pc) }()
	default:
		ln, err := net.Listen("tcp", listenAddr(cfg, p.opts.TLS != nil))
		if err != nil {
			return errs.New(errs.KindTransport, errs.CodeListenFailed, fmt.Sprintf("cannot listen: %v", err), err)
		}
		addr = ln.Addr()
		scheme := "tcp://"
		if p.opts.TLS != nil {
			ln, scheme = tls.NewListener(ln, p.opts.TLS), "tls://"
		}
		if len(p.allow) > 0 {
			ln = &allowListener{Listener: ln, allow: p.allow, log: st.event}
		}
		st.event(types.ServerEvent{Event: "listening", Remote: scheme + addr.String(), Detail: detail})
		go func() { served <- st.Serve(ln) }()
	}
	if ready != nil {
		ready(addr)
	}
	if cfg.Duration != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(cfg.Duration)*time.Second)
		defer cancel()
	}
	select {
	case err := <-served:
		if err != nil {
			return errs.New(errs.KindTransport, errs.CodeListenFailed, fmt.Sprintf("server stopped: %v", err), err)
		}
		return nil
	case <-ctx.Done():
	}
	st.event(types.ServerEvent{Event: "stopping"})
	_ = st.Close()
	// Serve returns nil after Close; anything else went wrong while stopping.
	if err := <-served; err != nil {
		st.event(types.ServerEvent{Event: "error", Detail: fmt.Sprintf("stopping: %v", err)})
	}
	return nil
}
