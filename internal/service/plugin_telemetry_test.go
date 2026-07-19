package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const telemetryTestVersion = "1.1.0"

func seedOfficialTelemetryPlugins(t *testing.T, db *gorm.DB, pluginIDs ...string) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	for _, pluginID := range pluginIDs {
		seedOfficialTelemetryPluginReleaseWithSigner(t, db, publicKey, privateKey, pluginID, []string{"telemetry.read"})
	}
}

func seedOfficialTelemetryPluginRelease(t *testing.T, db *gorm.DB, pluginID string, capabilities []string) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	seedOfficialTelemetryPluginReleaseWithSigner(t, db, publicKey, privateKey, pluginID, capabilities)
}

func seedOfficialTelemetryPluginReleaseWithSigner(
	t *testing.T,
	db *gorm.DB,
	publicKey ed25519.PublicKey,
	privateKey ed25519.PrivateKey,
	pluginID string,
	capabilities []string,
) {
	t.Helper()
	manifest := PluginManifest{
		ID: pluginID, Name: pluginID, Version: telemetryTestVersion, APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, ArtifactSHA256: strings.Repeat("a", 64), Capabilities: capabilities,
	}
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical))
	_, err = RegisterPluginRelease(db, string(canonical), signature, publicKey)
	require.NoError(t, err)
}

func TestRecordPluginTelemetryRequiresEnabledAssignment(t *testing.T) {
	db := newKernelTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Node{}))
	node := model.Node{Name: "telemetry", Host: "127.0.0.1"}
	require.NoError(t, db.Create(&node).Error)
	seedOfficialTelemetryPlugins(t, db, "machine-telemetry")
	service := &NodeService{db: db}
	receivedAt := time.Unix(1_800_000_000, 0).UTC()
	metrics := map[string]float64{
		"go_goroutines": 12,
		"plugin.machine-telemetry.cpu_usage_percent":    23.5,
		"plugin.machine-telemetry.memory_usage_percent": 41,
		"plugin.machine-telemetry.disk_usage_percent":   55,
		"plugin.machine-telemetry.uptime_seconds":       120,
		"plugin.machine-telemetry.sample_age_seconds":   2,
	}

	accepted, err := service.RecordPluginTelemetry(node.ID, metrics, receivedAt)
	require.NoError(t, err)
	require.Zero(t, accepted)
	require.NoError(t, db.Create(&model.NodeServiceAssignment{
		NodeID: node.ID, ServiceScope: "monitoring", PluginID: "machine-telemetry", Role: "collector", DesiredVersion: telemetryTestVersion, Enabled: true,
	}).Error)

	accepted, err = service.RecordPluginTelemetry(node.ID, metrics, receivedAt)
	require.NoError(t, err)
	require.Equal(t, 5, accepted)
	var state model.PluginTelemetryState
	require.NoError(t, db.First(&state, "node_id = ? AND plugin_id = ?", node.ID, "machine-telemetry").Error)
	require.Equal(t, receivedAt.Add(-2*time.Second), state.ObservedAt)
	stored := map[string]float64{}
	require.NoError(t, json.Unmarshal([]byte(state.MetricsJSON), &stored))
	require.Equal(t, 23.5, stored["cpu_usage_percent"])
	require.NotContains(t, stored, "go_goroutines")
	var updated model.Node
	require.NoError(t, db.First(&updated, node.ID).Error)
	require.Equal(t, 23.5, updated.CPUUsage)
	require.Equal(t, 41.0, updated.MemoryUsage)
	require.Equal(t, int64(120), updated.Uptime)
}

func TestRecordPluginTelemetryRejectsMalformedAndNonFiniteMetrics(t *testing.T) {
	db := newKernelTestDB(t)
	service := &NodeService{db: db}
	_, err := service.RecordPluginTelemetry(1, map[string]float64{"plugin.machine-telemetry.Bad Key": 1}, time.Now())
	require.ErrorContains(t, err, "invalid")
	_, err = service.RecordPluginTelemetry(1, map[string]float64{"plugin.machine-telemetry.cpu": math.Inf(1)}, time.Now())
	require.ErrorContains(t, err, "not finite")
	_, err = service.RecordPluginTelemetry(1, map[string]float64{"plugin.missing": 1}, time.Now())
	require.ErrorContains(t, err, "namespace")
}

