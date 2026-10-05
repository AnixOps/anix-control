package kernelalerts

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const week = 7 * 24 * time.Hour

func TestAgentCertificateRules(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		fresh := addProxyNode(t, db, "fresh", model.NodeStatusOnline)
		late := addProxyNode(t, db, "late_node", model.NodeStatusOnline)
		urgent := addProxyNode(t, db, "urgent", model.NodeStatusOnline)
		expired := addProxyNode(t, db, "expired", model.NodeStatusOffline)
		renewed := addProxyNode(t, db, "renewed", model.NodeStatusOnline)
		revoked := addProxyNode(t, db, "revoked", model.NodeStatusOnline)
		disabled := addProxyNode(t, db, "disabled", model.NodeStatusDisabled)
		forward := addForwardNode(t, db, "fwd", true)
		forwardOff := addForwardNode(t, db, "fwd-off", true)
		require.NoError(t, db.Model(&forwardOff).Update("enabled", false).Error)

		// A healthy certificate: issued now, a week to go. A fixed 14-day
		// window would alert here; the window is a sixth of the lifetime.
		addAgentCert(t, db, "s1", agentcontrol.NodeKindProxy, fresh.ID, week, 0)
		// Two thirds of the life gone is when holders renew: still silent.
		addAgentCert(t, db, "s2", agentcontrol.NodeKindForward, forward.ID, week, week*2/3+time.Hour)
		// Inside the last sixth (28 hours): warning.
		addAgentCert(t, db, "s3", agentcontrol.NodeKindProxy, late.ID, week, week-hours(24))
		// Inside the last quarter of the window (7 hours): critical.
		addAgentCert(t, db, "s4", agentcontrol.NodeKindProxy, urgent.ID, week, week-hours(5))
		// Expired, not yet pruned, node still enabled: critical.
		addAgentCert(t, db, "s5", agentcontrol.NodeKindProxy, expired.ID, week, week+hours(3))
		// Old certificate near its end, but a renewed one exists.
		addAgentCert(t, db, "s6a", agentcontrol.NodeKindProxy, renewed.ID, week, week-hours(2))
		addAgentCert(t, db, "s6b", agentcontrol.NodeKindProxy, renewed.ID, week, hours(1))
		// Revoked certificate near its end: ignored.
		addAgentCert(t, db, "s7", agentcontrol.NodeKindProxy, revoked.ID, week, week-hours(2))
		require.NoError(t, db.Model(&model.AgentCertificate{}).Where("serial = ?", "s7").Update("revoked_at", start).Error)
		// Disabled nodes, a deleted node: ignored.
		addAgentCert(t, db, "s8", agentcontrol.NodeKindProxy, disabled.ID, week, week-hours(2))
		addAgentCert(t, db, "s9", agentcontrol.NodeKindForward, forwardOff.ID, week, week-hours(2))
		addAgentCert(t, db, "s10", agentcontrol.NodeKindProxy, 9999, week, week-hours(2))

		found := kinds(scan(t, db, defaultSettings(), start))
		keys := []string{}
		for key := range found {
			keys = append(keys, key)
		}
		require.Len(t, found, 3, "%v", keys)

		warning := found[KindAgentCertificate+"/proxy-"+itoa(late.ID)]
		assert.Equal(t, model.KernelAlertWarning, warning.Severity)
		assert.Equal(t, SubjectNode, warning.SubjectKind)
		assert.Contains(t, warning.Message, "late-node", "markdown characters of a name are replaced")
		assert.NotContains(t, warning.Message, "late_node")
		assert.Equal(t, "late_node", warning.Detail["node_name"])
		assert.EqualValues(t, 28, warning.Detail["window_hours"])
		assert.NotNil(t, warning.ExpiresAt)

		assert.Equal(t, model.KernelAlertCritical, found[KindAgentCertificate+"/proxy-"+itoa(urgent.ID)].Severity)
		gone := found[KindAgentCertificate+"/proxy-"+itoa(expired.ID)]
		assert.Equal(t, model.KernelAlertCritical, gone.Severity)
		assert.Equal(t, true, gone.Detail["expired"])
		assert.Contains(t, gone.Message, "Enroll it again")
	})
}

