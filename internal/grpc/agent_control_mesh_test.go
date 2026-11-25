package grpc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/features"
	"github.com/AnixOps/anix-control/v5/internal/meshregion"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func enableMesh(t *testing.T) {
	t.Helper()
	features.SetVersion("5.0.13.dev.1")
	t.Cleanup(func() { features.SetVersion("") })
}

func meshHello() []*agentv1pb.Capability {
	return append(configCapabilities(), &agentv1pb.Capability{Name: agentcontrol.CapabilityMesh, Version: agentcontrol.CapabilityVersionV1})
}

func meshStatusReport(payload string) *agentv1pb.PackageReport {
	return &agentv1pb.PackageReport{PluginId: meshregion.StatusPluginID, Kind: meshregion.StatusKind, Version: meshregion.StatusVersion,
		ObservedAtUnixMs: time.Now().UnixMilli(), PayloadJson: []byte(payload)}
}

// mesh.v1 is offered only while the feature runs, to proxy and forward
// nodes whose Hello lists it; a forward node gets package-reports.v1 with it.
func TestAgentControlMeshNegotiation(t *testing.T) {
	server := newTestAgentControlServer(NewAgentControlManager())
	listed := func(node agentcontrol.AgentNode, agent []*agentv1pb.Capability, name string) bool {
		return agentcontrol.HasCapabilityVersion(server.serverCapabilities(node, agent), name, agentcontrol.CapabilityVersionV1)
	}
	forwardNode := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 1}
	proxyNode := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}
	features.SetVersion("5.0.13")
	assert.False(t, listed(proxyNode, meshHello(), agentcontrol.CapabilityMesh), "the feature is off in releases")
	enableMesh(t)
	for _, node := range []agentcontrol.AgentNode{forwardNode, proxyNode} {
		assert.True(t, listed(node, meshHello(), agentcontrol.CapabilityMesh), node.String())
		assert.True(t, listed(node, meshHello(), agentcontrol.CapabilityPackageReports), node.String())
		assert.False(t, listed(node, configCapabilities(), agentcontrol.CapabilityMesh), node.String())
	}
}

// A node in an enabled region gets the member "mesh"; a membership change
// pushes its configuration; its mesh status is recorded only on a session
// that negotiated mesh.v1.
func TestAgentControlMeshMemberAndStatus(t *testing.T) {
	enableMesh(t)
	env := newConfigTestEnvironment(t)
	requireAutoMigrate(t, &model.MeshRegion{}, &model.MeshMember{}, &model.MeshStatus{}, &model.ForwardNode{})
	node := env.agentNode()
	db := database.GetDB()
	ctx := context.Background()
	region, err := meshregion.CreateRegion(ctx, db, meshregion.RegionInput{Code: "cn", Name: "中国大陆", CIDR: "10.150.0.0/16", Enabled: true}, time.Now())
	require.NoError(t, err)

	session := openConfigSession(t, env, 0, meshHello()...)
	require.True(t, agentcontrol.HasCapabilityVersion(session.helloAck.ServerCapabilities, agentcontrol.CapabilityMesh, agentcontrol.CapabilityVersionV1))
	first := session.next().GetConfig()
	require.NotNil(t, first)
	assert.NotContains(t, string(first.GetConfigJson()), `"mesh"`, "not in a region yet")

	before := snapshotsSent(configTriggerMesh)
	_, err = meshregion.AddNode(ctx, db, region.ID, node, true, time.Now())
	require.NoError(t, err)
	pushed := session.next().GetConfig()
	require.NotNil(t, pushed, "the membership pushed the node's configuration")
	assert.Equal(t, before+1, snapshotsSent(configTriggerMesh))
	var document struct {
		Mesh struct {
			Networks []map[string]any `json:"networks"`
		} `json:"mesh"`
	}
	require.NoError(t, json.Unmarshal(pushed.GetConfigJson(), &document))
	require.Len(t, document.Mesh.Networks, 1)
	assert.Equal(t, "cn", document.Mesh.Networks[0]["id"])
	assert.Equal(t, "10.150.0.1/16", document.Mesh.Networks[0]["address"])

	accepted := readPackageReportCounts().results[packageReportAccepted]
	session.send(packageReportMessage("", uint32(node.ID), meshStatusReport(`{"networks":[{"id":"cn","running":true}]}`)))
	session.heartbeat()
	assert.Empty(t, session.untilHeartbeatAck(), "never acknowledged")
	assert.Equal(t, accepted+1, readPackageReportCounts().results[packageReportAccepted])
	var status model.MeshStatus
	require.NoError(t, db.First(&status, "node_ref = ?", node.String()).Error)
	assert.JSONEq(t, `{"networks":[{"id":"cn","running":true}]}`, status.Payload)

	bad := readPackageReportCounts().reasons[service.PackageReportRefusedBadPayload]
	session.send(packageReportMessage("", uint32(node.ID), meshStatusReport(`{"peers":[]}`)))
	session.heartbeat()
	session.untilHeartbeatAck()
	assert.Equal(t, bad+1, readPackageReportCounts().reasons[service.PackageReportRefusedBadPayload])
	require.NoError(t, session.stream.CloseSend())

	legacy := openConfigSession(t, env, pushed.GetConfigRevision(), configCapabilities()...)
	unnegotiated := readPackageReportCounts().reasons[service.PackageReportRefusedUnnegotiated]
	legacy.send(packageReportMessage("", uint32(node.ID), meshStatusReport(`{"networks":[]}`)))
	legacy.heartbeat()
	legacy.untilHeartbeatAck()
	assert.Equal(t, unnegotiated+1, readPackageReportCounts().reasons[service.PackageReportRefusedUnnegotiated])
}
