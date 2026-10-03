package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/sdk/telemetry/systemdreport"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const packageReportTestVersion = "4.1.0"

// packageReportSigner signs every release of these tests: the kernel trusts
// one AnixOps root.
var packageReportSigner = func() ed25519.PrivateKey {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	return privateKey
}()

// seedPackageReportRelease registers a signed official Agent release of
// pluginID at version with capabilities, and enables it on node.
func seedPackageReportRelease(t *testing.T, db *gorm.DB, nodeID uint, pluginID, version string, capabilities []string) {
	t.Helper()
	privateKey := packageReportSigner
	publicKey := privateKey.Public().(ed25519.PublicKey)
	manifest := PluginManifest{
		ID: pluginID, Name: pluginID, Version: version, APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, ArtifactSHA256: strings.Repeat("a", 64), Capabilities: capabilities,
	}
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	_, err = RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.NodeServiceAssignment{
		NodeID: nodeID, ServiceScope: "telemetry", PluginID: pluginID, Role: "agent", DesiredVersion: version, Enabled: true,
	}).Error)
}

const systemdTestPayload = `{"supported": true, "window_seconds": 600, "units": [
	{"name": "sshd.service", "active_state": "active", "sub_state": "running", "cpu_avg_percent": 1, "cpu_peak_percent": 2, "memory_bytes": 3, "memory_peak_bytes": 4, "pid": 1}]}`

func systemdTestInput(observedAt time.Time) PackageReportInput {
	return PackageReportInput{
		NodeKind: agentcontrol.NodeKindProxy, NodeID: 1, PluginID: "machine-telemetry", Kind: systemdreport.Kind,
		Version: packageReportTestVersion, Payload: []byte(systemdTestPayload), ObservedAt: observedAt,
	}
}

func requireRefused(t *testing.T, err error, reason string) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, reason, PackageReportRefusal(err), err.Error())
}