func TestLeafCapDaysBoundTheWindow(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		node := addProxyNode(t, db, "n", model.NodeStatusOnline)
		// 30-day certificate, 4 days left: a sixth is 5 days.
		addAgentCert(t, db, "s1", agentcontrol.NodeKindProxy, node.ID, 30*24*time.Hour, 26*24*time.Hour)
		assert.Len(t, scan(t, db, defaultSettings(), start).Findings, 1)
		settings := defaultSettings()
		settings.LeafExpiryDays = 3
		assert.Empty(t, scan(t, db, settings, start).Findings, "the cap of 3 days is below the 4 left")
		settings.LeafExpiryDays = 4
		assert.Len(t, scan(t, db, settings, start).Findings, 1)
	})
}

func TestLinkCertificatesAreScannedPerNode(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		node := addForwardNode(t, db, "fwd", true)
		issued := start.Add(-week + hours(20))
		link := func(serial string, notAfter time.Time, revoked bool) {
			row := model.ForwardLinkCertificate{
				Serial: serial, NodeKind: agentcontrol.NodeKindForward, NodeID: node.ID, Cluster: "prod", AgentSerial: "a",
				IssuerKeyID: "k", DNSName: "forward-1", SPIFFEID: "spiffe://x/y", NotAfter: notAfter, CreatedAt: issued,
			}
			require.NoError(t, db.Create(&row).Error)
			if revoked {
				require.NoError(t, db.Model(&row).Update("revoked_at", start).Error)
			}
		}
		link("l1", issued.Add(week), false)
		found := kinds(scan(t, db, defaultSettings(), start))
		require.Len(t, found, 1)
		finding := found[KindLinkCertificate+"/forward-"+itoa(node.ID)]
		assert.Equal(t, model.KernelAlertWarning, finding.Severity)
		assert.Contains(t, finding.Message, "renews with the Agent certificate")

		// A renewed (later) link certificate clears it; the revoked one is ignored.
		link("l2", start.Add(week), false)
		link("l3", start.Add(hours(1)), true)
		assert.Empty(t, scan(t, db, defaultSettings(), start).Findings)
	})
}

func TestModuleCertificatesAreScannedPerEnrollment(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		cert := func(serial, pkg, enrollment string, lifetime, issuedAgo time.Duration) {
			issued := start.Add(-issuedAgo)
			require.NoError(t, db.Create(&model.ModuleCertificate{
				Serial: serial, PackageID: pkg, Cluster: "prod", EnrollmentID: enrollment, IssuerKeyID: "k", NotAfter: issued.Add(lifetime), CreatedAt: issued,
			}).Error)
		}
		// 24-hour certificates renew at 16 hours; the window is 4 hours.
		cert("m1", "identity-platform", "11111111-aaaa", hours(24), hours(10))
		assert.Empty(t, scan(t, db, defaultSettings(), start).Findings)
		cert("m2", "forward", "22222222-bbbb", hours(24), hours(21))
		found := kinds(scan(t, db, defaultSettings(), start))
		require.Len(t, found, 1)
		finding := found[KindModuleCertificate+"/forward#22222222"]
		assert.Equal(t, SubjectModule, finding.SubjectKind)
		assert.Equal(t, model.KernelAlertWarning, finding.Severity)
		// A second instance of the same package under another enrollment
		// does not hide the stale one.
		cert("m3", "forward", "33333333-cccc", hours(24), hours(1))
		assert.Len(t, scan(t, db, defaultSettings(), start).Findings, 1)
		// The expired one is critical.
		found = kinds(scan(t, db, defaultSettings(), start.Add(hours(5))))
		assert.Equal(t, model.KernelAlertCritical, found[KindModuleCertificate+"/forward#22222222"].Severity)
	})
}

