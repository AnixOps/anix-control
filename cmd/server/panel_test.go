package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/agentinstall"
	"github.com/AnixOps/anix-control/v5/internal/cache"
	appconfig "github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/panelcert"
	"github.com/AnixOps/anix-control/v5/internal/panelcert/panelcerttest"
	"github.com/AnixOps/anix-control/v5/internal/requestorigin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const (
	panelTestDomain = "panel.example.test"
	panelTestIndex  = "<!doctype html><title>spa-index</title>"
	panelTestKEK    = "BwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwc="
)

// panelTestConfig is a configuration for the listener tests: the web app
// in a temporary directory, the database of database.Init, a release tag
// for the install metadata.
func panelTestConfig(t *testing.T) *appconfig.Config {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cache.InitMemory()
	require.NoError(t, database.Init(&appconfig.DatabaseConfig{Driver: "sqlite", Database: filepath.Join(t.TempDir(), "control.db"), LogLevel: "silent"}))
	t.Cleanup(func() { _ = database.Close() })
	require.NoError(t, database.Get().AutoMigrate(append(model.KernelModels(), &model.User{}, &model.ACMECacheEntry{})...))
	previous := requestorigin.Default()
	t.Cleanup(func() { requestorigin.SetDefault(previous) })

	webRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(webRoot, "index.html"), []byte(panelTestIndex), 0o600))
	cfg := &appconfig.Config{Env: "test"}
	cfg.JWT.Secret = "panel-test-secret"
	cfg.App.SubscribePath = "s"
	cfg.Frontend.Enable = true
	cfg.Frontend.Path = webRoot
	cfg.AgentInstall.AgentVersion = "v4.2.0"
	cfg.ModuleRuntime.CAKEK = panelTestKEK
	appconfig.Set(cfg)
	return cfg
}

// testPanel is a running panel: its listeners and certificates.
type testPanel struct {
	certs   *panelcert.Manager
	servers []*panelServer
	byName  map[string]string
}

// startTestPanel binds the layout on loopback ports, with handler
// wrapped so that X-Test-Peer sets the peer address the panel sees.
func startTestPanel(t *testing.T, cfg *appconfig.Config, panel appconfig.PanelSettings, tune func(*panelcert.Options)) *testPanel {
	t.Helper()
	tunePanelCertificates = func(o *panelcert.Options) {
		o.LocalAddresses = func() []net.IP { return []net.IP{net.IPv4(127, 0, 0, 1)} }
		o.Resolver = loopbackResolver{}
		o.RetryDelays = []time.Duration{time.Hour}
		o.Logf = t.Logf
		if tune != nil {
			tune(o)
		}
	}
	t.Cleanup(func() { tunePanelCertificates = nil })
	certs, err := newPanelCertificates(context.Background(), cfg, panel, database.Get())
	require.NoError(t, err)
	handler, err := newPanelHandler(cfg)
	require.NoError(t, err)
	peerOverride := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if peer := r.Header.Get("X-Test-Peer"); peer != "" {
			r.RemoteAddr = peer
			r.Header.Del("X-Test-Peer")
		}
		handler.ServeHTTP(w, r)
	})
	servers, err := bindPanelServers(cfg, panel, certs, peerOverride)
	require.NoError(t, err)
	running := &testPanel{certs: certs, servers: servers, byName: map[string]string{}}
	for _, server := range servers {
		running.byName[server.purpose] = server.listener.Addr().String()
		go func() { _ = server.serve() }()
	}
	t.Cleanup(func() {
		for _, server := range servers {
			_ = server.server.Close()
		}
	})
	return running
}

// run starts the certificate loop (the first ACME request goes out at
// once).
func (p *testPanel) run(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.certs.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

// loopbackResolver resolves every name to this test machine.
type loopbackResolver struct{}

func (loopbackResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.IPv4(127, 0, 0, 1)}}, nil
}

func get(t *testing.T, client *http.Client, url string, headers map[string]string) (int, string, http.Header) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	for name, value := range headers {
		if name == "Host" {
			request.Host = value
			continue
		}
		request.Header.Set(name, value)
	}
	response, err := client.Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, string(body), response.Header
}

