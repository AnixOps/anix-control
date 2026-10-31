package grpc

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"sync/atomic"
	"testing"
	"time"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v5/internal/agentpki"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

// The listener takes its certificate per handshake (the panel
// certificates): a renewed certificate reaches the next connection, and
// the listener still asks for and verifies the Agents' client certificates
// exactly as with a certificate file.
func TestAgentListenerServesTheCurrentCertificateAndStillAsksForClientCertificates(t *testing.T) {
	firstCert, firstKey, firstRoots := writeServerCertificate(t)
	secondCert, secondKey, secondRoots := writeServerCertificate(t)
	first, err := tls.LoadX509KeyPair(firstCert, firstKey)
	require.NoError(t, err)
	second, err := tls.LoadX509KeyPair(secondCert, secondKey)
	require.NoError(t, err)
	var current atomic.Pointer[tls.Certificate]
	current.Store(&first)
	l := startAgentListenerWith(t, true, func(cfg *ServerConfig) {
		cfg.TLSCertFile, cfg.TLSKeyFile = "", ""
		cfg.GetCertificate = func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return current.Load(), nil }
	})
	require.True(t, l.server.TLSEnabled())
	ctx := testContext(t)

	// dial reports whether the server asked for a client certificate.
	dial := func(roots *x509.CertPool, certificate *tls.Certificate) (*grpc.ClientConn, *atomic.Bool) {
		asked := &atomic.Bool{}
		config := &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12,
			GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
				asked.Store(true)
				if certificate == nil {
					return &tls.Certificate{}, nil
				}
				return certificate, nil
			}}
		conn, err := grpc.NewClient(l.addr, grpc.WithTransportCredentials(credentials.NewTLS(config)))
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })
		return conn, asked
	}

	anonymous, asked := dial(firstRoots, nil)
	credential, _, err := l.pki.CreateEnrollmentToken(ctx, agentpki.TokenRequest{Node: l.proxyNode(), TTL: time.Hour})
	require.NoError(t, err)
	agentCertificate, issued, err := enrollForTest(t, ctx, anonymous, credential)
	require.NoError(t, err, "enrollment needs no client certificate")
	assert.True(t, asked.Load(), "the server asks for a client certificate")

	// The certificate is renewed: a new connection gets the new one, and
	// an Agent that trusts only the old one is refused.
	current.Store(&second)
	stale, _ := dial(firstRoots, agentCertificate)
	_, err = agentv1pb.NewAgentEnrollmentClient(stale).GetTrustBundle(ctx, &agentv1pb.GetAgentTrustBundleRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(err))

	// Renew is authenticated by the client certificate alone, over the new
	// server certificate.
	conn, asked := dial(secondRoots, agentCertificate)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	require.NoError(t, err)
	renewed, err := agentv1pb.NewAgentEnrollmentClient(conn).Renew(ctx, &agentv1pb.RenewAgentCertificateRequest{CsrDer: csr})
	require.NoError(t, err)
	assert.True(t, asked.Load())
	assert.Equal(t, issued.GetSpiffeId(), renewed.GetCertificate().GetSpiffeId())
	assert.NotEqual(t, issued.GetSerial(), renewed.GetCertificate().GetSerial())

	// Without a client certificate Renew is refused, and a certificate the
	// agent CA did not sign is refused too.
	anonymous, _ = dial(secondRoots, nil)
	_, err = agentv1pb.NewAgentEnrollmentClient(anonymous).Renew(ctx, &agentv1pb.RenewAgentCertificateRequest{CsrDer: csr})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	foreign, err := tls.LoadX509KeyPair(firstCert, firstKey)
	require.NoError(t, err)
	stranger, _ := dial(secondRoots, &foreign)
	_, err = agentv1pb.NewAgentEnrollmentClient(stranger).Renew(ctx, &agentv1pb.RenewAgentCertificateRequest{CsrDer: csr})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}
