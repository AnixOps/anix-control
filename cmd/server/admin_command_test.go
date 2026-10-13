package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/adminclaim"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/panelcert"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var claimTokenPattern = regexp.MustCompile(`anixclaim_[A-Za-z0-9_-]{43}`)

func claimCommandDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.Open(&config.DatabaseConfig{Driver: "sqlite", Database: filepath.Join(t.TempDir(), "control.db"), LogLevel: "silent"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.CloseHandle(db) })
	_, err = migrations.Migrate(context.Background(), db, migrations.Options{ProgramVersion: version})
	require.NoError(t, err)
	return db
}

func claimConfig(t *testing.T, mode, domain string) *config.Config {
	t.Helper()
	cfg := config.Defaults()
	cfg.Domain = domain
	cfg.TLS.Mode = mode
	return cfg
}

// claimEnvironment answers with status (or ErrNoStatus when nil) and the
// machine's addresses.
func claimEnvironment(status *panelcert.Status, addresses ...string) claimLinkEnvironment {
	return claimLinkEnvironment{
		now: func() time.Time { return time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC) },
		loadStatus: func(context.Context, *gorm.DB) (panelcert.Status, error) {
			if status == nil {
				return panelcert.Status{}, panelcert.ErrNoStatus
			}
			return *status, nil
		},
		localAddresses: func() []net.IP {
			var ips []net.IP
			for _, address := range addresses {
				ips = append(ips, net.ParseIP(address))
			}
			return ips
		},
	}
}

func webStatus(trusted bool) *panelcert.Status {
	return &panelcert.Status{Mode: config.TLSModeACME, Certificates: []panelcert.CertificateStatus{
		{Name: "panel.example.com", Roles: []string{panelcert.RoleWeb, panelcert.RoleGRPC}, PubliclyTrusted: trusted},
	}}
}

func runClaimLink(t *testing.T, cfg *config.Config, db *gorm.DB, env claimLinkEnvironment, arguments ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := runAdminClaimCommandWith(context.Background(), cfg, db, append([]string{"claim-link"}, arguments...), &out, env)
	return out.String(), err
}

func TestAdminClaimLinkPrintsTheLinkWithItsToken(t *testing.T) {
	db := claimCommandDB(t)
	cfg := claimConfig(t, config.TLSModeACME, "panel.example.com")
	out, err := runClaimLink(t, cfg, db, claimEnvironment(webStatus(true), "203.0.113.10"))
	require.NoError(t, err)

	token := claimTokenPattern.FindString(out)
	require.NotEmpty(t, token, out)
	assert.True(t, adminclaim.WellFormed(token))
	assert.Contains(t, out, "\n  https://panel.example.com/claim#"+token+"\n")
	assert.Equal(t, 1, strings.Count(out, token), "a trusted certificate needs no second address")
	assert.Contains(t, out, "within 24 hours")
	assert.Contains(t, out, "The link expires at 2026-10-14 08:00 UTC.")

	// Only the hash is stored.
	var entry migrations.MetaEntry
	require.NoError(t, db.Where("key = ?", adminclaim.MetaKey).Take(&entry).Error)
	assert.Contains(t, entry.Value, adminclaim.Hash(token))
	assert.NotContains(t, entry.Value, token)

	// Again: a new token, and the old one no longer claims anything.
	again, err := runClaimLink(t, cfg, db, claimEnvironment(webStatus(true)), "-ttl", "30m")
	require.NoError(t, err)
	newer := claimTokenPattern.FindString(again)
	require.NotEqual(t, token, newer)
	assert.Contains(t, again, "within 30 minutes")
	assert.Contains(t, again, "The link expires at 2026-10-13 08:30 UTC.")
	claimer := &adminclaim.Claimer{DB: db, Cost: bcrypt.MinCost, Now: func() time.Time { return time.Date(2026, 10, 13, 8, 1, 0, 0, time.UTC) }}
	_, err = claimer.Claim(context.Background(), adminclaim.Request{Token: token, Email: "owner@example.com", Password: "secret-1"})
	require.ErrorIs(t, err, adminclaim.ErrInvalidToken)
	_, err = claimer.Claim(context.Background(), adminclaim.Request{Token: newer, Email: "owner@example.com", Password: "secret-1"})
	require.NoError(t, err)
}

