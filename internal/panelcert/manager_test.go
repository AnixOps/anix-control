package panelcert_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/panelcert"
	"github.com/AnixOps/anix-control/v5/internal/panelcert/panelcerttest"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const panelName = "panel.example.test"

var testKEK = bytes.Repeat([]byte{7}, 32)

func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "control.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	// Control creates the table with the migration 0003_acme_cache.
	require.NoError(t, db.AutoMigrate(&model.ACMECacheEntry{}))
	return db
}

// clock is the real time moved by an offset the test sets.
type clock struct {
	mu     sync.Mutex
	offset time.Duration
}

func newClock() *clock { return &clock{} }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Now().Add(c.offset)
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.offset += d
}

// loopbackResolver resolves every name to 127.0.0.1, this test machine's
// address.
type loopbackResolver struct{}

func (loopbackResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.IPv4(127, 0, 0, 1)}}, nil
}

// listeners are the panel's https and http listeners on loopback ports.
type listeners struct {
	httpsAddr, httpAddr string
}

func serveListeners(t *testing.T, m *panelcert.Manager) listeners {
	t.Helper()
	httpsListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	httpListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "panel") })
	httpsServer := &http.Server{Handler: app, TLSConfig: m.WebTLSConfig(), ReadHeaderTimeout: 5 * time.Second}
	httpServer := &http.Server{Handler: m.HTTPHandler(http.NotFoundHandler()), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = httpsServer.Serve(tls.NewListener(httpsListener, httpsServer.TLSConfig)) }()
	go func() { _ = httpServer.Serve(httpListener) }()
	t.Cleanup(func() {
		_ = httpsServer.Close()
		_ = httpServer.Close()
	})
	return listeners{httpsAddr: httpsListener.Addr().String(), httpAddr: httpListener.Addr().String()}
}

type harness struct {
	db    *gorm.DB
	ca    *panelcerttest.CA
	clock *clock
	m     *panelcert.Manager
	addr  listeners
	stop  context.CancelFunc
	done  chan struct{}
}

func newHarness(t *testing.T, db *gorm.DB, ca *panelcerttest.CA, clk *clock, mutate func(*panelcert.Options)) *harness {
	t.Helper()
	opts := panelcert.Options{
		DB: db, KEK: testKEK, Mode: "acme", Domain: panelName, GRPCName: panelName,
		Certificates: []panelcert.Spec{{Name: panelName, Roles: []string{panelcert.RoleWeb, panelcert.RoleGRPC}, Source: panelcert.SourceACME}},
		DirectoryURL: ca.DirectoryURL, Roots: ca.Roots, Now: clk.Now,
		RetryDelays:    []time.Duration{50 * time.Millisecond},
		AttemptTimeout: 30 * time.Second,
		LocalAddresses: func() []net.IP { return []net.IP{net.IPv4(127, 0, 0, 1)} },
		Resolver:       loopbackResolver{},
		Logf:           t.Logf,
	}
	if mutate != nil {
		mutate(&opts)
	}
	m, err := panelcert.New(opts)
	require.NoError(t, err)
	require.NoError(t, m.Start(context.Background()))
	return &harness{db: db, ca: ca, clock: clk, m: m, addr: serveListeners(t, m)}
}

func (h *harness) run(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	h.stop, h.done = cancel, make(chan struct{})
	go func() {
		defer close(h.done)
		h.m.Run(ctx)
	}()
	t.Cleanup(h.shutdown)
}

func (h *harness) shutdown() {
	if h.stop != nil {
		h.stop()
		<-h.done
		h.stop = nil
	}
}

