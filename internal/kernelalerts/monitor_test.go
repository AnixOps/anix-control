package kernelalerts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newMonitor(db *gorm.DB, now *clock, notifier Notifier) *Monitor {
	return &Monitor{DB: db, Settings: defaultSettings(), Notifier: notifier, Now: now.Now}
}

func rows(t *testing.T, db *gorm.DB) []model.KernelAlert {
	t.Helper()
	var found []model.KernelAlert
	require.NoError(t, db.Order("id").Find(&found).Error)
	return found
}

// Certificates are scanned against the monitor's clock, so the fixtures'
// issuance times are relative to start; the clock starts there.
func TestMonitorNotifiesOnceAndThrottles(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		clk := &clock{now: start}
		recorded := &recorder{}
		monitor := newMonitor(db, clk, recorded)
		node := addProxyNode(t, db, "edge", model.NodeStatusOnline)
		// 28 hours is the window; 24 hours are left: a warning.
		addAgentCert(t, db, "s1", agentcontrol.NodeKindProxy, node.ID, week, week-hours(24))

		summary, err := monitor.RunOnce(t.Context())
		require.NoError(t, err)
		assert.Equal(t, ScanSummary{Active: 1, Opened: 1, Notified: 1}, summary)
		require.Equal(t, 1, recorded.count())
		assert.Contains(t, recorded.last().Title, "1 need attention")
		assert.Contains(t, recorded.last().Content, "warning: The Agent certificate of node proxy-"+itoa(node.ID)+" (edge)")

		// Nothing changed: no second message, however often it scans.
		for range 3 {
			clk.Advance(15 * time.Minute)
			summary, err = monitor.RunOnce(t.Context())
			require.NoError(t, err)
			assert.Equal(t, 1, summary.Active)
			assert.Zero(t, summary.Notified)
		}
		assert.Equal(t, 1, recorded.count())
		stored := rows(t, db)
		require.Len(t, stored, 1)
		assert.Equal(t, 1, stored[0].NotifyCount)
		assert.Equal(t, KindAgentCertificate+"/proxy-"+itoa(node.ID), stored[0].Key)

		// The severity grows to critical (7 hours left): told at once, not
		// at the next interval.
		clk.Advance(hours(17))
		summary, err = monitor.RunOnce(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 1, summary.Notified)
		require.Equal(t, 2, recorded.count())
		assert.Contains(t, recorded.last().Title, "(1 critical)")

		// A critical alert repeats after a quarter of the interval (6h), not
		// before.
		clk.Advance(hours(5))
		_, err = monitor.RunOnce(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 2, recorded.count())
		clk.Advance(hours(1))
		_, err = monitor.RunOnce(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 3, recorded.count())
		assert.EqualValues(t, 3, rows(t, db)[0].NotifyCount)
	})
}

func TestMonitorWarningsRepeatAfterTheInterval(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		clk := &clock{now: start}
		recorded := &recorder{}
		monitor := newMonitor(db, clk, recorded)
		// A CA 50 days out stays a warning for days.
		require.NoError(t, db.Create(&model.ServiceCA{Cluster: "prod", State: model.ServiceCAStateCurrent, KeyID: "ca1", CertificatePEM: "pem", SealedKey: "sealed", NotBefore: start, NotAfter: start.Add(50 * 24 * time.Hour), CreatedAt: start}).Error)
		_, err := monitor.RunOnce(t.Context())
		require.NoError(t, err)
		require.Equal(t, 1, recorded.count())
		clk.Advance(hours(23))
		_, err = monitor.RunOnce(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 1, recorded.count())
		clk.Advance(hours(1))
		_, err = monitor.RunOnce(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 2, recorded.count(), "a warning repeats after renotify_interval")
	})
}

func TestMonitorResolvesWhenRenewedAndReopensWithoutFlapping(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		clk := &clock{now: start}
		recorded := &recorder{}
		monitor := newMonitor(db, clk, recorded)
		node := addProxyNode(t, db, "edge", model.NodeStatusOnline)
		addAgentCert(t, db, "s1", agentcontrol.NodeKindProxy, node.ID, week, week-hours(24))
		_, err := monitor.RunOnce(t.Context())
		require.NoError(t, err)
		require.Equal(t, 1, recorded.count())

		// The Agent renews: the alert resolves, nobody is messaged.
		addAgentCert(t, db, "s2", agentcontrol.NodeKindProxy, node.ID, week, 0)
		clk.Advance(hours(1))
		summary, err := monitor.RunOnce(t.Context())
		require.NoError(t, err)
		assert.Equal(t, ScanSummary{Resolved: 1}, summary)
		stored := rows(t, db)
		require.Len(t, stored, 1)
		require.NotNil(t, stored[0].ResolvedAt)
		assert.Equal(t, 1, recorded.count())

		// It comes back soon after (the new certificate is revoked and the
		// old one is the only one left): reopened, but not announced again
		// within the interval.
		require.NoError(t, db.Model(&model.AgentCertificate{}).Where("serial = ?", "s2").Update("revoked_at", clk.Now()).Error)
		clk.Advance(hours(1))
		summary, err = monitor.RunOnce(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 1, summary.Opened)
		assert.Zero(t, summary.Notified)
		stored = rows(t, db)
		require.Len(t, stored, 1, "one row per key")
		assert.Nil(t, stored[0].ResolvedAt)
		assert.Equal(t, 1, recorded.count())
	})
}

