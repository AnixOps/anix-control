package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newTopologyDeploymentExecutorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, EnsureKernelSchema(db))
	require.NoError(t, db.AutoMigrate(&model.Node{}))
	return db
}

func seedTopologyDeploymentAgentRelease(t *testing.T, db *gorm.DB, pluginID string) {
	t.Helper()
	require.NoError(t, db.FirstOrCreate(&model.Plugin{
		ID: pluginID, Name: pluginID, Publisher: "AnixOps", Official: true,
	}, model.Plugin{ID: pluginID}).Error)
	manifest := PluginManifest{
		ID: pluginID, Name: pluginID, Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, ArtifactSHA256: strings.Repeat("a", 64),
	}
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	require.NoError(t, db.FirstOrCreate(&model.PluginRelease{
		PluginID: pluginID, Version: manifest.Version, APIVersion: manifest.APIVersion,
		ManifestJSON: string(canonical), ArtifactSHA256: manifest.ArtifactSHA256, Signature: "topology-test",
	}, model.PluginRelease{PluginID: pluginID, Version: manifest.Version}).Error)
}

type topologyDeploymentFixture struct {
	db       *gorm.DB
	topology model.Topology
	revision model.TopologyRevision
	nodes    []model.Node
}

func newTopologyDeploymentFixture(t *testing.T, edges []model.TopologyEdge) topologyDeploymentFixture {
	t.Helper()
	db := newTopologyDeploymentExecutorTestDB(t)
	seedTopologyDeploymentAgentRelease(t, db, "topology-runtime-a")
	seedTopologyDeploymentAgentRelease(t, db, "topology-runtime-b")
	nodeA := model.Node{Name: "topology-node-a", Host: "10.10.0.1", APIKey: "topology-node-a-key"}
	nodeB := model.Node{Name: "topology-node-b", Host: "10.10.0.2", APIKey: "topology-node-b-key"}
	require.NoError(t, db.Create(&nodeA).Error)
	require.NoError(t, db.Create(&nodeB).Error)
	topology := model.Topology{Name: "topology-executor", ServiceScope: "forward"}
	require.NoError(t, db.Create(&topology).Error)
	revision := model.TopologyRevision{TopologyID: topology.ID, Revision: 1, State: "draft", ContentHash: strings.Repeat("b", 64), CreatedBy: 1}
	require.NoError(t, db.Create(&revision).Error)
	for _, assignment := range []model.NodeServiceAssignment{
		{NodeID: nodeA.ID, ServiceScope: "forward", PluginID: "topology-runtime-a", Role: "entry", DesiredVersion: "1.0.0", Enabled: true},
		{NodeID: nodeB.ID, ServiceScope: "forward", PluginID: "topology-runtime-b", Role: "exit", DesiredVersion: "1.0.0", Enabled: true},
	} {
		require.NoError(t, db.Create(&assignment).Error)
	}
	vertices := []model.TopologyVertex{
		{RevisionID: revision.ID, Key: "entry", Kind: "plugin", NodeID: &nodeA.ID, PluginID: "topology-runtime-a", Role: "entry", ConfigJSON: `{"listen_port":41001}`},
		{RevisionID: revision.ID, Key: "exit", Kind: "plugin", NodeID: &nodeB.ID, PluginID: "topology-runtime-b", Role: "exit", ConfigJSON: `{"listen_port":41002}`},
	}
	require.NoError(t, db.Create(&vertices).Error)
	for index := range edges {
		edges[index].RevisionID = revision.ID
		if edges[index].Protocol == "" {
			edges[index].Protocol = "tcp"
		}
		if edges[index].ConfigJSON == "" {
			edges[index].ConfigJSON = `{}`
		}
	}
	require.NoError(t, db.Create(&edges).Error)
	return topologyDeploymentFixture{db: db, topology: topology, revision: revision, nodes: []model.Node{nodeA, nodeB}}
}

func topologyDeploymentStep(t *testing.T, db *gorm.DB, deploymentID uint, key string) model.TopologyDeploymentStep {
	t.Helper()
	var step model.TopologyDeploymentStep
	require.NoError(t, db.First(&step, "deployment_id = ? AND vertex_key = ?", deploymentID, key).Error)
	return step
}