func waitUntil(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// One plain listener (tls.mode external) serves the web app, the API and
// the install paths: the script is the script, never the web app's HTML.
func TestExternalModeServesEverythingOnOnePlainListener(t *testing.T) {
	cfg := panelTestConfig(t)
	cfg.Domain = panelTestDomain
	cfg.TLS.Mode = appconfig.TLSModeExternal
	cfg.Listen = "127.0.0.1:0"
	cfg.Server.TrustedProxies = nil // loopback, the default
	panel := cfg.Panel()
	panel.GRPCCertSource, panel.ACMENames, panel.GRPCEnabled = appconfig.GRPCCertNone, nil, false
	panel.ChallengeHTTPAddress, panel.ChallengeTLSAddress = "", ""
	running := startTestPanel(t, cfg, panel, nil)
	require.Len(t, running.servers, 1, "one listener")
	base := "http://" + running.byName["plain http"]
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	status, body, header := get(t, client, base+"/install.sh", nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, string(agentinstall.Script), body)
	require.Contains(t, header.Get("Content-Type"), "text/x-shellscript")

	status, body, _ = get(t, client, base+"/install/agent.env", nil)
	require.Equal(t, http.StatusOK, status, body)
	require.Contains(t, body, "grpc_target "+panelTestDomain+":50051", "the gRPC target is <domain>:<grpc.port>")

	// /install/ip: the client's address, as the join needs it.
	for name, tc := range map[string]struct {
		headers map[string]string
		want    string
	}{
		"direct client, spoofed headers ignored": {map[string]string{"X-Test-Peer": "203.0.113.9:40000", "X-Forwarded-For": "198.51.100.66", "X-Real-IP": "198.51.100.67"}, "203.0.113.9"},
		"direct IPv6 client":                     {map[string]string{"X-Test-Peer": "[2001:db8::9]:40000"}, "2001:db8::9"},
		"the CDN's proxy on this machine":        {map[string]string{"X-Test-Peer": "127.0.0.1:40000", "X-Forwarded-For": "198.51.100.7"}, "198.51.100.7"},
		"a chain through the proxy":              {map[string]string{"X-Test-Peer": "127.0.0.1:40000", "X-Forwarded-For": "9.9.9.9, 198.51.100.7"}, "198.51.100.7"},
		"the proxy without headers":              {map[string]string{"X-Test-Peer": "127.0.0.1:40000"}, "127.0.0.1"},
	} {
		status, body, _ := get(t, client, base+"/install/ip", tc.headers)
		require.Equal(t, http.StatusOK, status, name)
		require.Equal(t, tc.want, strings.TrimSpace(body), name)
	}

	// The web app and its fallback; reserved prefixes answer 404.
	for _, path := range []string{"/", "/admin/login", "/dashboard/nodes", "/install", "/install.shx"} {
		status, body, header := get(t, client, base+path, nil)
		require.Equal(t, http.StatusOK, status, path)
		require.Equal(t, panelTestIndex, body, path)
		require.Contains(t, header.Get("Cache-Control"), "no-store", path)
	}
	for _, path := range []string{"/install/unknown", "/install/agent/v4.2.0/missing.zip", "/api/unknown", "/s/a/b", "/.well-known/acme-challenge/x"} {
		status, body, _ := get(t, client, base+path, nil)
		require.NotEqual(t, http.StatusOK, status, path)
		require.NotContains(t, body, "spa-index", path)
	}
	status, body, _ = get(t, client, base+"/api/v2/no-such-route", nil)
	require.NotEqual(t, http.StatusOK, status)
	require.NotContains(t, body, "spa-index")
	status, _, _ = get(t, client, base+"/livez", nil)
	require.Equal(t, http.StatusOK, status)

	// No HSTS over the proxy's plain hop without https, and none over
	// Control's own TLS (the acme test); a proxy that reports https gets it.
	_, _, header = get(t, client, base+"/livez", map[string]string{"X-Test-Peer": "127.0.0.1:1", "X-Forwarded-Proto": "https"})
	require.NotEmpty(t, header.Get("Strict-Transport-Security"))
}

// The legacy layout (no listen): server.port and frontend.port are the same
// handler, no proxy between them.
func TestLegacyPortsShareOneHandler(t *testing.T) {
	cfg := panelTestConfig(t)
	cfg.Server.Host = "127.0.0.1"
	panel := cfg.Panel()
	require.Equal(t, appconfig.TLSModeExternal, panel.Mode)
	panel.PlainAddresses = []string{"127.0.0.1:0", "127.0.0.1:0"}
	running := startTestPanel(t, cfg, panel, nil)
	require.Len(t, running.servers, 2)
	for _, server := range running.servers {
		status, body, _ := get(t, http.DefaultClient, "http://"+server.listener.Addr().String()+"/install.sh", nil)
		require.Equal(t, http.StatusOK, status)
		require.Equal(t, string(agentinstall.Script), body)
	}
}

// tls.mode acme end to end with an ACME test CA: the certificate is
// obtained over the panel's own listeners, the https listener serves it
// with the install paths, the http listener redirects and answers the
// HTTP-01 challenge, and the gRPC listener serves the same certificate.
func TestACMEModeObtainsTheCertificateAndServesTheSingleListener(t *testing.T) {
	cfg := panelTestConfig(t)
	cfg.Domain = panelTestDomain
	panel := cfg.Panel()
	require.Equal(t, appconfig.TLSModeACME, panel.Mode)
	panel.HTTPSAddress, panel.HTTPAddress = "127.0.0.1:0", "127.0.0.1:0"
	ca := panelcerttest.New(t, 0)
	running := startTestPanel(t, cfg, panel, func(o *panelcert.Options) {
		o.DirectoryURL, o.Roots = ca.DirectoryURL, ca.Roots
	})
	httpsAddr, httpAddr := running.byName["https"], running.byName["http (redirect to https, ACME HTTP-01)"]
	// TLS-ALPN-01 is filtered here: the CA falls back to HTTP-01 on port 80.
	ca.SetTargets(func(string) string { return "" }, func(string) string { return httpAddr })
	require.Equal(t, panelcert.StatePending, running.certs.Status().State)
	running.run(t)
	waitUntil(t, "the certificate", func() bool { return running.certs.Status().State == panelcert.StateValid })
	require.Equal(t, []string{"tls-alpn-01 " + panelTestDomain + " failed", "http-01 " + panelTestDomain + " ok"}, ca.Validations())

	// The single https listener, verified like a browser or a node does.
	client := &http.Client{
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{RootCAs: ca.Roots, ServerName: panelTestDomain, MinVersion: tls.VersionTLS12}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	status, body, header := get(t, client, "https://"+httpsAddr+"/install.sh", map[string]string{"Host": panelTestDomain})
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, string(agentinstall.Script), body)
	require.Empty(t, header.Get("Strict-Transport-Security"), "no HSTS over Control's own TLS")
	status, body, _ = get(t, client, "https://"+httpsAddr+"/install/ip", map[string]string{"Host": panelTestDomain})
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "127.0.0.1", strings.TrimSpace(body))
	status, body, _ = get(t, client, "https://"+httpsAddr+"/install/agent.env", map[string]string{"Host": panelTestDomain})
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, panelTestDomain+":50051")
	status, body, _ = get(t, client, "https://"+httpsAddr+"/admin/login", map[string]string{"Host": panelTestDomain})
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, panelTestIndex, body)

	// Port 80 redirects to https on the same name.
	_, httpsPort, _ := net.SplitHostPort(httpsAddr)
	status, _, header = get(t, client, "http://"+httpAddr+"/install.sh?x=1", map[string]string{"Host": panelTestDomain})
	require.Equal(t, http.StatusMovedPermanently, status)
	require.Equal(t, "https://"+net.JoinHostPort(panelTestDomain, httpsPort)+"/install.sh?x=1", header.Get("Location"))
	request, err := http.NewRequest(http.MethodPost, "http://"+httpAddr+"/api/v2/login", strings.NewReader("{}"))
	require.NoError(t, err)
	response, err := client.Do(request)
	require.NoError(t, err)
	_ = response.Body.Close()
	require.Equal(t, http.StatusBadRequest, response.StatusCode, "no redirect of a POST")

	// The gRPC listener takes the same certificate per handshake and asks
	// for client certificates.
	cfg.GRPC.Host, cfg.GRPC.Port = "127.0.0.1", freePort(t)
	grpcPanel := cfg.Panel()
	server, addr, err := startGRPCServer(cfg, grpcPanel, running.certs)
	require.NoError(t, err)
	t.Cleanup(server.Stop)
	require.True(t, server.TLSEnabled())
	// The client certificate request with the agent PKI is covered in
	// internal/grpc (TestAgentListenerServesTheCurrentCertificate...).
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: panelTestDomain, RootCAs: ca.Roots, MinVersion: tls.VersionTLS12, NextProtos: []string{"h2"}})
	require.NoError(t, err)
	require.Equal(t, ca.Issued()[0].SerialNumber, conn.ConnectionState().PeerCertificates[0].SerialNumber)
	_ = conn.Close()
}