func TestMonitorDoesNotResolveWhatItCouldNotCheck(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		clk := &clock{now: start}
		monitor := newMonitor(db, clk, nil)
		require.NoError(t, db.Create(&model.KernelAlert{
			Key: KindAgentCertificate + "/proxy-1", Kind: KindAgentCertificate, Severity: model.KernelAlertWarning, SubjectKind: SubjectNode, Subject: "proxy-1",
			Message: "m", FirstSeenAt: start, LastSeenAt: start,
		}).Error)
		require.NoError(t, db.Create(&model.KernelAlert{
			Key: KindCAExpiring + "/service_ca:x", Kind: KindCAExpiring, Severity: model.KernelAlertWarning, SubjectKind: SubjectCA, Subject: "service_ca:x",
			Message: "m", FirstSeenAt: start, LastSeenAt: start,
		}).Error)
		var summary ScanSummary
		_, err := monitor.reconcile(t.Context(), start, ScanResult{Unevaluated: map[string]bool{KindAgentCertificate: true}}, &summary)
		require.NoError(t, err)
		assert.Equal(t, 1, summary.Resolved)
		assert.Equal(t, 1, summary.Active, "the alert whose check failed stays open")
		for _, row := range rows(t, db) {
			assert.Equal(t, row.Kind != KindAgentCertificate, row.ResolvedAt != nil, row.Kind)
		}
	})
}

func TestMonitorRetriesAfterAFailedNotification(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		clk := &clock{now: start}
		recorded := &recorder{err: errors.New("no admin reachable")}
		monitor := newMonitor(db, clk, recorded)
		node := addProxyNode(t, db, "edge", model.NodeStatusOnline)
		addAgentCert(t, db, "s1", agentcontrol.NodeKindProxy, node.ID, week, week-hours(24))
		_, err := monitor.RunOnce(t.Context())
		require.ErrorContains(t, err, "notify administrators")
		stored := rows(t, db)
		require.Len(t, stored, 1, "the alert is recorded even when nobody could be told")
		assert.Nil(t, stored[0].LastNotifiedAt)
		assert.Zero(t, stored[0].NotifyCount)

		recorded.err = nil
		clk.Advance(15 * time.Minute)
		summary, err := monitor.RunOnce(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 1, summary.Notified)
		assert.Equal(t, 1, recorded.count())
	})
}

func TestPhaseAlertsRepeatWeekly(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		clk := &clock{now: start}
		recorded := &recorder{}
		monitor := newMonitor(db, clk, recorded)
		require.NoError(t, db.Create(&model.NodeSecretSplit{Table: "v2_node", Phase: "dual_write", UpdatedAt: start.Add(-hours(100))}).Error)
		_, err := monitor.RunOnce(t.Context())
		require.NoError(t, err)
		require.Equal(t, 1, recorded.count())
		clk.Advance(hours(24 * 6))
		_, err = monitor.RunOnce(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 1, recorded.count())
		clk.Advance(hours(24))
		_, err = monitor.RunOnce(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 2, recorded.count())
		// Finalizing the table clears the alert.
		finalized := clk.Now()
		require.NoError(t, db.Model(&model.NodeSecretSplit{}).Where("table_name = ?", "v2_node").
			Updates(map[string]any{"phase": "finalized", "finalized_at": finalized, "updated_at": finalized}).Error)
		summary, err := monitor.RunOnce(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 1, summary.Resolved)
	})
}

func TestResolvedAlertsAreDroppedAfterAMonth(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		clk := &clock{now: start}
		monitor := newMonitor(db, clk, nil)
		old, recent := start.Add(-31*24*time.Hour), start.Add(-2*24*time.Hour)
		for key, resolved := range map[string]time.Time{"a/old": old, "a/recent": recent} {
			require.NoError(t, db.Create(&model.KernelAlert{
				Key: key, Kind: "a", Severity: model.KernelAlertWarning, SubjectKind: "x", Subject: key, Message: "m",
				FirstSeenAt: old, LastSeenAt: old, ResolvedAt: &resolved,
			}).Error)
		}
		_, err := monitor.RunOnce(t.Context())
		require.NoError(t, err)
		stored := rows(t, db)
		require.Len(t, stored, 1)
		assert.Equal(t, "a/recent", stored[0].Key)
	})
}