func topologyDeploymentOperationState(t *testing.T, db *gorm.DB, operationID, state string) {
	t.Helper()
	require.NotEmpty(t, operationID)
	require.NoError(t, db.Model(&model.KernelOperation{}).Where("id = ?", operationID).Updates(map[string]any{
		"state": state, "last_error": "simulated Agent result",
	}).Error)
}

func TestTopologyDeploymentExecutorAppliesDAGThenRollsBackInReverseOrder(t *testing.T) {
	fixture := newTopologyDeploymentFixture(t, []model.TopologyEdge{{SourceKey: "entry", TargetKey: "exit", Protocol: "tcp", ConfigJSON: `{}`}})
	deployment, steps, err := PlanTopologyDeployment(fixture.db, TopologyDeploymentPlanInput{
		TopologyID: fixture.topology.ID, RevisionID: fixture.revision.ID, ActorID: 1,
	})
	require.NoError(t, err)
	require.Len(t, steps, 2)
	_, err = RequestTopologyDeploymentApply(fixture.db, deployment.ID, time.Now())
	require.NoError(t, err)
	executor, err := NewTopologyDeploymentExecutor(fixture.db)
	require.NoError(t, err)

	// entry.configure
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	entry := topologyDeploymentStep(t, fixture.db, deployment.ID, "entry")
	exit := topologyDeploymentStep(t, fixture.db, deployment.ID, "exit")
	require.Equal(t, topologyStepStateConfiguring, entry.State)
	require.Empty(t, exit.ConfigureOperationID)
	topologyDeploymentOperationState(t, fixture.db, entry.ConfigureOperationID, "succeeded")

	// entry.enable
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	entry = topologyDeploymentStep(t, fixture.db, deployment.ID, "entry")
	require.Equal(t, topologyStepStateEnabling, entry.State)
	topologyDeploymentOperationState(t, fixture.db, entry.EnableOperationID, "succeeded")

	// exit is not expanded until entry has reached a terminal success.
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	exit = topologyDeploymentStep(t, fixture.db, deployment.ID, "exit")
	require.Equal(t, topologyStepStateConfiguring, exit.State)
	topologyDeploymentOperationState(t, fixture.db, exit.ConfigureOperationID, "succeeded")
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	exit = topologyDeploymentStep(t, fixture.db, deployment.ID, "exit")
	require.Equal(t, topologyStepStateEnabling, exit.State)
	topologyDeploymentOperationState(t, fixture.db, exit.EnableOperationID, "succeeded")
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)

	status, err := GetTopologyDeploymentStatus(fixture.db, deployment.ID)
	require.NoError(t, err)
	require.Equal(t, topologyDeploymentStateSucceeded, status.Deployment.State)
	require.Len(t, status.Observed, 2)
	for _, observed := range status.Observed {
		require.Equal(t, "succeeded", observed.State)
		require.Equal(t, fixture.revision.Revision, observed.DesiredRevision)
	}

	// A manual rollback must compensate in reverse topological order: exit
	// before entry. New vertices have no previous topology to restore, so the
	// safe compensation operation is plugin.disable.
	_, err = RequestTopologyDeploymentRollback(fixture.db, deployment.ID, time.Now())
	require.NoError(t, err)
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	exit = topologyDeploymentStep(t, fixture.db, deployment.ID, "exit")
	entry = topologyDeploymentStep(t, fixture.db, deployment.ID, "entry")
	require.Equal(t, topologyStepStateRollbackDisabling, exit.State)
	require.NotEmpty(t, exit.RollbackDisableOperationID)
	require.Empty(t, entry.RollbackDisableOperationID)
	topologyDeploymentOperationState(t, fixture.db, exit.RollbackDisableOperationID, "succeeded")
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	entry = topologyDeploymentStep(t, fixture.db, deployment.ID, "entry")
	require.Equal(t, topologyStepStateRollbackDisabling, entry.State)
	topologyDeploymentOperationState(t, fixture.db, entry.RollbackDisableOperationID, "succeeded")
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)

	status, err = GetTopologyDeploymentStatus(fixture.db, deployment.ID)
	require.NoError(t, err)
	require.Equal(t, topologyDeploymentStateRolledBack, status.Deployment.State)
	for _, observed := range status.Observed {
		require.Equal(t, "rolled_back", observed.State)
	}
}

