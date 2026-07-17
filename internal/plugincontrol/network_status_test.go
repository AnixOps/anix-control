package plugincontrol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
)

func TestNftablesForwardStatusExecutorReportsRealAssignments(t *testing.T) {
	db := newGostMeshTestDB(t)
	now := time.Unix(1_800_100_000, 0).UTC()
	node := model.Node{Name: "cn-dedicated-one", Host: "10.0.0.10", APIKey: "nft-status", Status: model.NodeStatusOnline, RuntimeHealthy: true}
	require.NoError(t, db.Create(&node).Error)
	assignment := model.NodeServiceAssignment{
		NodeID: node.ID, ServiceScope: "forward", PluginID: NftablesForwardPluginID, Role: "cn_dedicated_nftables",
		DesiredVersion: NftablesForwardVersion, DesiredConfigRevision: 4, Enabled: true,
	}
	require.NoError(t, db.Create(&assignment).Error)
	require.NoError(t, db.Create(&model.NodeOperationRevision{NodeID: node.ID, DesiredRevision: 4, ObservedRevision: 4}).Error)
	nodeID := node.ID
	createGostMeshOperation(t, db, model.KernelOperation{
		ID: "nft-current", IdempotencyKey: "nft-current", NodeID: &nodeID, PluginID: NftablesForwardPluginID,
		TargetVersion: NftablesForwardVersion, Kind: "plugin.configure", Revision: 4, State: "succeeded",
		ConfigJSON: `{"family":"inet","table":"anixops_forward","rules":[` +
			`{"id":"dedicated-443","protocol":"tcp+udp","listen_address":"0.0.0.0","listen_port":443,"target_address":"198.51.100.20","target_port":443,"comment":"Shanghai 443"},` +
			`{"id":"dedicated-dns","protocol":"udp","listen_address":"0.0.0.0","listen_port":53,"target_address":"198.51.100.53","target_port":53}]}`,
		ResultJSON: `{"observed_version":"` + NftablesForwardVersion + `","observed_revision":4,"health":"healthy"}`,
		CreatedAt:  now.Add(-time.Minute), UpdatedAt: now.Add(-time.Minute),
	})
	require.NoError(t, db.Create(&model.NodePluginObservedState{
		NodeID: node.ID, PluginID: NftablesForwardPluginID, Version: NftablesForwardVersion,
		DesiredRevision: 4, ObservedRevision: 4, ConfigHash: "nft-status-config", Health: "healthy",
		RulesetSHA256: "nft-status-ruleset", CountersJSON: `[{"rule_id":"dedicated-443","packets":12,"bytes":2048},{"rule_id":"dedicated-dns","packets":4,"bytes":128}]`,
		ObservedAt: now, ReceivedAt: now,
	}).Error)

	executor := NewNftablesForwardExecutor(db)
	executor.now = func() time.Time { return now }
	response, err := executor.HandleRoute(context.Background(), RouteRequest{
		Method: http.MethodGet, Path: NftablesForwardStatusRoute, Query: url.Values{"limit": []string{"10"}},
	})
	require.NoError(t, err)
	status := response.Data.(NftablesForwardStatus)
	require.Equal(t, NftablesForwardSummary{Rules: 2, Ready: 2}, status.Summary)
	require.Equal(t, now, status.GeneratedAt)
	require.Equal(t, "dedicated-443", status.Rules[0].ID)
	require.Equal(t, "Shanghai 443", status.Rules[0].Name)
	require.Equal(t, "tcp+udp", status.Rules[0].Protocol)
	require.Equal(t, "0.0.0.0:443", status.Rules[0].Listen)
	require.Equal(t, "198.51.100.20:443", status.Rules[0].Target)
	require.True(t, status.Rules[0].Ready)
	require.Equal(t, uint64(12), status.Rules[0].Packets)
	require.Equal(t, uint64(2048), status.Rules[0].Bytes)

	// A previously healthy heartbeat must not leave the WebUI in Ready forever
	// when the Agent has stopped reporting runtime evidence.
	require.NoError(t, db.Model(&model.NodePluginObservedState{}).
		Where("node_id = ? AND plugin_id = ?", node.ID, NftablesForwardPluginID).
		Update("received_at", now.Add(-nftablesForwardRuntimeStaleAfter)).Error)
	response, err = executor.HandleRoute(context.Background(), RouteRequest{
		Method: http.MethodGet, Path: NftablesForwardStatusRoute, Query: url.Values{"limit": []string{"10"}},
	})
	require.NoError(t, err)
	status = response.Data.(NftablesForwardStatus)
	require.Equal(t, NftablesForwardSummary{Rules: 2, Degraded: 2}, status.Summary)
	for _, rule := range status.Rules {
		require.False(t, rule.Ready)
		require.True(t, rule.Degraded)
		require.Equal(t, "runtime observation is stale", rule.LastError)
	}
}

