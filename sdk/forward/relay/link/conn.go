package link

import (
	"crypto/x509"
	"net"
	"net/netip"
)

// Peer is the node at the other end of a link connection.
type Peer struct {
	// Identity is the peer's SPIFFE ID as its verified certificate carries
	// it. It is empty on a plaintext link, which has no authentication
	// beyond the listener's source admission.
	Identity string
	// Chain is the peer's verified certificate chain, leaf first; empty on
	// a plaintext link.
	Chain []*x509.Certificate

	usage x509.ExtKeyUsage // what the chain was verified for
}

// Encrypted reports whether the connection is TLS 1.3 with a verified peer.
func (p Peer) Encrypted() bool { return len(p.Chain) > 0 }

// Conn is a link connection: a net.Conn whose peer has been admitted and, on
// an encrypted link, authenticated and pinned. The carrier layer runs on it.
type Conn struct {
	net.Conn
	peer Peer
}

// NetConn returns the underlying TCP connection (below TLS on an encrypted
// link), the way crypto/tls.Conn.NetConn does. The carrier layer uses it to
// close a dead link without waiting for TLS's close_notify.
func (c *Conn) NetConn() net.Conn {
	if u, ok := c.Conn.(interface{ NetConn() net.Conn }); ok {
		return u.NetConn()
	}
	return c.Conn
}

// Peer returns the node at the other end.
func (c *Conn) Peer() Peer { return c.peer }

// Encrypted reports whether the connection is encrypted.
func (c *Conn) Encrypted() bool { return c.peer.Encrypted() }

// RemoteAddrPort returns the remote address, or the zero value for an address
// that is not an IP socket address.
func (c *Conn) RemoteAddrPort() netip.AddrPort { return addrPort(c.RemoteAddr()) }

func addrPort(a net.Addr) netip.AddrPort {
	if t, ok := a.(*net.TCPAddr); ok {
		if ip, ok := netip.AddrFromSlice(t.IP); ok {
			return netip.AddrPortFrom(ip.Unmap().WithZone(""), uint16(t.Port)) // #nosec G115 -- a TCP port is 0..65535
		}
		return netip.AddrPort{}
	}
	if a == nil {
		return netip.AddrPort{}
	}
	ap, err := netip.ParseAddrPort(a.String())
	if err != nil {
		return netip.AddrPort{}
	}
	return netip.AddrPortFrom(ap.Addr().Unmap().WithZone(""), ap.Port())
}