func TestRecordPackageReportStoresTheSanitizedLatestReport(t *testing.T) {
	db := newKernelTestDB(t)
	seedPackageReportRelease(t, db, 1, "machine-telemetry", packageReportTestVersion, []string{systemdreport.Capability})
	service := &NodeService{db: db}
	receivedAt := time.Unix(1_800_000_000, 0).UTC()

	stored, err := service.RecordPackageReport(systemdTestInput(receivedAt.Add(-time.Minute)), receivedAt)
	require.NoError(t, err)
	require.True(t, stored)
	report, err := service.LatestPackageReport(agentcontrol.NodeKindProxy, 1, "machine-telemetry", systemdreport.Kind, receivedAt)
	require.NoError(t, err)
	assert.JSONEq(t, `{"supported":true,"window_seconds":600,"units":[{"name":"sshd.service","active_state":"active","sub_state":"running","cpu_avg_percent":1,"cpu_peak_percent":2,"memory_bytes":3,"memory_peak_bytes":4}]}`, string(report.Payload))
	assert.Equal(t, packageReportTestVersion, report.Version)
	assert.True(t, report.ObservedAt.Equal(receivedAt.Add(-time.Minute)))
	assert.True(t, report.ReceivedAt.Equal(receivedAt))
	assert.False(t, report.Stale)

	// An older report is dropped; a newer one replaces the row.
	older := systemdTestInput(receivedAt.Add(-2 * time.Minute))
	older.Payload = []byte(`{"supported": false, "unsupported_reason": "older"}`)
	stored, err = service.RecordPackageReport(older, receivedAt)
	require.NoError(t, err)
	require.False(t, stored)
	newer := systemdTestInput(time.Time{})
	newer.Payload = []byte(`{"supported": false, "unsupported_reason": "cgroup v1"}`)
	stored, err = service.RecordPackageReport(newer, receivedAt.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, stored)
	var rows []model.PackageReportState
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1, "latest only")
	assert.JSONEq(t, `{"supported":false,"unsupported_reason":"cgroup v1","window_seconds":0,"units":[]}`, rows[0].PayloadJSON)
	assert.True(t, rows[0].ObservedAt.Equal(receivedAt.Add(time.Minute)), "no observed_at means the receive time")

	// Stale after 25 minutes.
	report, err = service.LatestPackageReport(agentcontrol.NodeKindProxy, 1, "machine-telemetry", systemdreport.Kind, receivedAt.Add(26*time.Minute))
	require.NoError(t, err)
	assert.False(t, report.Stale, "exactly 25 minutes old")
	report, err = service.LatestPackageReport(agentcontrol.NodeKindProxy, 1, "machine-telemetry", systemdreport.Kind, receivedAt.Add(27*time.Minute))
	require.NoError(t, err)
	assert.True(t, report.Stale)

	_, err = service.LatestPackageReport(agentcontrol.NodeKindForward, 1, "machine-telemetry", systemdreport.Kind, receivedAt)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestRecordPackageReportRefusals(t *testing.T) {
	db := newKernelTestDB(t)
	seedPackageReportRelease(t, db, 1, "machine-telemetry", packageReportTestVersion, []string{systemdreport.Capability})
	seedPackageReportRelease(t, db, 1, "node-exporter", packageReportTestVersion, []string{"telemetry.read"})
	seedPackageReportRelease(t, db, 2, "machine-telemetry", "4.1.1", []string{systemdreport.Capability})
	service := &NodeService{db: db}
	receivedAt := time.Unix(1_800_000_000, 0).UTC()
	edit := func(change func(*PackageReportInput)) PackageReportInput {
		input := systemdTestInput(receivedAt)
		change(&input)
		return input
	}
	for name, testCase := range map[string]struct {
		input  PackageReportInput
		reason string
	}{
		"no node":            {edit(func(in *PackageReportInput) { in.NodeID = 0 }), PackageReportRefusedInvalid},
		"bad plugin id":      {edit(func(in *PackageReportInput) { in.PluginID = "../x" }), PackageReportRefusedInvalid},
		"bad version":        {edit(func(in *PackageReportInput) { in.Version = "" }), PackageReportRefusedInvalid},
		"long version":       {edit(func(in *PackageReportInput) { in.Version = strings.Repeat("1", 65) }), PackageReportRefusedInvalid},
		"bad kind":           {edit(func(in *PackageReportInput) { in.Kind = "Systemd" }), PackageReportRefusedInvalid},
		"oversize":           {edit(func(in *PackageReportInput) { in.Payload = make([]byte, agentcontrol.MaxPackageReportPayloadBytes+1) }), PackageReportRefusedOversize},
		"future":             {edit(func(in *PackageReportInput) { in.ObservedAt = receivedAt.Add(2 * time.Minute) }), PackageReportRefusedFuture},
		"unknown kind":       {edit(func(in *PackageReportInput) { in.Kind = "systemd.timers" }), PackageReportRefusedUnknownKind},
		"forward node":       {edit(func(in *PackageReportInput) { in.NodeKind = agentcontrol.NodeKindForward }), PackageReportRefusedNotAssigned},
		"not assigned":       {edit(func(in *PackageReportInput) { in.NodeID = 3 }), PackageReportRefusedNotAssigned},
		"other version":      {edit(func(in *PackageReportInput) { in.Version = "4.0.0" }), PackageReportRefusedVersionMismatch},
		"missing capability": {edit(func(in *PackageReportInput) { in.PluginID = "node-exporter" }), PackageReportRefusedMissingCapability},
		"bad payload":        {edit(func(in *PackageReportInput) { in.Payload = []byte(`{"supported": true}`) }), PackageReportRefusedBadPayload},
		"description in unit": {edit(func(in *PackageReportInput) {
			in.Payload = []byte(strings.Replace(systemdTestPayload, `"pid"`, `"Description"`, 1))
		}), PackageReportRefusedBadPayload},
	} {
		_, err := service.RecordPackageReport(testCase.input, receivedAt)
		requireRefused(t, err, testCase.reason)
		require.Contains(t, PackageReportRefusalReasons, testCase.reason, name)
	}
	var count int64
	require.NoError(t, db.Model(&model.PackageReportState{}).Count(&count).Error)
	require.Zero(t, count)

	// Node 2 runs 4.1.1, which it is assigned.
	input := systemdTestInput(receivedAt)
	input.NodeID, input.Version = 2, "4.1.1"
	stored, err := service.RecordPackageReport(input, receivedAt)
	require.NoError(t, err)
	require.True(t, stored)
}