func TestRecordPluginTelemetryIgnoresUnofficialAssignment(t *testing.T) {
	db := newKernelTestDB(t)
	nodeID := uint(6)
	require.NoError(t, db.Create(&model.Plugin{ID: "third-party", Name: "third-party", Publisher: "Other", Official: false}).Error)
	require.NoError(t, db.Create(&model.NodeServiceAssignment{
		NodeID: nodeID, ServiceScope: "monitoring", PluginID: "third-party", Role: "collector", DesiredVersion: telemetryTestVersion, Enabled: true,
	}).Error)
	service := &NodeService{db: db}
	accepted, err := service.RecordPluginTelemetry(nodeID, map[string]float64{
		"plugin.third-party.connections": 9,
	}, time.Now())
	require.NoError(t, err)
	require.Zero(t, accepted)
	var count int64
	require.NoError(t, db.Model(&model.PluginTelemetryState{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestRecordPluginTelemetryDoesNotBackfillLegacyFieldsFromStaleSample(t *testing.T) {
	db := newKernelTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Node{}))
	node := model.Node{Name: "stale-telemetry", Host: "127.0.0.1", CPUUsage: 7}
	require.NoError(t, db.Create(&node).Error)
	seedOfficialTelemetryPlugins(t, db, "machine-telemetry")
	require.NoError(t, db.Create(&model.NodeServiceAssignment{
		NodeID: node.ID, ServiceScope: "monitoring", PluginID: "machine-telemetry", Role: "collector", DesiredVersion: telemetryTestVersion, Enabled: true,
	}).Error)
	service := &NodeService{db: db}
	_, err := service.RecordPluginTelemetry(node.ID, map[string]float64{
		"plugin.machine-telemetry.cpu_usage_percent":    99,
		"plugin.machine-telemetry.memory_usage_percent": 50,
		"plugin.machine-telemetry.disk_usage_percent":   60,
		"plugin.machine-telemetry.uptime_seconds":       100,
		"plugin.machine-telemetry.sample_age_seconds":   10 * 60,
	}, time.Now())
	require.NoError(t, err)
	var refreshed model.Node
	require.NoError(t, db.First(&refreshed, node.ID).Error)
	require.Equal(t, 7.0, refreshed.CPUUsage)
}

func TestRecordPluginTelemetryDoesNotRegressObservedSnapshot(t *testing.T) {
	db := newKernelTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Node{}))
	node := model.Node{Name: "monotonic-telemetry", Host: "127.0.0.1"}
	require.NoError(t, db.Create(&node).Error)
	seedOfficialTelemetryPlugins(t, db, "machine-telemetry")
	require.NoError(t, db.Create(&model.NodeServiceAssignment{
		NodeID: node.ID, ServiceScope: "monitoring", PluginID: "machine-telemetry", Role: "collector", DesiredVersion: telemetryTestVersion, Enabled: true,
	}).Error)
	service := &NodeService{db: db}
	newerAt := time.Unix(1_800_000_000, 0).UTC()
	base := map[string]float64{
		"plugin.machine-telemetry.cpu_usage_percent":    20,
		"plugin.machine-telemetry.memory_usage_percent": 30,
		"plugin.machine-telemetry.disk_usage_percent":   40,
		"plugin.machine-telemetry.uptime_seconds":       500,
		"plugin.machine-telemetry.sample_age_seconds":   0,
	}
	_, err := service.RecordPluginTelemetry(node.ID, base, newerAt)
	require.NoError(t, err)
	older := map[string]float64{
		"plugin.machine-telemetry.cpu_usage_percent":    90,
		"plugin.machine-telemetry.memory_usage_percent": 91,
		"plugin.machine-telemetry.disk_usage_percent":   92,
		"plugin.machine-telemetry.uptime_seconds":       100,
		"plugin.machine-telemetry.sample_age_seconds":   120,
	}
	_, err = service.RecordPluginTelemetry(node.ID, older, newerAt.Add(time.Minute))
	require.NoError(t, err)
	var state model.PluginTelemetryState
	require.NoError(t, db.First(&state, "node_id = ? AND plugin_id = ?", node.ID, "machine-telemetry").Error)
	require.Equal(t, newerAt, state.ObservedAt)
	require.Contains(t, state.MetricsJSON, `"cpu_usage_percent":20`)
	var refreshed model.Node
	require.NoError(t, db.First(&refreshed, node.ID).Error)
	require.Equal(t, 20.0, refreshed.CPUUsage)
}