func TestCAExpiryUsesTheCurrentAuthorityOnly(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		ca := func(table any, keyID, state string, notAfter time.Time) {
			switch table.(type) {
			case *model.ServiceCA:
				require.NoError(t, db.Create(&model.ServiceCA{Cluster: "prod", State: state, KeyID: keyID, CertificatePEM: "pem", SealedKey: "sealed", NotBefore: start.Add(-time.Hour), NotAfter: notAfter, CreatedAt: start}).Error)
			case *model.ForwardLinkCA:
				require.NoError(t, db.Create(&model.ForwardLinkCA{Cluster: "prod", State: state, KeyID: keyID, CertificatePEM: "pem", SealedKey: "sealed", NotBefore: start.Add(-time.Hour), NotAfter: notAfter, CreatedAt: start}).Error)
			}
		}
		day := 24 * time.Hour
		ca(&model.ServiceCA{}, "far", model.ServiceCAStateCurrent, start.Add(400*day))
		ca(&model.ServiceCA{}, "old", model.ServiceCAStateRetired, start.Add(2*day))
		assert.Empty(t, scan(t, db, defaultSettings(), start).Findings, "a far CA and a retired CA do not alert")

		require.NoError(t, db.Model(&model.ServiceCA{}).Where("key_id = ?", "far").Update("not_after", start.Add(59*day)).Error)
		found := kinds(scan(t, db, defaultSettings(), start))
		require.Len(t, found, 1)
		finding := found[KindCAExpiring+"/service_ca:far"]
		assert.Equal(t, model.KernelAlertWarning, finding.Severity)
		assert.Equal(t, false, finding.Detail["next_staged"])
		assert.Equal(t, "module", finding.Detail["ca"])
		assert.NotContains(t, finding.Message, "sealed")
		assert.Contains(t, finding.Message, "anix-control module ca rotate")

		ca(&model.ServiceCA{}, "next", model.ServiceCAStateNext, start.Add(900*day))
		found = kinds(scan(t, db, defaultSettings(), start))
		assert.Equal(t, true, found[KindCAExpiring+"/service_ca:far"].Detail["next_staged"])
		assert.Contains(t, found[KindCAExpiring+"/service_ca:far"].Message, "takes over signing by itself")

		// 61 days is outside the window; 10 days is critical (a quarter of 60 is 15).
		require.NoError(t, db.Model(&model.ServiceCA{}).Where("key_id = ?", "far").Update("not_after", start.Add(61*day)).Error)
		assert.Empty(t, scan(t, db, defaultSettings(), start).Findings)
		require.NoError(t, db.Model(&model.ServiceCA{}).Where("key_id = ?", "far").Update("not_after", start.Add(10*day)).Error)
		assert.Equal(t, model.KernelAlertCritical, kinds(scan(t, db, defaultSettings(), start))[KindCAExpiring+"/service_ca:far"].Severity)
		require.NoError(t, db.Model(&model.ServiceCA{}).Where("key_id = ?", "far").Update("not_after", start.Add(-day)).Error)
		expired := kinds(scan(t, db, defaultSettings(), start))[KindCAExpiring+"/service_ca:far"]
		assert.Equal(t, model.KernelAlertCritical, expired.Severity)
		assert.Equal(t, true, expired.Detail["expired"])

		// The forward link CA has its own subject.
		ca(&model.ForwardLinkCA{}, "link", model.ForwardLinkCAStateCurrent, start.Add(30*day))
		assert.Contains(t, kinds(scan(t, db, defaultSettings(), start)), KindCAExpiring+"/forward_link_ca:link")
		settings := defaultSettings()
		settings.CAExpiryDays = 20
		assert.NotContains(t, kinds(scan(t, db, settings, start)), KindCAExpiring+"/forward_link_ca:link")
	})
}

func TestNodeSecretsSplitPhases(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		row := func(table, phase string, updatedAgo time.Duration, finalized bool) {
			split := model.NodeSecretSplit{Table: table, Phase: phase, UpdatedAt: start.Add(-updatedAgo)}
			if finalized {
				at := start.Add(-updatedAgo)
				split.FinalizedAt = &at
			}
			require.NoError(t, db.Create(&split).Error)
		}
		row("v2_node", "dual_write", hours(80), false)
		row("v2_authorized_key", "dual_read", hours(100), false)
		row("v2_forward_node", "dual_write", hours(10), false)
		row("v2_forward_clean_agent", "finalized", hours(500), true)
		row("v2_node_protocol", "finalized", hours(2), false)
		row("v2_wireguard_peer", "finalized", hours(0), false)

		found := kinds(scan(t, db, defaultSettings(), start))
		require.Len(t, found, 3)
		stalled := found[KindNodeSecretsSplitStalled+"/v2_node"]
		assert.Equal(t, SubjectNodeSecrets, stalled.SubjectKind)
		assert.Equal(t, model.KernelAlertWarning, stalled.Severity)
		assert.Equal(t, "dual_write", stalled.Detail["phase"])
		assert.Contains(t, stalled.Message, "was not touched")
		assert.Contains(t, found, KindNodeSecretsSplitStalled+"/v2_authorized_key")
		interrupted := found[KindNodeSecretsFinalizeInterrupted+"/v2_node_protocol"]
		assert.Contains(t, interrupted.Message, "resumes")

		// The alert follows the threshold, and 0 turns phase alerts off.
		settings := defaultSettings()
		settings.PhaseStuckAfter = hours(120)
		assert.Len(t, scan(t, db, settings, start).Findings, 1, "only the interrupted finalize remains")
		settings.PhaseStuckAfter = 0
		assert.Empty(t, scan(t, db, settings, start).Findings)
	})
}

