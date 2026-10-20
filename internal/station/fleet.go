package station

import (
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/otfabric/go-dnp3/server"

	"github.com/otfabric/dnp3ctl/internal/types"
)

// Fleet is what `server start` serves: one outstation, or several behind the same
// connections, each at its own link address (a multi-drop channel, served by a
// server.Group).
type Fleet struct {
	stations []*Station
	group    *server.Group
}

// NewFleet builds an outstation of every model. With several, every event names the
// outstation it is about, and the models must have distinct link addresses.
func NewFleet(models []*Model, o Options) (*Fleet, error) {
	f := &Fleet{}
	o.Tag = len(models) > 1
	for _, m := range models {
		st, err := New(m, o)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
		f.stations = append(f.stations, st)
	}
	if len(f.stations) > 1 {
		servers := make([]*server.Server, 0, len(f.stations))
		for _, st := range f.stations {
			servers = append(servers, st.Server())
		}
		g, err := server.NewGroup(servers...)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
		f.group = g
	}
	return f, nil
}

// Stations returns the outstations of the fleet, in the order of the models.
func (f *Fleet) Stations() []*Station { return f.stations }

// Describe names the outstations for the event log: "Demo, link address 10".
func (f *Fleet) Describe() string {
	parts := make([]string, 0, len(f.stations))
	for _, st := range f.stations {
		name := st.model.Outstation.Name
		if name == "" {
			name = "outstation"
		}
		parts = append(parts, fmt.Sprintf("%s, link address %d", name, st.model.Address()))
	}
	return strings.Join(parts, "; ")
}

// Serve accepts connections on l until Close.
func (f *Fleet) Serve(l net.Listener) error {
	if f.group != nil {
		return f.group.Serve(l)
	}
	return f.stations[0].Serve(l)
}

// ServeUDP serves DNP3 over UDP on pc until Close. As for one outstation, the event log of
// every outstation names the master the last datagram came from as the remote.
func (f *Fleet) ServeUDP(pc net.PacketConn) error {
	if f.group != nil {
		pp := f.stations[0].watchUDP(pc, nil)
		for _, st := range f.stations[1:] {
			st.udp.Store(pp)
		}
		return f.group.ServeUDP(pp)
	}
	return f.stations[0].ServeUDP(pc)
}

// ServeConn serves one stream, a serial port for example, until it ends or Close.
func (f *Fleet) ServeConn(rw io.ReadWriteCloser, remote net.Addr) error {
	if f.group != nil {
		return f.group.ServeConn(rw, remote)
	}
	return f.stations[0].ServeConn(rw, remote)
}

// Close stops the outstations, their simulations and the connections.
func (f *Fleet) Close() error {
	if f.group != nil {
		_ = f.group.Close()
	}
	for _, st := range f.stations {
		_ = st.Close()
	}
	return nil
}

// event logs an event about the fleet as a whole, such as listening or stopping.
func (f *Fleet) event(e types.ServerEvent) {
	f.stations[0].emit(e, false)
}

// shutdown closes the fleet and waits for the result of its Serve. Serve returns nil after
// Close; anything else went wrong while stopping and is logged.
func (f *Fleet) shutdown(served <-chan error) {
	f.event(types.ServerEvent{Event: "stopping"})
	_ = f.Close()
	if err := <-served; err != nil {
		f.event(types.ServerEvent{Event: "error", Detail: fmt.Sprintf("stopping: %v", err)})
	}
}