func TestRecordPackageReportRequiresASignedOfficialRelease(t *testing.T) {
	receivedAt := time.Unix(1_800_000_000, 0).UTC()
	t.Run("signature no longer verifies", func(t *testing.T) {
		db := newKernelTestDB(t)
		seedPackageReportRelease(t, db, 1, "machine-telemetry", packageReportTestVersion, []string{systemdreport.Capability})
		require.NoError(t, db.Model(&model.PluginRelease{}).Where("plugin_id = ?", "machine-telemetry").Update("signature", "").Error)
		_, err := (&NodeService{db: db}).RecordPackageReport(systemdTestInput(receivedAt), receivedAt)
		requireRefused(t, err, PackageReportRefusedUnsigned)
	})
	t.Run("not official", func(t *testing.T) {
		db := newKernelTestDB(t)
		seedPackageReportRelease(t, db, 1, "machine-telemetry", packageReportTestVersion, []string{systemdreport.Capability})
		require.NoError(t, db.Model(&model.Plugin{}).Where("id = ?", "machine-telemetry").Update("official", false).Error)
		_, err := (&NodeService{db: db}).RecordPackageReport(systemdTestInput(receivedAt), receivedAt)
		requireRefused(t, err, PackageReportRefusedUnsigned)
	})
	t.Run("no stored release", func(t *testing.T) {
		db := newKernelTestDB(t)
		seedPackageReportRelease(t, db, 1, "machine-telemetry", packageReportTestVersion, []string{systemdreport.Capability})
		require.NoError(t, db.Where("plugin_id = ?", "machine-telemetry").Delete(&model.PluginRelease{}).Error)
		_, err := (&NodeService{db: db}).RecordPackageReport(systemdTestInput(receivedAt), receivedAt)
		requireRefused(t, err, PackageReportRefusedUnsigned)
	})
	t.Run("not an Agent release", func(t *testing.T) {
		db := newKernelTestDB(t)
		privateKey := packageReportSigner
		publicKey := privateKey.Public().(ed25519.PublicKey)
		manifest := PluginManifest{
			ID: "machine-telemetry", Name: "machine-telemetry", Version: packageReportTestVersion, APIVersion: "v1", Publisher: "AnixOps",
			Targets: []string{"control"}, ArtifactSHA256: strings.Repeat("a", 64), Capabilities: []string{systemdreport.Capability},
		}
		canonical, err := CanonicalPluginManifest(manifest)
		require.NoError(t, err)
		_, err = RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
		require.NoError(t, err)
		require.NoError(t, db.Create(&model.NodeServiceAssignment{
			NodeID: 1, ServiceScope: "telemetry", PluginID: "machine-telemetry", Role: "agent", DesiredVersion: packageReportTestVersion, Enabled: true,
		}).Error)
		_, err = (&NodeService{db: db}).RecordPackageReport(systemdTestInput(receivedAt), receivedAt)
		requireRefused(t, err, PackageReportRefusedMissingCapability)
	})
	t.Run("disabled assignment", func(t *testing.T) {
		db := newKernelTestDB(t)
		seedPackageReportRelease(t, db, 1, "machine-telemetry", packageReportTestVersion, []string{systemdreport.Capability})
		require.NoError(t, db.Model(&model.NodeServiceAssignment{}).Where("node_id = 1").Update("enabled", false).Error)
		_, err := (&NodeService{db: db}).RecordPackageReport(systemdTestInput(receivedAt), receivedAt)
		requireRefused(t, err, PackageReportRefusedNotAssigned)
	})
}

func TestRecordPackageReportDatabaseFailureIsNotARefusal(t *testing.T) {
	db := newKernelTestDB(t)
	seedPackageReportRelease(t, db, 1, "machine-telemetry", packageReportTestVersion, []string{systemdreport.Capability})
	require.NoError(t, db.Migrator().DropTable(&model.PackageReportState{}))
	receivedAt := time.Unix(1_800_000_000, 0).UTC()
	_, err := (&NodeService{db: db}).RecordPackageReport(systemdTestInput(receivedAt), receivedAt)
	require.Error(t, err)
	require.Empty(t, PackageReportRefusal(err))

	require.NoError(t, db.Migrator().DropTable(&model.NodeServiceAssignment{}))
	_, err = (&NodeService{db: db}).RecordPackageReport(systemdTestInput(receivedAt), receivedAt)
	require.Error(t, err)
	require.Empty(t, PackageReportRefusal(err))

	var nilService *NodeService
	_, err = nilService.RecordPackageReport(systemdTestInput(receivedAt), receivedAt)
	require.Error(t, err)
	_, err = nilService.LatestPackageReport(agentcontrol.NodeKindProxy, 1, "machine-telemetry", systemdreport.Kind, receivedAt)
	require.Error(t, err)
	require.Empty(t, PackageReportRefusal(errors.New("other")))
}

func TestPackageReportStale(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	assert.True(t, PackageReportStale(systemdreport.Kind, now.Add(-systemdreport.StaleAfter-time.Second), now))
	assert.False(t, PackageReportStale(systemdreport.Kind, now.Add(-systemdreport.StaleAfter), now))
	assert.True(t, PackageReportStale("unknown.kind", now.Add(-26*time.Minute), now))
}