func TestRecordPluginTelemetryKeepsPluginScopesIndependent(t *testing.T) {
	db := newKernelTestDB(t)
	nodeID := uint(7)
	seedOfficialTelemetryPlugins(t, db, "machine-telemetry", "example-metrics")
	for _, pluginID := range []string{"machine-telemetry", "example-metrics"} {
		require.NoError(t, db.Create(&model.NodeServiceAssignment{
			NodeID: nodeID, ServiceScope: "monitoring", PluginID: pluginID, Role: "collector", DesiredVersion: telemetryTestVersion, Enabled: true,
		}).Error)
	}
	service := &NodeService{db: db}
	accepted, err := service.RecordPluginTelemetry(nodeID, map[string]float64{
		"plugin.machine-telemetry.cpu_usage_percent": 10,
		"plugin.example-metrics.connections":         3,
	}, time.Now())
	require.NoError(t, err)
	require.Equal(t, 2, accepted)
	var states []model.PluginTelemetryState
	require.NoError(t, db.Order("plugin_id").Find(&states).Error)
	require.Len(t, states, 2)
	require.NotEqual(t, states[0].MetricsJSON, states[1].MetricsJSON)
}

func TestRecordPluginTelemetryUsesLongestDottedAssignmentPrefix(t *testing.T) {
	db := newKernelTestDB(t)
	nodeID := uint(8)
	seedOfficialTelemetryPlugins(t, db, "ops", "ops.telemetry")
	for _, pluginID := range []string{"ops", "ops.telemetry"} {
		require.NoError(t, db.Create(&model.NodeServiceAssignment{
			NodeID: nodeID, ServiceScope: "monitoring", PluginID: pluginID, Role: "collector", DesiredVersion: telemetryTestVersion, Enabled: true,
		}).Error)
	}
	service := &NodeService{db: db}
	accepted, err := service.RecordPluginTelemetry(nodeID, map[string]float64{
		"plugin.ops.telemetry.cpu_usage_percent": 22,
		"plugin.ops.connections":                 4,
	}, time.Now())
	require.NoError(t, err)
	require.Equal(t, 2, accepted)
	var dotted model.PluginTelemetryState
	require.NoError(t, db.First(&dotted, "node_id = ? AND plugin_id = ?", nodeID, "ops.telemetry").Error)
	require.JSONEq(t, `{"cpu_usage_percent":22}`, dotted.MetricsJSON)
	var short model.PluginTelemetryState
	require.NoError(t, db.First(&short, "node_id = ? AND plugin_id = ?", nodeID, "ops").Error)
	require.JSONEq(t, `{"connections":4}`, short.MetricsJSON)
}

func TestRecordPluginTelemetryKeepsValidPluginWhenAnotherSnapshotFails(t *testing.T) {
	db := newKernelTestDB(t)
	nodeID := uint(9)
	seedOfficialTelemetryPlugins(t, db, "bad-telemetry", "good-telemetry")
	for _, pluginID := range []string{"bad-telemetry", "good-telemetry"} {
		require.NoError(t, db.Create(&model.NodeServiceAssignment{
			NodeID: nodeID, ServiceScope: "monitoring", PluginID: pluginID, Role: "collector", DesiredVersion: telemetryTestVersion, Enabled: true,
		}).Error)
	}
	service := &NodeService{db: db}
	accepted, err := service.RecordPluginTelemetry(nodeID, map[string]float64{
		"plugin.bad-telemetry.sample_age_seconds": maxPluginTelemetrySampleAge.Seconds() + 1,
		"plugin.good-telemetry.connections":       5,
	}, time.Now())
	require.ErrorContains(t, err, "sample age")
	require.Equal(t, 1, accepted)
	var good model.PluginTelemetryState
	require.NoError(t, db.First(&good, "node_id = ? AND plugin_id = ?", nodeID, "good-telemetry").Error)
	require.JSONEq(t, `{"connections":5}`, good.MetricsJSON)
	var count int64
	require.NoError(t, db.Model(&model.PluginTelemetryState{}).Where("node_id = ? AND plugin_id = ?", nodeID, "bad-telemetry").Count(&count).Error)
	require.Zero(t, count)
}

func TestRecordPluginTelemetryRequiresSignedTelemetryCapability(t *testing.T) {
	db := newKernelTestDB(t)
	seedOfficialTelemetryPluginRelease(t, db, "official-non-telemetry", nil)
	nodeID := uint(10)
	require.NoError(t, db.Create(&model.NodeServiceAssignment{
		NodeID: nodeID, ServiceScope: "monitoring", PluginID: "official-non-telemetry", Role: "collector",
		DesiredVersion: telemetryTestVersion, Enabled: true,
	}).Error)
	service := &NodeService{db: db}
	accepted, err := service.RecordPluginTelemetry(nodeID, map[string]float64{
		"plugin.official-non-telemetry.connections": 5,
	}, time.Now())
	require.ErrorContains(t, err, "does not declare telemetry.read")
	require.Zero(t, accepted)
	var count int64
	require.NoError(t, db.Model(&model.PluginTelemetryState{}).Count(&count).Error)
	require.Zero(t, count)
}
