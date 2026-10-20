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
	switch {
	case cfg.CACert != "":
		return nil, fmt.Errorf("--tls-ca is for a tls:// --dial; masters that connect are verified with --tls-client-ca")
	case cfg.Cert == "" && cfg.ClientCA != "":
		return nil, fmt.Errorf("--tls-client-ca needs --tls-cert and --tls-key")
	case cfg.Cert == "":
		return nil, nil
	}
	return dnp.ServerTLS(cfg.Cert, cfg.Key, cfg.ClientCA)
}

// dialTLS builds the TLS configuration of a tls:// --dial, the outstation being the TLS
// client: --tls-ca verifies the master, --tls-cert and --tls-key are presented to it.
func dialTLS(cfg *config.ServerConfig) (*tls.Config, error) {
	if cfg.ClientCA != "" {
		return nil, fmt.Errorf("--tls-client-ca does not apply to --dial: verify the master with --tls-ca")
	}
	return dnp.ClientTLS(config.TLSConfig{CACert: cfg.CACert, Cert: cfg.Cert, Key: cfg.Key})
}

// dialTarget returns the master of --dial, and the TLS configuration to reach it with.
func dialTarget(cfg *config.ServerConfig) (*config.Target, *tls.Config, error) {
	t, err := config.ParseURL(cfg.Dial)
	if err != nil {
		return nil, nil, fmt.Errorf("--dial: %w", err)
	}
	switch {
	case cfg.Serial != "" || cfg.UDP || cfg.Listen != "" || cfg.Port != 0 || len(cfg.Allow) > 0:
		return nil, nil, fmt.Errorf("--dial cannot be combined with --serial, --udp, --listen, --port or --allow")
	case t.TLS():
		tc, err := dialTLS(cfg)
		return &t, tc, err
	case t.Scheme != config.SchemeTCP:
		return nil, nil, fmt.Errorf("--dial %s: the outstation connects over tcp:// or tls://", t)
	case cfg.Cert != "" || cfg.Key != "" || cfg.CACert != "" || cfg.ClientCA != "":
		return nil, nil, fmt.Errorf("TLS flags need a tls:// --dial; the master is %s", t)
	}
	return &t, nil, nil
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

// LoadModels returns the models of cfg: the files --config names, or the built-in demo. The
// outstations of several models must have distinct link addresses.
func LoadModels(cfg *config.ServerConfig) ([]*Model, error) {
	var models []*Model
	addresses := map[uint16]string{}
	for _, file := range cfg.Config {
		if strings.TrimSpace(file) == "" {
			continue
		}
		m, err := Load(file)
		if err != nil {
			return nil, err
		}
		if other, ok := addresses[m.Address()]; ok {
			return nil, fmt.Errorf("%s and %s both have link address %d: the outstations of one server need distinct addresses", other, file, m.Address())
		}
		addresses[m.Address()] = file
		models = append(models, m)
	}
	if len(models) == 0 {
		models = []*Model{Demo()}
	}
	return models, nil
}

// plan is a validated configuration of `server start`.
type plan struct {
	models []*Model
	opts   Options
	allow  []netip.Prefix
	peer   net.Addr       // the master of --udp-peer, or nil
	dial   *config.Target // the master of --dial, or nil
}

func check(cfg *config.ServerConfig) (*plan, error) {
	models, err := LoadModels(cfg)
	if err != nil {
		return nil, err
	}
	if len(models) > 1 && (cfg.Dial != "" || cfg.UDPPeer != "") {
		return nil, fmt.Errorf("--dial and --udp-peer serve one outstation: give one --config")
	}
	var (
		tc   *tls.Config
		dial *config.Target
	)
	if cfg.Dial != "" {
		dial, tc, err = dialTarget(cfg)
	} else {
		tc, err = serverTLS(cfg)
	}
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
	case cfg.UDPPeer != "" && !cfg.UDP:
		return nil, fmt.Errorf("--udp-peer needs --udp")
	case cfg.Serial != "":
		if _, err := dnp.SerialSettings(cfg.Serial, cfg.SerialConfig); err != nil {
			return nil, err
		}
	}
	var peer net.Addr
	if cfg.UDPPeer != "" {
		addr, err := net.ResolveUDPAddr("udp", cfg.UDPPeer)
		if err != nil {
			return nil, fmt.Errorf("invalid --udp-peer %q: use host:port: %w", cfg.UDPPeer, err)
		}
		peer = addr
	}
	user, err := dnp.SecureAuthUser(cfg.AuthConfig)
	if err != nil {
		return nil, err
	}
	if user == nil && (cfg.SAReplyTimeout != 0 || cfg.SAStatisticsClass != 0 || cfg.SAStatisticsEvery != 0) {
		return nil, fmt.Errorf("--sa-reply-timeout and the security statistics flags need secure authentication: set --sa-update-key-file or --sa-key-file (or their environment variables)")
	}
	if cfg.SAStatisticsClass > 3 {
		return nil, fmt.Errorf("invalid --sa-statistics-class %d: use 1-3", cfg.SAStatisticsClass)
	}
	c, t, r, k := dnp.LinkSettings(cfg.LinkConfig)
	return &plan{models: models, allow: allow, peer: peer, dial: dial, opts: Options{
		Link: server.Link{Confirmed: c, Timeout: t, Retries: r, KeepAlive: k}, Auth: user, TLS: tc, MaxConnections: int(cfg.MaxSessions),
		AuthReplyTimeout: time.Duration(cfg.SAReplyTimeout) * time.Millisecond, StatisticsClass: cfg.SAStatisticsClass, StatisticsThreshold: cfg.SAStatisticsEvery,
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
// with the address once the outstation serves (nil with --dial).
func Run(ctx context.Context, cfg *config.ServerConfig, log func(types.ServerEvent), trace io.Writer, ready func(net.Addr)) error {
	p, err := check(cfg)
	if err != nil {
		return errs.InvalidInput(errs.CodeInvalidConfig, err.Error(), err)
	}
	p.opts.Log = log
	if cfg.Debug && trace != nil {
		p.opts.Metrics, p.opts.Logger = &dnp.Tracer{W: trace}, dnp.DebugLogger(trace)
	}
	f, err := NewFleet(p.models, p.opts)
	if err != nil {
		return errs.InvalidInput(errs.CodeInvalidConfig, err.Error(), err)
	}
	defer func() { _ = f.Close() }()

	detail := f.Describe()
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
		f.event(types.ServerEvent{Event: "listening", Remote: "serial://" + cfg.Serial, Detail: detail})
		go func() { served <- f.ServeConn(port, remote) }()
	case p.dial != nil:
		f.event(types.ServerEvent{Event: "dialing", Remote: p.dial.String(), Detail: detail})
		// Only Close ends DialAndServe, so that it returns nil when the server stops.
		st, dialCtx := f.Stations()[0], context.WithoutCancel(ctx)
		go func() { served <- st.DialAndServe(dialCtx, p.dial.Addr()) }()
	case cfg.UDP:
		pc, err := net.ListenPacket("udp", listenAddr(cfg, false))
		if err != nil {
			return errs.New(errs.KindTransport, errs.CodeListenFailed, fmt.Sprintf("cannot listen: %v", err), err)
		}
		addr = pc.LocalAddr()
		f.event(types.ServerEvent{Event: "listening", Remote: "udp://" + addr.String(), Detail: detail})
		if p.peer != nil {
			st := f.Stations()[0]
			go func() { served <- st.ServeUDPTo(pc, p.peer) }()
		} else {
			go func() { served <- f.ServeUDP(pc) }()
		}
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
			ln = &allowListener{Listener: ln, allow: p.allow, log: f.event}
		}
		f.event(types.ServerEvent{Event: "listening", Remote: scheme + addr.String(), Detail: detail})
		go func() { served <- f.Serve(ln) }()
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
	f.shutdown(served)
	return nil
}