func TestDigestIsBoundedOrderedAndSecretFree(t *testing.T) {
	var due []model.KernelAlert
	for i := range 30 {
		severity := model.KernelAlertWarning
		if i == 29 {
			severity = model.KernelAlertCritical
		}
		due = append(due, model.KernelAlert{
			Key: fmt.Sprintf("k/%02d", i), Kind: "k", Severity: severity, Subject: fmt.Sprintf("%02d", i),
			Message: fmt.Sprintf("Message %02d", i),
		})
	}
	built := digest(due)
	lines := strings.Split(built.Content, "\n")
	require.Len(t, lines, digestMaxAlerts+1)
	assert.Equal(t, "critical: Message 29", lines[0], "critical first")
	assert.Contains(t, lines[len(lines)-1], "... and 10 more")
	assert.Equal(t, "AnixOps alert: 30 need attention (1 critical)", built.Title)
	assert.Len(t, built.Alerts, 30)

	long := model.KernelAlert{Key: "k/long", Kind: "k", Severity: model.KernelAlertWarning, Message: strings.Repeat("x", 500)}
	bounded := digest2(long, 20)
	assert.LessOrEqual(t, len(bounded.Content), digestMaxChars+200)
}

func digest2(alert model.KernelAlert, count int) Notification {
	due := make([]model.KernelAlert, count)
	for i := range due {
		due[i] = alert
		due[i].Key = fmt.Sprintf("%s%d", alert.Key, i)
	}
	return digest(due)
}

func TestAlertsHoldNoSecrets(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		clk := &clock{now: start}
		recorded := &recorder{}
		monitor := newMonitor(db, clk, recorded)
		node := addProxyNode(t, db, "edge", model.NodeStatusOnline)
		issued := start.Add(-week + hours(10))
		require.NoError(t, db.Create(&model.AgentCertificate{
			Serial: "SERIAL-0123456789ABCDEF", NodeKind: agentcontrol.NodeKindProxy, NodeID: node.ID, Cluster: "prod",
			EnrollmentID: "enrollment-uuid-secretish", IssuerKeyID: "issuer-key-id", NotAfter: issued.Add(week), CreatedAt: issued,
		}).Error)
		require.NoError(t, db.Create(&model.ServiceCA{Cluster: "prod", State: model.ServiceCAStateCurrent, KeyID: "ca-key-id", CertificatePEM: "-----BEGIN CERTIFICATE-----pem", SealedKey: "SEALED-PRIVATE-KEY-MATERIAL", NotBefore: start, NotAfter: start.Add(10 * 24 * time.Hour), CreatedAt: start}).Error)
		_, err := monitor.RunOnce(t.Context())
		require.NoError(t, err)
		page, err := List(t.Context(), db, Query{Status: StatusAll})
		require.NoError(t, err)
		require.Len(t, page.Alerts, 2)
		everything := toJSON(page) + recorded.last().Title + recorded.last().Content
		for _, forbidden := range []string{"SERIAL-0123", "enrollment-uuid", "issuer-key-id", "SEALED-PRIVATE", "BEGIN CERTIFICATE", "PRIVATE"} {
			assert.NotContains(t, everything, forbidden)
		}
		for _, row := range rows(t, db) {
			assert.NotContains(t, row.Message+row.Detail, "SERIAL-0123")
			assert.NotContains(t, row.Message+row.Detail, "SEALED")
		}
	})
}

func TestRunStopsWithItsContextAndIsOffWhenDisabled(t *testing.T) {
	db := openSQLite(t)
	recorded := &recorder{}
	monitor := &Monitor{DB: db, Settings: defaultSettings(), Notifier: recorded}
	monitor.Settings.CheckInterval = time.Minute
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		monitor.Run(ctx)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the monitor did not stop with its context")
	}

	off := &Monitor{DB: db, Settings: config.AlertSettings{}, Notifier: recorded}
	finished := make(chan struct{})
	go func() {
		off.Run(t.Context())
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("a disabled monitor must return at once")
	}
	(*Monitor)(nil).Run(t.Context())
}

func TestMonitorWithoutTablesDoesNothing(t *testing.T) {
	bare := openBare(t)
	monitor := &Monitor{DB: bare, Settings: defaultSettings(), Notifier: &recorder{}}
	summary, err := monitor.RunOnce(t.Context())
	require.NoError(t, err)
	assert.Equal(t, ScanSummary{}, summary)
}

func TestScanWithoutCertificateTables(t *testing.T) {
	bare := openBare(t)
	require.NoError(t, bare.AutoMigrate(&model.KernelAlert{}))
	summary, err := (&Monitor{DB: bare, Settings: defaultSettings()}).RunOnce(t.Context())
	require.NoError(t, err)
	assert.Equal(t, ScanSummary{}, summary)
}

func TestScanStopsOnACancelledContext(t *testing.T) {
	db := openSQLite(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := (&Scanner{DB: db, Settings: defaultSettings()}).Scan(ctx)
	require.ErrorIs(t, err, context.Canceled)
}
