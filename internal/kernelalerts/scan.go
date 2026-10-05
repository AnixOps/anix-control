package kernelalerts

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"gorm.io/gorm"
)

// Scanner reads the certificate stores and the phase tables and answers
// what is wrong now. It never writes.
type Scanner struct {
	DB       *gorm.DB
	Settings config.AlertSettings
	// Now defaults to time.Now.
	Now func() time.Time
}

// ScanResult is what one scan found.
type ScanResult struct {
	Findings []Finding
	// Unevaluated lists the kinds whose check failed. The alerts of those
	// kinds are left as they are: a failed scan proves nothing is resolved.
	Unevaluated map[string]bool
}

func (s *Scanner) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Scan runs every check. A check that fails is logged and its kinds are
// reported in Unevaluated; the others still run. Only a cancelled context
// fails the scan.
func (s *Scanner) Scan(ctx context.Context) (ScanResult, error) {
	now := s.now()
	result := ScanResult{Unevaluated: map[string]bool{}}
	for _, check := range []struct {
		kinds []string
		run   func(context.Context, time.Time) ([]Finding, error)
	}{
		{[]string{KindAgentCertificate}, s.agentCertificates},
		{[]string{KindLinkCertificate}, s.linkCertificates},
		{[]string{KindModuleCertificate}, s.moduleCertificates},
		{[]string{KindCAExpiring}, s.authorities},
		{[]string{KindNodeSecretsSplitStalled, KindNodeSecretsFinalizeInterrupted}, s.nodeSecretsSplit},
		{[]string{KindIdentityImportStalled, KindIdentityCutoverPending}, s.identityAuthority},
	} {
		findings, err := check.run(ctx, now)
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if err != nil {
			log.Printf("Alert scan of %v failed: %v", check.kinds, err)
			for _, kind := range check.kinds {
				result.Unevaluated[kind] = true
			}
			continue
		}
		result.Findings = append(result.Findings, findings...)
	}
	return result, nil
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func days(n int) time.Duration { return time.Duration(n) * 24 * time.Hour }

// leafWindow is how close to its end an unrenewed leaf certificate has to
// be to alert: the smaller of the configured cap and a sixth of its own
// lifetime. Healthy holders renew at two thirds of the lifetime, so a
// certificate in its last sixth missed its renewal by half the renewal
// window; a fixed 14 days would fire for every fresh 7-day certificate.
func leafWindow(lifetime time.Duration, capDays int) time.Duration {
	limit := days(capDays)
	if lifetime <= 0 {
		return limit
	}
	return min(lifetime/6, limit)
}

// severityFor is critical for an expired certificate or one in the last
// quarter of its window, warning before.
func severityFor(remaining, window time.Duration) string {
	if remaining <= 0 || remaining <= window/4 {
		return model.KernelAlertCritical
	}
	return model.KernelAlertWarning
}

// certRow is the part of a certificate record the scan reads.
type certRow struct {
	NodeKind     string
	NodeID       uint
	PackageID    string
	EnrollmentID string
	NotAfter     time.Time
	CreatedAt    time.Time
}

// latestCerts answers, per holder, the not-revoked certificate that ends
// last, for holders whose latest certificate ends within the leaf cap of
// now (it may already have ended). Holders that hold a certificate beyond
// the cap are healthy and left out. holderColumns identify a holder.
func (s *Scanner) latestCerts(ctx context.Context, record any, holderColumns []string, key func(certRow) string, now time.Time) (map[string]certRow, error) {
	db := s.DB.WithContext(ctx)
	if !db.Migrator().HasTable(record) {
		return nil, nil
	}
	horizon := now.Add(days(s.Settings.LeafExpiryDays))
	columns := append(append([]string(nil), holderColumns...), "not_after", "created_at")
	var near []certRow
	if err := db.Model(record).Select(columns).Where("revoked_at IS NULL AND not_after <= ?", horizon).Scan(&near).Error; err != nil {
		return nil, err
	}
	if len(near) == 0 {
		return nil, nil
	}
	distinct := make([]any, len(holderColumns))
	for i, column := range holderColumns {
		distinct[i] = column
	}
	var beyond []certRow
	if err := db.Model(record).Distinct(distinct...).Where("revoked_at IS NULL AND not_after > ?", horizon).Scan(&beyond).Error; err != nil {
		return nil, err
	}
	healthy := make(map[string]bool, len(beyond))
	for _, row := range beyond {
		healthy[key(row)] = true
	}
	latest := map[string]certRow{}
	for _, row := range near {
		holder := key(row)
		if healthy[holder] {
			continue
		}
		if current, ok := latest[holder]; !ok || row.NotAfter.After(current.NotAfter) {
			latest[holder] = row
		}
	}
	return latest, nil
}

func nodeKey(row certRow) string {
	return row.NodeKind + "-" + strconv.FormatUint(uint64(row.NodeID), 10)
}

func moduleKey(row certRow) string { return row.PackageID + "#" + row.EnrollmentID }

// nodeCertFindings turns the latest certificates of nodes into findings,
// skipping nodes that no longer exist or are disabled: their credentials
// are revoked or revocable, and an expiry there is expected.
func (s *Scanner) nodeCertFindings(ctx context.Context, record any, kind string, now time.Time, describe func(name string, row certRow, expired bool) string) ([]Finding, error) {
	latest, err := s.latestCerts(ctx, record, []string{"node_kind", "node_id"}, nodeKey, now)
	if err != nil || len(latest) == 0 {
		return nil, err
	}
	holders := make([]string, 0, len(latest))
	for holder := range latest {
		holders = append(holders, holder)
	}
	sort.Strings(holders)
	var findings []Finding
	for _, holder := range holders {
		row := latest[holder]
		lifetime := row.NotAfter.Sub(row.CreatedAt)
		window := leafWindow(lifetime, s.Settings.LeafExpiryDays)
		remaining := row.NotAfter.Sub(now)
		if remaining > window {
			continue
		}
		node := agentcontrol.AgentNode{Kind: row.NodeKind, ID: uint32(row.NodeID)} // #nosec G115 -- node ids are stored from uint32 values.
		if err := agentpki.CheckNodeEnabled(ctx, s.DB, node); err != nil {
			if errors.Is(err, agentpki.ErrInvalidNode) || !s.nodeTableExists(ctx, node.Kind) {
				// Gone, disabled, or a node kind whose table does not
				// exist (the v4.1 forward tables once they are dropped).
				continue
			}
			return nil, err
		}
		expired := remaining <= 0
		name := s.nodeName(ctx, node)
		message := describe(name, row, expired)
		expires := row.NotAfter.UTC()
		findings = append(findings, Finding{
			Kind: kind, Severity: severityFor(remaining, window), SubjectKind: SubjectNode, Subject: holder,
			Message: truncate(message, 500), ExpiresAt: &expires,
			Detail: map[string]any{
				"node": holder, "node_name": name, "not_after": stamp(row.NotAfter), "expired": expired,
				"lifetime_hours": int64(lifetime / time.Hour), "window_hours": int64(window / time.Hour),
			},
		})
	}
	return findings, nil
}

func nodeLabel(subject, name string) string {
	if name = plain(name); name != "" {
		return subject + " (" + name + ")"
	}
	return subject
}

// agentCertificates finds Agent client certificates that were not renewed.
func (s *Scanner) agentCertificates(ctx context.Context, now time.Time) ([]Finding, error) {
	return s.nodeCertFindings(ctx, &model.AgentCertificate{}, KindAgentCertificate, now, func(name string, row certRow, expired bool) string {
		label := nodeLabel(nodeKey(row), name)
		if expired {
			return fmt.Sprintf("The Agent certificate of node %s expired at %s and the node is still enabled: its Agent cannot connect any more. Enroll it again with a new enrollment token.", label, stamp(row.NotAfter))
		}
		return fmt.Sprintf("The Agent certificate of node %s ends at %s and the Agent has not renewed it. If the Agent stays offline the node has to enroll again.", label, stamp(row.NotAfter))
	})
}

// linkCertificates finds forward link certificates (H28) that were not
// renewed; they renew with the Agent certificate.
func (s *Scanner) linkCertificates(ctx context.Context, now time.Time) ([]Finding, error) {
	return s.nodeCertFindings(ctx, &model.ForwardLinkCertificate{}, KindLinkCertificate, now, func(name string, row certRow, expired bool) string {
		label := nodeLabel(nodeKey(row), name)
		if expired {
			return fmt.Sprintf("The forward link certificate of node %s expired at %s: encrypted hops through this node fail until its Agent renews it.", label, stamp(row.NotAfter))
		}
		return fmt.Sprintf("The forward link certificate of node %s ends at %s and was not renewed (it renews with the Agent certificate).", label, stamp(row.NotAfter))
	})
}

// moduleCertificates finds module certificates that were not renewed. A
// module's latest certificate per enrollment counts; revoking the
// enrollment revokes its certificates, which ends the alert.
func (s *Scanner) moduleCertificates(ctx context.Context, now time.Time) ([]Finding, error) {
	latest, err := s.latestCerts(ctx, &model.ModuleCertificate{}, []string{"package_id", "enrollment_id"}, moduleKey, now)
	if err != nil || len(latest) == 0 {
		return nil, err
	}
	holders := make([]string, 0, len(latest))
	for holder := range latest {
		holders = append(holders, holder)
	}
	sort.Strings(holders)
	var findings []Finding
	for _, holder := range holders {
		row := latest[holder]
		lifetime := row.NotAfter.Sub(row.CreatedAt)
		window := leafWindow(lifetime, s.Settings.LeafExpiryDays)
		remaining := row.NotAfter.Sub(now)
		if remaining > window {
			continue
		}
		expired := remaining <= 0
		subject := truncate(row.PackageID, 100) + "#" + truncate(row.EnrollmentID, 8)
		message := fmt.Sprintf("The certificate of module %s ends at %s and was not renewed. Check that the module is running and reaches Control, or revoke its enrollment.", plain(row.PackageID), stamp(row.NotAfter))
		if expired {
			message = fmt.Sprintf("The certificate of module %s expired at %s: the module cannot connect any more. Restart it, enroll it again, or revoke its enrollment.", plain(row.PackageID), stamp(row.NotAfter))
		}
		expires := row.NotAfter.UTC()
		findings = append(findings, Finding{
			Kind: KindModuleCertificate, Severity: severityFor(remaining, window), SubjectKind: SubjectModule, Subject: subject,
			Message: truncate(message, 500), ExpiresAt: &expires,
			Detail: map[string]any{
				"package_id": row.PackageID, "not_after": stamp(row.NotAfter), "expired": expired,
				"lifetime_hours": int64(lifetime / time.Hour), "window_hours": int64(window / time.Hour),
			},
		})
	}
	return findings, nil
}

// authorities finds the current module (Agent, kernel) CA and forward link
// CA when they end within ca_expiry_days.
func (s *Scanner) authorities(ctx context.Context, now time.Time) ([]Finding, error) {
	window := days(s.Settings.CAExpiryDays)
	var findings []Finding
	for _, authority := range []struct {
		name, label, subject, rotate string
		record                       any
		current                      func(db *gorm.DB) *gorm.DB
		next                         func(db *gorm.DB) *gorm.DB
	}{
		{"module", "module (Agent and kernel) CA", "service_ca", "anix-control module ca rotate", &model.ServiceCA{},
			func(db *gorm.DB) *gorm.DB { return db.Where("state = ?", model.ServiceCAStateCurrent) },
			func(db *gorm.DB) *gorm.DB { return db.Where("state = ?", model.ServiceCAStateNext) }},
		{"forward_link", "forward link CA", "forward_link_ca", "anix-control agent link-ca rotate", &model.ForwardLinkCA{},
			func(db *gorm.DB) *gorm.DB { return db.Where("state = ?", model.ForwardLinkCAStateCurrent) },
			func(db *gorm.DB) *gorm.DB { return db.Where("state = ?", model.ForwardLinkCAStateNext) }},
	} {
		db := s.DB.WithContext(ctx)
		if !db.Migrator().HasTable(authority.record) {
			continue
		}
		type caRow struct {
			KeyID    string
			NotAfter time.Time
			Cluster  string
		}
		var current []caRow
		if err := authority.current(db.Model(authority.record)).Select("key_id, not_after, cluster").Where("not_after <= ?", now.Add(window)).Scan(&current).Error; err != nil {
			return nil, err
		}
		for _, row := range current {
			var nextStaged int64
			if err := authority.next(db.Model(authority.record)).Where("cluster = ?", row.Cluster).Count(&nextStaged).Error; err != nil {
				return nil, err
			}
			remaining := row.NotAfter.Sub(now)
			expired := remaining <= 0
			advice := fmt.Sprintf("Rotate it now (%s): the new CA is trusted at once and takes over signing after one certificate lifetime.", authority.rotate)
			if nextStaged > 0 {
				advice = "A next CA is staged and takes over signing by itself after one certificate lifetime; if this alert stays, check the CA maintenance in the Control log."
			}
			message := fmt.Sprintf("The current %s ends at %s. %s", authority.label, stamp(row.NotAfter), advice)
			if expired {
				message = fmt.Sprintf("The current %s ended at %s: certificates it signed fail to verify. %s", authority.label, stamp(row.NotAfter), advice)
			}
			expires := row.NotAfter.UTC()
			findings = append(findings, Finding{
				Kind: KindCAExpiring, Severity: severityFor(remaining, window), SubjectKind: SubjectCA,
				Subject: authority.subject + ":" + truncate(row.KeyID, 64), Message: truncate(message, 500), ExpiresAt: &expires,
				Detail: map[string]any{
					"ca": authority.name, "cluster": row.Cluster, "not_after": stamp(row.NotAfter), "expired": expired,
					"next_staged": nextStaged > 0, "window_days": s.Settings.CAExpiryDays,
				},
			})
		}
	}
	return findings, nil
}

// nodeTableExists reports whether the table that holds nodes of kind exists.
func (s *Scanner) nodeTableExists(ctx context.Context, kind string) bool {
	migrator := s.DB.WithContext(ctx).Migrator()
	switch kind {
	case agentcontrol.NodeKindProxy:
		return migrator.HasTable(&model.Node{})
	case agentcontrol.NodeKindForward:
		return migrator.HasTable(&model.ForwardNode{})
	}
	return false
}

// nodeName reads a node's display name; empty when it cannot be read.
func (s *Scanner) nodeName(ctx context.Context, node agentcontrol.AgentNode) string {
	db := s.DB.WithContext(ctx)
	var names []string
	switch node.Kind {
	case agentcontrol.NodeKindProxy:
		if !db.Migrator().HasTable(&model.Node{}) {
			return ""
		}
		_ = db.Model(&model.Node{}).Where("id = ?", node.ID).Limit(1).Pluck("name", &names).Error
	case agentcontrol.NodeKindForward:
		if !db.Migrator().HasTable(&model.ForwardNode{}) {
			return ""
		}
		_ = db.Model(&model.ForwardNode{}).Where("id = ?", node.ID).Limit(1).Pluck("name", &names).Error
	}
	if len(names) == 0 {
		return ""
	}
	return truncate(names[0], 100)
}

// nodeSecretsSplit finds split tables that wait in a non-final phase.
//
// The split row has no "entered phase at": updated_at moves with every
// backfill, verification and phase change. The scan therefore alerts on
// "not touched for phase_stuck_after", which never fires early (the phase
// is at least that old) and can fire late for a table somebody still
// verifies. A finalize that began (phase finalized, no finalized_at) writes
// the row once when it starts, so its updated_at is its start time.
func (s *Scanner) nodeSecretsSplit(ctx context.Context, now time.Time) ([]Finding, error) {
	stuck := s.Settings.PhaseStuckAfter
	db := s.DB.WithContext(ctx)
	if stuck <= 0 || !db.Migrator().HasTable(&model.NodeSecretSplit{}) {
		return nil, nil
	}
	var rows []model.NodeSecretSplit
	if err := db.Select("table_name", "phase", "finalized_at", "updated_at").Order("table_name").Find(&rows).Error; err != nil {
		return nil, err
	}
	var findings []Finding
	for _, row := range rows {
		idle := now.Sub(row.UpdatedAt)
		switch {
		case row.Phase == nodesecrets.PhaseFinalized && row.FinalizedAt == nil && idle >= finalizeInterruptedAfter:
			findings = append(findings, Finding{
				Kind: KindNodeSecretsFinalizeInterrupted, Severity: model.KernelAlertWarning, SubjectKind: SubjectNodeSecrets, Subject: row.Table,
				Message: fmt.Sprintf("The node credential split finalize of table %s started at %s and did not complete: some legacy columns may still hold secrets. Run `anix-control node-secrets finalize -confirm %s` again; it resumes.", row.Table, stamp(row.UpdatedAt), row.Table),
				Detail:  map[string]any{"table": row.Table, "phase": row.Phase, "since": stamp(row.UpdatedAt)},
			})
		case (row.Phase == nodesecrets.PhaseDualWrite || row.Phase == nodesecrets.PhaseDualRead) && idle >= stuck:
			findings = append(findings, Finding{
				Kind: KindNodeSecretsSplitStalled, Severity: model.KernelAlertWarning, SubjectKind: SubjectNodeSecrets, Subject: row.Table,
				Message: fmt.Sprintf("The node credential split of table %s is in phase %s and was not touched (backfill, verification or phase change) since %s. Legacy columns still hold the secrets: verify and finalize it (anix-control node-secrets), or leave it and set alerts.phase_stuck_after to 0.", row.Table, row.Phase, stamp(row.UpdatedAt)),
				Detail:  map[string]any{"table": row.Table, "phase": row.Phase, "since": stamp(row.UpdatedAt), "stuck_after_hours": int64(stuck / time.Hour)},
			})
		}
	}
	return findings, nil
}

// identityAuthority finds an identity import without progress, and an
// identity cutover left unfinalized.
//
// While importing, every batch of the import saves the authority row, so
// its updated_at is the last progress. The time identity took over is the
// latest "cutover" event (the row's updated_at is also written by a
// rollback and an abort); without such an event no alert is raised.
func (s *Scanner) identityAuthority(ctx context.Context, now time.Time) ([]Finding, error) {
	stuck := s.Settings.PhaseStuckAfter
	db := s.DB.WithContext(ctx)
	if stuck <= 0 || !db.Migrator().HasTable(&model.IdentityAuthority{}) {
		return nil, nil
	}
	var authorities []model.IdentityAuthority
	if err := db.Where("id = ?", 1).Limit(1).Find(&authorities).Error; err != nil || len(authorities) == 0 {
		return nil, err
	}
	authority := authorities[0]
	switch authority.State {
	case model.IdentityAuthorityImporting:
		if now.Sub(authority.UpdatedAt) < stuck {
			return nil, nil
		}
		return []Finding{{
			Kind: KindIdentityImportStalled, Severity: model.KernelAlertWarning, SubjectKind: SubjectIdentity, Subject: "authority",
			Message: fmt.Sprintf("The identity authority has been in state importing with no progress since %s: the account import did not finish or the cutover was not started. Resume the import or start the cutover (POST /api/v4/kernel/identity/import, /cutover).", stamp(authority.UpdatedAt)),
			Detail:  map[string]any{"state": authority.State, "since": stamp(authority.UpdatedAt), "stuck_after_hours": int64(stuck / time.Hour)},
		}}, nil
	case model.IdentityAuthorityIdentity:
		if !db.Migrator().HasTable(&model.IdentityCutoverEvent{}) {
			return nil, nil
		}
		var events []model.IdentityCutoverEvent
		if err := db.Where("action = ?", model.IdentityCutoverActionCutover).Order("id DESC").Limit(1).Find(&events).Error; err != nil || len(events) == 0 {
			return nil, err
		}
		since := events[0].CreatedAt
		if now.Sub(since) < stuck {
			return nil, nil
		}
		return []Finding{{
			Kind: KindIdentityCutoverPending, Severity: model.KernelAlertWarning, SubjectKind: SubjectIdentity, Subject: "authority",
			Message: fmt.Sprintf("Identity has owned logins since the cutover at %s, but the legacy credentials were not finalized: they are still mirrored. Finalize (POST /api/v4/kernel/identity/finalize) once identity is verified, or roll back.", stamp(since)),
			Detail:  map[string]any{"state": authority.State, "since": stamp(since), "stuck_after_hours": int64(stuck / time.Hour)},
		}}, nil
	}
	return nil, nil
}
