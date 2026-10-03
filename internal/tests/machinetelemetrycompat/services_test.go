package machinetelemetrycompat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/telemetry/systemdreport"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/packages/machine-telemetry/native"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The package computes the route id of its services route as the kernel
// does, and its manifest declares the route.
func TestServicesRouteIsTheKernelsControlRoute(t *testing.T) {
	require.Equal(t, service.PluginControlBridgeRouteID(native.PackageID, native.NodesRoute), native.NodesRouteID)
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "packages", "machine-telemetry", "manifest.template.json"))
	require.NoError(t, err)
	var manifest struct {
		ControlRoutes []string `json:"control_routes"`
		Capabilities  []string `json:"capabilities"`
		Permissions   []string `json:"permissions"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Contains(t, manifest.ControlRoutes, native.NodesRoute)
	require.Contains(t, manifest.Capabilities, systemdreport.Capability)
	require.Contains(t, manifest.Capabilities, "kernel.view:kapi_package_report_v1")
	require.Contains(t, manifest.Capabilities, "kernel.view:kapi_plugin_configuration_v1")
	require.Contains(t, manifest.Permissions, "machine-telemetry.services.view")
}

type servicesKernel struct {
	db  *gorm.DB
	now time.Time
}

func newServicesKernel(t *testing.T) *servicesKernel {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.PluginInstallation{}, &model.PluginConfiguration{}, &model.PackageReportState{}))
	require.NoError(t, packagestore.EnsureKernelAPIViews(db))
	return &servicesKernel{db: db, now: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)}
}

func (k *servicesKernel) configure(t *testing.T, document string) {
	t.Helper()
	installation := model.PluginInstallation{PluginID: native.PackageID, Target: "agent", DesiredVersion: "4.1.0", State: "enabled"}
	require.NoError(t, k.db.Where("plugin_id = ? AND target = ?", native.PackageID, "agent").FirstOrCreate(&installation).Error)
	require.NoError(t, k.db.Save(&model.PluginConfiguration{
		InstallationID: installation.ID, Revision: 1, ConfigJSON: document, ConfigHash: strings.Repeat("a", 64),
	}).Error)
}

func (k *servicesKernel) report(t *testing.T, nodeID uint, payload string, observedAt time.Time) {
	t.Helper()
	report, err := systemdreport.Sanitize([]byte(payload))
	require.NoError(t, err)
	stored, err := json.Marshal(report)
	require.NoError(t, err)
	require.NoError(t, k.db.Save(&model.PackageReportState{
		NodeKind: "proxy", NodeID: nodeID, PluginID: native.PackageID, Kind: systemdreport.Kind, Version: "4.1.0",
		PayloadJSON: string(stored), ObservedAt: observedAt, ReceivedAt: observedAt, UpdatedAt: observedAt,
	}).Error)
}

func (k *servicesKernel) get(t *testing.T, path string, principal pluginhostsdk.Principal) (int, map[string]json.RawMessage) {
	t.Helper()
	svc := &native.Service{
		Open: func(ctx context.Context) (*gorm.DB, error) { return k.db.WithContext(ctx), nil },
		Now:  func() time.Time { return k.now },
	}
	response, err := svc.NodeServices(context.Background(), pluginhostsdk.NativeRequest{
		RouteID: native.NodesRouteID, Method: "GET", Principal: principal, Metadata: pluginhostsdk.RequestMetadata{Path: path},
	})
	require.NoError(t, err)
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(response.Body, &body))
	return int(response.StatusCode), body
}

const servicesPayload = `{"supported": true, "window_seconds": 600, "units": [
	{"name": "nginx.service", "active_state": "active", "sub_state": "running", "cpu_avg_percent": 1.5, "cpu_peak_percent": 12.25, "memory_bytes": 52428800, "memory_peak_bytes": 73400320},
	{"name": "cron.service", "active_state": "inactive", "sub_state": "dead"},
	{"name": "backup.service", "active_state": "failed", "sub_state": "failed", "memory_bytes": 4096, "memory_peak_bytes": 8192},
	{"name": "nginx-debug.service", "active_state": "activating", "sub_state": "start"}
]}`

// An enabled node's table: the latest report with the node's globs applied,
// counted by state.
func TestServicesRouteAnswersAnEnabledNodesTable(t *testing.T) {
	kernel := newServicesKernel(t)
	kernel.configure(t, `{"interval_seconds":30,"systemd_services":{"nodes":{"7":{"enabled":true,"exclude":["*-debug.service"]}}}}`)
	kernel.report(t, 7, servicesPayload, kernel.now.Add(-5*time.Minute))
	kernel.report(t, 8, servicesPayload, kernel.now)

	status, body := kernel.get(t, "/api/v3/plugins/machine-telemetry/nodes/7/services", admin)
	require.Equal(t, 200, status)
	require.JSONEq(t, `{
		"node_id": 7, "enabled": true, "include": [], "exclude": ["*-debug.service"],
		"reported": true, "supported": true, "unsupported_reason": "", "stale": false,
		"observed_at": "2026-10-03T11:55:00Z", "version": "4.1.0", "window_seconds": 600,
		"summary": {"total": 3, "failed": 1, "active": 1, "inactive": 1},
		"units": [
			{"name": "backup.service", "active_state": "failed", "sub_state": "failed", "cpu_avg_percent": 0, "cpu_peak_percent": 0, "memory_bytes": 4096, "memory_peak_bytes": 8192},
			{"name": "cron.service", "active_state": "inactive", "sub_state": "dead", "cpu_avg_percent": 0, "cpu_peak_percent": 0, "memory_bytes": 0, "memory_peak_bytes": 0},
			{"name": "nginx.service", "active_state": "active", "sub_state": "running", "cpu_avg_percent": 1.5, "cpu_peak_percent": 12.25, "memory_bytes": 52428800, "memory_peak_bytes": 73400320}
		]}`, string(body["data"]))
}

// A node that is not enabled shows no units, even with a stored report; a
// node without settings at all is off too.
func TestServicesRouteShowsNothingForANodeThatIsNotEnabled(t *testing.T) {
	kernel := newServicesKernel(t)
	status, body := kernel.get(t, "/api/v3/plugins/machine-telemetry/nodes/7/services", admin)
	require.Equal(t, 200, status)
	require.JSONEq(t, `{"node_id": 7, "enabled": false, "include": [], "exclude": [], "reported": false, "supported": false,
		"unsupported_reason": "", "stale": false, "observed_at": null, "version": "", "window_seconds": 0,
		"summary": {"total": 0, "failed": 0, "active": 0, "inactive": 0}, "units": []}`, string(body["data"]))

	kernel.configure(t, `{"systemd_services":{"nodes":{"7":{"enabled":false,"include":["nginx*"]}}}}`)
	kernel.report(t, 7, servicesPayload, kernel.now)
	_, body = kernel.get(t, "/api/v3/plugins/machine-telemetry/nodes/7/services", admin)
	var answer native.NodeServices
	require.NoError(t, json.Unmarshal(body["data"], &answer))
	require.False(t, answer.Enabled)
	require.Equal(t, []string{"nginx*"}, answer.Include)
	require.Empty(t, answer.Units)
	require.False(t, answer.Reported)
}

// An enabled node without a report yet, a stale report and an unsupported
// node.
func TestServicesRouteReportsMissingStaleAndUnsupportedTables(t *testing.T) {
	kernel := newServicesKernel(t)
	kernel.configure(t, `{"systemd_services":{"nodes":{"7":{"enabled":true},"9":{"enabled":true}}}}`)
	_, body := kernel.get(t, "/api/v3/plugins/machine-telemetry/nodes/7/services", admin)
	var answer native.NodeServices
	require.NoError(t, json.Unmarshal(body["data"], &answer))
	require.True(t, answer.Enabled)
	require.False(t, answer.Reported)
	require.Nil(t, answer.ObservedAt)

	kernel.report(t, 7, servicesPayload, kernel.now.Add(-26*time.Minute))
	_, body = kernel.get(t, "/api/v3/plugins/machine-telemetry/nodes/7/services", admin)
	answer = native.NodeServices{}
	require.NoError(t, json.Unmarshal(body["data"], &answer))
	require.True(t, answer.Stale)
	require.Len(t, answer.Units, 4)

	kernel.report(t, 9, `{"supported": false, "unsupported_reason": "cgroup v1", "units": []}`, kernel.now)
	_, body = kernel.get(t, "/api/v3/plugins/machine-telemetry/nodes/9/services", admin)
	answer = native.NodeServices{}
	require.NoError(t, json.Unmarshal(body["data"], &answer))
	require.True(t, answer.Reported)
	require.False(t, answer.Supported)
	require.Equal(t, "cgroup v1", answer.UnsupportedReason)
	require.Empty(t, answer.Units)
}

// The node id is a decimal proxy node id without leading zeros; the route is
// GET only and for administrators.
func TestServicesRouteRefusesMalformedRequests(t *testing.T) {
	kernel := newServicesKernel(t)
	for _, path := range []string{
		"/api/v3/plugins/machine-telemetry/nodes/0/services",
		"/api/v3/plugins/machine-telemetry/nodes/07/services",
		"/api/v3/plugins/machine-telemetry/nodes/-1/services",
		"/api/v3/plugins/machine-telemetry/nodes/abc/services",
		"/api/v3/plugins/machine-telemetry/nodes/4294967296/services",
		"/api/v3/plugins/machine-telemetry/nodes/7",
		"/api/v3/plugins/machine-telemetry/nodes/7/services/",
		"/api/v3/plugins/machine-telemetry/nodes/7/services/extra",
		"/api/v3/plugins/machine-telemetry/nodes",
	} {
		status, body := kernel.get(t, path, admin)
		require.Equal(t, 404, status, path)
		require.Contains(t, string(body["error"]), "not_found", path)
	}
	status, _ := kernel.get(t, "/api/v3/plugins/machine-telemetry/nodes/7/services", pluginhostsdk.Principal{ActorID: 2})
	require.Equal(t, 403, status)

	svc := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) { return kernel.db.WithContext(ctx), nil }}
	response, err := svc.NodeServices(context.Background(), pluginhostsdk.NativeRequest{
		Method: "POST", Principal: admin, Metadata: pluginhostsdk.RequestMetadata{Path: "/api/v3/plugins/machine-telemetry/nodes/7/services"},
	})
	require.NoError(t, err)
	require.EqualValues(t, 405, response.StatusCode)
}
