package gost

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// ActiveConns answers, per upstream of the applied hops (address and
// port), the established TCP connections and connected UDP sockets the
// host holds towards it, read with `ss -H -t -u -n` (no privilege). It is
// the source of least-connections re-weighting (sdk/forward/leastconn,
// L1) for gost hops: gost counts connections per service, not per
// upstream. It is approximate: a mux or encrypted link counts its
// carriers, not the streams inside them; sockets of other processes to the
// same address and port count too, and an upstream that several hops
// share counts once for all of them. An upstream without a socket is
// absent (0).
func (d *Driver) ActiveConns(ctx context.Context) (map[netip.AddrPort]uint64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d.mu.RLock()
	h, err := d.readHost()
	d.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	out := map[netip.AddrPort]uint64{}
	if !h.owned || !h.state.applied() {
		return out, nil
	}
	ups := map[netip.AddrPort]bool{}
	for _, mh := range h.state.Hops {
		for _, u := range mh.Upstreams {
			a, err := netip.ParseAddr(u.Address)
			if err != nil || u.Port > 65535 {
				continue
			}
			ups[netip.AddrPortFrom(a, uint16(u.Port))] = true // #nosec G115 -- checked
		}
	}
	raw, err := d.runner.Run(ctx, "ss", []string{"-H", "-t", "-u", "-n"}, nil)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		return nil, fmt.Errorf("gost driver: list connections: %w", err)
	}
	peers, err := parsePeers(raw)
	if err != nil {
		return nil, err
	}
	for _, p := range peers {
		if ups[p] {
			out[p]++
		}
	}
	return out, nil
}

// parsePeers reads the peers of the established sockets in ss's listing
// without -l: "Netid State Recv-Q Send-Q Local:Port Peer:Port".
func parsePeers(out []byte) ([]netip.AddrPort, error) {
	var peers []netip.AddrPort
	for line := range strings.Lines(string(out)) {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if len(f) < 6 {
			return nil, fmt.Errorf("gost driver: ss printed %q", firstLine(line))
		}
		if f[0] != "tcp" && f[0] != "udp" || f[1] != "ESTAB" {
			continue
		}
		peer := f[5]
		i := strings.LastIndexByte(peer, ':')
		if i < 0 {
			continue
		}
		port, err := strconv.ParseUint(peer[i+1:], 10, 16)
		if err != nil {
			continue
		}
		host := strings.TrimSuffix(strings.TrimPrefix(peer[:i], "["), "]")
		if j := strings.IndexByte(host, '%'); j >= 0 {
			host = host[:j]
		}
		a, err := netip.ParseAddr(host)
		if err != nil {
			continue
		}
		peers = append(peers, netip.AddrPortFrom(a.Unmap(), uint16(port))) // #nosec G115 -- parsed as 16 bits
	}
	return peers, nil
}
