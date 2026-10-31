// Package fakeagent is a scripted Agent Control client and the Control
// listener it talks to, for the parity and chaos tests of
// docs/architecture/node-ops-service.md section 9: Hello,
// acknowledgements, observed states, refusals, replays after a reconnect
// and a replaced session. Every credential is fake.
package fakeagent

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v5/internal/agentpki"
	grpcserver "github.com/AnixOps/anix-control/v5/internal/grpc"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"gorm.io/gorm"
)

// Control is the real node-facing listener (internal/grpc.Server) with
// TLS and the agent PKI: agents enroll with a one-time credential and
// stream by certificate. It reads the kernel's global database (database.Get),
// which the caller initializes.
type Control struct {
	Server  *grpcserver.Server
	Addr    string
	PKI     *agentpki.Service
	Streams *grpcserver.AgentStreams
	roots   *x509.CertPool
}

// StartControl starts the listener on a free port over db, the kernel's
// global database with the module CA and agent PKI tables migrated.
func StartControl(t testing.TB, db *gorm.DB) *Control {
	t.Helper()
	kek := make([]byte, 32)
	_, err := rand.Read(kek)
	require.NoError(t, err)
	authority, err := modulepki.New(modulepki.Options{DB: db, Cluster: "test", KEK: kek})
	require.NoError(t, err)
	require.NoError(t, authority.Ensure(context.Background()))
	pki, err := agentpki.New(agentpki.Options{DB: db, Authority: authority})
	require.NoError(t, err)

	cfg := grpcserver.DefaultServerConfig()
	cfg.Host, cfg.Port, cfg.AgentPKI = "127.0.0.1", 0, pki
	var roots *x509.CertPool
	cfg.TLSCertFile, cfg.TLSKeyFile, roots = writeServerCertificate(t)
	server := grpcserver.NewServer(cfg)
	require.NoError(t, server.Start())
	control := &Control{Server: server, Addr: server.Addr(), PKI: pki, Streams: grpcserver.GetAgentStreams(), roots: roots}
	t.Cleanup(server.Stop)
	return control
}

// writeServerCertificate writes a throwaway self-signed server certificate
// for 127.0.0.1.
func writeServerCertificate(t testing.TB) (string, string, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "control.test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)}, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key")
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600))
	leaf, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	return certFile, keyFile, roots
}

// Dial connects to the listener over TLS, presenting certificate when it
// is not nil.
func (c *Control) Dial(t testing.TB, certificate *tls.Certificate) *grpc.ClientConn {
	t.Helper()
	tlsConfig := &tls.Config{RootCAs: c.roots, MinVersion: tls.VersionTLS12}
	if certificate != nil {
		tlsConfig.Certificates = []tls.Certificate{*certificate}
	}
	conn, err := grpc.NewClient(c.Addr, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// Enroll obtains node's agent certificate with a one-time enrollment
// credential, as the node's Agent does at its first start.
func (c *Control) Enroll(t testing.TB, node agentcontrol.AgentNode) *tls.Certificate {
	t.Helper()
	credential, _, err := c.PKI.CreateEnrollmentToken(context.Background(), agentpki.TokenRequest{Node: node, Actor: "fakeagent"})
	require.NoError(t, err)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: node.String()}}, key)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, agentcontrol.MetadataNodeID, strconv.FormatUint(uint64(node.ID), 10),
		agentcontrol.MetadataNodeKind, node.Kind)
	response, err := agentv1pb.NewAgentEnrollmentClient(c.Dial(t, nil)).Enroll(ctx, &agentv1pb.EnrollAgentRequest{
		CsrDer: csr, AgentVersion: "fake-agent", InstanceId: "fake-" + node.String(), EnrollmentCredential: credential,
	})
	require.NoError(t, err)
	return &tls.Certificate{Certificate: [][]byte{response.GetCertificate().GetCertificateDer()}, PrivateKey: key}
}

// StartEdge starts a listener in the shape of grpc.edge_proxies
// (docs/design/edge-grpc) over the agent PKI of control: plain HTTP/2 from
// 127.0.0.1 only, the node's certificate taken from the proxy's header.
func StartEdge(t testing.TB, control *Control) *Control {
	t.Helper()
	cfg := grpcserver.DefaultServerConfig()
	cfg.Host, cfg.Port, cfg.AgentPKI = "127.0.0.1", 0, control.PKI
	cfg.EdgeProxies = []string{"127.0.0.1/32"}
	server := grpcserver.NewServer(cfg)
	require.NoError(t, server.Start())
	t.Cleanup(server.Stop)
	return &Control{Server: server, Addr: server.Addr(), PKI: control.PKI, Streams: grpcserver.GetAgentStreams()}
}

// DialEdge connects to an edge listener the way the proxy does: plain
// HTTP/2, the node's certificate (nil: none) in the EdgeClientCertHeader.
func (c *Control) DialEdge(t testing.TB, certificate *tls.Certificate) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(c.Addr, append([]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
		withEdgeHeader(certificate)...)...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// DialForgedHeader connects over TLS without a client certificate but with
// the edge header carrying certificate: a header from a peer that is not an
// edge proxy must not be believed.
func (c *Control) DialForgedHeader(t testing.TB, certificate *tls.Certificate) *grpc.ClientConn {
	t.Helper()
	tlsConfig := &tls.Config{RootCAs: c.roots, MinVersion: tls.VersionTLS12}
	conn, err := grpc.NewClient(c.Addr, append([]grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))},
		withEdgeHeader(certificate)...)...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// withEdgeHeader adds the EdgeClientCertHeader of certificate (none when
// nil) to every unary and stream call.
func withEdgeHeader(certificate *tls.Certificate) []grpc.DialOption {
	value := ""
	if certificate != nil {
		var text []byte
		for _, der := range certificate.Certificate {
			text = append(text, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
		}
		value = url.PathEscape(string(text))
	}
	outgoing := func(ctx context.Context) context.Context {
		if value == "" {
			return ctx
		}
		return metadata.AppendToOutgoingContext(ctx, grpcserver.EdgeClientCertHeader, value)
	}
	return []grpc.DialOption{
		grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			return invoker(outgoing(ctx), method, req, reply, cc, opts...)
		}),
		grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			return streamer(outgoing(ctx), desc, cc, method, opts...)
		}),
	}
}

// APIKeyHash hashes a node API key as v2_node.api_key_hash stores it.
func APIKeyHash(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:])
}
