package station

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dnp3 "github.com/otfabric/go-dnp3"
	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/link"
	"github.com/otfabric/go-dnp3/secauth"
	"github.com/otfabric/go-dnp3/server"

	"github.com/otfabric/dnp3ctl/internal/dnp"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// Options configure a Station beyond its model.
type Options struct {
	// Link configures the data link layer.
	Link server.Link
	// Auth enables Secure Authentication v5 with this user.
	Auth *secauth.User
	// TLS makes Run serve TLS on its listener (Serve takes connections as they are), and is
	// the TLS client configuration of DialAndServe.
	TLS *tls.Config
	// MaxConnections bounds concurrent connections (0 = the library default, 16).
	MaxConnections int
	// Log receives every event; nil discards them.
	Log func(types.ServerEvent)
	// Metrics, when set, also receives every callback (the --debug trace).
	Metrics dnp3.Metrics
	// Now is the clock of time stamps; nil is time.Now.
	Now func() time.Time
	// Logger receives go-dnp3's own log entries (the --debug log); nil is silent.
	Logger dnp3.Logger
	// AuthReplyTimeout, StatisticsClass and StatisticsThreshold tune Secure Authentication;
	// zero selects go-dnp3's defaults.
	AuthReplyTimeout    time.Duration
	StatisticsClass     uint8
	StatisticsThreshold uint32
}

// Station is a simulated outstation serving a model.
type Station struct {
	model    *Model
	opts     Options
	srv      *server.Server
	controls map[pointKey]*control
	files    *fileServer

	udp  atomic.Pointer[peerConn] // the socket of ServeUDP, when serving UDP
	stop chan struct{}
	wg   sync.WaitGroup
	mu   sync.Mutex // serializes the events of the log
}

// control is a control of the model, resolved.
type control struct {
	spec   ControlSpec
	kind   string
	target *pointKey
	status *app.CommandStatus
}

// New builds the outstation of m and starts its simulations. Serve it with Serve,
// ServeUDP or ServeConn; Close stops it.
func New(m *Model, o Options) (*Station, error) {
	if o.Now == nil {
		o.Now = time.Now
	}
	st := &Station{model: m, opts: o, controls: map[pointKey]*control{}, stop: make(chan struct{})}
	files, err := newFileServer(m, st.event, o.Now)
	if err != nil {
		return nil, err
	}
	st.files = files
	opts, err := st.serverOptions()
	if err != nil {
		return nil, err
	}
	st.srv, err = server.New(opts...)
	if err != nil {
		return nil, err
	}
	if err := st.load(false); err != nil {
		_ = st.srv.Close()
		return nil, err
	}
	st.resolveControls()
	var iin app.IIN
	for _, name := range m.Outstation.Indications {
		iin |= indicationBits[strings.ToLower(strings.TrimSpace(name))]
	}
	if iin != 0 {
		st.srv.SetIndications(iin, true)
	}
	st.simulate()
	return st, nil
}

