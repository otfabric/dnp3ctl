// SPDX-License-Identifier: MIT

package station

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/otfabric/go-dnp3/secauth"
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
	return dnp.ParsePrefixes(list, "--allow")
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
	fixed  bool           // --udp-fixed: responses always go to peer
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
	if user == nil && (cfg.SAReplyTimeout != 0 || cfg.SAStatisticsClass != 0 || cfg.SAStatisticsEvery != 0 || len(cfg.SAThresholds) > 0 ||
		cfg.SANoAggressive || cfg.SAExpectedKeyInterval != 0 || cfg.SAExpectedKeyCount != 0 || cfg.SAMaxKeyStatus != 0) {
		return nil, fmt.Errorf("--sa-reply-timeout, the security statistics flags and the expected key change flags need secure authentication: set --sa-update-key-file or --sa-key-file (or their environment variables)")
	}
	thresholds, err := parseThresholds(cfg.SAThresholds)
	if err != nil {
		return nil, err
	}
	switch {
	case cfg.SAStatisticsClass > 3:
		return nil, fmt.Errorf("invalid --sa-statistics-class %d: use 1-3", cfg.SAStatisticsClass)
	case cfg.SAStatisticsEvery > math.MaxUint16:
		return nil, fmt.Errorf("invalid --sa-statistics-threshold %d: use 1-65535", cfg.SAStatisticsEvery)
	case cfg.SAMaxKeyStatus == 1:
		return nil, fmt.Errorf("invalid --sa-max-key-status-requests 1: use 2-255")
	case cfg.FileTimeout > 3600:
		return nil, fmt.Errorf("invalid --file-timeout %d: use 1-3600 seconds", cfg.FileTimeout)
	case cfg.FileTimeoutClass > 3:
		return nil, fmt.Errorf("invalid --file-timeout-class %d: use 1-3", cfg.FileTimeoutClass)
	case cfg.UDPFixed && cfg.UDPPeer == "":
		return nil, fmt.Errorf("--udp-fixed needs --udp-peer")
	}
	for name, n := range map[string]uint16{"--max-tx-fragment": cfg.MaxTxFragment, "--max-rx-fragment": cfg.MaxRxFragment} {
		if n != 0 && n < minFragment {
			return nil, fmt.Errorf("invalid %s %d: use at least %d", name, n, minFragment)
		}
	}
	masterIPs, err := dnp.ParsePrefixes(cfg.MasterIPs, "--master-ips")
	if err != nil {
		return nil, err
	}
	if len(masterIPs) > 0 && cfg.Serial != "" {
		return nil, fmt.Errorf("--master-ips does not apply to --serial")
	}
	redial, source, err := redialSettings(cfg)
	if err != nil {
		return nil, err
	}
	c, t, r, k := dnp.LinkSettings(cfg.LinkConfig)
	return &plan{models: models, allow: allow, peer: peer, dial: dial, fixed: cfg.UDPFixed, opts: Options{
		Link: server.Link{Confirmed: c, Timeout: t, Retries: r, KeepAlive: k}, Auth: user, TLS: tc, MaxConnections: int(cfg.MaxSessions),
		AuthReplyTimeout: time.Duration(cfg.SAReplyTimeout) * time.Millisecond, StatisticsClass: cfg.SAStatisticsClass, StatisticsThreshold: cfg.SAStatisticsEvery,
		Thresholds: thresholds, NoAggressive: cfg.SANoAggressive, ExpectedKeyChangeInterval: time.Duration(cfg.SAExpectedKeyInterval) * time.Second,
		ExpectedKeyChangeCount: cfg.SAExpectedKeyCount, MaxKeyStatusRequests: int(cfg.SAMaxKeyStatus),
		MasterIPs: masterIPs, MaxControls: int(cfg.MaxControls), MaxTxFragment: int(cfg.MaxTxFragment), MaxRxFragment: int(cfg.MaxRxFragment),
		DeadbandIntegerOnly: cfg.DeadbandIntegerOnly, FileTimeout: time.Duration(cfg.FileTimeout) * time.Second, FileTimeoutClass: cfg.FileTimeoutClass,
		Redial: redial, DialSource: source,
	}}, nil
}

// redialSettings are the backoff and the source address of --dial.
func redialSettings(cfg *config.ServerConfig) (server.Redial, netip.Addr, error) {
	source, err := dnp.ParseSourceAddress("--dial-source-address", cfg.DialSourceAddress)
	if err != nil {
		return server.Redial{}, netip.Addr{}, err
	}
	if cfg.Dial == "" && (source.IsValid() || cfg.RedialMinDelay != 0 || cfg.RedialMaxDelay != 0 || cfg.RedialJitter != 0) {
		return server.Redial{}, netip.Addr{}, fmt.Errorf("--dial-source-address and the --redial flags need --dial")
	}
	if cfg.RedialJitter > 100 {
		return server.Redial{}, netip.Addr{}, fmt.Errorf("invalid --redial-jitter %d: use a percentage, 0-100", cfg.RedialJitter)
	}
	if cfg.RedialMinDelay != 0 && cfg.RedialMaxDelay != 0 && cfg.RedialMaxDelay < cfg.RedialMinDelay {
		return server.Redial{}, netip.Addr{}, fmt.Errorf("--redial-max-delay %d is below --redial-min-delay %d", cfg.RedialMaxDelay, cfg.RedialMinDelay)
	}
	return server.Redial{
		MinDelay: time.Duration(cfg.RedialMinDelay) * time.Millisecond,
		MaxDelay: time.Duration(cfg.RedialMaxDelay) * time.Millisecond,
		Jitter:   float64(cfg.RedialJitter) / 100,
	}, source, nil
}

// minFragment is the smallest fragment IEEE 1815-2012 clause 4.3 allows a device to be
// limited to.
const minFragment = 249

// parseThresholds parses --sa-threshold: index=threshold pairs of the security statistics
// of IEEE 1815-2012 Table 7-6.
func parseThresholds(list []string) (map[secauth.Statistic]uint16, error) {
	var out map[secauth.Statistic]uint16
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		k, v, ok := strings.Cut(item, "=")
		i, ierr := strconv.ParseUint(strings.TrimSpace(k), 10, 16)
		n, nerr := strconv.ParseUint(strings.TrimSpace(v), 10, 16)
		if !ok || ierr != nil || nerr != nil || i >= uint64(secauth.NumStatistics) || n == 0 {
			return nil, fmt.Errorf("invalid --sa-threshold %q: use index=threshold, index 0-%d, threshold 1-65535", item, secauth.NumStatistics-1)
		}
		if out == nil {
			out = map[secauth.Statistic]uint16{}
		}
		out[secauth.Statistic(i)] = uint16(n)
	}
	return out, nil
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
		switch st := f.Stations()[0]; {
		case p.peer != nil && p.fixed:
			go func() { served <- st.ServeUDPFixed(pc, p.peer) }()
		case p.peer != nil:
			go func() { served <- st.ServeUDPTo(pc, p.peer) }()
		default:
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
