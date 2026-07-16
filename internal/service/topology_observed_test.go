package service

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newObservedTopologyFixture(t *testing.T) (*model.TopologyDeployment, uint, *gorm.DB) {
	t.Helper()
	db := newKernelTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Node{}))
	node := model.Node{Name: "observed-node", Host: "127.0.0.1"}
	require.NoError(t, db.Create(&node).Error)
	topology := model.Topology{Name: "observed-topology", ServiceScope: "forward"}
	require.NoError(t, db.Create(&topology).Error)
	revision := model.TopologyRevision{TopologyID: topology.ID, Revision: 1, State: "published", ContentHash: "hash", CreatedBy: 1}
	require.NoError(t, db.Create(&revision).Error)
	require.NoError(t, db.Create(&model.TopologyVertex{RevisionID: revision.ID, Key: "entry", Kind: "plugin", NodeID: &node.ID, PluginID: "nftables-forward", Role: "cn_dedicated_nftables", ConfigJSON: `{}`}).Error)
	deployment := &model.TopologyDeployment{TopologyID: topology.ID, RevisionID: revision.ID, State: "planned", FailurePolicy: "stop_and_rollback", CreatedBy: 1}
	require.NoError(t, db.Create(deployment).Error)
	return deployment, node.ID, db
}

func TestApplyTopologyObservedStateIsMonotonicAndAggregatesDeployment(t *testing.T) {
	deployment, nodeID, db := newObservedTopologyFixture(t)
	firstAt := time.Unix(1_800_000_000, 0).UTC()
	state, changed, err := ApplyTopologyObservedState(db, TopologyObservedStateUpdate{
		DeploymentID: deployment.ID, NodeID: nodeID, DesiredRevision: 1, ObservedRevision: 0,
		State: "applying", HealthJSON: `{"healthy":false}`, ObservedAt: firstAt,
	})
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "applying", state.State)

	completedAt := firstAt.Add(time.Second)
	state, changed, err = ApplyTopologyObservedState(db, TopologyObservedStateUpdate{
		DeploymentID: deployment.ID, NodeID: nodeID, DesiredRevision: 1, ObservedRevision: 1,
		State: "succeeded", HealthJSON: `{"healthy":true}`, ObservedAt: completedAt,
	})
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, int64(1), state.ObservedRevision)
	var storedDeployment model.TopologyDeployment
	require.NoError(t, db.First(&storedDeployment, deployment.ID).Error)
	require.Equal(t, "succeeded", storedDeployment.State)
	require.NotNil(t, storedDeployment.CompletedAt)

	stale, changed, err := ApplyTopologyObservedState(db, TopologyObservedStateUpdate{
		DeploymentID: deployment.ID, NodeID: nodeID, DesiredRevision: 1, ObservedRevision: 0,
		State: "applying", ObservedAt: completedAt.Add(time.Second),
	})
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, int64(1), stale.ObservedRevision)
	require.Equal(t, "succeeded", stale.State)
}

func TestApplyTopologyObservedStateRejectsUnknownNodeAndInvalidHealth(t *testing.T) {
	deployment, nodeID, db := newObservedTopologyFixture(t)
	_, _, err := ApplyTopologyObservedState(db, TopologyObservedStateUpdate{
		DeploymentID: deployment.ID, NodeID: nodeID + 100, DesiredRevision: 1,
		ObservedRevision: 1, State: "succeeded",
	})
	require.ErrorContains(t, err, "not part of topology deployment")

	_, _, err = ApplyTopologyObservedState(db, TopologyObservedStateUpdate{
		DeploymentID: deployment.ID, NodeID: nodeID, DesiredRevision: 1,
		ObservedRevision: 1, State: "succeeded", HealthJSON: "not-json",
	})
	require.ErrorContains(t, err, "health must be valid JSON")
}

func TestApplyTopologyObservedStateWaitsForEveryDeploymentNode(t *testing.T) {
	deployment, firstNodeID, db := newObservedTopologyFixture(t)
	secondNode := model.Node{Name: "observed-node-two", Host: "127.0.0.2", APIKey: "observed-node-two-key"}
	require.NoError(t, db.Create(&secondNode).Error)
	require.NoError(t, db.Create(&model.TopologyVertex{
		RevisionID: deployment.RevisionID, Key: "exit", Kind: "plugin", NodeID: &secondNode.ID,
		PluginID: "nftables-forward", Role: "cn_dedicated_nftables", ConfigJSON: `{}`,
	}).Error)

	_, changed, err := ApplyTopologyObservedState(db, TopologyObservedStateUpdate{
		DeploymentID: deployment.ID, NodeID: firstNodeID, DesiredRevision: 1,
		ObservedRevision: 1, State: "succeeded",
	})
	require.NoError(t, err)
	require.True(t, changed)
	var stored model.TopologyDeployment
	require.NoError(t, db.First(&stored, deployment.ID).Error)
	require.Equal(t, "applying", stored.State)

	_, changed, err = ApplyTopologyObservedState(db, TopologyObservedStateUpdate{
		DeploymentID: deployment.ID, NodeID: secondNode.ID, DesiredRevision: 1,
		ObservedRevision: 1, State: "succeeded",
	})
	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, db.First(&stored, deployment.ID).Error)
	require.Equal(t, "succeeded", stored.State)
}