func (st *Station) serverOptions() ([]server.Option, error) {
	m, o := st.model, st.model.Outstation
	opts := []server.Option{
		server.WithAddress(link.Address(m.Address())),
		server.WithHandler(&handler{st: st}),
		server.WithLink(st.opts.Link),
		server.WithMetrics(&eventMetrics{st: st, trace: st.opts.Metrics}),
		server.WithFileServer(st.files),
		server.WithSelfAddress(o.SelfAddress),
	}
	if len(o.Masters) > 0 {
		addrs := make([]link.Address, 0, len(o.Masters))
		for _, a := range o.Masters {
			addrs = append(addrs, link.Address(a))
		}
		opts = append(opts, server.WithMasters(addrs...))
	}
	if u := o.Unsolicited; u.Enabled {
		classes := make([]server.Class, 0, len(u.Classes))
		for _, c := range u.Classes {
			classes = append(classes, server.Class(c))
		}
		opts = append(opts, server.WithUnsolicited(server.Unsolicited{
			Enabled: true, Classes: classes, HoldTime: time.Duration(u.HoldTime), HoldCount: u.HoldCount,
			Retries: u.Retries, RetryDelay: time.Duration(u.RetryDelay),
		}))
	}
	if d := time.Duration(o.TimeSyncInterval); d != 0 {
		opts = append(opts, server.WithTimeSync(d))
	}
	if d := time.Duration(o.SelectTimeout); d != 0 {
		opts = append(opts, server.WithSelectTimeout(d))
	}
	if d := time.Duration(o.ConfirmTimeout); d != 0 {
		opts = append(opts, server.WithConfirmTimeout(d))
	}
	if o.EventLimit != 0 {
		limits := map[server.PointType]int{}
		for _, t := range pointTypes {
			limits[t] = o.EventLimit
		}
		opts = append(opts, server.WithEventLimits(limits))
	}
	if len(o.Attributes) > 0 {
		attrs := map[uint8]app.Attribute{}
		for k, v := range o.Attributes {
			variation, err := attributeVariation(k)
			if err != nil {
				return nil, err
			}
			attrs[variation] = app.StringAttribute(v)
		}
		opts = append(opts, server.WithAttributes(attrs))
	}
	for _, as := range o.AttributeSets {
		attrs, err := as.attributes()
		if err != nil {
			return nil, err
		}
		opts = append(opts, server.WithAttributeSet(as.Set, attrs, as.writable()...))
	}
	if st.opts.TLS != nil {
		opts = append(opts, server.WithTLS(st.opts.TLS))
	}
	if st.opts.MaxConnections != 0 {
		opts = append(opts, server.WithMaxConnections(st.opts.MaxConnections))
	}
	if st.opts.Auth != nil {
		opts = append(opts, server.WithSecureAuthentication(server.SecureAuth{
			User: *st.opts.Auth, ReplyTimeout: st.opts.AuthReplyTimeout,
			StatisticsClass: server.Class(st.opts.StatisticsClass), StatisticsThreshold: st.opts.StatisticsThreshold,
		}))
	}
	if st.opts.Logger != nil {
		opts = append(opts, server.WithLogger(st.opts.Logger))
	}
	return opts, nil
}

// load puts every point of the model into the database with its initial value; reset only
// sets the values back, without events.
func (st *Station) load(reset bool) error {
	db := st.srv.Database()
	for _, p := range st.model.Points {
		t := pointTypes[strings.ToLower(strings.TrimSpace(p.Type))]
		v, err := initialValue(t, p)
		if err != nil {
			return err
		}
		if reset {
			_ = db.Update(t, p.Index, v, server.EventSuppress)
			continue
		}
		cfg := server.PointConfig{Class: server.Class(p.Class), StaticVariation: p.StaticVariation, EventVariation: p.EventVariation,
			Deadband: p.Deadband, ExcludeFromClass0: p.ExcludeFromClass0,
			CommandEventClass: server.Class(p.CommandEventClass), CommandEventVariation: p.CommandEventVariation}
		if err := db.Add(t, p.Index, cfg, v); err != nil {
			return fmt.Errorf("%s %d: %w", typeName(t), p.Index, err)
		}
	}
	return nil
}

func (st *Station) resolveControls() {
	db := st.srv.Database()
	for _, c := range st.model.Controls {
		kind := strings.ToLower(strings.TrimSpace(c.Type))
		ctl := &control{spec: c, kind: kind}
		targets := controlTargets[kind]
		switch {
		case c.Target != nil:
			ctl.target = &pointKey{pointTypes[strings.ToLower(strings.TrimSpace(c.Target.Type))], c.Target.Index}
		default:
			if _, ok := db.Get(targets[0], c.Index); ok {
				ctl.target = &pointKey{targets[0], c.Index}
			}
		}
		if c.Status != "" {
			s := commandStatuses[strings.ToUpper(strings.TrimSpace(c.Status))]
			ctl.status = &s
		}
		st.controls[pointKey{targets[0], c.Index}] = ctl
	}
}