// A renewal reaches the next connection of both the https listener and the
// node gRPC listener, without a restart; a connection made before keeps
// its certificate.
func TestRenewedCertificateReachesHTTPSAndGRPC(t *testing.T) {
	cfg := panelTestConfig(t)
	cfg.Domain = panelTestDomain
	cfg.GRPC.Host, cfg.GRPC.Port = "127.0.0.1", freePort(t)
	panel := cfg.Panel()
	panel.HTTPSAddress, panel.HTTPAddress = "127.0.0.1:0", "127.0.0.1:0"
	// Nine-second certificates: each is renewed about six seconds after it
	// was issued (a third of the lifetime before its end).
	ca := panelcerttest.New(t, 9*time.Second)
	running := startTestPanel(t, cfg, panel, func(o *panelcert.Options) {
		o.DirectoryURL, o.Roots = ca.DirectoryURL, ca.Roots
		o.RetryDelays = []time.Duration{100 * time.Millisecond}
	})
	httpsAddr := running.byName["https"]
	ca.SetTargets(func(string) string { return httpsAddr }, nil)
	server, grpcAddr, err := startGRPCServer(cfg, cfg.Panel(), running.certs)
	require.NoError(t, err)
	t.Cleanup(server.Stop)
	running.run(t)
	waitUntil(t, "the first certificate", func() bool { return len(ca.Issued()) >= 1 && running.certs.Status().State == panelcert.StateValid })
	first := ca.Issued()[0].SerialNumber

	verified := func(address string, protocols ...string) (*big.Int, *tls.Conn) {
		conn, err := tls.Dial("tcp", address, &tls.Config{ServerName: panelTestDomain, RootCAs: ca.Roots, MinVersion: tls.VersionTLS12, NextProtos: protocols})
		if err != nil {
			return nil, nil
		}
		return conn.ConnectionState().PeerCertificates[0].SerialNumber, conn
	}
	serial, before := verified(grpcAddr, "h2")
	require.NotNil(t, before)
	t.Cleanup(func() { _ = before.Close() })
	require.Equal(t, first, serial)

	renewed := func(serial *big.Int) bool { return serial != nil && serial.Cmp(first) != 0 }
	waitUntil(t, "the renewed certificate on the https listener", func() bool {
		serial, conn := verified(httpsAddr, "http/1.1")
		if conn != nil {
			_ = conn.Close()
		}
		return renewed(serial)
	})
	waitUntil(t, "the renewed certificate on the gRPC listener", func() bool {
		serial, conn := verified(grpcAddr, "h2")
		if conn != nil {
			_ = conn.Close()
		}
		return renewed(serial)
	})
	require.Equal(t, first, before.ConnectionState().PeerCertificates[0].SerialNumber, "an open connection keeps its certificate")
	require.GreaterOrEqual(t, len(ca.Issued()), 2)
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}