func TestIdentityPhases(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		setState := func(state string, updatedAgo time.Duration) {
			require.NoError(t, db.Where("1 = 1").Delete(&model.IdentityAuthority{}).Error)
			require.NoError(t, db.Create(&model.IdentityAuthority{ID: 1, State: state, Checkpoint: "secret-looking-checkpoint", UpdatedAt: start.Add(-updatedAgo)}).Error)
		}
		cutover := func(action string, ago time.Duration) {
			require.NoError(t, db.Create(&model.IdentityCutoverEvent{Action: action, CreatedAt: start.Add(-ago)}).Error)
		}

		assert.Empty(t, scan(t, db, defaultSettings(), start).Findings, "no row means kernel")
		setState(model.IdentityAuthorityKernel, hours(900))
		assert.Empty(t, scan(t, db, defaultSettings(), start).Findings)

		setState(model.IdentityAuthorityImporting, hours(10))
		assert.Empty(t, scan(t, db, defaultSettings(), start).Findings)
		setState(model.IdentityAuthorityImporting, hours(100))
		found := kinds(scan(t, db, defaultSettings(), start))
		require.Len(t, found, 1)
		finding := found[KindIdentityImportStalled+"/authority"]
		assert.Equal(t, SubjectIdentity, finding.SubjectKind)
		assert.NotContains(t, finding.Message+" "+toJSON(finding.Detail), "checkpoint", "the import checkpoint stays out of alerts")

		// identity: the cutover event dates the phase, not updated_at.
		setState(model.IdentityAuthorityIdentity, hours(1))
		assert.Empty(t, scan(t, db, defaultSettings(), start).Findings, "no cutover event, no reliable start")
		cutover(model.IdentityCutoverActionCutover, hours(200))
		cutover(model.IdentityCutoverActionRollback, hours(150))
		cutover(model.IdentityCutoverActionCutover, hours(30))
		assert.Empty(t, scan(t, db, defaultSettings(), start).Findings, "the latest cutover is 30 hours old")
		settings := defaultSettings()
		settings.PhaseStuckAfter = hours(24)
		found = kinds(scan(t, db, settings, start))
		require.Len(t, found, 1)
		assert.Contains(t, found[KindIdentityCutoverPending+"/authority"].Message, "legacy credentials were not finalized")

		setState(model.IdentityAuthorityFinalized, hours(900))
		assert.Empty(t, scan(t, db, defaultSettings(), start).Findings)
	})
}

// Once the v4.1 forward tables are dropped, forward nodes cannot be looked
// up: their certificates are skipped without failing the proxy checks.
func TestMissingForwardNodeTableSkipsForwardNodesOnly(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		proxy := addProxyNode(t, db, "edge", model.NodeStatusOnline)
		addAgentCert(t, db, "s1", agentcontrol.NodeKindProxy, proxy.ID, week, week-hours(24))
		addAgentCert(t, db, "s2", agentcontrol.NodeKindForward, 5, week, week-hours(24))
		require.NoError(t, db.Migrator().DropTable(&model.ForwardNode{}))
		found := kinds(scan(t, db, defaultSettings(), start))
		require.Len(t, found, 1)
		assert.Contains(t, found, KindAgentCertificate+"/proxy-"+itoa(proxy.ID))
	})
}
