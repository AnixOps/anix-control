// Package kernelalerts raises the kernel's operational alerts: leaf
// certificates (Agent, forward link, module) that were not renewed in time,
// CAs close to their end, and phased processes (the node credential split,
// the identity cutover) left in a non-final phase. A Monitor scans the
// certificate stores and the phase tables read-only, keeps one row per
// alert in v4_kernel_alert, and sends the administrators one digest of the
// alerts that are new, worse or due again. GET /api/v4/kernel/alerts lists
// the rows. Nothing here changes a certificate or a phase.
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
	// KindNodeSecretsSplitStalled: a node credential split table sits in
	// dual_write or dual_read and was not touched for phase_stuck_after.
	KindNodeSecretsSplitStalled = "node_secrets_split_stalled"
	// KindNodeSecretsFinalizeInterrupted: a table's finalize started and
	// did not complete.
	KindNodeSecretsFinalizeInterrupted = "node_secrets_finalize_interrupted"
	// KindIdentityImportStalled: the identity authority sits in importing
	// without progress for phase_stuck_after.
	KindIdentityImportStalled = "identity_import_stalled"
	// KindIdentityCutoverPending: identity owns logins since the latest
	// cutover, but legacy credentials were not finalized for
	// phase_stuck_after.
	KindIdentityCutoverPending = "identity_cutover_not_finalized"
)

// Subject kinds.
const (
	SubjectNode        = "node"
	SubjectCA          = "ca"
	SubjectModule      = "module"
	SubjectNodeSecrets = "node_secrets"
	SubjectIdentity    = "identity"
)

// finalizeInterruptedAfter is how long a started finalize may take before
// it counts as interrupted. Its start time is the only write to the table's
// row while it runs, so the row's updated_at is the time it started.
const finalizeInterruptedAfter = time.Hour

// phaseKinds are the alert kinds about phased processes: operator-paced
// rollouts that are reminded of weekly rather than daily.
var phaseKinds = map[string]bool{
	KindNodeSecretsSplitStalled:        true,
	KindNodeSecretsFinalizeInterrupted: true,
	KindIdentityImportStalled:          true,
	KindIdentityCutoverPending:         true,
}

const phaseRenotifyFactor = 7

// resolvedRetention is how long a resolved alert stays listed.
const resolvedRetention = 30 * 24 * time.Hour

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
