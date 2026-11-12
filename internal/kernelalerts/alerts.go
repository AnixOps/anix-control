// Package kernelalerts raises the kernel's operational alerts: leaf
// certificates (Agent, forward link, module) that were not renewed in time,
// CAs close to their end, the panel's own certificates (close to their end,
// or failing to renew), and phased processes (the node credential split,
// the identity cutover) left in a non-final phase. A Monitor scans the
// certificate stores and the phase tables read-only, keeps one row per
// alert in v4_kernel_alert, and sends the administrators one digest of the
// alerts that are new, worse or due again. GET /api/v4/kernel/alerts lists
// the rows. It also watches the backups: a bundle that failed its
// verification, and no verified bundle for too long. Nothing here changes a
// certificate, a phase or a backup.
package kernelalerts

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Alert kinds.
const (
	// KindAgentCertificate: a node's latest Agent client certificate is
	// close to its end (or past it) and was not renewed.
	KindAgentCertificate = "agent_certificate_expiring"
	// KindLinkCertificate: the same for a forward node's link certificate
	// (H28).
	KindLinkCertificate = "link_certificate_expiring"
	// KindModuleCertificate: the same for a module's certificate.
	KindModuleCertificate = "module_certificate_expiring"
	// KindCAExpiring: the current module (Agent, kernel) CA or forward link
	// CA is within ca_expiry_days of its end.
	KindCAExpiring = "ca_expiring"
	// KindBuiltinPackageInvalid: an official package embedded in the Control
	// binary does not verify against plugins.official_public_key (signature,
	// digest, contents), so it was not installed.
	KindBuiltinPackageInvalid = "builtin_package_invalid"
	// KindBuiltinPackageFailed: a built-in package verified, but its
	// installation failed or is not enabled at the built-in version.
	KindBuiltinPackageFailed = "builtin_package_failed"
	// KindPanelCertificateExpiring: a certificate Control serves itself
	// (the panel's, the gRPC listener's) ends within its window: the
	// smaller of leaf_expiry_days (14) and a sixth of its lifetime. An ACME
	// certificate is renewed a third of its lifetime before its end, so
	// this one missed its renewal; files need replacing.
	KindPanelCertificateExpiring = "panel_certificate_expiring"
	// KindPanelCertificateRenewalFailing: Control cannot obtain or renew a
	// certificate it serves, or cannot use its files: critical while it
	// serves the self-signed fallback (no node can connect), warning while
	// the old certificate still lasts.
	KindPanelCertificateRenewalFailing = "panel_certificate_renewal_failing"
	// KindBackupVerifyFailed: the newest backup bundle that was verified
	// failed its verification (internal/backup): it would not restore.
	KindBackupVerifyFailed = "backup_verify_failed"
	// KindBackupStale: no backup bundle that passed its verification is
	// younger than BackupStaleAfter (and the database is older than that).
	KindBackupStale = "backup_stale"
	// KindExitMainlandDestinations: an exit node that refuses mainland-China
	// destinations (internal/exitpolicy) refused some within the last 24
	// hours: mainland traffic reaches it through an entry or a client
	// routing it the wrong way.
	KindExitMainlandDestinations = "exit_mainland_destinations"
	// KindCrossBorderLinkDown: a cross-border transport (kind anx, not
	// trusted) has been down on the devices at its ends for
	// CrossBorderDownAfter: critical while its traffic has no standby
	// channel, warning once it is switched (internal/l2standby).
	KindCrossBorderLinkDown = "cross_border_link_down"
	// KindCrossBorderWideOutage: half or more of at least two cross-border
	// transports are down at once, the sign of a targeted jacket.
	KindCrossBorderWideOutage = "cross_border_wide_outage"
)

// Subject kinds.
const (
	SubjectNode    = "node"
	SubjectCA      = "ca"
	SubjectModule  = "module"
	SubjectPackage = "package"
	// SubjectPanelCertificate: a certificate of the panel, by name.
	SubjectPanelCertificate = "panel_certificate"
	// SubjectBackups: the panel's backups, one subject ("backups").
	SubjectBackups = "backups"
	// SubjectTransport: a transport of the network model, by id ("all"
	// for the wide outage).
	SubjectTransport = "transport"
)

// resolvedRetention is how long a resolved alert stays listed.
const resolvedRetention = 30 * 24 * time.Hour

// BuiltinPackageProblem is a built-in package that needs an administrator,
// as the kernel's check reports it (service.BuiltinPackageProblems).
type BuiltinPackageProblem struct {
	PackageID string
	Version   string
	// Invalid: the embedded package did not verify.
	Invalid   bool
	State     string
	LastError string
	Reason    string
}

// Finding is one alert condition a scan found.
type Finding struct {
	Kind        string
	Severity    string
	SubjectKind string
	Subject     string
	Message     string
	// Detail holds the facts behind the alert; no secret, ever.
	Detail    map[string]any
	ExpiresAt *time.Time
}

// Key is the alert's identity: its kind and subject.
func (f Finding) Key() string { return f.Kind + "/" + f.Subject }

func severityRank(severity string) int {
	switch severity {
	case "critical":
		return 2
	case "warning":
		return 1
	}
	return 0
}

// plain makes a name safe to put in a Telegram Markdown message: the
// characters that open formatting are replaced.
func plain(value string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '_', '*', '`', '[', ']', '\n', '\r':
			return '-'
		}
		return r
	}, strings.TrimSpace(value))
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut]
}
