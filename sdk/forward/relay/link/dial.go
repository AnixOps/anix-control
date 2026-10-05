package link

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"time"
)

// Defaults of the link layer (anixops-protocol.md sections 2.5 and 4.6).
const (
	// DefaultHandshakeTimeout bounds the dial and the TLS handshake.
	DefaultHandshakeTimeout = 10 * time.Second
	// DefaultMaxPending is the number of handshakes a listener runs at once.
	DefaultMaxPending = 64
	// UserTimeout is the TCP_USER_TIMEOUT of link connections (Linux).
	UserTimeout = 30 * time.Second
)

// serverNamePattern is a DNS host name such as a node's identity name.
var serverNamePattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)

// DialConfig configures DialTLS: one encrypted link connection to the next
// hop.
type DialConfig struct {
	// Credentials are this node's link credentials. Required.
	Credentials *Credentials
	// ServerName is the DNS name the listener's certificate must carry: the
	// link's server_name, which is the next node's identity name
	// (forward-41) unless the link says otherwise. Required, a host name
	// (not an IP address).
	ServerName string
	// PeerIdentity is the SPIFFE ID the listener must present, the
	// upstream's peer_identity. Required, canonical.
	PeerIdentity string
	// Protocol is the ALPN protocol both ends must negotiate. Required.
	Protocol string
	// HandshakeTimeout bounds the dial and the handshake; the default is
	// DefaultHandshakeTimeout.
	HandshakeTimeout time.Duration
	// Dialer is used to open the TCP connection; the zero value if nil.
	Dialer *net.Dialer
}

// DialTLS opens a TCP connection to address and runs TLS 1.3 with mutual
// authentication over it. It succeeds only if the listener's certificate
// chains to the link trust bundle, carries ServerName as its only DNS name
// and PeerIdentity as its only URI name, and ALPN Protocol was negotiated;
// otherwise it returns a *HandshakeError and the connection is closed. There
// is no trust on first use and no way to skip a check.
//
// In TLS 1.3 the client's handshake completes before the server has checked
// the client's certificate, so a nil error does not yet mean the listener
// accepted this node: a listener that refuses it closes the connection on
// the first read (the carrier layer's SETTINGS exchange).
func DialTLS(ctx context.Context, address string, cfg DialConfig) (*Conn, error) {
	switch {
	case cfg.Credentials == nil:
		return nil, errors.New("link: DialTLS needs credentials")
	case cfg.Protocol == "":
		return nil, errors.New("link: DialTLS needs an ALPN protocol")
	case !serverNamePattern.MatchString(cfg.ServerName) || netIsIP(cfg.ServerName):
		return nil, fmt.Errorf("link: server name %q is not a DNS name", cfg.ServerName)
	}
	if _, err := ParseIdentity(cfg.PeerIdentity); err != nil {
		return nil, err
	}
	timeout := cfg.HandshakeTimeout
	if timeout == 0 {
		timeout = DefaultHandshakeTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var d net.Dialer
	if cfg.Dialer != nil {
		d = *cfg.Dialer
	}
	raw, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, classify(err, "")
	}
	_ = tuneConn(raw) // best effort: the carrier's idle timeout covers a dead link
	tc := tls.Client(raw, cfg.Credentials.clientTLS(cfg.ServerName, cfg.PeerIdentity, cfg.Protocol))
	if err := tc.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, classify(err, "")
	}
	return &Conn{Conn: tc, peer: peerFor(tc.ConnectionState(), x509.ExtKeyUsageServerAuth)}, nil
}

func netIsIP(s string) bool {
	_, err := netip.ParseAddr(s)
	return err == nil
}

// PlainDialConfig configures DialPlain.
type PlainDialConfig struct {
	// TrustedLink must be true. A plaintext link has no confidentiality,
	// integrity or replay protection and authenticates the dialler only by
	// the listener's source admission; it is allowed only between nodes that
	// both carry a trusted-link label, on an administrator's route
	// (anixops-protocol.md section 5.3, decision P4), which validation
	// checks. Setting the field is the caller asserting that check passed.
	TrustedLink bool
	// Timeout bounds the dial; the default is DefaultHandshakeTimeout.
	Timeout time.Duration
	// Dialer is used to open the TCP connection; the zero value if nil.
	Dialer *net.Dialer
}

// DialPlain opens a plaintext TCP connection to address. Nothing is verified:
// use it only for links whose validation allowed PLAIN. It is never a
// fallback for DialTLS.
func DialPlain(ctx context.Context, address string, cfg PlainDialConfig) (*Conn, error) {
	if !cfg.TrustedLink {
		return nil, errors.New("link: a plaintext link is for trusted links only (PlainDialConfig.TrustedLink)")
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultHandshakeTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var d net.Dialer
	if cfg.Dialer != nil {
		d = *cfg.Dialer
	}
	raw, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, classify(err, "")
	}
	_ = tuneConn(raw)
	return &Conn{Conn: raw}, nil
}