// hasAttribute reports whether the model gives the attribute v of set 0.
func (st *Station) hasAttribute(v uint8) bool {
	for k := range st.model.Outstation.Attributes {
		if variation, err := attributeVariation(k); err == nil && variation == v {
			return true
		}
	}
	return false
}

// Server returns the go-dnp3 outstation.
func (st *Station) Server() *server.Server { return st.srv }

// Model returns the model the station serves.
func (st *Station) Model() *Model { return st.model }

// Serve accepts connections on l until Close.
func (st *Station) Serve(l net.Listener) error { return st.srv.Serve(l) }

// ServeUDP serves DNP3 over UDP on pc until Close. The outstation answers the endpoint
// the last datagram came from, which the event log names as the remote.
func (st *Station) ServeUDP(pc net.PacketConn) error {
	return st.srv.ServeUDP(st.watchUDP(pc, nil))
}

// ServeUDPTo is ServeUDP with the master's address known from the start, so that
// unsolicited responses reach it before it has sent anything.
func (st *Station) ServeUDPTo(pc net.PacketConn, peer net.Addr) error {
	return st.srv.ServeUDPTo(st.watchUDP(pc, peer), peer)
}

// watchUDP wraps the socket so that the event log can name the master, peer when known.
func (st *Station) watchUDP(pc net.PacketConn, peer net.Addr) *peerConn {
	pp := &peerConn{PacketConn: pc}
	if peer != nil {
		s := peer.String()
		pp.peer.Store(&s)
	}
	st.udp.Store(pp)
	return pp
}

// peerConn records where the last datagram came from: go-dnp3 names a UDP channel by
// the local address of its socket, the master is the peer.
type peerConn struct {
	net.PacketConn
	peer atomic.Pointer[string]
}

func (c *peerConn) ReadFrom(b []byte) (int, net.Addr, error) {
	n, addr, err := c.PacketConn.ReadFrom(b)
	if addr != nil {
		s := addr.String()
		c.peer.Store(&s)
	}
	return n, addr, err
}

// remoteText names the remote end of a channel in the event log: for UDP the master the
// outstation answers, for a stream its address.
func (st *Station) remoteText(a net.Addr) string {
	if a == nil {
		return ""
	}
	if pp := st.udp.Load(); pp != nil && a.String() == pp.LocalAddr().String() {
		if peer := pp.peer.Load(); peer != nil {
			return *peer
		}
		return ""
	}
	return a.String()
}

// DialAndServe connects to the master at addr, with TLS when the options have it, serves
// the connection and connects again whenever it ends (the dual end point), until Close.
func (st *Station) DialAndServe(ctx context.Context, addr string) error {
	return st.srv.DialAndServe(ctx, addr, server.Redial{})
}

// ServeConn serves one stream, a serial port for example, until it ends or Close.
func (st *Station) ServeConn(rw io.ReadWriteCloser, remote net.Addr) error {
	return st.srv.ServeConn(rw, remote)
}

// Close stops the simulations and the outstation.
func (st *Station) Close() error {
	select {
	case <-st.stop:
	default:
		close(st.stop)
	}
	st.wg.Wait()
	return st.srv.Close()
}

func (st *Station) event(e types.ServerEvent) {
	if st.opts.Log == nil {
		return
	}
	e.Timestamp = st.opts.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
	st.mu.Lock()
	defer st.mu.Unlock()
	st.opts.Log(e)
}

func index(i uint32) *uint32 { return &i }

// handler executes controls and the optional functions of the outstation.
type handler struct {
	st *Station
}

// check returns the status a control gets before it is executed.
func (h *handler) check(c server.Command) (*control, app.CommandStatus) {
	kind := "analog"
	if c.GV.Group() == 12 {
		kind = "crob"
	}
	ctl, ok := h.st.controls[pointKey{controlTargets[kind][0], c.Index}]
	if !ok {
		return nil, app.StatusNotSupported
	}
	if ctl.status != nil {
		return ctl, *ctl.status
	}
	switch v := c.Value.(type) {
	case app.CROB:
		if c.GV != app.G12V1 {
			return ctl, app.StatusNotSupported // pattern control blocks are not simulated
		}
	case app.AnalogOutput:
		if (ctl.spec.Min != nil && v.Value < *ctl.spec.Min) || (ctl.spec.Max != nil && v.Value > *ctl.spec.Max) {
			return ctl, app.StatusOutOfRange
		}
	}
	return ctl, app.StatusSuccess
}

