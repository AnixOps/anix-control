package link

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"slices"
	"sort"
	"strings"
)

// Reason says why a handshake or an admission failed. The names are the
// bounded labels of anixops-protocol.md section 7.3 (never a client address
// or an identity), so a counter per reason cannot grow without bound.
type Reason uint8

const (
	// ReasonOther is a failure none of the others describes (a reset, a
	// short read, a closed listener).
	ReasonOther Reason = iota
	// ReasonUnknownCA: the peer's chain does not lead to a CA of the link
	// trust bundle.
	ReasonUnknownCA
	// ReasonCertificate: the chain is from the bundle but unusable: expired
	// or not yet valid, without the needed key usage, or with names a link
	// certificate does not carry.
	ReasonCertificate
	// ReasonIdentityMismatch: a dialler reached a node that is not the one it
	// pinned (another SPIFFE identity, or a DNS name other than the link's
	// server name).
	ReasonIdentityMismatch
	// ReasonPeerNotAllowed: a node the listener does not list in its
	// ingress_peers presented a genuine certificate.
	ReasonPeerNotAllowed
	// ReasonSourceNotAllowed: the connection came from an address outside
	// the listener's ingress_sources and was closed before any handshake.
	ReasonSourceNotAllowed
	// ReasonHandshakeLimit: the listener had its maximum of handshakes in
	// flight and closed the connection at once.
	ReasonHandshakeLimit
	// ReasonTimeout: the handshake did not finish in time.
	ReasonTimeout
	// ReasonProtocol: the peer did not speak the protocol: not TLS 1.3, no
	// or another ALPN, an attempt to resume a session, or garbage.
	ReasonProtocol
	// ReasonRemoteRejected: the other end refused with a TLS alert (it did
	// not accept our certificate, or the protocol).
	ReasonRemoteRejected

	// NumReasons is the number of reasons, for arrays indexed by Reason.
	NumReasons = int(iota)
)

// String returns the metrics label of the reason.
func (r Reason) String() string {
	switch r {
	case ReasonUnknownCA:
		return "unknown_ca"
	case ReasonCertificate:
		return "certificate"
	case ReasonIdentityMismatch:
		return "identity_mismatch"
	case ReasonPeerNotAllowed:
		return "peer_not_allowed"
	case ReasonSourceNotAllowed:
		return "source_not_allowed"
	case ReasonHandshakeLimit:
		return "handshake_limit"
	case ReasonTimeout:
		return "timeout"
	case ReasonProtocol:
		return "protocol"
	case ReasonRemoteRejected:
		return "remote_rejected"
	}
	return "other"
}

// HandshakeError is a failed handshake or admission. Identity is the SPIFFE
// ID the peer's certificate claimed when it got far enough to have one, for
// the structured log (never for a metrics label).
type HandshakeError struct {
	Reason   Reason
	Identity string
	Err      error
}

func (e *HandshakeError) Error() string {
	if e.Identity != "" {
		return fmt.Sprintf("link: handshake refused (%s, peer %s): %v", e.Reason, e.Identity, e.Err)
	}
	return fmt.Sprintf("link: handshake refused (%s): %v", e.Reason, e.Err)
}

func (e *HandshakeError) Unwrap() error { return e.Err }

// ReasonOf classifies an error returned by a dial or a handshake.
func ReasonOf(err error) Reason {
	if err == nil {
		return ReasonOther
	}
	var he *HandshakeError
	if errors.As(err, &he) {
		return he.Reason
	}
	return classifyQUIC(err, nil).Reason // QUIC's errors, and everything else as classify
}

func refuse(reason Reason, identity, format string, args ...any) *HandshakeError {
	return &HandshakeError{Reason: reason, Identity: identity, Err: fmt.Errorf(format, args...)}
}

// classify turns whatever crypto/tls or the network returned into a
// HandshakeError.
func classify(err error, identity string) *HandshakeError {
	var he *HandshakeError
	if errors.As(err, &he) {
		return he
	}
	var (
		unknown  x509.UnknownAuthorityError
		invalid  x509.CertificateInvalidError
		hostname x509.HostnameError
		alert    tls.AlertError // an alert this end sent
		opErr    *net.OpError
		netErr   net.Error
	)
	reason := ReasonOther
	switch {
	case errors.As(err, &unknown):
		reason = ReasonUnknownCA
	case errors.As(err, &invalid):
		reason = ReasonCertificate
	case errors.As(err, &hostname):
		reason = ReasonIdentityMismatch
	case errors.As(err, &opErr) && opErr.Op == "remote error": // the peer's alert
		reason = ReasonRemoteRejected
	case errors.As(err, &netErr) && netErr.Timeout():
		reason = ReasonTimeout
	default:
		var rhe tls.RecordHeaderError
		if errors.As(err, &rhe) {
			reason = ReasonProtocol
		}
		// crypto/tls reports these refusals as plain errors.
		msg := err.Error()
		if strings.Contains(msg, "didn't provide a certificate") {
			reason = ReasonCertificate
		}
	}
	// The protocol refusals, ours (alerts 70 and 120 sent) and the peer's
	// (crypto/tls only gives their text).
	msg := err.Error()
	if errors.As(err, &alert) && (alert == 70 || alert == 120) ||
		strings.Contains(msg, "protocol version not supported") || strings.Contains(msg, "no application protocol") ||
		strings.Contains(msg, "unsupported versions") || strings.Contains(msg, "unsupported application protocols") {
		reason = ReasonProtocol
	}
	return &HandshakeError{Reason: reason, Identity: identity, Err: err}
}