func TestNatEgressStatusExecutorReportsRollbackAndCleanup(t *testing.T) {
	db := newGostMeshTestDB(t)
	now := time.Unix(1_800_100_000, 0).UTC()
	node := model.Node{Name: "hk-egress-01", Host: "10.0.0.20", APIKey: "nat-status", Status: model.NodeStatusOnline, RuntimeHealthy: false, RuntimeError: "runtime failed"}
	require.NoError(t, db.Create(&node).Error)
	assignment := model.NodeServiceAssignment{
		NodeID: node.ID, ServiceScope: "forward", PluginID: NatEgressPluginID, Role: "nat_egress",
		DesiredVersion: NatEgressVersion, DesiredConfigRevision: 7, Enabled: true,
	}
	require.NoError(t, db.Create(&assignment).Error)
	require.NoError(t, db.Create(&model.NodeOperationRevision{NodeID: node.ID, DesiredRevision: 7, ObservedRevision: 6}).Error)
	nodeID := node.ID
	createGostMeshOperation(t, db, model.KernelOperation{
		ID: "nat-current", IdempotencyKey: "nat-current", NodeID: &nodeID, PluginID: NatEgressPluginID,
		TargetVersion: NatEgressVersion, Kind: "plugin.update", Revision: 7, State: "failed",
		ConfigJSON: `{"egress_interface":"eth0","policy_table":100,"health_check_target":"203.0.113.20:443"}`,
		ResultJSON: `{"observed_version":"0.9.0","observed_revision":6,"health":"unhealthy","cleanup_pending":true,"last_error":"cleanup incomplete"}`,
		LastError:  "update failed", CreatedAt: now.Add(-time.Minute), UpdatedAt: now.Add(-time.Minute),
	})

	executor := NewNatEgressExecutor(db)
	executor.now = func() time.Time { return now }
	response, err := executor.HandleRoute(context.Background(), RouteRequest{Method: http.MethodGet, Path: NatEgressStatusRoute})
	require.NoError(t, err)
	status := response.Data.(NatEgressStatus)
	require.Equal(t, NatEgressSummary{Exits: 1, Degraded: 1, RollbackRequired: 1}, status.Summary)
	require.Len(t, status.Exits, 1)
	exit := status.Exits[0]
	require.Equal(t, "hk-egress-01", exit.NodeName)
	require.Equal(t, "eth0", exit.EgressInterface)
	require.Equal(t, "203.0.113.20", exit.PublicIP)
	require.Equal(t, 100, exit.PolicyTable)
	require.True(t, exit.CleanupPending)
	require.True(t, exit.RollbackRequired)
	require.True(t, exit.Degraded)
	require.Equal(t, "cleanup incomplete", exit.LastError)
}

func TestNetworkStatusExecutorContractsAndRegistry(t *testing.T) {
	db := newGostMeshTestDB(t)
	for _, executor := range []Executor{NewNftablesForwardExecutor(db), NewNatEgressExecutor(db)} {
		_, err := executor.HandleRoute(context.Background(), RouteRequest{Method: http.MethodPost, Path: statusRouteFor(executor.PluginID())})
		require.ErrorIs(t, err, ErrMethodNotAllowed)
		_, err = executor.HandleRoute(context.Background(), RouteRequest{Method: http.MethodGet, Path: "/missing"})
		require.ErrorIs(t, err, ErrRouteNotFound)
		configured, err := executor.ExecuteLifecycle(context.Background(), LifecycleRequest{Kind: "plugin.configure", Config: json.RawMessage(`{"apply":false}`)})
		require.NoError(t, err)
		require.JSONEq(t, `{"apply":false}`, string(configured))
	}
	_, err := NewNftablesForwardExecutor(nil).HandleRoute(context.Background(), RouteRequest{Method: http.MethodGet, Path: NftablesForwardStatusRoute})
	require.EqualError(t, err, "nftables-forward database is not initialized")
	_, err = NewNatEgressExecutor(nil).HandleRoute(context.Background(), RouteRequest{Method: http.MethodGet, Path: NatEgressStatusRoute})
	require.EqualError(t, err, "nat-egress database is not initialized")

	registry, err := DefaultRegistry(db)
	require.NoError(t, err)
	_, ok := registry.Lookup(NftablesForwardPluginID, NftablesForwardVersion)
	require.True(t, ok)
	_, ok = registry.Lookup(NatEgressPluginID, NatEgressVersion)
	require.True(t, ok)
	for _, route := range []struct {
		pluginID string
		version  string
		path     string
	}{
		{NftablesForwardPluginID, NftablesForwardVersion, NftablesForwardStatusRoute},
		{NatEgressPluginID, NatEgressVersion, NatEgressStatusRoute},
	} {
		response, err := registry.ExecuteRoute(context.Background(), route.pluginID, route.version, RouteRequest{Method: http.MethodGet, Path: route.path})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.Status)
	}
}

func statusRouteFor(pluginID string) string {
	if pluginID == NftablesForwardPluginID {
		return NftablesForwardStatusRoute
	}
	return NatEgressStatusRoute
}
