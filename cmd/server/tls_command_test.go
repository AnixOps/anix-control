package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appconfig "github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/panelcert"
	"github.com/AnixOps/anix-control/v5/internal/panelcert/panelcerttest"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openCommandDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "control.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	// Control creates v5_acme_cache with the migration 0003_acme_cache.
	require.NoError(t, db.AutoMigrate(&model.ACMECacheEntry{}))
	return db
}

// failingManager runs a certificate manager whose CA validates nothing,
// and returns it once its status says failed.
func failingManager(t *testing.T, db *gorm.DB, ca *panelcerttest.CA) *panelcert.Manager {
	t.Helper()
	m, err := panelcert.New(panelcert.Options{
		DB: db, KEK: bytes.Repeat([]byte{7}, 32), Mode: appconfig.TLSModeACME, Domain: panelTestDomain, GRPCName: panelTestDomain,
		Certificates: []panelcert.Spec{{Name: panelTestDomain, Roles: []string{panelcert.RoleWeb, panelcert.RoleGRPC}, Source: panelcert.SourceACME}},
		DirectoryURL: ca.DirectoryURL, Roots: ca.Roots, RetryDelays: []time.Duration{time.Hour},
		LocalAddresses: func() []net.IP { return []net.IP{net.IPv4(127, 0, 0, 1)} }, Resolver: loopbackResolver{}, Logf: t.Logf,
	})
	require.NoError(t, err)
	require.NoError(t, m.Start(context.Background()))
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
	waitUntil(t, "the failed status", func() bool {
		status, err := panelcert.LoadStatus(context.Background(), db)
		return err == nil && status.State == panelcert.StateFailed
	})
	return m
}

func TestTLSStatusPrintsTheStoredStatus(t *testing.T) {
	db := openCommandDB(t)
	var out bytes.Buffer
	err := runTLSCommand(context.Background(), db, []string{"status"}, &out)
	require.ErrorIs(t, err, panelcert.ErrNoStatus, "nothing published yet")

	ca := panelcerttest.New(t, 0)
	failingManager(t, db, ca)
	out.Reset()
	require.NoError(t, runTLSCommand(context.Background(), db, []string{"status"}, &out))
	text := out.String()
	require.Contains(t, text, "Panel certificates: failed (tls.mode acme")
	require.Contains(t, text, panelTestDomain+" (web, grpc, from acme): failed, serving self_signed")
	require.Contains(t, text, "error: the CA refused the certificate")
	require.Contains(t, text, "fix:")
	require.Contains(t, text, "next request:")
	require.Contains(t, text, "ACME directory: "+ca.DirectoryURL)

	out.Reset()
	require.NoError(t, runTLSCommand(context.Background(), db, []string{"status", "-json"}, &out))
	var status panelcert.Status
	require.NoError(t, json.Unmarshal(out.Bytes(), &status))
	require.Equal(t, panelcert.StateFailed, status.State)
	require.Len(t, status.Certificates, 1)
	require.Equal(t, panelcert.ProblemUnreachable, status.Certificates[0].Problem)

	require.Error(t, runTLSCommand(context.Background(), db, []string{"renew"}, &out))
	require.Error(t, runTLSCommand(context.Background(), db, nil, &out))
}

func TestTLSRetryIsPickedUpByTheRunningControl(t *testing.T) {
	db := openCommandDB(t)
	ca := panelcerttest.New(t, 0)
	ca.SetTargets(func(string) string { return "" }, func(string) string { return "" })
	m := failingManager(t, db, ca)

	// The ports open; the retry from the command line goes through.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	go serveChallengesOnly(listener, m)
	ca.SetTargets(func(string) string { return listener.Addr().String() }, nil)

	var out bytes.Buffer
	require.NoError(t, runTLSCommand(context.Background(), db, []string{"retry", "-wait", "60s"}, &out), out.String())
	require.Contains(t, out.String(), "Asked the running Control")
	require.Contains(t, out.String(), "Panel certificates: valid")
}

// serveChallengesOnly answers TLS-ALPN-01 handshakes on listener.
func serveChallengesOnly(listener net.Listener, m *panelcert.Manager) {
	config := m.ChallengeOnlyTLSConfig()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer func() { _ = conn.Close() }()
			_ = tls.Server(conn, config).Handshake()
		}()
	}
}

func TestTLSRetryWithoutAControlSaysSo(t *testing.T) {
	db := openCommandDB(t)
	var out bytes.Buffer
	err := runTLSCommand(context.Background(), db, []string{"retry", "-wait", "1s"}, &out)
	require.ErrorIs(t, err, errTLSNotValid)
	require.Contains(t, out.String(), "does not seem to run")
	pending, err := panelcert.RetryPending(context.Background(), db)
	require.NoError(t, err)
	require.True(t, pending, "the request waits for the next start")
	require.Equal(t, 1, adminCommandExitCode(errTLSNotValid))
}

// fakeResolver answers fixed addresses per name.
type fakeResolver map[string][]string

func (r fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	addresses, ok := r[host]
	if !ok {
		return nil, errors.New("no such host")
	}
	var out []net.IPAddr
	for _, address := range addresses {
		out = append(out, net.IPAddr{IP: net.ParseIP(address)})
	}
	return out, nil
}

func doctorByID(checks []doctorCheck) map[string]doctorCheck {
	byID := map[string]doctorCheck{}
	for _, check := range checks {
		byID[check.ID] = check
	}
	return byID
}

