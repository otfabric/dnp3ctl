// SPDX-License-Identifier: MIT

package dnp

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/otfabric/go-dnp3/link"

	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// MaxDiscoverHosts is the number of hosts `discover` probes without --force-large-scan.
const MaxDiscoverHosts = 4096

// defaultProbeTimeout bounds one probe when --timeout is 0.
const defaultProbeTimeout = time.Second

// ExpandHosts turns the --subnets values into the list of addresses to probe: a CIDR prefix
// (network and broadcast address left out for IPv4 prefixes shorter than /31), a range on
// the last octet ("192.168.1.10-20"), a single address or a host name.
func ExpandHosts(specs []string, limit int) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	add := func(h string) error {
		if seen[h] {
			return nil
		}
		if limit > 0 && len(out) >= limit {
			return fmt.Errorf("more than %d hosts to probe: narrow --subnets, or pass --force-large-scan", limit)
		}
		seen[h] = true
		out = append(out, h)
		return nil
	}
	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		switch {
		case strings.Contains(spec, "/"):
			prefix, err := netip.ParsePrefix(spec)
			if err != nil {
				return nil, fmt.Errorf("invalid subnet %q: %w", spec, err)
			}
			if !prefix.Addr().Is4() {
				return nil, fmt.Errorf("invalid subnet %q: only IPv4 prefixes can be swept; give IPv6 hosts one by one", spec)
			}
			prefix = prefix.Masked()
			first := binary.BigEndian.Uint32(prefix.Addr().AsSlice())
			size := uint64(1) << (32 - prefix.Bits())
			lo, hi := uint64(0), size-1
			if prefix.Bits() < 31 {
				lo, hi = 1, size-2
			}
			if limit > 0 && hi-lo+1 > uint64(limit) {
				return nil, fmt.Errorf("subnet %s has %d hosts, more than %d: narrow --subnets, or pass --force-large-scan", spec, hi-lo+1, limit)
			}
			for i := lo; i <= hi; i++ {
				var b [4]byte
				binary.BigEndian.PutUint32(b[:], first+uint32(i))
				if err := add(netip.AddrFrom4(b).String()); err != nil {
					return nil, err
				}
			}
		case strings.Contains(spec, "-") && strings.Count(spec, ".") == 3:
			base, last, _ := strings.Cut(spec, "-")
			addr, err := netip.ParseAddr(base)
			end, perr := strconv.ParseUint(last, 10, 8)
			if err != nil || perr != nil || !addr.Is4() {
				return nil, fmt.Errorf("invalid range %q: use the form 192.168.1.10-20", spec)
			}
			b := addr.As4()
			if uint8(end) < b[3] {
				return nil, fmt.Errorf("invalid range %q: the end is below the start", spec)
			}
			for i := int(b[3]); i <= int(end); i++ {
				b[3] = uint8(i)
				if err := add(netip.AddrFrom4(b).String()); err != nil {
					return nil, err
				}
			}
		default:
			if err := add(spec); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func parsePorts(list []string) ([]uint16, error) {
	var out []uint16
	for _, p := range list {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseUint(p, 10, 16)
		if err != nil || n == 0 {
			return nil, fmt.Errorf("invalid port %q: use 1-65535", p)
		}
		out = append(out, uint16(n))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no port given: set --ports")
	}
	return out, nil
}

func parseAddresses(list []string) ([]link.Address, error) {
	var out []link.Address
	for _, s := range list {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		n, err := strconv.ParseUint(s, 0, 16)
		if err != nil || (link.Address(n) > link.MaxStationAddress && link.Address(n) != link.SelfAddress) {
			return nil, fmt.Errorf("invalid link address %q in --outstations: use 0-65519, or 65532 for the self address", s)
		}
		out = append(out, link.Address(n))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no link address given: set --outstations")
	}
	return out, nil
}

func discoverLimit(cfg *config.DiscoverConfig) int {
	if cfg.ForceLarge {
		return 0
	}
	return MaxDiscoverHosts
}

// CheckDiscover validates the configuration of `discover`.
func CheckDiscover(cfg *config.DiscoverConfig) error {
	hosts, err := ExpandHosts(cfg.Subnets, discoverLimit(cfg))
	if err != nil {
		return err
	}
	if len(hosts) == 0 {
		return fmt.Errorf("nothing to probe: set --subnets (e.g. --subnets 192.168.1.0/24)")
	}
	if _, err := parsePorts(cfg.Ports); err != nil {
		return err
	}
	if _, err := parseAddresses(cfg.Outstations); err != nil {
		return err
	}
	if link.Address(cfg.Master) > link.MaxStationAddress {
		return fmt.Errorf("invalid --master %d: use 0-65519", cfg.Master)
	}
	if cfg.Parallel == 0 || cfg.Parallel > 256 {
		return fmt.Errorf("invalid --parallel %d: use 1-256", cfg.Parallel)
	}
	return nil
}

// Probe results.
const (
	probeClosed = ""
	probeOpen   = "open"
	probeDNP3   = "dnp3"
)

// probeResult is what one probe found.
type probeResult struct {
	status             string
	outstation, master uint16
	rtt                time.Duration
}

// probe connects to addr and sends REQUEST_LINK_STATUS to each of the outstation
// addresses, the least a master can say: an outstation answers it with LINK_STATUS from its
// own address, without any application layer exchange.
func probe(ctx context.Context, addr string, master link.Address, outstations []link.Address, timeout time.Duration) probeResult {
	begin := time.Now()
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return probeResult{}
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	for _, dest := range outstations {
		f := link.Frame{Dir: true, Function: link.PrimaryRequestLinkStatus, Dest: dest, Src: master}
		b, err := f.Encode()
		if err != nil {
			return probeResult{status: probeOpen, rtt: time.Since(begin)}
		}
		if _, err := conn.Write(b); err != nil {
			return probeResult{status: probeOpen, rtt: time.Since(begin)}
		}
	}
	r := link.NewReader(conn)
	for range 8 { // an outstation may send something else first, or the reader skip noise
		f, err := r.ReadFrame()
		if errors.Is(err, link.ErrFraming) {
			continue
		}
		if err != nil {
			return probeResult{status: probeOpen, rtt: time.Since(begin)}
		}
		if !f.Function.Primary() && !f.Dir {
			return probeResult{status: probeDNP3, outstation: uint16(f.Src), master: uint16(f.Dest), rtt: time.Since(begin)}
		}
	}
	return probeResult{status: probeOpen, rtt: time.Since(begin)}
}

// Discover probes every address of the subnets for a DNP3 outstation. progress receives a
// line per outstation found.
func Discover(ctx context.Context, cfg *config.DiscoverConfig, progress io.Writer) (*types.DiscoverResult, error) {
	hosts, err := ExpandHosts(cfg.Subnets, discoverLimit(cfg))
	if err != nil {
		return nil, err
	}
	ports, err := parsePorts(cfg.Ports)
	if err != nil {
		return nil, err
	}
	outstations, err := parseAddresses(cfg.Outstations)
	if err != nil {
		return nil, err
	}
	timeout := defaultProbeTimeout
	if cfg.Timeout != 0 {
		timeout = time.Duration(cfg.Timeout) * time.Millisecond
	}

	type job struct {
		host string
		port uint16
	}
	jobs := make(chan job)
	var (
		mu      sync.Mutex
		devices []types.DiscoveredDevice
		wg      sync.WaitGroup
	)
	begin := time.Now()
	for range int(cfg.Parallel) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				p := probe(ctx, net.JoinHostPort(j.host, strconv.Itoa(int(j.port))), link.Address(cfg.Master), outstations, timeout)
				if p.status == probeClosed {
					continue
				}
				dev := types.DiscoveredDevice{IP: j.host, Port: j.port, Status: p.status, RTTMs: ms(p.rtt)}
				if p.status == probeDNP3 {
					dev.Outstation, dev.Master = &p.outstation, &p.master
				}
				mu.Lock()
				devices = append(devices, dev)
				if progress != nil && (p.status == probeDNP3 || cfg.Debug) {
					_, _ = fmt.Fprintf(progress, "discover: %s port %d: %s\n", j.host, j.port, p.status)
				}
				mu.Unlock()
			}
		}()
	}
feed:
	for _, h := range hosts {
		for _, p := range ports {
			select {
			case jobs <- job{h, p}:
			case <-ctx.Done():
				break feed
			}
		}
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, WrapError(err)
	}

	sort.Slice(devices, func(i, j int) bool {
		a, b := devices[i], devices[j]
		ai, aerr := netip.ParseAddr(a.IP)
		bi, berr := netip.ParseAddr(b.IP)
		switch {
		case aerr == nil && berr == nil && ai != bi:
			return ai.Less(bi)
		case a.IP != b.IP:
			return a.IP < b.IP
		}
		return a.Port < b.Port
	})
	res := &types.DiscoverResult{Subnets: cfg.Subnets, Ports: ports, Hosts: len(hosts), DurationMs: ms(time.Since(begin)), Devices: []types.DiscoveredDevice{}}
	for _, d := range devices {
		res.Devices = append(res.Devices, d)
		if d.Status == probeDNP3 {
			res.Found++
		}
	}
	return res, nil
}