func controlValue(v app.Value) any {
	switch c := v.(type) {
	case app.CROB:
		return dnp.CROBText(c)
	case app.AnalogOutput:
		return c.Value
	}
	return nil
}

// Select implements server.Handler.
func (h *handler) Select(_ context.Context, c server.Command) app.CommandStatus {
	_, status := h.check(c)
	h.st.event(types.ServerEvent{Event: "control", Function: "SELECT", Object: c.GV.String(), Index: index(c.Index), Value: controlValue(c.Value),
		Status: status.String(), Detail: fmt.Sprintf("master %d", c.Master)})
	return status
}

// Operate implements server.Handler.
func (h *handler) Operate(_ context.Context, c server.Command, op server.OperateType) app.CommandStatus {
	ctl, status := h.check(c)
	if status == app.StatusSuccess && op != server.SelectBeforeOperate && ctl.spec.SelectRequired {
		status = app.StatusNoSelect
	}
	detail := fmt.Sprintf("%s, master %d", op, c.Master)
	if status == app.StatusSuccess && ctl.target != nil {
		if applied := h.apply(ctl, c); applied != "" {
			detail += ", " + applied
		}
	}
	h.st.event(types.ServerEvent{Event: "control", Function: "OPERATE", Object: c.GV.String(), Index: index(c.Index), Value: controlValue(c.Value),
		Status: status.String(), Detail: detail})
	return status
}

// apply sets the target of a control and returns what changed.
func (h *handler) apply(ctl *control, c server.Command) string {
	db := h.st.srv.Database()
	t := ctl.target
	switch v := c.Value.(type) {
	case app.CROB:
		var on bool
		switch {
		case v.TripClose == app.TCCClose:
			on = true
		case v.TripClose == app.TCCTrip:
			on = false
		case v.Op == app.OpLatchOn || v.Op == app.OpPulseOn:
			on = true
		case v.Op == app.OpLatchOff || v.Op == app.OpPulseOff:
			on = false
		default:
			return ""
		}
		cur, _ := db.Get(t.t, t.index)
		b, _ := cur.(app.Binary)
		b.Value, b.Time = on, 0
		_ = db.Update(t.t, t.index, b, server.EventOnChange)
		return fmt.Sprintf("%s %d = %t", typeName(t.t), t.index, on)
	case app.AnalogOutput:
		cur, _ := db.Get(t.t, t.index)
		a, _ := cur.(app.Analog)
		a.Value, a.Time = v.Value, 0
		_ = db.Update(t.t, t.index, a, server.EventOnChange)
		return fmt.Sprintf("%s %d = %g", typeName(t.t), t.index, v.Value)
	}
	return ""
}

// Restart implements server.RestartHandler.
func (h *handler) Restart(_ context.Context, cold bool) (time.Duration, error) {
	kind := "warm"
	if cold {
		kind = "cold"
	}
	delay := time.Duration(h.st.model.Outstation.RestartDelay)
	if delay == 0 {
		h.st.event(types.ServerEvent{Event: "restart", Detail: kind + " restart refused: the model has no restart_delay"})
		return 0, errors.New("restarts are not enabled")
	}
	h.st.event(types.ServerEvent{Event: "restart", Detail: fmt.Sprintf("%s restart in %s", kind, delay)})
	h.st.wg.Add(1)
	go func() {
		defer h.st.wg.Done()
		select {
		case <-h.st.stop:
			return
		case <-time.After(delay):
		}
		if cold {
			_ = h.st.load(true)
		}
		h.st.srv.Restart()
		h.st.event(types.ServerEvent{Event: "restart", Detail: kind + " restart done"})
	}()
	return delay, nil
}

