package edgegrpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/cache"
	grpcserver "github.com/AnixOps/anix-control/v5/internal/grpc"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/AnixOps/anix-control/v5/internal/tests/fakeagent"
	"github.com/AnixOps/anix-control/v5/internal/tests/packagecompat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"gorm.io/gorm"
)

// TestNginxEdgeVouchesForTheNodeCertificate runs the documented layout with
// a real nginx: the example config (config/deploy/examples/nginx/
// edge-two-hosts.conf) on a container, grpc_pass to the edge listener of
// Control. It needs Docker, and runs only with ANIX_EDGE_NGINX_E2E=1.
func TestNginxEdgeVouchesForTheNodeCertificate(t *testing.T) {
	if os.Getenv("ANIX_EDGE_NGINX_E2E") != "1" {
		t.Skip("set ANIX_EDGE_NGINX_E2E=1 to run against Docker's nginx")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed")
	}
	models := []any{&model.Node{}, &model.OperationLog{}}
	packagecompat.ForEachKernelDatabase(t, models, func(t *testing.T, db *gorm.DB) {
		cache.InitMemory()
		require.NoError(t, service.MigrateKernelModelsForTest(db))
		key := "edge-nginx-api-key"
		require.NoError(t, db.Create(&model.Node{ID: edgeNodeID + 1, Name: "edge-nginx", Host: "198.51.100.1", Port: 443,
			APIKey: key, APIKeyHash: fakeagent.APIKeyHash(key), Status: model.NodeStatusOnline}).Error)
		control := fakeagent.StartControl(t, db)
		node := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: edgeNodeID + 1}
		certificate := control.Enroll(t, node, key)
		edge := fakeagent.StartEdge(t, control)

		// The public certificate of the gRPC name, held by nginx.
		certFile, keyFile, roots := writeNamedCertificate(t, "grpc.new-x.anixops.com")
		config := edgeNginxConfig(t, edge.Addr, certFile, keyFile)
		startNginx(t, config, filepath.Dir(certFile))

		tlsFor := func(certificates ...tls.Certificate) grpc.DialOption {
			return grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
				RootCAs: roots, ServerName: "grpc.new-x.anixops.com", MinVersion: tls.VersionTLS12, Certificates: certificates,
			}))
		}
		dial := func(options ...grpc.DialOption) *grpc.ClientConn {
			conn, err := grpc.NewClient("127.0.0.1:8443", options...)
			require.NoError(t, err)
			t.Cleanup(func() { _ = conn.Close() })
			return conn
		}

		t.Run("the node's certificate through nginx opens the stream", func(t *testing.T) {
			agent := fakeagent.ConnectWithCertificate(t, dial(tlsFor(*certificate)), node, fakeagent.Script{})
			assert.NotNil(t, agent.HelloAck)
		})
		t.Run("without a certificate nginx forwards an empty header and the stream is refused", func(t *testing.T) {
			requireRefused(t, dial(tlsFor()), node)
		})
		t.Run("a forged header without a certificate is overwritten by nginx and refused", func(t *testing.T) {
			// The client presents no certificate, but sends the node's own
			// certificate in the header nginx sets. nginx must replace it.
			requireRefused(t, dial(append([]grpc.DialOption{tlsFor()}, withHeader(grpcserver.EdgeClientCertHeader, escapedCertificate(certificate))...)...), node)
		})
	})
}

// edgeNginxConfig is the documented example with this test's addresses: the
// edge listener of Control, the certificate files, and unprivileged ports.
func edgeNginxConfig(t *testing.T, edgeAddress, certFile, keyFile string) string {
	t.Helper()
	example, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "deploy", "examples", "nginx", "edge-two-hosts.conf"))
	require.NoError(t, err)
	replacer := strings.NewReplacer(
		"grpc://10.0.0.1:50051", "grpc://"+edgeAddress,
		"http://10.0.0.1:8080", "http://127.0.0.1:9",
		"listen 443 ssl http2;", "listen 8443 ssl http2;",
		"listen 80;", "listen 8081;",
		"/etc/letsencrypt/live/grpc.new-x.anixops.com/fullchain.pem", filepath.Join("/certs", filepath.Base(certFile)),
		"/etc/letsencrypt/live/grpc.new-x.anixops.com/privkey.pem", filepath.Join("/certs", filepath.Base(keyFile)),
		"/etc/letsencrypt/live/new-x.anixops.com/fullchain.pem", filepath.Join("/certs", filepath.Base(certFile)),
		"/etc/letsencrypt/live/new-x.anixops.com/privkey.pem", filepath.Join("/certs", filepath.Base(keyFile)),
	)
	return replacer.Replace(string(example))
}

// startNginx runs nginx in a container on the host's network with config as
// its only server file, and stops it when the test ends.
func startNginx(t *testing.T, config, certDir string) {
	t.Helper()
	configFile := filepath.Join(t.TempDir(), "default.conf")
	require.NoError(t, os.WriteFile(configFile, []byte(config), 0o644))
	name := fmt.Sprintf("anix-edge-e2e-%d", time.Now().UnixNano())
	run := exec.Command("docker", "run", "--rm", "-d", "--name", name, "--network", "host",
		"-v", configFile+":/etc/nginx/conf.d/default.conf:ro",
		"-v", certDir+":/certs:ro",
		"nginx:1.27-alpine")
	output, err := run.CombinedOutput()
	require.NoError(t, err, string(output))
	t.Cleanup(func() {
		if t.Failed() {
			logs, _ := exec.Command("docker", "logs", name).CombinedOutput()
			t.Logf("nginx logs:\n%s", logs)
		}
		_ = exec.Command("docker", "rm", "-f", name).Run()
	})
	// nginx is up when the port accepts connections.
	deadline := time.Now().Add(20 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:8443", time.Second)
		if err == nil {
			_ = conn.Close()
			return
		}
		if time.Now().After(deadline) {
			logs, _ := exec.Command("docker", "logs", name).CombinedOutput()
			t.Fatalf("nginx did not listen on 8443: %v\n%s", err, logs)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// writeNamedCertificate writes a self-signed certificate for name and its
// key, and returns a pool that trusts it.
func writeNamedCertificate(t *testing.T, name string) (string, string, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames: []string{name}, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "grpc.crt"), filepath.Join(dir, "grpc.key")
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o644))
	leaf, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	return certFile, keyFile, roots
}

// withHeader adds a header to every unary and stream call of a connection.
func withHeader(name, value string) []grpc.DialOption {
	outgoing := func(ctx context.Context) context.Context { return metadata.AppendToOutgoingContext(ctx, name, value) }
	return []grpc.DialOption{
		grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			return invoker(outgoing(ctx), method, req, reply, cc, opts...)
		}),
		grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			return streamer(outgoing(ctx), desc, cc, method, opts...)
		}),
	}
}

// escapedCertificate is a certificate chain as nginx's $ssl_client_escaped_cert
// writes it.
func escapedCertificate(certificate *tls.Certificate) string {
	var text []byte
	for _, der := range certificate.Certificate {
		text = append(text, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	return url.PathEscape(string(text))
}
