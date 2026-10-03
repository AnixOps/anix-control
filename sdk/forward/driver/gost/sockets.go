package gost

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// socket is a listening TCP or unconnected UDP socket on the host. addr
// is invalid for a wildcard (0.0.0.0, ::, *).
type socket struct {
	network string
	addr    netip.Addr
	port    uint32
}

// listSockets reads the host's listening sockets with `ss -H -l -n -t -u`.
// It needs no privilege: without -p ss does not name the processes, so
// the driver tells its own sockets from others' by what its applied
// configuration declares (see conflicts).
func listSockets(ctx context.Context, r Runner) ([]socket, error) {
	out, err := r.Run(ctx, "ss", []string{"-H", "-l", "-n", "-t", "-u"}, nil)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		return nil, fmt.Errorf("gost driver: list sockets: %w", err)
	}
	return parseSS(out)
}

// parseSS reads ss's listing: "Netid State Recv-Q Send-Q Local:Port
// Peer:Port [Process]", one socket per line, the local address as
// 0.0.0.0, *, [::], [2001:db8::1] or 127.0.0.53%lo.
func parseSS(out []byte) ([]socket, error) {
	var socks []socket
	for line := range strings.Lines(string(out)) {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if len(f) < 5 {
			return nil, fmt.Errorf("gost driver: ss printed %q", firstLine(line))
		}
		network := f[0]
		if network != "tcp" && network != "udp" {
			continue
		}
		local := f[4]
		i := strings.LastIndexByte(local, ':')
		if i < 0 {
			return nil, fmt.Errorf("gost driver: ss printed local address %q", clip(local))
		}
		port, err := strconv.ParseUint(local[i+1:], 10, 16)
		if err != nil {
			return nil, fmt.Errorf("gost driver: ss printed local address %q", clip(local))
		}
		host := strings.TrimSuffix(strings.TrimPrefix(local[:i], "["), "]")
		if j := strings.IndexByte(host, '%'); j >= 0 {
			host = host[:j]
		}
		s := socket{network: network, port: uint32(port)}
		if host != "*" {
			a, err := netip.ParseAddr(host)
			if err != nil {
				return nil, fmt.Errorf("gost driver: ss printed local address %q", clip(local))
			}
			if a = a.Unmap(); !a.IsUnspecified() {
				s.addr = a
			}
		}
		socks = append(socks, s)
	}
	return socks, nil
}

// overlaps reports whether a socket and a listener of a configuration
// compete for the same port: the same network and port, and the same
// address or a wildcard on either side (conservatively: gost listens on
// every address of both families when the hop names none).
func overlaps(s socket, l manifestListener) bool {
	if s.network != l.Network || s.port != l.Port {
		return false
	}
	if !s.addr.IsValid() || l.Address == "" {
		return true
	}
	a, err := netip.ParseAddr(l.Address)
	return err == nil && a.Unmap() == s.addr
}

// conflicts answers ErrConflict when a socket the driver does not own
// holds a port of the new configuration. The sockets that may be the
// driver's are those its running, owned configuration declares (ours).
func conflicts(socks []socket, want, ours []manifestListener) error {
	for _, l := range want {
		for _, s := range socks {
			if !overlaps(s, l) {
				continue
			}
			mine := false
			for _, o := range ours {
				if overlaps(s, o) {
					mine = true
					break
				}
			}
			if !mine {
				return fmt.Errorf("%w: %s port %d is held by a process the driver does not run", driver.ErrConflict, l.Network, l.Port)
			}
		}
	}
	return nil
}

// bound reports whether every listener has a socket on the host.
func bound(socks []socket, want []manifestListener) bool {
	for _, l := range want {
		found := false
		for _, s := range socks {
			if s.network == l.Network && s.port == l.Port && (l.Address == "" && !s.addr.IsValid() || l.Address != "" && s.addr.String() == l.Address) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