// SetTime implements server.TimeHandler.
func (h *handler) SetTime(_ context.Context, t time.Time) {
	h.st.event(types.ServerEvent{Event: "time-sync", Detail: t.UTC().Format("2006-01-02T15:04:05.000Z07:00")})
}

// userAttributes are the attributes of set 0 that IEEE 1815-2012 leaves to the user to
// assign: the ones a master may write, when the model has them.
var userAttributes = map[uint8]bool{
	app.AttrOwnerName: true, app.AttrUserLocation: true, app.AttrUserID: true, app.AttrUserDeviceName: true,
}

// WriteAttribute implements server.WriteHandler. Of set 0 the user-assigned attributes the
// model gives can be written; others are refused (PARAMETER_ERROR).
func (h *handler) WriteAttribute(_ context.Context, variation uint8, a app.Attribute) error {
	e := types.ServerEvent{Event: "write", Object: "g0v" + fmt.Sprint(variation), Value: a.Text(), Detail: dnp.AttributeName(variation)}
	if !userAttributes[variation] || !h.st.hasAttribute(variation) {
		e.Detail += ": refused, not a writable attribute"
		h.st.event(e)
		return fmt.Errorf("attribute %d is not writable", variation)
	}
	h.st.event(e)
	return nil
}

// WriteAttributeSet implements server.AttributeSetWriteHandler.
func (h *handler) WriteAttributeSet(_ context.Context, set uint16, variation uint8, a app.Attribute) error {
	h.st.event(types.ServerEvent{Event: "write", Object: "g0v" + fmt.Sprint(variation), Value: dnp.AttributeValue(a, set), Detail: fmt.Sprintf("attribute set %d", set)})
	return nil
}

// Application implements server.ApplicationHandler: the management functions are accepted
// and logged, except those the model refuses.
func (h *handler) Application(_ context.Context, fc app.FunctionCode, _ []app.Header) app.IIN {
	e := types.ServerEvent{Event: "manage", Function: fc.String()}
	for _, name := range h.st.model.Outstation.RefusedFunctions {
		if refused, err := dnp.ManagementFunction(name); err == nil && refused == fc {
			e.Detail = "refused"
			h.st.event(e)
			return app.IINNoFuncCodeSupport
		}
	}
	h.st.event(e)
	return 0
}

// WriteOctetString implements server.WriteHandler.
func (h *handler) WriteOctetString(_ context.Context, i uint32, data []byte) error {
	h.st.event(types.ServerEvent{Event: "write", Object: "g110", Index: index(i), Value: string(data)})
	return nil
}

// simulate starts a goroutine per simulated point.
func (st *Station) simulate() {
	for _, p := range st.model.Points {
		if p.Simulate == nil {
			continue
		}
		t := pointTypes[strings.ToLower(strings.TrimSpace(p.Type))]
		st.wg.Add(1)
		go st.run(t, p)
	}
}

func (st *Station) run(t server.PointType, p PointSpec) {
	defer st.wg.Done()
	s := p.Simulate
	tick := time.NewTicker(time.Duration(s.Interval))
	defer tick.Stop()
	db := st.srv.Database()
	step := s.Step
	if step == 0 {
		step = 1
	}
	for n := 1; ; n++ {
		select {
		case <-st.stop:
			return
		case <-tick.C:
		}
		cur, ok := db.Get(t, p.Index)
		if !ok {
			continue
		}
		var next app.Value
		switch v := cur.(type) {
		case app.Binary:
			v.Value, v.Time = !v.Value, 0
			next = v
		case app.DoubleBitBinary:
			v.Value, v.Time = app.DoubleBitOn, 0
			if cur.(app.DoubleBitBinary).Value == app.DoubleBitOn {
				v.Value = app.DoubleBitOff
			}
			next = v
		case app.Counter:
			v.Value, v.Time = uint32(int64(v.Value)+int64(step)), 0
			next = v
		case app.Analog:
			v.Value, v.Time = simValue(s, v.Value, n, step), 0
			next = v
		}
		if next != nil {
			_ = db.Update(t, p.Index, next, server.EventOnChange)
		}
	}
}