// The CDN mode: the panel's name behind a CDN is fine in external mode,
// the gRPC name must resolve straight to this machine.
func TestDoctorChecksDNSAndPortsOfTheCDNMode(t *testing.T) {
	db := openCommandDB(t)
	cfg := &appconfig.Config{Domain: "panel.example.com", Listen: "127.0.0.1:8080"}
	cfg.TLS.Mode = appconfig.TLSModeExternal
	cfg.GRPC.PublicName = "grpc.example.com"
	cfg.ModuleRuntime.CAKEK = panelTestKEK
	env := doctorEnvironment{
		// This machine has a public address of its own.
		localAddresses: func() []net.IP { return []net.IP{net.ParseIP("192.0.2.10"), net.ParseIP("10.0.0.5")} },
		resolver: fakeResolver{
			"panel.example.com": {"198.51.100.1"}, // the CDN
			"grpc.example.com":  {"198.51.100.2"}, // proxied too: wrong
		},
		checkPort: func(address string) (string, string) {
			if strings.HasSuffix(address, ":80") {
				return panelcert.PortError, "permission denied"
			}
			return panelcert.PortInUse, "in use"
		},
		probeTLS: func(string, string) ([]string, error) { return []string{"grpc.example.com"}, nil },
	}
	checks := doctorChecks(context.Background(), cfg, db, env)
	byID := doctorByID(checks)
	require.Equal(t, doctorOK, byID["config"].Level, byID["config"].Message)
	require.Contains(t, byID["config"].Message, "tls.mode external")
	require.Contains(t, byID["config"].Message, "grpc.example.com:50051")
	require.Equal(t, doctorOK, byID["root_key"].Level)
	require.Equal(t, doctorWarn, byID["certificates"].Level, "no status stored yet")
	require.Equal(t, doctorInfo, byID["dns:panel"].Level, "a CDN in front of the panel is expected in external mode")
	require.Equal(t, doctorFail, byID["dns:grpc"].Level)
	require.Contains(t, byID["dns:grpc"].Message, "CDN")
	require.Contains(t, byID["dns:grpc"].Fix, "DNS-only")
	require.Equal(t, doctorOK, byID["port:0.0.0.0:50051"].Level)
	require.Contains(t, byID["port:0.0.0.0:50051"].Message, "answers with a certificate for grpc.example.com")
	require.Equal(t, doctorWarn, byID["port::80"].Level, "the HTTP-01 port could not be checked")
	require.Contains(t, byID["port::80"].Fix, "ss -ltnp")
	require.Equal(t, doctorInfo, byID["firewall"].Level)
	require.Contains(t, byID["cdn:client_address"].Fix, "server.trusted_proxies")

	// The name moved to this machine: the check passes.
	env.resolver = fakeResolver{"panel.example.com": {"198.51.100.1"}, "grpc.example.com": {"192.0.2.10"}}
	byID = doctorByID(doctorChecks(context.Background(), cfg, db, env))
	require.Equal(t, doctorOK, byID["dns:grpc"].Level)

	// Behind NAT nothing can be told for sure: a warning with what to check.
	env.localAddresses = func() []net.IP { return []net.IP{net.ParseIP("10.0.0.5")} }
	env.resolver = fakeResolver{"panel.example.com": {"198.51.100.1"}, "grpc.example.com": {"203.0.113.7"}}
	byID = doctorByID(doctorChecks(context.Background(), cfg, db, env))
	require.Equal(t, doctorWarn, byID["dns:grpc"].Level)
	require.Contains(t, byID["dns:grpc"].Message, "provider's console")
}

func TestDoctorReportsAFailedCertificateAndAMissingRecord(t *testing.T) {
	db := openCommandDB(t)
	ca := panelcerttest.New(t, 0)
	failingManager(t, db, ca)
	cfg := &appconfig.Config{Domain: panelTestDomain}
	env := doctorEnvironment{
		localAddresses: func() []net.IP { return []net.IP{net.ParseIP("192.0.2.10")} },
		resolver:       fakeResolver{},
		checkPort:      func(string) (string, string) { return panelcert.PortFree, "" },
	}
	byID := doctorByID(doctorChecks(context.Background(), cfg, db, env))
	require.Equal(t, doctorFail, byID["config"].Level, "acme without the root key does not start")
	require.Equal(t, doctorFail, byID["root_key"].Level)
	certificate := byID["certificate:"+panelTestDomain]
	require.Equal(t, doctorFail, certificate.Level)
	require.Contains(t, certificate.Message, "self-signed")
	require.NotEmpty(t, certificate.Fix)
	require.Equal(t, doctorFail, byID["dns:panel"].Level)
	require.Contains(t, byID["dns:panel"].Fix, "A record")
	require.Equal(t, doctorWarn, byID["port::443"].Level)
	require.Contains(t, byID["port::443"].Message, "Control is not running")
}

func TestDoctorCommandPrintsAndFails(t *testing.T) {
	db := openCommandDB(t)
	cfg := &appconfig.Config{}
	cfg.Server.Host, cfg.Server.Port = "127.0.0.1", freePort(t)
	var out bytes.Buffer
	err := runDoctorCommand(context.Background(), cfg, db, nil, &out)
	require.ErrorIs(t, err, errDoctorFailed, "no root key")
	require.Contains(t, out.String(), "[fail] module_runtime.ca_kek is not set")
	require.Contains(t, out.String(), "       fix: ")
	require.Contains(t, out.String(), "[warn] domain is not set")
	require.Equal(t, 1, adminCommandExitCode(err))

	out.Reset()
	_ = runDoctorCommand(context.Background(), cfg, db, []string{"-json"}, &out)
	var answer struct {
		Checks []doctorCheck `json:"checks"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &answer))
	require.NotEmpty(t, answer.Checks)
	require.Error(t, runDoctorCommand(context.Background(), cfg, db, []string{"extra"}, &out))
}