// checkSession enforces what every handshake must have: TLS 1.3, the ALPN
// protocol, no resumed session. Go does not fail a handshake whose client
// offered no ALPN at all, so the negotiated protocol is checked here, on both
// ends.
func checkSession(cs tls.ConnectionState, protocol string) *HandshakeError {
	switch {
	case cs.Version != tls.VersionTLS13:
		return refuse(ReasonProtocol, "", "TLS 1.3 is required")
	case cs.NegotiatedProtocol != protocol:
		return refuse(ReasonProtocol, "", "ALPN protocol %q negotiated, want %q", cs.NegotiatedProtocol, protocol)
	case cs.DidResume:
		return refuse(ReasonProtocol, "", "session resumption is not accepted")
	case len(cs.PeerCertificates) == 0:
		return refuse(ReasonCertificate, "", "the peer presented no certificate")
	}
	return nil
}

// verifyServerConnection is the dialler's check of the listener it reached,
// run by crypto/tls after chain and name verification (VerifyConnection runs
// on full and resumed handshakes alike): the certificate carries exactly one
// URI name, equal to the pinned peer_identity, and exactly one DNS name,
// equal to the link's server name.
func verifyServerConnection(cs tls.ConnectionState, serverName, peerIdentity, protocol string) error {
	if he := checkSession(cs, protocol); he != nil {
		return he
	}
	leaf := cs.PeerCertificates[0]
	if !slices.Contains(leaf.ExtKeyUsage, x509.ExtKeyUsageServerAuth) {
		return refuse(ReasonCertificate, "", "the listener's certificate does not state serverAuth")
	}
	id, _, err := certIdentity(leaf)
	if err != nil {
		return refuse(ReasonIdentityMismatch, "", "the listener's certificate is not a link certificate: %v", err)
	}
	if id != peerIdentity {
		return refuse(ReasonIdentityMismatch, id, "reached %s, want %s", id, peerIdentity)
	}
	if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != serverName {
		return refuse(ReasonIdentityMismatch, id, "the listener's DNS names are %q, want exactly %q", leaf.DNSNames, serverName)
	}
	return nil
}

// verifyClientConnection is the listener's check of a dialler whose chain
// crypto/tls verified for client authentication: exactly one URI name that is
// one of the listener's ingress_peers.
func verifyClientConnection(cs tls.ConnectionState, peers *peerSet, protocol string) error {
	if he := checkSession(cs, protocol); he != nil {
		return he
	}
	if !slices.Contains(cs.PeerCertificates[0].ExtKeyUsage, x509.ExtKeyUsageClientAuth) {
		return refuse(ReasonCertificate, "", "the dialler's certificate does not state clientAuth")
	}
	id, _, err := certIdentity(cs.PeerCertificates[0])
	if err != nil {
		return refuse(ReasonCertificate, "", "the dialler's certificate is not a link certificate: %v", err)
	}
	if !peers.has(id) {
		return refuse(ReasonPeerNotAllowed, id, "%s is not one of the listener's ingress peers", id)
	}
	return nil
}

// peerSet is an immutable set of identities.
type peerSet struct{ ids map[string]struct{} }

func newPeerSet(ids []string) (*peerSet, error) {
	if len(ids) == 0 {
		return nil, errors.New("link: no ingress peers: an encrypted listener admits only the previous hop's identities")
	}
	set := &peerSet{ids: make(map[string]struct{}, len(ids))}
	for _, id := range ids {
		canonical, err := ParseIdentity(id)
		if err != nil {
			return nil, err
		}
		set.ids[canonical] = struct{}{}
	}
	return set, nil
}

func (s *peerSet) has(id string) bool {
	if s == nil {
		return false
	}
	_, ok := s.ids[id]
	return ok
}

// without returns the identities of s that next does not have, sorted.
func (s *peerSet) without(next *peerSet) []string {
	var out []string
	for id := range s.ids {
		if !next.has(id) {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