// When the CA cannot be reached, the panel starts anyway: the https
// listener serves the self-signed fallback and the status says why.
func TestACMEFailureKeepsAWorkingSelfSignedListener(t *testing.T) {
	cfg := panelTestConfig(t)
	cfg.Domain = panelTestDomain
	panel := cfg.Panel()
	panel.HTTPSAddress, panel.HTTPAddress = "127.0.0.1:0", "127.0.0.1:0"
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	directory := "http://" + closed.Addr().String() + "/directory"
	require.NoError(t, closed.Close())
	running := startTestPanel(t, cfg, panel, func(o *panelcert.Options) { o.DirectoryURL = directory })
	running.run(t)
	waitUntil(t, "the failure", func() bool { return running.certs.Status().State == panelcert.StateFailed })

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: panelTestDomain, MinVersion: tls.VersionTLS12}}} // #nosec G402 -- the test reads the fallback.
	status, body, _ := get(t, client, "https://"+running.byName["https"]+"/install.sh", map[string]string{"Host": panelTestDomain})
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, string(agentinstall.Script), body)
	conn, err := tls.Dial("tcp", running.byName["https"], &tls.Config{InsecureSkipVerify: true, ServerName: panelTestDomain, MinVersion: tls.VersionTLS12}) // #nosec G402 -- as above.
	require.NoError(t, err)
	leaf := conn.ConnectionState().PeerCertificates[0]
	_ = conn.Close()
	require.Equal(t, leaf.Subject.String(), leaf.Issuer.String(), "self-signed")

	certificate, ok := running.certs.Status().Certificate(panelcert.RoleWeb)
	require.True(t, ok)
	require.Equal(t, panelcert.StateFailed, certificate.State)
	require.Equal(t, panelcert.ServingSelfSigned, certificate.Serving)
	require.Equal(t, panelcert.ProblemCAUnreachable, certificate.Problem)
	require.NotEmpty(t, certificate.Fix)
}