func waitFor(t *testing.T, what string, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func certificateState(m *panelcert.Manager, name string) panelcert.CertificateStatus {
	for _, certificate := range m.Status().Certificates {
		if certificate.Name == name {
			return certificate
		}
	}
	return panelcert.CertificateStatus{}
}

// handshake connects to addr with SNI name and returns the server's leaf.
// With roots it verifies the chain like an Agent does.
func handshake(t *testing.T, addr, name string, roots *x509.CertPool) (*x509.Certificate, error) {
	t.Helper()
	config := &tls.Config{ServerName: name, RootCAs: roots, MinVersion: tls.VersionTLS12}
	if roots == nil {
		config.InsecureSkipVerify = true // #nosec G402 -- the test inspects a self-signed certificate.
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, config)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	return conn.ConnectionState().PeerCertificates[0], nil
}

func TestIssuesOverTLSALPN01(t *testing.T) {
	ca := panelcerttest.New(t, 0)
	h := newHarness(t, openDB(t), ca, newClock(), nil)
	ca.SetTargets(func(string) string { return h.addr.httpsAddr }, func(string) string { return h.addr.httpAddr })

	// Before the first request completes, a handshake gets the fallback at
	// once: it never waits for the CA.
	before, err := handshake(t, h.addr.httpsAddr, panelName, nil)
	require.NoError(t, err)
	require.Equal(t, before.Subject.String(), before.Issuer.String(), "self-signed fallback")
	require.Contains(t, before.DNSNames, panelName)
	require.Equal(t, panelcert.StatePending, h.m.Status().State)

	h.run(t)
	waitFor(t, "the certificate", 20*time.Second, func() bool { return certificateState(h.m, panelName).State == panelcert.StateValid })

	require.Equal(t, []string{"tls-alpn-01 " + panelName + " ok"}, ca.Validations(), "TLS-ALPN-01 first, and enough")
	leaf, err := handshake(t, h.addr.httpsAddr, panelName, ca.Roots)
	require.NoError(t, err, "the served certificate verifies against the CA")
	require.Equal(t, ca.Issued()[0].SerialNumber, leaf.SerialNumber)

	status := certificateState(h.m, panelName)
	require.Equal(t, panelcert.ServingACME, status.Serving)
	require.True(t, status.PubliclyTrusted)
	require.NotNil(t, status.NotAfter)
	require.NotNil(t, status.RenewAt)
	require.Empty(t, status.LastError)
	require.Equal(t, panelcert.StateValid, h.m.Status().State)
}

func TestFallsBackToHTTP01WhenTLSALPN01Fails(t *testing.T) {
	ca := panelcerttest.New(t, 0)
	h := newHarness(t, openDB(t), ca, newClock(), nil)
	// Port 443 is not this Control's (a filtered port): only port 80 works.
	ca.SetTargets(func(string) string { return "" }, func(string) string { return h.addr.httpAddr })
	h.run(t)
	waitFor(t, "the certificate", 20*time.Second, func() bool { return certificateState(h.m, panelName).State == panelcert.StateValid })

	require.Equal(t, []string{"tls-alpn-01 " + panelName + " failed", "http-01 " + panelName + " ok"}, ca.Validations())
	leaf, err := handshake(t, h.addr.httpsAddr, panelName, ca.Roots)
	require.NoError(t, err)
	require.Equal(t, ca.Issued()[0].SerialNumber, leaf.SerialNumber)
}

func TestFailureServesTheSelfSignedFallbackAndSaysWhy(t *testing.T) {
	ca := panelcerttest.New(t, 0)
	h := newHarness(t, openDB(t), ca, newClock(), func(o *panelcert.Options) {
		o.RetryDelays = []time.Duration{time.Hour}
	})
	ca.SetTargets(func(string) string { return "" }, func(string) string { return "" })
	h.run(t)
	waitFor(t, "the failure", 20*time.Second, func() bool { return certificateState(h.m, panelName).State == panelcert.StateFailed })

	status := certificateState(h.m, panelName)
	require.Equal(t, panelcert.ServingSelfSigned, status.Serving)
	require.Equal(t, panelcert.ProblemUnreachable, status.Problem)
	require.Contains(t, status.LastError, "Timeout during connect")
	require.Contains(t, status.LastError, "tls-alpn-01")
	require.Contains(t, status.LastError, "http-01")
	require.Contains(t, status.Fix, "port 443")
	require.NotNil(t, status.NextAttemptAt)
	require.Equal(t, panelcert.StateFailed, h.m.Status().State)

	// The listener works, with the fallback.
	leaf, err := handshake(t, h.addr.httpsAddr, panelName, nil)
	require.NoError(t, err)
	require.Equal(t, leaf.Subject.String(), leaf.Issuer.String())
	_, err = handshake(t, h.addr.httpsAddr, panelName, ca.Roots)
	require.Error(t, err, "an Agent refuses the fallback")

	// The status is published for the command line and the alerts.
	waitFor(t, "the published status", 5*time.Second, func() bool {
		published, err := panelcert.LoadStatus(context.Background(), h.db)
		return err == nil && published.State == panelcert.StateFailed
	})
}

func TestUnreachableDirectoryIsReported(t *testing.T) {
	ca := panelcerttest.New(t, 0)
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	directory := "http://" + closed.Addr().String() + "/directory"
	require.NoError(t, closed.Close())
	h := newHarness(t, openDB(t), ca, newClock(), func(o *panelcert.Options) {
		o.DirectoryURL = directory
		o.RetryDelays = []time.Duration{time.Hour}
	})
	h.run(t)
	waitFor(t, "the failure", 20*time.Second, func() bool { return certificateState(h.m, panelName).State == panelcert.StateFailed })
	status := certificateState(h.m, panelName)
	require.Equal(t, panelcert.ProblemCAUnreachable, status.Problem)
	require.Contains(t, status.Fix, directory)
	require.Equal(t, directory, h.m.Status().Directory, "a directory other than the public CA's is shown")
}

func TestRenewalReplacesTheCertificateForNewConnections(t *testing.T) {
	// Six-second certificates: the renewal (a third of the lifetime before
	// the end) comes about four seconds after each issuance.
	ca := panelcerttest.New(t, 6*time.Second)
	h := newHarness(t, openDB(t), ca, newClock(), nil)
	ca.SetTargets(func(string) string { return h.addr.httpsAddr }, func(string) string { return h.addr.httpAddr })
	h.run(t)
	waitFor(t, "the certificate", 20*time.Second, func() bool { return certificateState(h.m, panelName).State == panelcert.StateValid })
	first, err := handshake(t, h.addr.httpsAddr, panelName, ca.Roots)
	require.NoError(t, err)

	// A connection made before the renewal keeps its certificate.
	oldConn, err := tls.Dial("tcp", h.addr.httpsAddr, &tls.Config{ServerName: panelName, RootCAs: ca.Roots, MinVersion: tls.VersionTLS12})
	require.NoError(t, err)
	defer func() { _ = oldConn.Close() }()

	renewed := func(serial *big.Int) bool {
		issued := ca.Issued()
		for _, leaf := range issued[1:] {
			if leaf.SerialNumber.Cmp(serial) == 0 {
				return true
			}
		}
		return false
	}
	waitFor(t, "the renewal", 20*time.Second, func() bool { return len(ca.Issued()) >= 2 })
	waitFor(t, "the renewed certificate on new connections", 10*time.Second, func() bool {
		leaf, err := handshake(t, h.addr.httpsAddr, panelName, ca.Roots)
		return err == nil && renewed(leaf.SerialNumber)
	})
	require.Equal(t, first.SerialNumber, oldConn.ConnectionState().PeerCertificates[0].SerialNumber)
	require.Equal(t, ca.Issued()[0].SerialNumber, first.SerialNumber)

	// The gRPC role serves the renewed certificate too.
	grpcListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	grpcTLS := tls.NewListener(grpcListener, &tls.Config{GetCertificate: h.m.GetCertificate(panelcert.RoleGRPC), ClientAuth: tls.RequestClientCert, MinVersion: tls.VersionTLS12})
	defer func() { _ = grpcTLS.Close() }()
	go func() {
		for {
			conn, err := grpcTLS.Accept()
			if err != nil {
				return
			}
			_ = conn.(*tls.Conn).Handshake()
			_ = conn.Close()
		}
	}()
	leaf, err := handshake(t, grpcListener.Addr().String(), panelName, ca.Roots)
	require.NoError(t, err)
	require.True(t, renewed(leaf.SerialNumber))
}

func TestStoredCertificateSurvivesARestartSealed(t *testing.T) {
	ca := panelcerttest.New(t, 0)
	db := openDB(t)
	h := newHarness(t, db, ca, newClock(), nil)
	ca.SetTargets(func(string) string { return h.addr.httpsAddr }, func(string) string { return h.addr.httpAddr })
	h.run(t)
	waitFor(t, "the certificate", 20*time.Second, func() bool { return certificateState(h.m, panelName).State == panelcert.StateValid })
	h.shutdown()

	var rows []model.ACMECacheEntry
	require.NoError(t, db.Find(&rows).Error)
	sealed := 0
	for _, row := range rows {
		require.NotContains(t, string(row.Data), "PRIVATE KEY", row.Key)
		if row.Sealed {
			sealed++
		}
		if strings.HasSuffix(row.Key, "/"+panelName) || strings.HasSuffix(row.Key, "acme_account+key") {
			require.True(t, row.Sealed, row.Key)
		}
	}
	require.GreaterOrEqual(t, sealed, 3, "account key, certificate and fallback")

	// A new process with the same database and root key serves it at once,
	// without asking the CA.
	again := newHarness(t, db, ca, newClock(), nil)
	leaf, err := handshake(t, again.addr.httpsAddr, panelName, ca.Roots)
	require.NoError(t, err)
	require.Equal(t, ca.Issued()[0].SerialNumber, leaf.SerialNumber)
	again.run(t)
	time.Sleep(200 * time.Millisecond)
	require.Len(t, ca.Issued(), 1)

	// Another root key opens nothing: the certificate is requested again
	// rather than served from a row it cannot verify.
	other, err := panelcert.NewStore(db, bytes.Repeat([]byte{9}, 32), nil)
	require.NoError(t, err)
	for _, row := range rows {
		if row.Sealed {
			_, err := other.Get(context.Background(), row.Key)
			require.ErrorIs(t, err, panelcert.ErrSealed)
		}
	}
}

func TestAnotherDirectoryDoesNotReuseTheCertificate(t *testing.T) {
	staging := panelcerttest.New(t, 0)
	db := openDB(t)
	h := newHarness(t, db, staging, newClock(), nil)
	staging.SetTargets(func(string) string { return h.addr.httpsAddr }, nil)
	h.run(t)
	waitFor(t, "the staging certificate", 20*time.Second, func() bool { return certificateState(h.m, panelName).State == panelcert.StateValid })
	h.shutdown()

	production := panelcerttest.New(t, 0)
	again := newHarness(t, db, production, newClock(), nil)
	require.Equal(t, panelcert.StatePending, certificateState(again.m, panelName).State, "the staging certificate is not served for the production directory")
	production.SetTargets(func(string) string { return again.addr.httpsAddr }, nil)
	again.run(t)
	waitFor(t, "the production certificate", 20*time.Second, func() bool { return certificateState(again.m, panelName).State == panelcert.StateValid })
	require.Len(t, production.Issued(), 1)
}

func TestRequestsStayWithinTheHourlyLimit(t *testing.T) {
	ca := panelcerttest.New(t, 0)
	clk := newClock()
	h := newHarness(t, openDB(t), ca, clk, func(o *panelcert.Options) {
		o.RetryDelays = []time.Duration{10 * time.Millisecond}
	})
	ca.SetTargets(func(string) string { return "" }, func(string) string { return "" })
	h.run(t)
	waitFor(t, "three failed requests", 30*time.Second, func() bool { return len(ca.Validations()) >= 6 })
	time.Sleep(300 * time.Millisecond)
	require.Len(t, ca.Validations(), 6, "three requests, each trying both challenges, then the hourly limit")

	result := h.m.Retry()
	require.Empty(t, result.Retrying)
	require.Contains(t, result.Waiting, panelName)
	require.WithinDuration(t, clk.Now().Add(time.Hour), result.Waiting[panelName], 5*time.Second)

	// An hour later, and with the ports open, the retry goes through.
	clk.Advance(time.Hour + time.Minute)
	ca.SetTargets(func(string) string { return h.addr.httpsAddr }, nil)
	require.Equal(t, []string{panelName}, h.m.Retry().Retrying)
	waitFor(t, "the certificate", 20*time.Second, func() bool { return certificateState(h.m, panelName).State == panelcert.StateValid })
}

func TestCommandLineRetryRequestIsPickedUp(t *testing.T) {
	ca := panelcerttest.New(t, 0)
	db := openDB(t)
	h := newHarness(t, db, ca, newClock(), func(o *panelcert.Options) {
		o.RetryDelays = []time.Duration{time.Hour}
	})
	ca.SetTargets(func(string) string { return "" }, func(string) string { return "" })
	h.run(t)
	waitFor(t, "the failure", 20*time.Second, func() bool { return certificateState(h.m, panelName).State == panelcert.StateFailed })

	ca.SetTargets(func(string) string { return h.addr.httpsAddr }, nil)
	require.NoError(t, panelcert.RequestRetry(context.Background(), db, time.Now()))
	waitFor(t, "the retried certificate", 30*time.Second, func() bool { return certificateState(h.m, panelName).State == panelcert.StateValid })
}

// writeKeyPair writes a certificate for name signed by itself, valid for
// lifetime, and returns its serial.
func writeKeyPair(t *testing.T, certFile, keyFile, name string, notAfter time.Time) *big.Int {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: name}, DNSNames: []string{name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))
	return serial
}

