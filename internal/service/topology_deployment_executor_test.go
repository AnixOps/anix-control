package service

import (
	"context"
	"fmt"
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

type rolloutTopologyFixture struct {
	db          *gorm.DB
	topology    model.Topology
	oldRevision model.TopologyRevision
	revision    model.TopologyRevision
	nodes       []model.Node
}

func newRolloutTopologyFixture(t *testing.T, nodeCount int) rolloutTopologyFixture {
	t.Helper()
	db := newTopologyDeploymentExecutorTestDB(t)
	topology := model.Topology{Name: "topology-canary", ServiceScope: "forward"}
	require.NoError(t, db.Create(&topology).Error)
	oldRevision := model.TopologyRevision{
		TopologyID: topology.ID, Revision: 1, State: "active",
		ContentHash: strings.Repeat("e", 64), CreatedBy: 1,
	}
	require.NoError(t, db.Create(&oldRevision).Error)
	require.NoError(t, db.Model(&topology).Update("active_revision_id", oldRevision.ID).Error)
	topology.ActiveRevisionID = &oldRevision.ID
	revision := model.TopologyRevision{
		TopologyID: topology.ID, Revision: 2, State: "draft",
		ContentHash: strings.Repeat("f", 64), CreatedBy: 1,
	}
	require.NoError(t, db.Create(&revision).Error)

	nodes := make([]model.Node, 0, nodeCount)
	vertices := make([]model.TopologyVertex, 0, nodeCount)
	for index := 0; index < nodeCount; index++ {
		pluginID := fmt.Sprintf("canary-plugin-%d", index+1)
		seedTopologyDeploymentAgentRelease(t, db, pluginID)
		node := model.Node{
			Name:   fmt.Sprintf("canary-node-%d", index+1),
			Host:   fmt.Sprintf("10.30.0.%d", index+1),
			APIKey: fmt.Sprintf("canary-node-%d-key", index+1),
		}
		require.NoError(t, db.Create(&node).Error)
		nodes = append(nodes, node)
		rolloutGroup := "stable"
		if index == 0 {
			rolloutGroup = "canary"
		}
		require.NoError(t, db.Create(&model.NodeServiceAssignment{
			NodeID: node.ID, ServiceScope: "forward", PluginID: pluginID,
			Role: fmt.Sprintf("role-%d", index+1), DesiredVersion: "1.0.0",
			Enabled: true, RolloutGroup: rolloutGroup,
		}).Error)
		vertices = append(vertices, model.TopologyVertex{
			RevisionID: revision.ID, Key: fmt.Sprintf("vertex-%d", index+1), Kind: "plugin",
			NodeID: &nodes[index].ID, PluginID: pluginID,
			Role: fmt.Sprintf("role-%d", index+1), ConfigJSON: fmt.Sprintf(`{"listen_port":%d}`, 43000+index),
		})
	}
	require.NoError(t, db.Create(&vertices).Error)
	return rolloutTopologyFixture{db: db, topology: topology, oldRevision: oldRevision, revision: revision, nodes: nodes}
}

func completeTopologyDeploymentSuccessfully(t *testing.T, executor *TopologyDeploymentExecutor, db *gorm.DB, deploymentID uint) {
	t.Helper()
	_, err := executor.RunOnce(context.Background())
	require.NoError(t, err)
	var steps []model.TopologyDeploymentStep
	require.NoError(t, db.Where("deployment_id = ?", deploymentID).Order("apply_order, id").Find(&steps).Error)
	require.NotEmpty(t, steps)
	for _, step := range steps {
		topologyDeploymentOperationState(t, db, step.ConfigureOperationID, "succeeded")
	}
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	require.NoError(t, db.Where("deployment_id = ?", deploymentID).Order("apply_order, id").Find(&steps).Error)
	for _, step := range steps {
		require.NotEmpty(t, step.EnableOperationID)
		topologyDeploymentOperationState(t, db, step.EnableOperationID, "succeeded")
	}
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
}

func TestTopologyDeploymentCanaryDoesNotActivateUntilFullRollout(t *testing.T) {
	fixture := newRolloutTopologyFixture(t, 5)
	canary, canarySteps, err := PlanTopologyDeployment(fixture.db, TopologyDeploymentPlanInput{
		TopologyID: fixture.topology.ID, RevisionID: fixture.revision.ID, ActorID: 1, RolloutGroup: "canary",
	})
	require.NoError(t, err)
	require.Len(t, canarySteps, 1)
	require.Equal(t, fixture.nodes[0].ID, canarySteps[0].NodeID)
	_, err = RequestTopologyDeploymentApply(fixture.db, canary.ID, time.Now())
	require.NoError(t, err)
	executor, err := NewTopologyDeploymentExecutor(fixture.db)
	require.NoError(t, err)
	completeTopologyDeploymentSuccessfully(t, executor, fixture.db, canary.ID)

	status, err := GetTopologyDeploymentStatus(fixture.db, canary.ID)
	require.NoError(t, err)
	require.Equal(t, topologyDeploymentStateSucceeded, status.Deployment.State)
	require.Len(t, status.Observed, 1)
	require.Equal(t, fixture.nodes[0].ID, status.Observed[0].NodeID)
	var topology model.Topology
	require.NoError(t, fixture.db.First(&topology, fixture.topology.ID).Error)
	require.NotNil(t, topology.ActiveRevisionID)
	require.Equal(t, fixture.oldRevision.ID, *topology.ActiveRevisionID)
	var revision model.TopologyRevision
	require.NoError(t, fixture.db.First(&revision, fixture.revision.ID).Error)
	require.Equal(t, "draft", revision.State)
	_, _, err = ApplyTopologyObservedState(fixture.db, TopologyObservedStateUpdate{
		DeploymentID: canary.ID, NodeID: fixture.nodes[1].ID, DesiredRevision: fixture.revision.Revision,
		ObservedRevision: fixture.revision.Revision, State: "succeeded",
	})
	require.ErrorContains(t, err, "not part of topology deployment")

	full, fullSteps, err := PlanTopologyDeployment(fixture.db, TopologyDeploymentPlanInput{
		TopologyID: fixture.topology.ID, RevisionID: fixture.revision.ID, ActorID: 1,
	})
	require.NoError(t, err)
	require.Len(t, fullSteps, 5)
	_, err = RequestTopologyDeploymentApply(fixture.db, full.ID, time.Now())
	require.NoError(t, err)
	completeTopologyDeploymentSuccessfully(t, executor, fixture.db, full.ID)
	status, err = GetTopologyDeploymentStatus(fixture.db, full.ID)
	require.NoError(t, err)
	require.Equal(t, topologyDeploymentStateSucceeded, status.Deployment.State)
	require.Len(t, status.Observed, 5)
	require.NoError(t, fixture.db.First(&topology, fixture.topology.ID).Error)
	require.NotNil(t, topology.ActiveRevisionID)
	require.Equal(t, fixture.revision.ID, *topology.ActiveRevisionID)
	require.NoError(t, fixture.db.First(&revision, fixture.revision.ID).Error)
	require.Equal(t, "active", revision.State)
}

func TestTopologyDeploymentCanaryRejectsDependencyOutsideRolloutGroup(t *testing.T) {
	fixture := newRolloutTopologyFixture(t, 2)
	require.NoError(t, fixture.db.Create(&model.TopologyEdge{
		RevisionID: fixture.revision.ID, SourceKey: "vertex-2", TargetKey: "vertex-1", Protocol: "tcp", ConfigJSON: `{}`,
	}).Error)
	_, _, err := PlanTopologyDeployment(fixture.db, TopologyDeploymentPlanInput{
		TopologyID: fixture.topology.ID, RevisionID: fixture.revision.ID, ActorID: 1, RolloutGroup: "canary",
	})
	require.ErrorContains(t, err, "selects vertex \"vertex-1\" but not its dependency \"vertex-2\"")
}

func TestTopologyDeploymentCanaryCanPlanPureRemoval(t *testing.T) {
	db := newTopologyDeploymentExecutorTestDB(t)
	seedTopologyDeploymentAgentRelease(t, db, "removal-plugin")
	node := model.Node{Name: "removal-node", Host: "10.40.0.1", APIKey: "removal-node-key"}
	require.NoError(t, db.Create(&node).Error)
	topology := model.Topology{Name: "topology-removal", ServiceScope: "forward"}
	require.NoError(t, db.Create(&topology).Error)
	oldRevision := model.TopologyRevision{TopologyID: topology.ID, Revision: 1, State: "active", ContentHash: strings.Repeat("a", 64), CreatedBy: 1}
	newRevision := model.TopologyRevision{TopologyID: topology.ID, Revision: 2, State: "draft", ContentHash: strings.Repeat("b", 64), CreatedBy: 1}
	require.NoError(t, db.Create(&oldRevision).Error)
	require.NoError(t, db.Create(&newRevision).Error)
	require.NoError(t, db.Model(&topology).Update("active_revision_id", oldRevision.ID).Error)
	assignment := model.NodeServiceAssignment{NodeID: node.ID, ServiceScope: "forward", PluginID: "removal-plugin", Role: "entry", DesiredVersion: "1.0.0", Enabled: true, RolloutGroup: "canary"}
	require.NoError(t, db.Create(&assignment).Error)
	require.NoError(t, db.Create(&model.TopologyVertex{
		RevisionID: oldRevision.ID, Key: "old-entry", Kind: "plugin", NodeID: &node.ID,
		PluginID: assignment.PluginID, Role: assignment.Role, ConfigJSON: `{"listen_port":44001}`,
	}).Error)
	deployment, steps, err := PlanTopologyDeployment(db, TopologyDeploymentPlanInput{
		TopologyID: topology.ID, RevisionID: newRevision.ID, ActorID: 1, RolloutGroup: "canary",
	})
	require.NoError(t, err)
	require.NotNil(t, deployment)
	require.Len(t, steps, 1)
	require.True(t, steps[0].Removal)
}

func TestTopologyDeploymentCanaryRollbackOnlyTouchesSelectedNodes(t *testing.T) {
	fixture := newRolloutTopologyFixture(t, 5)
	deployment, steps, err := PlanTopologyDeployment(fixture.db, TopologyDeploymentPlanInput{
		TopologyID: fixture.topology.ID, RevisionID: fixture.revision.ID, ActorID: 1, RolloutGroup: "canary",
	})
	require.NoError(t, err)
	require.Len(t, steps, 1)
	_, err = RequestTopologyDeploymentApply(fixture.db, deployment.ID, time.Now())
	require.NoError(t, err)
	executor, err := NewTopologyDeploymentExecutor(fixture.db)
	require.NoError(t, err)
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	var canaryStep model.TopologyDeploymentStep
	require.NoError(t, fixture.db.First(&canaryStep, "deployment_id = ?", deployment.ID).Error)
	topologyDeploymentOperationState(t, fixture.db, canaryStep.ConfigureOperationID, "failed")
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	require.NoError(t, fixture.db.First(&canaryStep, "deployment_id = ?", deployment.ID).Error)
	require.Equal(t, topologyStepStateRollbackDisabling, canaryStep.State)
	require.NotEmpty(t, canaryStep.RollbackDisableOperationID)
	topologyDeploymentOperationState(t, fixture.db, canaryStep.RollbackDisableOperationID, "succeeded")
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)

	status, err := GetTopologyDeploymentStatus(fixture.db, deployment.ID)
	require.NoError(t, err)
	require.Equal(t, topologyDeploymentStateRolledBack, status.Deployment.State)
	require.Len(t, status.Observed, 1)
	require.Equal(t, fixture.nodes[0].ID, status.Observed[0].NodeID)
	require.Equal(t, "rolled_back", status.Observed[0].State)
	var topology model.Topology
	require.NoError(t, fixture.db.First(&topology, fixture.topology.ID).Error)
	require.NotNil(t, topology.ActiveRevisionID)
	require.Equal(t, fixture.oldRevision.ID, *topology.ActiveRevisionID)
	var observedCount int64
	require.NoError(t, fixture.db.Model(&model.TopologyObservedState{}).Where("deployment_id = ?", deployment.ID).Count(&observedCount).Error)
	require.Equal(t, int64(1), observedCount)
}
