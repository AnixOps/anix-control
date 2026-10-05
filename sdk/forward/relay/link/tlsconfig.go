package link

import (
	"crypto/tls"
	"crypto/x509"
)

// clientTLS is the dialler's configuration for one new connection: the node's
// certificate, the trust bundle as it is now, the link's server name, and the
// identity check. crypto/tls does the chain and DNS-name verification
// (RootCAs and ServerName, never InsecureSkipVerify); VerifyConnection adds
// only the pinning. Built per connection so a reload applies to the next one.
func (c *Credentials) clientTLS(serverName, peerIdentity, protocol string) *tls.Config {
	st := c.state.Load()
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
		NextProtos:   []string{protocol},
		ServerName:   serverName,
		RootCAs:      st.roots,
		Certificates: nil,
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return &st.cert, nil
		},
		VerifyConnection: func(cs tls.ConnectionState) error {
			return verifyServerConnection(cs, serverName, peerIdentity, protocol)
		},
		// ClientSessionCache stays nil: no resumption (P3), and TLS early
		// data is never sent.
	}
}

// serverTLS is the listener's configuration. The configuration for each
// handshake is built when its ClientHello arrives from the credentials and
// trust bundle as they are then, and the identity check reads the peers as
// they are when it runs, so a reload or a change of ingress_peers applies to
// the next handshake without anything being re-created.
func (l *Listener) serverTLS() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		MaxVersion: tls.VersionTLS13,
		NextProtos: []string{l.protocol},
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			st := l.creds.state.Load()
			return &tls.Config{
				MinVersion:   tls.VersionTLS13,
				MaxVersion:   tls.VersionTLS13,
				NextProtos:   []string{l.protocol},
				Certificates: []tls.Certificate{st.cert},
				ClientCAs:    st.roots,
				ClientAuth:   tls.RequireAndVerifyClientCert,
				// No session tickets: resumption is off (P3) and, with it,
				// every handshake runs the identity check on a fresh chain.
				SessionTicketsDisabled: true,
				VerifyConnection: func(cs tls.ConnectionState) error {
					return verifyClientConnection(cs, l.peers.Load(), l.protocol)
				},
			}, nil
		},
	}
}

// peerFor builds the Peer of a connection whose handshake succeeded.
func peerFor(cs tls.ConnectionState, usage x509.ExtKeyUsage) Peer {
	id, _, _ := certIdentity(cs.PeerCertificates[0]) // verified by VerifyConnection
	return Peer{Identity: id, Chain: cs.PeerCertificates, usage: usage}
}
