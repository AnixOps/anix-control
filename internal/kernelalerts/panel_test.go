package kernelalerts

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/panelcert"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const day = 24 * time.Hour

// panelCertificate is a served certificate of lifetime that ends in left.
func panelCertificate(name, source string, lifetime, left time.Duration) panelcert.CertificateStatus {
	notAfter := start.Add(left)
	notBefore := notAfter.Add(-lifetime)
	return panelcert.CertificateStatus{
		Name: name, Roles: []string{panelcert.RoleWeb, panelcert.RoleGRPC}, Source: source, State: panelcert.StateValid,
		Serving: panelcert.ServingACME, PubliclyTrusted: true, NotBefore: &notBefore, NotAfter: &notAfter, CertFile: "/etc/anix/" + name + ".crt",
	}
}

func scanPanel(t *testing.T, db *gorm.DB, certificates ...panelcert.CertificateStatus) map[string]Finding {
	t.Helper()
	scanner := &Scanner{DB: db, Settings: defaultSettings(), Now: func() time.Time { return start },
		PanelCertificates: func(context.Context) (panelcert.Status, error) {
			return panelcert.Status{Mode: "acme", Certificates: certificates}, nil
		}}
	result, err := scanner.Scan(t.Context())
	require.NoError(t, err)
	require.Empty(t, result.Unevaluated)
	return kinds(result)
}

func TestPanelCertificateExpiring(t *testing.T) {
	db := openSQLite(t)
	healthy := panelCertificate("panel.example.com", panelcert.SourceACME, 90*day, 40*day)
	missedRenewal := panelCertificate("grpc.example.com", panelcert.SourceACME, 90*day, 10*day)
	urgent := panelCertificate("urgent.example.com", panelcert.SourceACME, 90*day, 2*day)
	file := panelCertificate("files.example.com", panelcert.SourceFiles, 365*day, 13*day)
	short := panelCertificate("short.example.com", panelcert.SourceACME, 6*day, 2*day) // a sixth: one day
	found := scanPanel(t, db, healthy, missedRenewal, urgent, file, short)
	require.Len(t, found, 3, "%v", found)

	warning := found[KindPanelCertificateExpiring+"/grpc.example.com"]
	assert.Equal(t, model.KernelAlertWarning, warning.Severity, "ten days left of a 14-day window")
	assert.Equal(t, SubjectPanelCertificate, warning.SubjectKind)
	assert.Contains(t, warning.Message, "renews it by itself")
	assert.EqualValues(t, 14*24, warning.Detail["window_hours"])
	require.NotNil(t, warning.ExpiresAt)

	assert.Equal(t, model.KernelAlertCritical, found[KindPanelCertificateExpiring+"/urgent.example.com"].Severity)
	replace := found[KindPanelCertificateExpiring+"/files.example.com"]
	assert.Contains(t, replace.Message, "Replace /etc/anix/files.example.com.crt")
}

func TestPanelCertificateRenewalFailing(t *testing.T) {
	db := openSQLite(t)
	// No usable certificate: the self-signed fallback is served.
	failed := panelcert.CertificateStatus{
		Name: "panel.example.com", Roles: []string{panelcert.RoleWeb, panelcert.RoleGRPC}, Source: panelcert.SourceACME,
		State: panelcert.StateFailed, Serving: panelcert.ServingSelfSigned,
		LastError: "the CA refused the certificate: http-01: Timeout during connect", Problem: panelcert.ProblemUnreachable,
		Fix: "Open ports 80 and 443.",
	}
	// A renewal that fails while the old certificate lasts.
	renewing := panelCertificate("grpc.example.com", panelcert.SourceACME, 90*day, 20*day)
	renewing.RenewalFailing = true
	renewing.LastError = "the CA refused the certificate: dns: NXDOMAIN"
	pending := panelcert.CertificateStatus{Name: "new.example.com", Source: panelcert.SourceACME, State: panelcert.StatePending, Serving: panelcert.ServingSelfSigned}
	found := scanPanel(t, db, failed, renewing, pending)
	require.Len(t, found, 2, "a pending first request is not an alert")

	critical := found[KindPanelCertificateRenewalFailing+"/panel.example.com"]
	assert.Equal(t, model.KernelAlertCritical, critical.Severity)
	assert.Contains(t, critical.Message, "self-signed")
	assert.Contains(t, critical.Message, "Timeout during connect")
	assert.Contains(t, critical.Message, "Fix: Open ports 80 and 443.")
	assert.Equal(t, panelcert.ProblemUnreachable, critical.Detail["problem"])
	assert.Nil(t, critical.ExpiresAt)

	warning := found[KindPanelCertificateRenewalFailing+"/grpc.example.com"]
	assert.Equal(t, model.KernelAlertWarning, warning.Severity)
	assert.Contains(t, warning.Message, "NXDOMAIN")
	assert.NotNil(t, warning.ExpiresAt)
}

// Without a status source the scan reads what the running Control stored;
// a database without the table or without a status has nothing to check.
func TestPanelCertificatesFromTheStoredStatus(t *testing.T) {
	db := openSQLite(t)
	result, err := (&Scanner{DB: db, Settings: defaultSettings(), Now: func() time.Time { return start }}).Scan(t.Context())
	require.NoError(t, err)
	require.Empty(t, result.Unevaluated)
	require.Empty(t, result.Findings)

	// Control creates v5_acme_cache with the migration 0003_acme_cache.
	require.NoError(t, db.AutoMigrate(&model.ACMECacheEntry{}))
	manager, err := panelcert.New(panelcert.Options{
		DB: db, Mode: "files", Domain: "panel.example.com",
		Certificates: []panelcert.Spec{{Name: "panel.example.com", Roles: []string{panelcert.RoleWeb}, Source: panelcert.SourceFiles,
			CertFile: filepath.Join(t.TempDir(), "missing.crt"), KeyFile: filepath.Join(t.TempDir(), "missing.key")}},
		Logf: t.Logf,
	})
	require.NoError(t, err)
	require.NoError(t, manager.Start(context.Background()))
	found := kinds(scan(t, db, defaultSettings(), start))
	failing, ok := found[KindPanelCertificateRenewalFailing+"/panel.example.com"]
	require.True(t, ok, "%v", found)
	assert.Equal(t, model.KernelAlertCritical, failing.Severity)
	assert.Contains(t, failing.Message, "missing.crt")

	// The monitor records it and notifies once.
	notifier := &recorder{}
	monitor := &Monitor{DB: db, Settings: defaultSettings(), Notifier: notifier, Now: func() time.Time { return start }}
	summary, err := monitor.RunOnce(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Opened)
	require.Equal(t, 1, notifier.count())
	assert.Contains(t, notifier.last().Content, "self-signed")
}
