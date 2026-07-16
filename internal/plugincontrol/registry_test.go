package plugincontrol

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type registryTestExecutor struct {
	id      string
	version string
}

func (e registryTestExecutor) PluginID() string { return e.id }
func (e registryTestExecutor) Version() string  { return e.version }
func (e registryTestExecutor) HandleRoute(_ context.Context, _ RouteRequest) (RouteResponse, error) {
	return RouteResponse{Status: http.StatusOK, Data: map[string]any{"version": e.version}}, nil
}
func (e registryTestExecutor) ExecuteLifecycle(_ context.Context, request LifecycleRequest) (json.RawMessage, error) {
	return json.Marshal(map[string]string{"kind": request.Kind, "version": e.version})
}

func TestRegistryRequiresExactPluginVersion(t *testing.T) {
	registry, err := NewRegistry(registryTestExecutor{id: "example", version: "1.0.0"})
	require.NoError(t, err)

	response, err := registry.ExecuteRoute(context.Background(), "example", "1.0.0", RouteRequest{})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.Status)

	_, err = registry.ExecuteRoute(context.Background(), "example", "2.0.0", RouteRequest{})
	require.ErrorIs(t, err, ErrExecutorNotFound)
	require.Error(t, registry.Register(registryTestExecutor{id: "example", version: "1.0.0"}))
}

func TestMachineTelemetryExecutorReturnsBoundedHeartbeatMetrics(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Node{}))
	now := time.Unix(1_800_000_000, 0).UTC()
	recent := now.Add(-time.Minute).Unix()
	stale := now.Add(-10 * time.Minute).Unix()
	nodes := []model.Node{
		{Name: "online", Host: "10.0.0.1", APIKey: "key-online", Status: model.NodeStatusOnline, LastCheckAt: &recent, RuntimeHealthy: true, CPUUsage: 12.5, MemoryUsage: 42, DiskUsage: 17},
		{Name: "offline", Host: "10.0.0.2", APIKey: "key-offline", Status: model.NodeStatusOnline, LastCheckAt: &stale, RuntimeHealthy: true},
		{Name: "disabled", Host: "10.0.0.3", APIKey: "key-disabled", Status: model.NodeStatusDisabled, LastCheckAt: &recent, RuntimeHealthy: true},
	}
	for index := range nodes {
		require.NoError(t, db.Create(&nodes[index]).Error)
	}
	require.NoError(t, db.Model(&nodes[1]).Updates(map[string]any{"runtime_healthy": false, "runtime_error": "process stopped"}).Error)

	executor := NewMachineTelemetryExecutor(db)
	executor.now = func() time.Time { return now }
	response, err := executor.HandleRoute(context.Background(), RouteRequest{
		Method: http.MethodGet, Path: MachineTelemetryStatusRoute, Query: url.Values{"limit": []string{"10"}},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.Status)
	status, ok := response.Data.(MachineTelemetryStatus)
	require.True(t, ok)
	require.Equal(t, MachineTelemetryPluginID, status.PluginID)
	require.Equal(t, MachineTelemetryVersion, status.Version)
	require.Equal(t, MachineTelemetrySummary{Total: 3, Online: 1, Offline: 1, Disabled: 1, Unhealthy: 1}, status.Summary)
	require.Len(t, status.Nodes, 3)
	require.Equal(t, 12.5, status.Nodes[0].CPUUsage)
	require.False(t, status.Nodes[1].RuntimeHealthy)
	require.Equal(t, "process stopped", status.Nodes[1].RuntimeError)

	_, err = executor.HandleRoute(context.Background(), RouteRequest{Method: http.MethodPost, Path: MachineTelemetryStatusRoute})
	require.ErrorIs(t, err, ErrMethodNotAllowed)
	_, err = executor.HandleRoute(context.Background(), RouteRequest{Method: http.MethodGet, Path: MachineTelemetryStatusRoute, Query: url.Values{"limit": []string{"0"}}})
	require.ErrorIs(t, err, ErrInvalidPluginInput)
	_, err = executor.HandleRoute(context.Background(), RouteRequest{Method: http.MethodGet, Path: "/api/v3/plugins/machine-telemetry/missing"})
	require.ErrorIs(t, err, ErrRouteNotFound)
}

func TestMachineTelemetryLifecycleIsIdempotentAndReadOnly(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Node{}))
	executor := NewMachineTelemetryExecutor(db)
	executor.now = func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }

	first, err := executor.ExecuteLifecycle(context.Background(), LifecycleRequest{
		OperationID: "op-1", Kind: "plugin.enable", Target: MachineTelemetryVersion, Config: json.RawMessage(`{"interval_seconds":30}`),
	})
	require.NoError(t, err)
	second, err := executor.ExecuteLifecycle(context.Background(), LifecycleRequest{
		OperationID: "op-1", Kind: "plugin.enable", Target: MachineTelemetryVersion, Config: json.RawMessage(`{"interval_seconds":30}`),
	})
	require.NoError(t, err)
	require.JSONEq(t, string(first), string(second))

	_, err = executor.ExecuteLifecycle(context.Background(), LifecycleRequest{Kind: "plugin.configure", Config: json.RawMessage(`[]`)})
	require.True(t, errors.Is(err, ErrInvalidPluginInput))
}
