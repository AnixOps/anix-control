package grpc

import (
	"context"
	"encoding/pem"

	"net"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// edgeContext is the context of a call that arrived through an edge proxy
// at the given address, carrying the proxy's headers.
func edgeContext(headers ...string) context.Context {
	ctx := peer.NewContext(context.Background(), &peer.Peer{
		Addr:     &net.TCPAddr{IP: net.ParseIP("10.0.0.2"), Port: 40000},
		AuthInfo: edgeAuthInfo{},
	})
	if len(headers) > 0 {
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(headers...))
	}
	return ctx
}

// directContext is the context of a call that arrived on a TLS connection
// from addr; a header it carries must be ignored.
func directContext(addr string, headers ...string) context.Context {
	ctx := peer.NewContext(context.Background(), &peer.Peer{
		Addr: &net.TCPAddr{IP: net.ParseIP(addr), Port: 40000},
	})
	if len(headers) > 0 {
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(headers...))
	}
	return ctx
}

// escapedPEM is the header value nginx $ssl_client_escaped_cert produces.
func escapedPEM(blocks ...*pem.Block) string {
	var text []byte
	for _, block := range blocks {
		text = append(text, pem.EncodeToMemory(block)...)
	}
	return url.PathEscape(string(text))
}

func TestEdgeClientChainReadsTheVouchedCertificate(t *testing.T) {
	// The base64 of these bytes is "++++": the header must keep the '+'
	// characters, which a query-style unescape would turn into spaces.
	der := []byte{0xfb, 0xef, 0xbe}
	ctx := edgeContext(EdgeClientCertHeader, escapedPEM(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	assert.Equal(t, [][]byte{der}, peerCertificateChain(ctx))
}

func TestEdgeClientChainTakesEveryCertificateOfTheChain(t *testing.T) {
	first, second := []byte{1, 2, 3}, []byte{4, 5, 6}
	ctx := edgeContext(EdgeClientCertHeader, escapedPEM(
		&pem.Block{Type: "CERTIFICATE", Bytes: first},
		&pem.Block{Type: "CERTIFICATE", Bytes: second},
	))
	assert.Equal(t, [][]byte{first, second}, peerCertificateChain(ctx))
}

func TestEdgeClientChainTreatsAbsentOrMalformedHeadersAsNoCertificate(t *testing.T) {
	cases := map[string]context.Context{
		"no header":          edgeContext(),
		"empty header":       edgeContext(EdgeClientCertHeader, ""),
		"not pem":            edgeContext(EdgeClientCertHeader, "garbage"),
		"trailing text":      edgeContext(EdgeClientCertHeader, escapedPEM(&pem.Block{Type: "CERTIFICATE", Bytes: []byte{1}})+"junk"),
		"a private key":      edgeContext(EdgeClientCertHeader, escapedPEM(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte{1}})),
		"bad percent escape": edgeContext(EdgeClientCertHeader, "%zz"),
	}
	for name, ctx := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Nil(t, peerCertificateChain(ctx))
		})
	}
}

// A header from a peer that is not an edge proxy is never read: the
// certificate comes only from the TLS handshake, which is absent here.
func TestHeadersFromOtherPeersAreIgnored(t *testing.T) {
	header := escapedPEM(&pem.Block{Type: "CERTIFICATE", Bytes: []byte{1, 2, 3}})
	ctx := directContext("198.51.100.9", EdgeClientCertHeader, header, edgeForwardedForHeader, "203.0.113.7")
	assert.Nil(t, peerCertificateChain(ctx))
	assert.Equal(t, "198.51.100.9", remoteHost(ctx), "the peer's own address, not the header's")
}

func TestEdgeAddressIsTheVouchedNodeAddress(t *testing.T) {
	assert.Equal(t, "203.0.113.7", remoteHost(edgeContext(edgeForwardedForHeader, "203.0.113.7")))
	assert.Equal(t, "2001:db8::7", remoteHost(edgeContext(edgeForwardedForHeader, "2001:db8::7")))
	assert.Equal(t, "10.0.0.9", remoteHost(edgeContext(edgeForwardedForHeader, "203.0.113.7, 10.0.0.9")),
		"the last entry is the one the proxy set")
}

func TestEdgeAddressNeedsAValidAddress(t *testing.T) {
	assert.Equal(t, "", remoteHost(edgeContext()), "no header: unknown, not the proxy")
	assert.Equal(t, "", remoteHost(edgeContext(edgeForwardedForHeader, "not-an-ip")))
}

// GetPeerAddr (the v4 node registration's address) follows the same rule: the
// node's address through the edge, "unknown" without one, and host:port on
// a direct connection.
func TestGetPeerAddrFollowsTheEdge(t *testing.T) {
	assert.Equal(t, "203.0.113.7", GetPeerAddr(edgeContext(edgeForwardedForHeader, "203.0.113.7")))
	assert.Equal(t, "unknown", GetPeerAddr(edgeContext()))
	assert.Equal(t, "198.51.100.9:40000", GetPeerAddr(directContext("198.51.100.9", edgeForwardedForHeader, "203.0.113.7")),
		"a header on a direct connection is not the address")
}

// The listener admits its proxies and nobody else: a stranger's connection
// is closed before the gRPC layer sees it.
func TestEdgeListenerAdmitsOnlyItsProxies(t *testing.T) {
	t.Run("a proxy is served", func(t *testing.T) {
		server := startEdgeServer(t, "127.0.0.1/32")
		conn, err := grpc.NewClient(server.Addr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })
		conn.Connect()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for state := conn.GetState(); state != connectivity.Ready; state = conn.GetState() {
			require.True(t, conn.WaitForStateChange(ctx, state), "the proxy's connection never became ready: %s", state)
		}
	})
	t.Run("a stranger is closed", func(t *testing.T) {
		server := startEdgeServer(t, "10.255.255.1/32")
		raw, err := net.DialTimeout("tcp", server.Addr(), 5*time.Second)
		require.NoError(t, err)
		t.Cleanup(func() { _ = raw.Close() })
		require.NoError(t, raw.SetDeadline(time.Now().Add(5*time.Second)))
		_, _ = raw.Write([]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"))
		buf := make([]byte, 1)
		n, err := raw.Read(buf)
		assert.Zero(t, n, "the stranger gets no answer")
		assert.Error(t, err, "the stranger's connection is closed (EOF or reset)")
	})
}

func TestEdgeListenerServesNoCertificate(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.Host, cfg.Port = "127.0.0.1", 0
	cfg.EdgeProxies = []string{"127.0.0.1/32"}
	cfg.TLSCertFile, cfg.TLSKeyFile = "/etc/a.pem", "/etc/a.key"
	err := NewServer(cfg).Start()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "serve no gRPC certificate")
}

func startEdgeServer(t *testing.T, proxies string) *Server {
	t.Helper()
	cfg := DefaultServerConfig()
	cfg.Host, cfg.Port = "127.0.0.1", 0
	cfg.EdgeProxies = []string{proxies}
	server := NewServer(cfg)
	require.NoError(t, server.Start())
	t.Cleanup(server.Stop)
	return server
}
