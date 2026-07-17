package plugincontrol

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newGostMeshTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(
		&model.Node{},
		&model.NodeServiceAssignment{},
		&model.KernelOperation{},
		&model.NodeOperationRevision{},
		&model.NodePluginObservedState{},
	))
	return db
}

func createGostMeshOperation(t *testing.T, db *gorm.DB, operation model.KernelOperation) {
	t.Helper()
	require.NoError(t, db.Create(&operation).Error)
}

func TestGostMeshExecutorRouteAggregatesObservedTunnelState(t *testing.T) {
	db := newGostMeshTestDB(t)
	now := time.Unix(1_800_000_000, 0).UTC()
	nodes := []model.Node{
		{Name: "dual-role", Host: "10.0.0.1", APIKey: "gost-dual", Status: model.NodeStatusOnline, RuntimeHealthy: true},
		{Name: "reconciling", Host: "10.0.0.2", APIKey: "gost-reconciling", Status: model.NodeStatusOnline, RuntimeHealthy: true},
		{Name: "cleanup", Host: "10.0.0.3", APIKey: "gost-cleanup", Status: model.NodeStatusOnline, RuntimeHealthy: true, RuntimeError: "node runtime failed"},
	}
	for index := range nodes {
		require.NoError(t, db.Create(&nodes[index]).Error)
	}
	assignments := []model.NodeServiceAssignment{
		{NodeID: nodes[0].ID, ServiceScope: "forward", PluginID: GostMeshPluginID, Role: "entry", DesiredVersion: GostMeshVersion, DesiredConfigRevision: 5, Enabled: true},
		{NodeID: nodes[0].ID, ServiceScope: "forward", PluginID: GostMeshPluginID, Role: "exit", DesiredVersion: GostMeshVersion, DesiredConfigRevision: 5, Enabled: true},
		{NodeID: nodes[1].ID, ServiceScope: "forward", PluginID: GostMeshPluginID, Role: "entry", DesiredVersion: GostMeshVersion, DesiredConfigRevision: 2, Enabled: true},
		{NodeID: nodes[2].ID, ServiceScope: "forward", PluginID: GostMeshPluginID, Role: "exit", DesiredVersion: GostMeshVersion, DesiredConfigRevision: 4, Enabled: true},
	}
	for index := range assignments {
		require.NoError(t, db.Create(&assignments[index]).Error)
	}
	for _, cursor := range []model.NodeOperationRevision{
		{NodeID: nodes[0].ID, DesiredRevision: 5, ObservedRevision: 5},
		{NodeID: nodes[1].ID, DesiredRevision: 2, ObservedRevision: 1},
		{NodeID: nodes[2].ID, DesiredRevision: 4, ObservedRevision: 3},
	} {
		require.NoError(t, db.Create(&cursor).Error)
	}

	nodeID := nodes[0].ID
	createGostMeshOperation(t, db, model.KernelOperation{
		ID: "dual-old", IdempotencyKey: "dual-old", NodeID: &nodeID, PluginID: GostMeshPluginID,
		TargetVersion: "0.9.0", Kind: "plugin.update", Revision: 4, State: "failed",
		ConfigJSON: `{"tunnels":[{"id":"stale","role":"entry","transport":"wss"}]}`,
		LastError:  "stale failure", CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour),
	})
	createGostMeshOperation(t, db, model.KernelOperation{
		ID: "dual-current", IdempotencyKey: "dual-current", NodeID: &nodeID, PluginID: GostMeshPluginID,
		TargetVersion: GostMeshVersion, Kind: "plugin.configure", Revision: 5, State: "succeeded",
		ConfigJSON: `{"tunnels":[` +
			`{"id":"dual-entry","role":"entry","transport":"quic","listen":{"address":"0.0.0.0","port":8443},"remote":{"host":"198.51.100.10","port":443}},` +
			`{"id":"dual-exit","role":"exit","transport":"wss","listen":{"address":"::","port":9443}}]}`,
		ResultJSON: `{"observed_revision":5,"health":"healthy"}`,
		CreatedAt:  now.Add(-time.Hour),
		UpdatedAt:  now.Add(-time.Hour),
	})
	nodeID = nodes[1].ID
	createGostMeshOperation(t, db, model.KernelOperation{
		ID: "reconciling-current", IdempotencyKey: "reconciling-current", NodeID: &nodeID, PluginID: GostMeshPluginID,
		TargetVersion: GostMeshVersion, Kind: "plugin.configure", Revision: 2, State: "running",
		ConfigJSON: `{"tunnels":[{"id":"pending-entry","role":"entry","transport":"wss","listen":{"address":"127.0.0.1","port":10443},"remote":{"host":"203.0.113.20","port":443}}]}`,
		ResultJSON: `{"observed_version":"1.0.0","observed_revision":1,"health":"healthy"}`,
		CreatedAt:  now.Add(-30 * time.Minute),
		UpdatedAt:  now.Add(-30 * time.Minute),
	})
	nodeID = nodes[2].ID
	createGostMeshOperation(t, db, model.KernelOperation{
		ID: "cleanup-current", IdempotencyKey: "cleanup-current", NodeID: &nodeID, PluginID: GostMeshPluginID,
		TargetVersion: GostMeshVersion, Kind: "plugin.update", Revision: 4, State: "failed",
		ConfigJSON: `{"tunnels":[{"id":"failed-exit","role":"exit","transport":"quic","listen":{"address":"0.0.0.0","port":11443}}]}`,
		ResultJSON: `{"observed_version":"0.9.0","observed_revision":3,"health":"unhealthy","cleanup_pending":true,"last_error":"cleanup incomplete"}`,
		LastError:  "operation failed",
		CreatedAt:  now.Add(-15 * time.Minute),
		UpdatedAt:  now.Add(-15 * time.Minute),
	})

	executor := NewGostMeshExecutor(db)
	executor.now = func() time.Time { return now }
	response, err := executor.HandleRoute(context.Background(), RouteRequest{
		Method: http.MethodGet,
		Path:   GostMeshStatusRoute,
		Query:  url.Values{"limit": []string{"10"}},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.Status)
	status, ok := response.Data.(GostMeshStatus)
	require.True(t, ok)
	require.Equal(t, GostMeshPluginID, status.PluginID)
	require.Equal(t, GostMeshVersion, status.Version)
	require.Equal(t, now, status.GeneratedAt)
	require.Equal(t, GostMeshSummary{Tunnels: 4, Ready: 2, Reconciling: 1, CleanupPending: 1, RollbackRequired: 1}, status.Summary)
	require.Len(t, status.Tunnels, 4, "dual-role assignments must not multiply every aggregate tunnel")

	byID := make(map[string]GostMeshTunnelStatus, len(status.Tunnels))
	for _, tunnel := range status.Tunnels {
		byID[tunnel.ID] = tunnel
	}
	entry := byID["dual-entry"]
	require.True(t, entry.Ready)
	require.Equal(t, "dual-role", entry.EntryNode)
	require.Empty(t, entry.ExitNode)
	require.Equal(t, "quic", entry.Transport)
	require.Equal(t, "0.0.0.0:8443", entry.Listen)
	require.Equal(t, "198.51.100.10:443", entry.Upstream)
	require.Equal(t, GostMeshVersion, entry.ObservedVersion, "a successful operation falls back to its target version")
	require.EqualValues(t, 5, entry.ObservedRevision)

	exit := byID["dual-exit"]
	require.True(t, exit.Ready)
	require.Equal(t, "dual-role", exit.ExitNode)
	require.Equal(t, "[::]:9443", exit.Listen)

	reconciling := byID["pending-entry"]
	require.True(t, reconciling.Reconciling)
	require.False(t, reconciling.Ready)
	require.EqualValues(t, 1, reconciling.ObservedRevision)

	cleanup := byID["failed-exit"]
	require.True(t, cleanup.CleanupPending)
	require.True(t, cleanup.RollbackRequired)
	require.False(t, cleanup.Reconciling)
	require.False(t, cleanup.Ready)
	require.Equal(t, "cleanup incomplete", cleanup.LastError)

	limited, err := executor.HandleRoute(context.Background(), RouteRequest{
		Method: http.MethodGet,
		Path:   GostMeshStatusRoute,
		Query:  url.Values{"limit": []string{"2"}},
	})
	require.NoError(t, err)
	limitedStatus := limited.Data.(GostMeshStatus)
	require.Len(t, limitedStatus.Tunnels, 2)
	require.Equal(t, GostMeshSummary{Tunnels: 2, Ready: 2}, limitedStatus.Summary)
}