func TestTopologyDeploymentExecutorFailureStopsExpansionAndCompensates(t *testing.T) {
	fixture := newTopologyDeploymentFixture(t, []model.TopologyEdge{{SourceKey: "entry", TargetKey: "exit", Protocol: "tcp", ConfigJSON: `{}`}})
	deployment, _, err := PlanTopologyDeployment(fixture.db, TopologyDeploymentPlanInput{TopologyID: fixture.topology.ID, RevisionID: fixture.revision.ID, ActorID: 1})
	require.NoError(t, err)
	_, err = RequestTopologyDeploymentApply(fixture.db, deployment.ID, time.Now())
	require.NoError(t, err)
	executor, err := NewTopologyDeploymentExecutor(fixture.db)
	require.NoError(t, err)
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	entry := topologyDeploymentStep(t, fixture.db, deployment.ID, "entry")
	topologyDeploymentOperationState(t, fixture.db, entry.ConfigureOperationID, "failed")

	// The failed root transitions directly into rollback; exit never gets a
	// configure operation even though it is otherwise valid and assigned.
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	exit := topologyDeploymentStep(t, fixture.db, deployment.ID, "exit")
	entry = topologyDeploymentStep(t, fixture.db, deployment.ID, "entry")
	require.Equal(t, topologyStepStateSkipped, exit.State)
	require.Empty(t, exit.ConfigureOperationID)
	require.Equal(t, topologyStepStateRollbackDisabling, entry.State)
	topologyDeploymentOperationState(t, fixture.db, entry.RollbackDisableOperationID, "succeeded")
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)

	status, err := GetTopologyDeploymentStatus(fixture.db, deployment.ID)
	require.NoError(t, err)
	require.Equal(t, topologyDeploymentStateRolledBack, status.Deployment.State)
	require.Contains(t, status.Deployment.LastError, "simulated Agent result")
}

func TestPlanTopologyDeploymentCapturesPreviousConfigForCompensation(t *testing.T) {
	db := newTopologyDeploymentExecutorTestDB(t)
	seedTopologyDeploymentAgentRelease(t, db, "topology-runtime-a")
	node := model.Node{Name: "topology-restore-node", Host: "10.20.0.1", APIKey: "topology-restore-key"}
	require.NoError(t, db.Create(&node).Error)
	topology := model.Topology{Name: "topology-restore", ServiceScope: "forward"}
	require.NoError(t, db.Create(&topology).Error)
	oldRevision := model.TopologyRevision{TopologyID: topology.ID, Revision: 1, State: "active", ContentHash: strings.Repeat("c", 64), CreatedBy: 1}
	newRevision := model.TopologyRevision{TopologyID: topology.ID, Revision: 2, State: "draft", ContentHash: strings.Repeat("d", 64), CreatedBy: 1}
	require.NoError(t, db.Create(&oldRevision).Error)
	require.NoError(t, db.Create(&newRevision).Error)
	require.NoError(t, db.Model(&topology).Update("active_revision_id", oldRevision.ID).Error)
	topology.ActiveRevisionID = &oldRevision.ID
	assignment := model.NodeServiceAssignment{NodeID: node.ID, ServiceScope: "forward", PluginID: "topology-runtime-a", Role: "entry", DesiredVersion: "1.0.0", Enabled: true}
	require.NoError(t, db.Create(&assignment).Error)
	oldVertex := model.TopologyVertex{RevisionID: oldRevision.ID, Key: "entry", Kind: "plugin", NodeID: &node.ID, PluginID: assignment.PluginID, Role: assignment.Role, ConfigJSON: `{"listen_port":42001}`}
	newVertex := model.TopologyVertex{RevisionID: newRevision.ID, Key: "entry", Kind: "plugin", NodeID: &node.ID, PluginID: assignment.PluginID, Role: assignment.Role, ConfigJSON: `{"listen_port":42002}`}
	require.NoError(t, db.Create(&oldVertex).Error)
	require.NoError(t, db.Create(&newVertex).Error)
	deployment, steps, err := PlanTopologyDeployment(db, TopologyDeploymentPlanInput{TopologyID: topology.ID, RevisionID: newRevision.ID, ActorID: 1})
	require.NoError(t, err)
	require.Equal(t, oldRevision.ID, *deployment.PreviousRevisionID)
	require.Len(t, steps, 1)
	require.Equal(t, topologyRollbackModeRestore, steps[0].RollbackMode)
	require.JSONEq(t, oldVertex.ConfigJSON, steps[0].RollbackConfigJSON)
}