// writeSelfSignedPair writes a certificate for name and returns its serial.
func writeSelfSignedPair(t *testing.T, certFile, keyFile, name string) *big.Int {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: name}, DNSNames: []string{name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))
	later := time.Now().Add(time.Duration(serial.Int64()%1000+1) * time.Second)
	require.NoError(t, os.Chtimes(certFile, later, later))
	return serial
}

func servedSerial(t *testing.T, address, name string) *big.Int {
	t.Helper()
	conn, err := tls.Dial("tcp", address, &tls.Config{InsecureSkipVerify: true, ServerName: name, MinVersion: tls.VersionTLS12}) // #nosec G402 -- the test reads the served certificate.
	if err != nil {
		return nil
	}
	defer func() { _ = conn.Close() }()
	return conn.ConnectionState().PeerCertificates[0].SerialNumber
}

// tls.mode files: the https listener serves the files and picks up new
// ones without a restart.
func TestFilesModeReloadsTheWebCertificate(t *testing.T) {
	cfg := panelTestConfig(t)
	dir := t.TempDir()
	cfg.Domain = panelTestDomain
	cfg.TLS.Mode = appconfig.TLSModeFiles
	cfg.TLS.CertFile, cfg.TLS.KeyFile = filepath.Join(dir, "panel.crt"), filepath.Join(dir, "panel.key")
	first := writeSelfSignedPair(t, cfg.TLS.CertFile, cfg.TLS.KeyFile, panelTestDomain)
	panel := cfg.Panel()
	panel.HTTPSAddress, panel.HTTPAddress = "127.0.0.1:0", "127.0.0.1:0"
	running := startTestPanel(t, cfg, panel, func(o *panelcert.Options) { o.FilePollInterval = 20 * time.Millisecond })
	running.run(t)
	require.False(t, running.certs.HandlesChallenges())
	httpsAddr := running.byName["https"]
	require.Equal(t, first, servedSerial(t, httpsAddr, panelTestDomain))

	second := writeSelfSignedPair(t, cfg.TLS.CertFile, cfg.TLS.KeyFile, panelTestDomain)
	waitUntil(t, "the reloaded certificate", func() bool {
		serial := servedSerial(t, httpsAddr, panelTestDomain)
		return serial != nil && serial.Cmp(second) == 0
	})
}