func TestGostMeshExecutorRouteContractAndBounds(t *testing.T) {
	executor := NewGostMeshExecutor(newGostMeshTestDB(t))

	_, err := executor.HandleRoute(context.Background(), RouteRequest{Method: http.MethodGet, Path: "/api/v3/plugins/gost-mesh/missing"})
	require.ErrorIs(t, err, ErrRouteNotFound)
	_, err = executor.HandleRoute(context.Background(), RouteRequest{Method: http.MethodPost, Path: GostMeshStatusRoute})
	require.ErrorIs(t, err, ErrMethodNotAllowed)
	for _, limit := range []string{"0", "501", "not-a-number"} {
		_, err = executor.HandleRoute(context.Background(), RouteRequest{
			Method: http.MethodGet, Path: GostMeshStatusRoute, Query: url.Values{"limit": []string{limit}},
		})
		require.ErrorIs(t, err, ErrInvalidPluginInput)
	}
	_, err = NewGostMeshExecutor(nil).HandleRoute(context.Background(), RouteRequest{Method: http.MethodGet, Path: GostMeshStatusRoute})
	require.EqualError(t, err, "gost-mesh database is not initialized")
}

func TestGostMeshLifecycleIsDeterministicReadOnlyAndValidatesInput(t *testing.T) {
	db := newGostMeshTestDB(t)
	executor := NewGostMeshExecutor(db)
	executor.now = func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }

	installed, err := executor.ExecuteLifecycle(context.Background(), LifecycleRequest{Kind: "plugin.install"})
	require.NoError(t, err)
	require.JSONEq(t, `{"state":"installed"}`, string(installed))
	disabled, err := executor.ExecuteLifecycle(context.Background(), LifecycleRequest{Kind: "plugin.disable"})
	require.NoError(t, err)
	require.JSONEq(t, `{"state":"disabled"}`, string(disabled))
	configured, err := executor.ExecuteLifecycle(context.Background(), LifecycleRequest{
		Kind: "plugin.configure", Config: json.RawMessage(`{"tunnels":[]}`),
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"tunnels":[]}`, string(configured))

	for _, kind := range []string{"plugin.enable", "plugin.update", "plugin.rollback", "plugin.health", "plugin.inspect"} {
		first, err := executor.ExecuteLifecycle(context.Background(), LifecycleRequest{OperationID: "same-operation", Kind: kind, Config: json.RawMessage(`{}`)})
		require.NoError(t, err)
		second, err := executor.ExecuteLifecycle(context.Background(), LifecycleRequest{OperationID: "same-operation", Kind: kind, Config: json.RawMessage(`{}`)})
		require.NoError(t, err)
		require.JSONEq(t, string(first), string(second))
		var status GostMeshStatus
		require.NoError(t, json.Unmarshal(first, &status))
		require.Equal(t, GostMeshPluginID, status.PluginID)
		require.Equal(t, GostMeshVersion, status.Version)
		require.Empty(t, status.Tunnels)
	}

	for _, invalid := range []json.RawMessage{json.RawMessage(`[]`), json.RawMessage(`null`), json.RawMessage(`{"broken"`)} {
		_, err = executor.ExecuteLifecycle(context.Background(), LifecycleRequest{Kind: "plugin.configure", Config: invalid})
		require.ErrorIs(t, err, ErrInvalidPluginInput)
	}
	_, err = executor.ExecuteLifecycle(context.Background(), LifecycleRequest{Kind: "plugin.purge", Config: json.RawMessage(`{}`)})
	require.EqualError(t, err, "unsupported gost-mesh lifecycle operation")

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = executor.ExecuteLifecycle(cancelled, LifecycleRequest{Kind: "plugin.inspect", Config: json.RawMessage(`{}`)})
	require.True(t, errors.Is(err, context.Canceled))
}

func TestDefaultRegistryIncludesGostMeshExecutor(t *testing.T) {
	_, err := DefaultRegistry(nil)
	require.EqualError(t, err, "control plugin registry requires a database")

	db := newGostMeshTestDB(t)
	registry, err := DefaultRegistry(db)
	require.NoError(t, err)
	sameRegistry, err := DefaultRegistry(db)
	require.NoError(t, err)
	require.Same(t, registry, sameRegistry)

	executor, ok := registry.Lookup(GostMeshPluginID, GostMeshVersion)
	require.True(t, ok)
	gostExecutor, ok := executor.(*GostMeshExecutor)
	require.True(t, ok)
	require.Same(t, db, gostExecutor.db)
	_, ok = registry.Lookup(MachineTelemetryPluginID, MachineTelemetryVersion)
	require.True(t, ok)

	response, err := registry.ExecuteRoute(context.Background(), GostMeshPluginID, GostMeshVersion, RouteRequest{
		Method: http.MethodGet, Path: GostMeshStatusRoute,
	})
	require.NoError(t, err)
	status := response.Data.(GostMeshStatus)
	require.Equal(t, GostMeshSummary{}, status.Summary)
}