func TestAdminClaimLinkAddsTheMachineAddressUntilTheCertificateIsTrusted(t *testing.T) {
	db := claimCommandDB(t)
	cfg := claimConfig(t, config.TLSModeACME, "panel.example.com")
	addresses := []string{"10.0.0.5", "100.64.1.2", "2001:db8::7", "203.0.113.10"}
	for name, status := range map[string]*panelcert.Status{"untrusted": webStatus(false), "no status yet": nil} {
		t.Run(name, func(t *testing.T) {
			out, err := runClaimLink(t, cfg, db, claimEnvironment(status, addresses...))
			require.NoError(t, err)
			token := claimTokenPattern.FindString(out)
			assert.Contains(t, out, "\n  https://panel.example.com/claim#"+token+"\n")
			assert.Contains(t, out, "not publicly trusted yet")
			assert.Contains(t, out, "\n  https://203.0.113.10/claim#"+token+"\n", "the first public IPv4 address")
		})
	}
	// Only IPv6 is public: it is bracketed.
	out, err := runClaimLink(t, cfg, db, claimEnvironment(webStatus(false), "192.168.1.4", "2001:db8::7"))
	require.NoError(t, err)
	assert.Contains(t, out, "https://[2001:db8::7]/claim#")
	// No public address known: just the domain link.
	out, err = runClaimLink(t, cfg, db, claimEnvironment(webStatus(false), "192.168.1.4"))
	require.NoError(t, err)
	assert.NotContains(t, out, "not publicly trusted")
	assert.Equal(t, 1, strings.Count(out, "/claim#"))
}

func TestAdminClaimLinkBehindAReverseProxy(t *testing.T) {
	db := claimCommandDB(t)
	// A domain behind the proxy: its https address, no fallback (Control
	// serves no certificate for the web).
	out, err := runClaimLink(t, claimConfig(t, config.TLSModeExternal, "panel.example.com"), db, claimEnvironment(nil, "203.0.113.10"))
	require.NoError(t, err)
	token := claimTokenPattern.FindString(out)
	assert.Contains(t, out, "\n  https://panel.example.com/claim#"+token+"\n")
	assert.Equal(t, 1, strings.Count(out, "/claim#"))

	// No domain (the installer's file): the path, and the local listener.
	out, err = runClaimLink(t, claimConfig(t, "", ""), db, claimEnvironment(nil, "203.0.113.10"))
	require.NoError(t, err)
	token = claimTokenPattern.FindString(out)
	assert.Contains(t, out, "\n  <the panel's address>/claim#"+token+"\n")
	assert.Contains(t, out, "domain is not set")
	assert.Contains(t, out, "\n  http://127.0.0.1:8080/claim#"+token+"\n")
}

func TestAdminClaimLinkRefusesOnceThereIsAnAdministrator(t *testing.T) {
	db := claimCommandDB(t)
	require.NoError(t, db.Create(&model.User{Email: "owner@example.com", Password: "x", UUID: "u-1", Token: "t-1", IsAdmin: 1}).Error)
	out, err := runClaimLink(t, claimConfig(t, config.TLSModeACME, "panel.example.com"), db, claimEnvironment(webStatus(true)))
	require.ErrorIs(t, err, adminclaim.ErrAdministratorExists)
	assert.Empty(t, out, "nothing on stdout")
	assert.Contains(t, err.Error(), "Sign in at https://panel.example.com/login")
	assert.Equal(t, 1, adminCommandExitCode(err))
	var entries int64
	require.NoError(t, db.Model(&migrations.MetaEntry{}).Where("key = ?", adminclaim.MetaKey).Count(&entries).Error)
	assert.Zero(t, entries)
}

func TestAdminClaimLinkArguments(t *testing.T) {
	db := claimCommandDB(t)
	cfg := claimConfig(t, config.TLSModeACME, "panel.example.com")
	for _, arguments := range [][]string{nil, {"claim"}, {"claim-link", "extra"}, {"claim-link", "-ttl"}, {"claim-link", "-ttl", "soon"}, {"claim-link", "-unknown"}} {
		var out bytes.Buffer
		err := runAdminClaimCommandWith(context.Background(), cfg, db, arguments, &out, claimEnvironment(nil))
		require.Error(t, err, arguments)
		assert.Contains(t, err.Error(), "anix-control admin claim-link [-ttl 24h]", arguments)
		assert.Equal(t, 2, adminCommandExitCode(err), arguments)
		assert.Empty(t, out.String())
	}
	for _, ttl := range []string{"0s", "30s", "25h", "-1h"} {
		out, err := runClaimLink(t, cfg, db, claimEnvironment(nil), "-ttl", ttl)
		require.ErrorIs(t, err, adminclaim.ErrInvalidTTL, ttl)
		assert.Equal(t, 2, adminCommandExitCode(err))
		assert.Empty(t, out)
	}
}

func TestDescribeTTL(t *testing.T) {
	for ttl, want := range map[time.Duration]string{
		24 * time.Hour: "24 hours", time.Hour: "1 hour", time.Minute: "1 minute", 90 * time.Minute: "90 minutes", 90 * time.Second: "1m30s",
	} {
		assert.Equal(t, want, describeTTL(ttl))
	}
}

