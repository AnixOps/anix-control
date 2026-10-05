// Package link makes the connections of the AnixOps relay protocol
// (docs/architecture/anixops-protocol.md, owner decision H22): the TLS 1.3
// connection between two nodes with mutual authentication and per-identity
// pinning, over TCP or inside QUIC, the plaintext connection of a trusted link,
// and what guards a listener before and during the handshake. It returns plain
// net.Conns (or, for QUIC, quic.Conns) with the peer's verified identity
// attached; the stream multiplexer that runs on them is the sibling package
// relay. Its cryptography is crypto/tls's alone (crypto/x509 for the
// certificates; golang.org/x/sys for TCP_USER_TIMEOUT on Linux); QUIC is
// quic-go (MIT, one pinned version, decision P2), which adds none of its own. It
// does not import the kernel, and does not depend on package relay.
//
// # Credentials
//
// A node's link credentials are the three files the Agent writes for the
// relay (H28, internal/agentpki/link.go): its link certificate, its key and
// the link trust bundle (link-ca.crt: the current, next and retired link CAs).
// LoadCredentials refuses anything that is not a link certificate: one URI
// name that is an agent identity (spiffe://anixops/<cluster>/agent/<node>),
// one DNS name that is that identity's node name, no other names, serverAuth
// and clientAuth, valid now, chaining to a CA of the bundle for both uses.
// Credentials.Reload replaces certificate, key and bundle atomically without
// re-creating a listener or a connection: new handshakes use the new ones,
// established connections run on, and a failed reload changes nothing
// (anixops-protocol.md section 3.5). PeerStillTrusted re-checks a connection's
// peer against the current bundle, so the carrier layer can close the
// connections whose CA was dropped; OnReload tells it when to.
//
// # The handshake (anixops-protocol.md section 3.2)
//
// TLS 1.3 only (MinVersion and MaxVersion), the ALPN protocol the caller
// names, mutual authentication, no session tickets and no client session
// cache (resumption is off, P3; TLS early data is never offered or accepted),
// no renegotiation, and never InsecureSkipVerify: crypto/tls verifies every
// chain (RootCAs, or ClientCAs with RequireAndVerifyClientCert) and the
// dialler's DNS name (ServerName), and VerifyConnection, which crypto/tls
// calls on every handshake, adds the pinning.
//
//	dialler (DialTLS)   chain to the bundle for serverAuth; exactly one DNS name,
//	                    equal to the link's server_name; exactly one URI name,
//	                    equal to the upstream's peer_identity; no IP or email
//	                    names; serverAuth stated
//	listener (Listener) chain to the bundle for clientAuth; exactly one URI name,
//	                    a member of the hop's ingress_peers; no IP or email
//	                    names; clientAuth stated
//	both                TLS 1.3, the ALPN protocol negotiated (Go does not fail a
//	                    client that offers none, so it is checked), no resumption
//
// There is no trust on first use and no way to skip a check. A failure is a
// *HandshakeError with a Reason (unknown_ca, certificate, identity_mismatch,
// peer_not_allowed, source_not_allowed, handshake_limit, timeout, protocol,
// remote_rejected, other): the bounded labels of the metrics of section 7.3,
// with the peer's claimed identity attached for the log. In TLS 1.3 the client
// finishes its handshake before the server has verified its certificate, so a
// dialler learns it was refused on the first read (the carrier layer's
// SETTINGS exchange), not from DialTLS.
//
// # Admission and limits (anixops-protocol.md sections 2.5 and 4.6)
//
//	check                              default  where
//	source in ingress_sources          before any handshake byte; refused with a bare close (no signature made)
//	handshakes in flight per listener  64       ListenerConfig.MaxPending; the next is closed at once
//	handshake deadline                 10 s     ListenerConfig.HandshakeTimeout, DialConfig.HandshakeTimeout
//	TCP_USER_TIMEOUT                   30 s     Linux, on every link connection
//
// A listener with no sources or, for TLS, no peers is a construction error: it
// would admit nobody, and "admit everybody" does not exist. Sources, peers and
// credentials change in place (SetSources, SetPeers, Reload). SetPeers returns
// the identities it removed: from then on their handshakes fail with
// peer_not_allowed, and the carrier layer closes the connections already
// authenticated as them (GOAWAY peer_not_allowed), the revocation path of
// section 3.5; no CRL or OCSP is consulted (P9). A finished handshake nobody
// has accepted yet keeps its handshake slot, so connections cannot pile up
// beyond the limit.
//
// # QUIC links (anixops-protocol.md section 5.2)
//
// DialQUIC and QUICListener make the same connection inside QUIC version 1
// (RFC 9000): the handshake is crypto/tls's own through its QUIC API (quic-go
// carries it), with the very configurations of the TCP link, so every check of
// the table above runs on a QUIC handshake as well, by construction: the
// dialler's and the listener's tls.Config, VerifyConnection and the ALPN check
// are the same code, and the listeners share one admission state (credentials,
// ingress_sources, ingress_peers), so SetSources, SetPeers and Credentials.Reload
// mean the same thing. Each refusal carries the same bounded Reason; the tests
// run the certificate cases of the TCP link through QUIC and assert the same
// reasons.
//
//	check                              QUIC
//	source in ingress_sources          on the first Initial of an attempt (quic-go's ConnContext): refused with one
//	                                   small CONNECTION_REFUSED packet, before any TLS state exists or a signature is made
//	handshakes in flight               ListenerConfig.MaxPending (64): the next attempt is refused the same way
//	address validation                 QUIC's Retry, asked of an admitted address when over half of the slots are taken
//	handshake deadline                 10 s, hard (quic-go aborts at twice HandshakeIdleTimeout, which is set to half)
//	QUIC version                       1 only: others get no answer (no version negotiation packets are sent)
//	0-RTT                              never accepted by the listener, never sent by the dialler; no session tickets, no token store
//	after the handshake                QUIC version 1, TLS 1.3, the ALPN protocol, no resumption, no 0-RTT, checked on both ends
//
// Differences from TCP that the carrier layer sees: a refused source or a full
// queue is a refusal the dialler sees as the peer's close during its handshake
// (remote_rejected); the dialler's handshake completes before the listener has
// verified it, as with TLS 1.3 over TCP, so a listener that refuses the node
// closes the connection a moment later; quic-go reports a failed handshake as a
// TLS alert and its text, so the listener keeps the typed refusal of its own
// VerifyConnection in the context of the attempt and classifies by it, and by
// the alert or the text for what crypto/tls refused first (an unknown CA, an
// expired certificate). Retry and the slot bound together mean that spoofed
// Initials from an admitted address cannot hold more than half of the slots: past
// that only an address that answers its Retry gets one.
//
// QUICListener.StopAccepting stops accepting and abandons the handshakes while
// leaving the established connections up, so the carrier layer can drain them;
// Close then closes the transport and the UDP socket. QUICListenerConfig's
// StatelessResetKey is the caller's: kept in its state directory it lets a
// restarted listener end its predecessor's connections at once; this package
// never generates or stores a key. QUIC needs large UDP socket buffers (see
// package relay).
//
// # Plaintext links (anixops-protocol.md section 5.3)
//
// ListenPlain and DialPlain make plain TCP connections for trusted links
// (IEPL and IPLC private lines). They offer no confidentiality, integrity or
// replay protection, and authenticate the dialler only by source admission,
// exactly as nftables' RAW links do: a plain listener requires at least one
// source. Both refuse to run unless TrustedLink is set, which is the caller
// asserting that validation allowed PLAIN for this link (both nodes labelled
// link=iepl or link=iplc on an administrator's route, decision P4). A plain
// connection has no Peer identity, and nothing here ever falls back to
// plaintext.
//
// # Testing
//
// Package relaytest issues link CAs and node certificates of the H28 shape
// (including the spiffe://anixops name constraint) and every deviation a test
// needs. The tests cover each check above with a certificate that breaks
// exactly that rule, over TCP and over QUIC, the TLS version and ALPN
// requirements, resumption and early data, source admission, the handshake
// deadline and limit, Retry, reloads and peer changes under concurrent use,
// stateless resets, and the Linux socket option; the fuzz targets
// (FuzzParseIdentity, FuzzParseSources, FuzzVerifyConnection, FuzzHandshake,
// FuzzCredentialsPEM, and FuzzQUICListenerPackets, which throws arbitrary UDP
// datagrams at a QUIC listener) run with their seed corpus under go test.
package link