func TestFilesAreReloadedWhenTheyChange(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "panel.crt"), filepath.Join(dir, "panel.key")
	firstSerial := writeKeyPair(t, certFile, keyFile, panelName, time.Now().Add(30*24*time.Hour))
	ca := panelcerttest.New(t, 0)
	h := newHarness(t, openDB(t), ca, newClock(), func(o *panelcert.Options) {
		o.Mode = "files"
		o.Certificates = []panelcert.Spec{{Name: panelName, Roles: []string{panelcert.RoleWeb, panelcert.RoleGRPC}, Source: panelcert.SourceFiles, CertFile: certFile, KeyFile: keyFile}}
		o.FilePollInterval = 20 * time.Millisecond
		o.Roots = nil
	})
	require.False(t, h.m.HandlesChallenges())
	leaf, err := handshake(t, h.addr.httpsAddr, panelName, nil)
	require.NoError(t, err)
	require.Equal(t, firstSerial, leaf.SerialNumber, "served from the first handshake")
	status := certificateState(h.m, panelName)
	require.Equal(t, panelcert.StateValid, status.State)
	require.Equal(t, panelcert.ServingFile, status.Serving)
	require.False(t, status.PubliclyTrusted, "a self-signed file does not verify against the system roots")
	require.Equal(t, panelcert.ProblemUntrusted, status.Problem)
	h.run(t)

	secondSerial := writeKeyPair(t, certFile, keyFile, panelName, time.Now().Add(60*24*time.Hour))
	later := time.Now().Add(time.Minute)
	require.NoError(t, os.Chtimes(certFile, later, later))
	waitFor(t, "the reloaded certificate", 5*time.Second, func() bool {
		leaf, err := handshake(t, h.addr.httpsAddr, panelName, nil)
		return err == nil && leaf.SerialNumber.Cmp(secondSerial) == 0
	})

	// A broken file keeps the certificate served so far, and says why.
	require.NoError(t, os.WriteFile(certFile, []byte("not a certificate"), 0o600))
	waitFor(t, "the reload error", 5*time.Second, func() bool {
		return certificateState(h.m, panelName).Problem == panelcert.ProblemFileUnreadable
	})
	leaf, err = handshake(t, h.addr.httpsAddr, panelName, nil)
	require.NoError(t, err)
	require.Equal(t, secondSerial, leaf.SerialNumber)
	require.Equal(t, panelcert.StateValid, certificateState(h.m, panelName).State)
}