// A start without an administrator logs one line that says how to claim
// the panel, never a token, and nothing once there is an administrator.
func TestStartupNoticeWithoutAnAdministrator(t *testing.T) {
	db := claimCommandDB(t)
	issued, err := adminclaim.Issue(context.Background(), db, adminclaim.DefaultTTL, time.Now())
	require.NoError(t, err)
	var lines []string
	logf := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	noteMissingAdministrator(context.Background(), db, logf)
	require.Equal(t, []string{"No administrator yet. Run `sudo anix-control admin claim-link` to get the claim link."}, lines)
	assert.NotContains(t, strings.Join(lines, "\n"), issued.Token)
	assert.NotContains(t, strings.Join(lines, "\n"), "anixclaim_")

	require.NoError(t, db.Create(&model.User{Email: "owner@example.com", Password: "x", UUID: "u-1", Token: "t-1", IsAdmin: 1}).Error)
	lines = nil
	noteMissingAdministrator(context.Background(), db, logf)
	assert.Empty(t, lines)
}

// The start never creates an administrator any more, whatever the
// environment says.
func TestBootstrapCreatesNoAdministrator(t *testing.T) {
	bootstrapDatabases(t, func(t *testing.T, cfg *config.Config) {
		savedDir := forwardLegacyDataDir
		forwardLegacyDataDir = t.TempDir()
		t.Cleanup(func() { forwardLegacyDataDir = savedDir })
		require.NoError(t, bootstrapDatabase(context.Background(), cfg))
		var users int64
		require.NoError(t, database.Get().Model(&model.User{}).Count(&users).Error)
		assert.Zero(t, users)
	})
}

// The command as an operator runs it: the real run() in a child process,
// with the link on stdout only, the logs on stderr without the token, and
// the exit statuses.
func TestAdminClaimLinkProcess(t *testing.T) {
	if os.Getenv("ANIX_CLAIM_LINK_CHILD") == "1" {
		os.Args = append([]string{"anix-control"}, strings.Fields(os.Getenv("ANIX_CLAIM_LINK_ARGS"))...)
		os.Exit(run())
	}
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte("domain: panel.example.com\n"), 0o600))
	dataDir := filepath.Join(dir, "data")
	require.NoError(t, os.Mkdir(dataDir, 0o700))
	databaseFile := filepath.Join(dir, "control.db")

	child := func(arguments string) (string, string, int) {
		command := exec.Command(os.Args[0], "-test.run=^TestAdminClaimLinkProcess$", "-test.count=1")
		command.Env = append(os.Environ(),
			"ANIX_CLAIM_LINK_CHILD=1", "ANIX_CLAIM_LINK_ARGS="+arguments,
			"ANIX_CONTROL_CONFIG="+configFile, "ANIX_CONTROL_DATA_DIR="+dataDir,
			"ANIX_CONTROL_DATABASE_DRIVER=sqlite", "ANIX_CONTROL_DATABASE_DATABASE="+databaseFile,
			"ANIX_CONTROL_LOG_LEVEL=debug",
		)
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		code := 0
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		} else {
			require.NoError(t, err)
		}
		return stdout.String(), stderr.String(), code
	}

	stdout, stderr, code := child("admin claim-link")
	require.Equal(t, 0, code, "stdout:\n%s\nstderr:\n%s", stdout, stderr)
	token := claimTokenPattern.FindString(stdout)
	require.NotEmpty(t, token, stdout)
	assert.Contains(t, stdout, "https://panel.example.com/claim#"+token)
	assert.NotContains(t, stderr, token, "the token is in the logs")
	assert.NotContains(t, stderr, "anixclaim_")
	assert.NotContains(t, stdout, "AnixOps Control v", "the banner goes to stderr")

	// Claim it, then the command refuses with status 1.
	db, err := database.Open(&config.DatabaseConfig{Driver: "sqlite", Database: databaseFile, LogLevel: "silent"})
	require.NoError(t, err)
	_, err = (&adminclaim.Claimer{DB: db, Cost: bcrypt.MinCost}).Claim(context.Background(),
		adminclaim.Request{Token: token, Email: "owner@example.com", Password: "secret-1"})
	require.NoError(t, err)
	require.NoError(t, database.CloseHandle(db))

	stdout, stderr, code = child("admin claim-link")
	assert.Equal(t, 1, code, stderr)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "the panel already has an administrator")

	stdout, stderr, code = child("admin claim-link -ttl 48h")
	assert.Equal(t, 2, code, stderr)
	assert.Empty(t, stdout)
	_, stderr, code = child("admin")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "anix-control admin claim-link")
}
