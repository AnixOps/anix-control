package kernelalerts

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// Notification is one digest for the administrators: every alert that is
// new, worse or due again in a scan, in one message.
type Notification struct {
	Title   string
	Content string
	Alerts  []model.KernelAlert
}

// Notifier delivers a digest to the administrators. An error means nothing
// was delivered: the alerts stay due and the next scan tries again.
type Notifier interface {
	Notify(ctx context.Context, notification Notification) error
}

// Digest limits: Telegram refuses messages over 4096 characters.
const (
	digestMaxAlerts = 20
	digestMaxChars  = 3500
)

// ScanSummary is what one scan did.
type ScanSummary struct {
	Active   int
	Opened   int
	Resolved int
	Notified int
}

// Monitor keeps v4_kernel_alert in step with what the Scanner finds and
// notifies the administrators of what needs their attention. Run it in one
// process at a time (the singleton workers' lease).
type Monitor struct {
	DB       *gorm.DB
	Settings config.AlertSettings
	// Notifier may be nil: alerts are then only recorded and listed.
	Notifier Notifier
	// Now defaults to time.Now.
	Now func() time.Time
}

func (m *Monitor) now() time.Time {
	if m.Now != nil {
		return m.Now().UTC()
	}
	return time.Now().UTC()
}

// Run scans every check interval until ctx ends. A disabled monitor, or one
// without a database, returns at once.
func (m *Monitor) Run(ctx context.Context) {
	if m == nil || m.DB == nil || !m.Settings.Enabled {
		return
	}
	interval := m.Settings.CheckInterval
	if interval <= 0 {
		interval = config.DefaultAlertCheckInterval
	}
	for {
		if _, err := m.RunOnce(ctx); err != nil && ctx.Err() == nil {
			log.Printf("Alert scan failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

// RunOnce scans, updates the alert rows and sends the digest of what is due.
func (m *Monitor) RunOnce(ctx context.Context) (ScanSummary, error) {
	var summary ScanSummary
	if !m.DB.WithContext(ctx).Migrator().HasTable(&model.KernelAlert{}) {
		return summary, nil
	}
	scanner := &Scanner{DB: m.DB, Settings: m.Settings, Now: m.Now}
	result, err := scanner.Scan(ctx)
	if err != nil {
		return summary, err
	}
	now := m.now()
	due, err := m.reconcile(ctx, now, result, &summary)
	if err != nil {
		return summary, err
	}
	if len(due) == 0 || m.Notifier == nil {
		return summary, nil
	}
	if err := m.Notifier.Notify(ctx, digest(due)); err != nil {
		return summary, fmt.Errorf("notify administrators: %w", err)
	}
	if err := m.markNotified(ctx, due, now); err != nil {
		return summary, err
	}
	summary.Notified = len(due)
	return summary, nil
}

// reconcile writes the findings to the alert table: new findings open
// alerts, known ones are refreshed (and reopened when they had resolved),
// open alerts without a finding resolve unless their kind was not
// evaluated, and old resolved alerts are dropped. It returns the alerts
// that are due for a notification.
func (m *Monitor) reconcile(ctx context.Context, now time.Time, result ScanResult, summary *ScanSummary) ([]model.KernelAlert, error) {
	var due []model.KernelAlert
	err := m.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		due = due[:0]
		keys := make([]string, 0, len(result.Findings))
		found := make(map[string]Finding, len(result.Findings))
		for _, finding := range result.Findings {
			found[finding.Key()] = finding
			keys = append(keys, finding.Key())
		}
		existing := map[string]model.KernelAlert{}
		var open []model.KernelAlert
		if err := tx.Where("resolved_at IS NULL").Find(&open).Error; err != nil {
			return err
		}
		for _, row := range open {
			existing[row.Key] = row
		}
		for start := 0; start < len(keys); start += 200 {
			var rows []model.KernelAlert
			if err := tx.Where("alert_key IN ?", keys[start:min(start+200, len(keys))]).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				existing[row.Key] = row
			}
		}

		sort.Strings(keys)
		for _, key := range keys {
			finding := found[key]
			detail, err := json.Marshal(finding.Detail)
			if err != nil {
				return err
			}
			row, known := existing[key]
			reopened := known && row.ResolvedAt != nil
			switch {
			case !known:
				row = model.KernelAlert{Key: key, Kind: finding.Kind, SubjectKind: finding.SubjectKind, Subject: finding.Subject, FirstSeenAt: now}
				summary.Opened++
			case reopened:
				row.FirstSeenAt = now
				summary.Opened++
			}
			row.Severity, row.Message, row.Detail, row.ExpiresAt = finding.Severity, finding.Message, string(detail), finding.ExpiresAt
			row.LastSeenAt, row.ResolvedAt = now, nil
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
			summary.Active++
			if m.due(row, now) {
				due = append(due, row)
			}
		}
		for _, row := range open {
			if _, still := found[row.Key]; still || result.Unevaluated[row.Kind] {
				if !still {
					summary.Active++
				}
				continue
			}
			if err := tx.Model(&model.KernelAlert{}).Where("id = ?", row.ID).Updates(map[string]any{"resolved_at": now, "updated_at": now}).Error; err != nil {
				return err
			}
			summary.Resolved++
		}
		return tx.Where("resolved_at IS NOT NULL AND resolved_at < ?", now.Add(-resolvedRetention)).Delete(&model.KernelAlert{}).Error
	})
	if err != nil {
		return nil, err
	}
	return due, nil
}

// renotifyAfter is how long an alert waits between notifications: the
// configured interval; a quarter of it when critical; seven times it for
// phase alerts, which are operator-paced rollouts.
func (m *Monitor) renotifyAfter(row model.KernelAlert) time.Duration {
	interval := m.Settings.RenotifyInterval
	if interval <= 0 {
		interval = config.DefaultAlertRenotifyInterval
	}
	if phaseKinds[row.Kind] {
		return interval * phaseRenotifyFactor
	}
	if row.Severity == model.KernelAlertCritical {
		return interval / 4
	}
	return interval
}

// due reports whether an active alert is to be notified now: it never was,
// its severity grew since the last notification, or the re-notification
// interval passed. A reopened alert keeps its last notification time, so a
// finding that flaps is not announced again within the interval.
func (m *Monitor) due(row model.KernelAlert, now time.Time) bool {
	if row.LastNotifiedAt == nil || severityRank(row.Severity) > severityRank(row.NotifiedSeverity) {
		return true
	}
	return now.Sub(*row.LastNotifiedAt) >= m.renotifyAfter(row)
}

// markNotified records the notification on the alerts of the digest.
func (m *Monitor) markNotified(ctx context.Context, alerts []model.KernelAlert, now time.Time) error {
	return m.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, alert := range alerts {
			if err := tx.Model(&model.KernelAlert{}).Where("id = ?", alert.ID).Updates(map[string]any{
				"last_notified_at": now, "notified_severity": alert.Severity, "notify_count": gorm.Expr("notify_count + 1"), "updated_at": now,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// digest builds the one message for a scan's due alerts, critical first,
// within the limits of a Telegram message. It lists messages, which hold
// no secret.
func digest(due []model.KernelAlert) Notification {
	alerts := append([]model.KernelAlert(nil), due...)
	sort.SliceStable(alerts, func(i, j int) bool {
		if a, b := severityRank(alerts[i].Severity), severityRank(alerts[j].Severity); a != b {
			return a > b
		}
		return alerts[i].Key < alerts[j].Key
	})
	critical := 0
	for _, alert := range alerts {
		if alert.Severity == model.KernelAlertCritical {
			critical++
		}
	}
	title := fmt.Sprintf("AnixOps alert: %d need attention", len(alerts))
	if critical > 0 {
		title = fmt.Sprintf("AnixOps alert: %d need attention (%d critical)", len(alerts), critical)
	}
	var content strings.Builder
	shown := 0
	for _, alert := range alerts {
		line := fmt.Sprintf("%s: %s\n", alert.Severity, alert.Message)
		if shown >= digestMaxAlerts || content.Len()+len(line) > digestMaxChars {
			break
		}
		content.WriteString(line)
		shown++
	}
	if rest := len(alerts) - shown; rest > 0 {
		fmt.Fprintf(&content, "... and %d more: see Alerts in the admin dashboard (GET /api/v4/kernel/alerts).\n", rest)
	}
	return Notification{Title: title, Content: strings.TrimRight(content.String(), "\n"), Alerts: alerts}
}
