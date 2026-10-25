package grpc

import (
	"context"
	"encoding/pem"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/requestorigin"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// The edge (grpc.edge_proxies, docs/design/edge-grpc) is a reverse proxy on
// another machine that terminates the nodes' TLS with the public certificate
// and sends their gRPC here in plain HTTP/2 over the private network. It
// vouches for the node's client certificate in EdgeClientCertHeader (the
// proxy sets it from $ssl_client_escaped_cert, and overwrites whatever the
// client sent) and for the node's address in the X-Forwarded-For header.
//
// The listener admits only connections from the proxies, and it reads those
// headers only from them. Control still verifies the certificate itself, so
// the proxy never decides who a node is.
const (
	// EdgeClientCertHeader carries the node's client certificate chain as
	// URL-escaped PEM (nginx $ssl_client_escaped_cert). Empty: no certificate.
	EdgeClientCertHeader = "x-anix-client-cert"
	// edgeForwardedForHeader carries the node's address (nginx $remote_addr).
	edgeForwardedForHeader = "x-forwarded-for"
	// edgeSecurityProtocol names the transport on the wire and in AuthInfo.
	edgeSecurityProtocol = "anix-edge"
)

// edgeAuthInfo is the AuthInfo of a connection from an edge proxy. Its
// presence in peer.AuthInfo tells the authenticators to read the client
// certificate from the header, never from a TLS handshake.
type edgeAuthInfo struct{}

// AuthType names the authentication of the connection.
func (edgeAuthInfo) AuthType() string { return edgeSecurityProtocol }

// edgeCredentials is the server side of a plain transport: the connection is
// used as it is, and the connection is marked as coming from the edge.
type edgeCredentials struct{}

// ClientHandshake is never used: Control is a server on this listener.
func (edgeCredentials) ClientHandshake(context.Context, string, net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return nil, nil, errors.New("the edge listener has no client side")
}

// ServerHandshake keeps the connection plain and marks it as an edge one.
func (edgeCredentials) ServerHandshake(conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return conn, edgeAuthInfo{}, nil
}

// Info names the protocol of the connection.
func (edgeCredentials) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{SecurityProtocol: edgeSecurityProtocol}
}

// Clone returns a copy; the credentials carry no state.
func (edgeCredentials) Clone() credentials.TransportCredentials { return edgeCredentials{} }

// OverrideServerName is unused by gRPC and has no effect here.
func (edgeCredentials) OverrideServerName(string) error { return nil }

// edgeListener admits only the connections of the edge proxies. Any other
// peer is closed before it reaches the gRPC layer: the listener is plain, so
// a stranger must never talk to it.
type edgeListener struct {
	net.Listener
	proxies *requestorigin.Policy
}

// Accept returns the next connection from an edge proxy.
func (l edgeListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if l.admits(conn.RemoteAddr()) {
			return conn, nil
		}
		slog.Warn("refused a gRPC connection from outside grpc.edge_proxies", "component", "grpc", "addr", conn.RemoteAddr().String())
		_ = conn.Close()
	}
}

func (l edgeListener) admits(addr net.Addr) bool {
	if addr == nil {
		return false
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return false
	}
	return l.proxies.TrustedIP(net.ParseIP(host))
}

// isEdgeConnection tells whether the call arrived through an edge proxy.
func isEdgeConnection(ctx context.Context) bool {
	remote, ok := peer.FromContext(ctx)
	if !ok || remote.AuthInfo == nil {
		return false
	}
	_, edge := remote.AuthInfo.(edgeAuthInfo)
	return edge
}

// edgeClientChain returns the client certificate chain the proxy vouched
// for, nil when it sent none or sent something that is not PEM certificates.
// A malformed header counts as no certificate: the call then fails the same
// checks as a call without one.
func edgeClientChain(ctx context.Context) [][]byte {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil
	}
	values := md.Get(EdgeClientCertHeader)
	if len(values) == 0 || strings.TrimSpace(values[0]) == "" {
		return nil
	}
	// PathUnescape, not QueryUnescape: a '+' is base64 in PEM, not a space.
	text, err := url.PathUnescape(values[0])
	if err != nil {
		return nil
	}
	var chain [][]byte
	rest := []byte(text)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			return nil
		}
		chain = append(chain, block.Bytes)
	}
	if len(strings.TrimSpace(string(rest))) > 0 {
		return nil
	}
	return chain
}

// edgeClientAddress returns the node's address the proxy vouched for: the
// last X-Forwarded-For entry, which the proxy sets to the node's own address.
// Empty when the proxy sent no valid address.
func edgeClientAddress(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(edgeForwardedForHeader)
	if len(values) == 0 {
		return ""
	}
	entries := strings.Split(values[len(values)-1], ",")
	address := strings.TrimSpace(entries[len(entries)-1])
	if net.ParseIP(address) == nil {
		return ""
	}
	return address
}
