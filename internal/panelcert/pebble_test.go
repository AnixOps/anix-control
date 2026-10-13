package panelcert_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/panelcert"
	"github.com/stretchr/testify/require"
)

// The panel certificates against a real ACME server: Pebble, the CA test
// server of the public CA's maintainers, with pebble-challtestsrv as its
// DNS (every name resolves to 127.0.0.1). The test runs only when CI (or a
// developer) starts them and names them in the environment:
//
//	ANIX_CONTROL_TEST_PEBBLE_DIRECTORY  https://127.0.0.1:14000/dir
//	ANIX_CONTROL_TEST_PEBBLE_CA_FILE    the PEM of Pebble's own https certificate
//	ANIX_CONTROL_TEST_PEBBLE_ROOTS_URL  https://127.0.0.1:15000/roots/0 (the issuing root)
//	ANIX_CONTROL_TEST_PEBBLE_TLS_PORT   Pebble's tlsPort (5001), where TLS-ALPN-01 is validated
//	ANIX_CONTROL_TEST_PEBBLE_HTTP_PORT  Pebble's httpPort (5002), where HTTP-01 is validated
//
// Run Pebble with PEBBLE_VA_NOSLEEP=1 and PEBBLE_AUTHZREUSE=0, so that every
// order validates its challenge again. docs/reference/configuration.md
// ("Testing ACME") has the commands.
func TestPebbleIssuesOverBothChallenges(t *testing.T) {
	directory := os.Getenv("ANIX_CONTROL_TEST_PEBBLE_DIRECTORY")
	if directory == "" {
		t.Skip("ANIX_CONTROL_TEST_PEBBLE_DIRECTORY is not set: no Pebble to test against")
	}
	caPEM, err := os.ReadFile(os.Getenv("ANIX_CONTROL_TEST_PEBBLE_CA_FILE"))
	require.NoError(t, err)
	apiRoots := x509.NewCertPool()
	require.True(t, apiRoots.AppendCertsFromPEM(caPEM))
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: apiRoots, MinVersion: tls.VersionTLS12}}}
	response, err := client.Get(os.Getenv("ANIX_CONTROL_TEST_PEBBLE_ROOTS_URL"))
	require.NoError(t, err)
	rootPEM, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	require.NoError(t, err)
	issuingRoots := x509.NewCertPool()
	require.True(t, issuingRoots.AppendCertsFromPEM(rootPEM), "Pebble's issuing root")
	tlsPort, httpPort := envOr("ANIX_CONTROL_TEST_PEBBLE_TLS_PORT", "5001"), envOr("ANIX_CONTROL_TEST_PEBBLE_HTTP_PORT", "5002")

	for _, tc := range []struct {
		name        string
		host        string
		tlsListener bool
	}{
		{"TLS-ALPN-01 on the https listener", "alpn.anixops-pebble.test", true},
		{"HTTP-01 when the https port is closed", "http.anixops-pebble.test", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := panelcert.New(panelcert.Options{
				DB: openDB(t), KEK: testKEK, Mode: "acme", Domain: tc.host, GRPCName: tc.host,
				Certificates: []panelcert.Spec{{Name: tc.host, Roles: []string{panelcert.RoleWeb, panelcert.RoleGRPC}, Source: panelcert.SourceACME}},
				DirectoryURL: directory, HTTPClient: client, Roots: issuingRoots,
				RetryDelays: []time.Duration{time.Hour}, AttemptTimeout: 2 * time.Minute, Logf: t.Logf,
				LocalAddresses: func() []net.IP { return []net.IP{net.IPv4(127, 0, 0, 1)} }, Resolver: loopbackResolver{},
			})
			require.NoError(t, err)
			require.NoError(t, m.Start(context.Background()))
			httpListener, err := net.Listen("tcp", "127.0.0.1:"+httpPort)
			require.NoError(t, err)
			httpServer := &http.Server{Handler: m.HTTPHandler(http.NotFoundHandler()), ReadHeaderTimeout: 5 * time.Second}
			go func() { _ = httpServer.Serve(httpListener) }()
			t.Cleanup(func() { _ = httpServer.Close() })
			httpsAddr := ""
			if tc.tlsListener {
				listener, err := net.Listen("tcp", "127.0.0.1:"+tlsPort)
				require.NoError(t, err)
				httpsServer := &http.Server{Handler: http.NotFoundHandler(), TLSConfig: m.WebTLSConfig(), ReadHeaderTimeout: 5 * time.Second}
				go func() { _ = httpsServer.Serve(tls.NewListener(listener, httpsServer.TLSConfig)) }()
				t.Cleanup(func() { _ = httpsServer.Close() })
				httpsAddr = listener.Addr().String()
			}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				defer close(done)
				m.Run(ctx)
			}()
			t.Cleanup(func() {
				cancel()
				<-done
			})
			waitFor(t, "the certificate from Pebble", 2*time.Minute, func() bool {
				status := certificateState(m, tc.host)
				if status.State == panelcert.StateFailed {
					t.Fatalf("issuance failed: %s", status.LastError)
				}
				return status.State == panelcert.StateValid
			})
			status := certificateState(m, tc.host)
			require.True(t, status.PubliclyTrusted, "verifies against Pebble's root")
			require.True(t, strings.HasPrefix(status.Issuer, "Pebble"), status.Issuer)
			if httpsAddr != "" {
				leaf, err := handshake(t, httpsAddr, tc.host, issuingRoots)
				require.NoError(t, err)
				require.Equal(t, []string{tc.host}, leaf.DNSNames)
			}
		})
	}
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