func TestMissingFilesFailToTheFallback(t *testing.T) {
	dir := t.TempDir()
	ca := panelcerttest.New(t, 0)
	h := newHarness(t, openDB(t), ca, newClock(), func(o *panelcert.Options) {
		o.Mode = "files"
		o.Certificates = []panelcert.Spec{{Name: panelName, Roles: []string{panelcert.RoleWeb}, Source: panelcert.SourceFiles, CertFile: filepath.Join(dir, "missing.crt"), KeyFile: filepath.Join(dir, "missing.key")}}
	})
	status := certificateState(h.m, panelName)
	require.Equal(t, panelcert.StateFailed, status.State)
	require.Equal(t, panelcert.ProblemFileUnreadable, status.Problem)
	require.Contains(t, status.Fix, "missing.crt")
	leaf, err := handshake(t, h.addr.httpsAddr, panelName, nil)
	require.NoError(t, err)
	require.Equal(t, leaf.Subject.String(), leaf.Issuer.String())
}

func TestHandshakesPickTheCertificateByName(t *testing.T) {
	ca := panelcerttest.New(t, 0)
	const grpcName = "grpc.example.test"
	h := newHarness(t, openDB(t), ca, newClock(), func(o *panelcert.Options) {
		o.Mode, o.GRPCName = "external", grpcName
		o.Certificates = []panelcert.Spec{{Name: grpcName, Roles: []string{panelcert.RoleGRPC}, Source: panelcert.SourceACME}}
	})
	require.False(t, h.m.HasRole(panelcert.RoleWeb))
	require.True(t, h.m.HasRole(panelcert.RoleGRPC))
	ca.SetTargets(func(string) string { return "" }, func(string) string { return h.addr.httpAddr })
	h.run(t)
	waitFor(t, "the gRPC certificate", 20*time.Second, func() bool { return certificateState(h.m, grpcName).State == panelcert.StateValid })
	grpcStatus, ok := h.m.GRPCCertificate()
	require.True(t, ok)
	require.Equal(t, grpcName, grpcStatus.Name)
	require.True(t, grpcStatus.PubliclyTrusted)

	// HTTP-01 answers only the names of this Control.
	request, err := http.NewRequest(http.MethodGet, "http://"+h.addr.httpAddr+"/.well-known/acme-challenge/x", nil)
	require.NoError(t, err)
	request.Host = "other.example.test"
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	_ = response.Body.Close()
	require.Equal(t, http.StatusForbidden, response.StatusCode)
}

func TestStatusWithoutCertificatesIsNotConfigured(t *testing.T) {
	m, err := panelcert.New(panelcert.Options{Mode: "external"})
	require.NoError(t, err)
	require.NoError(t, m.Start(context.Background()))
	status := m.Status()
	require.Equal(t, panelcert.StateNotConfigured, status.State)
	require.Empty(t, status.Certificates)
	require.False(t, status.Persistent)
	_, ok := m.GRPCCertificate()
	require.False(t, ok)
	require.Equal(t, fmt.Sprint([]panelcert.CertificateStatus{}), fmt.Sprint(status.Certificates))
}