// tls.mode external with grpc.tls_cert_file: the gRPC listener's
// certificate files are watched and reloaded too.
func TestExternalModeReloadsTheGRPCCertificateFiles(t *testing.T) {
	cfg := panelTestConfig(t)
	dir := t.TempDir()
	cfg.Domain = panelTestDomain
	cfg.TLS.Mode = appconfig.TLSModeExternal
	cfg.Listen = "127.0.0.1:0"
	cfg.TLS.CertFile, cfg.TLS.KeyFile = filepath.Join(dir, "grpc.crt"), filepath.Join(dir, "grpc.key")
	cfg.GRPC.Host, cfg.GRPC.Port = "127.0.0.1", freePort(t)
	first := writeSelfSignedPair(t, cfg.TLS.CertFile, cfg.TLS.KeyFile, panelTestDomain)
	panel := cfg.Panel()
	require.Equal(t, appconfig.GRPCCertFiles, panel.GRPCCertSource)
	require.False(t, panel.NeedsACME())
	running := startTestPanel(t, cfg, panel, func(o *panelcert.Options) { o.FilePollInterval = 20 * time.Millisecond })
	running.run(t)
	require.Len(t, running.servers, 1, "external mode binds listen only")
	server, addr, err := startGRPCServer(cfg, panel, running.certs)
	require.NoError(t, err)
	t.Cleanup(server.Stop)
	require.Equal(t, first, servedSerial(t, addr, panelTestDomain))

	second := writeSelfSignedPair(t, cfg.TLS.CertFile, cfg.TLS.KeyFile, panelTestDomain)
	waitUntil(t, "the reloaded gRPC certificate", func() bool {
		serial := servedSerial(t, addr, panelTestDomain)
		return serial != nil && serial.Cmp(second) == 0
	})
	certificate, ok := running.certs.GRPCCertificate()
	require.True(t, ok)
	require.Equal(t, panelcert.ServingFile, certificate.Serving)
	require.False(t, certificate.PubliclyTrusted)
}

// A port the layout needs that is taken fails the start; an ACME
// challenge port of external mode that is taken only shows in the status.
func TestPanelListenersAndPortConflicts(t *testing.T) {
	cfg := panelTestConfig(t)
	cfg.Domain = panelTestDomain
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = taken.Close() })

	panel := cfg.Panel()
	panel.HTTPSAddress, panel.HTTPAddress = "127.0.0.1:0", taken.Addr().String()
	certs, err := newPanelCertificates(context.Background(), cfg, panel, database.Get())
	require.NoError(t, err)
	_, err = bindPanelServers(cfg, panel, certs, http.NotFoundHandler())
	require.Error(t, err)
	require.Contains(t, err.Error(), taken.Addr().String())

	cfg.TLS.Mode = appconfig.TLSModeExternal
	cfg.GRPC.PublicName = "grpc.example.test"
	cfg.Listen = "127.0.0.1:0"
	panel = cfg.Panel()
	require.Equal(t, []string{"grpc.example.test"}, panel.ACMENames)
	panel.ChallengeHTTPAddress, panel.ChallengeTLSAddress = taken.Addr().String(), "127.0.0.1:0"
	certs, err = newPanelCertificates(context.Background(), cfg, panel, database.Get())
	require.NoError(t, err)
	servers, err := bindPanelServers(cfg, panel, certs, http.NotFoundHandler())
	require.NoError(t, err)
	t.Cleanup(func() {
		for _, server := range servers {
			_ = server.listener.Close()
		}
	})
	require.Len(t, servers, 2, "listen and the TLS-ALPN-01 listener")
	require.Len(t, certs.Status().Listeners, 1)
	require.Equal(t, taken.Addr().String(), certs.Status().Listeners[0].Address)
}