// simValue returns the next value of a numeric simulation.
func simValue(s *SimSpec, cur float64, n int, step float64) float64 {
	switch strings.ToLower(s.Mode) {
	case "ramp":
		next := cur + step
		if next > s.Max || next < s.Min {
			next = s.Min
		}
		return next
	case "sine":
		period := s.Step
		if period == 0 {
			period = 60
		}
		mid, amp := (s.Max+s.Min)/2, (s.Max-s.Min)/2
		return mid + amp*math.Sin(2*math.Pi*float64(n)/period)
	case "random":
		return s.Min + rand.Float64()*(s.Max-s.Min)
	default: // count
		return cur + step
	}
}

// eventMetrics turns the callbacks of the outstation into events of the log, and hands them
// on to the --debug trace.
type eventMetrics struct {
	st    *Station
	trace dnp3.Metrics
}

func (m *eventMetrics) OnConnect(remote net.Addr) {
	m.st.event(types.ServerEvent{Event: "connected", Remote: m.st.remoteText(remote)})
	if m.trace != nil {
		m.trace.OnConnect(remote)
	}
}

func (m *eventMetrics) OnDisconnect(remote net.Addr, err error) {
	e := types.ServerEvent{Event: "disconnected", Remote: m.st.remoteText(remote)}
	if err != nil {
		e.Detail = err.Error()
	}
	m.st.event(e)
	if m.trace != nil {
		m.trace.OnDisconnect(remote, err)
	}
}

func (m *eventMetrics) OnFrameSent(remote net.Addr, f *link.Frame, size int) {
	if m.trace != nil {
		m.trace.OnFrameSent(remote, f, size)
	}
}

func (m *eventMetrics) OnFrameReceived(remote net.Addr, f *link.Frame, size int) {
	if m.trace != nil {
		m.trace.OnFrameReceived(remote, f, size)
	}
}

func (m *eventMetrics) OnFramingError(remote net.Addr, err error) {
	if m.trace != nil {
		m.trace.OnFramingError(remote, err)
	}
}

func (m *eventMetrics) OnFragmentSent(remote net.Addr, f *app.Fragment, size int) {
	if m.trace != nil {
		m.trace.OnFragmentSent(remote, f, size)
	}
}

func (m *eventMetrics) OnFragmentReceived(remote net.Addr, f *app.Fragment, size int) {
	if m.trace != nil {
		m.trace.OnFragmentReceived(remote, f, size)
	}
}

func (m *eventMetrics) OnDecodeError(remote net.Addr, err error) {
	m.st.event(types.ServerEvent{Event: "error", Remote: m.st.remoteText(remote), Detail: "undecodable request: " + err.Error()})
	if m.trace != nil {
		m.trace.OnDecodeError(remote, err)
	}
}

// OnHandled implements dnp3.ServerMetrics.
func (m *eventMetrics) OnHandled(remote net.Addr, fc app.FunctionCode, d time.Duration) {
	m.st.event(types.ServerEvent{Event: "request", Remote: m.st.remoteText(remote), Function: fc.String(), Detail: fmt.Sprintf("%.1f ms", float64(d)/float64(time.Millisecond))})
}

// OnEventsDiscarded implements dnp3.ServerMetrics.
func (m *eventMetrics) OnEventsDiscarded(class int, n int) {
	m.st.event(types.ServerEvent{Event: "events-discarded", Detail: fmt.Sprintf("%d events of class %d: the buffer is full", n, class)})
}

// OnUnsolicited implements dnp3.ServerMetrics.
func (m *eventMetrics) OnUnsolicited(remote net.Addr, events int, err error) {
	e := types.ServerEvent{Event: "unsolicited", Remote: m.st.remoteText(remote), Detail: fmt.Sprintf("%d events confirmed", events)}
	if events == 0 {
		e.Detail = "null response confirmed"
	}
	if err != nil {
		e.Detail = fmt.Sprintf("%d events not confirmed: %v", events, err)
	}
	m.st.event(e)
}
