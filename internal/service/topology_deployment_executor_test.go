package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
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
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manifest := PluginManifest{
		ID: pluginID, Name: pluginID, Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, ArtifactSHA256: strings.Repeat("a", 64),
	}
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	_, err = RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
}

type topologyDeploymentFixture struct {
	db       *gorm.DB
	topology model.Topology
	revision model.TopologyRevision
	nodes    []model.Node
}

type observedTopologyDeploymentFixture struct {
	db       *gorm.DB
	topology model.Topology
	revision model.TopologyRevision
	node     model.Node
}

func newObservedTopologyDeploymentFixture(t *testing.T) observedTopologyDeploymentFixture {
	t.Helper()
	db := newTopologyDeploymentExecutorTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manifest := PluginManifest{
		ID: "nftables-forward", Name: "nftables Forward", Version: "1.2.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, ArtifactSHA256: strings.Repeat("a", 64), Capabilities: []string{"kernel.observed-state"},
	}
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	_, err = RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	node := model.Node{Name: "observed-topology-node", Host: "10.88.0.1", APIKey: "observed-topology-key"}
	require.NoError(t, db.Create(&node).Error)
	topology := model.Topology{Name: "observed-topology", ServiceScope: "forward"}
	require.NoError(t, db.Create(&topology).Error)
	revision := model.TopologyRevision{TopologyID: topology.ID, Revision: 1, State: "draft", ContentHash: strings.Repeat("b", 64), CreatedBy: 1}
	require.NoError(t, db.Create(&revision).Error)
	require.NoError(t, db.Create(&model.NodeServiceAssignment{
		NodeID: node.ID, ServiceScope: "forward", PluginID: manifest.ID, Role: "cn_dedicated_nftables", DesiredVersion: manifest.Version, Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&model.TopologyVertex{
		RevisionID: revision.ID, Key: "nft-entry", Kind: "plugin", NodeID: &node.ID, PluginID: manifest.ID, Role: "cn_dedicated_nftables",
		ConfigJSON: `{"rules":[{"id":"tcp-443","protocol":"tcp","listen_address":"198.51.100.10","listen_port":443,"target_address":"203.0.113.10","target_port":8443}]}`,
	}).Error)
	return observedTopologyDeploymentFixture{db: db, topology: topology, revision: revision, node: node}
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
	var operation model.KernelOperation
	require.NoError(t, db.First(&operation, "id = ?", operationID).Error)
	updates := map[string]any{"state": state, "last_error": "simulated Agent result", "observed_at": time.Now()}
	if state == "succeeded" {
		enabled, health := true, "healthy"
		configHash := operation.ConfigHash
		if operation.Kind == "plugin.configure" {
			enabled, health = false, "installed"
		} else if operation.Kind == "plugin.disable" {
			enabled, health = false, "disabled"
		} else if operation.Kind == "plugin.enable" && operation.TopologyStepID != nil {
			var configured model.KernelOperation
			require.NoError(t, db.Where("topology_step_id = ? AND kind = ? AND revision < ?", *operation.TopologyStepID, "plugin.configure", operation.Revision).
				Order("revision DESC").First(&configured).Error)
			configHash = configured.ConfigHash
		}
		result, err := json.Marshal(map[string]any{
			"id": operation.PluginID, "desired_version": operation.TargetVersion,
			"observed_version": operation.TargetVersion, "enabled": enabled, "health": health,
			"desired_revision": operation.Revision, "observed_revision": operation.Revision,
			"config_hash": configHash, "last_error": "",
		})
		require.NoError(t, err)
		updates["result_json"] = string(result)
		updates["last_error"] = ""
	}
	require.NoError(t, db.Model(&operation).Updates(updates).Error)
}

func prepareObservedTopologyForHealthGate(t *testing.T, fixture observedTopologyDeploymentFixture, executor *TopologyDeploymentExecutor) (*model.TopologyDeployment, model.TopologyDeploymentStep, model.KernelOperation, model.KernelOperation) {
	t.Helper()
	deployment, _, err := PlanTopologyDeployment(fixture.db, TopologyDeploymentPlanInput{
		TopologyID: fixture.topology.ID, RevisionID: fixture.revision.ID, ActorID: 1,
	})
	require.NoError(t, err)
	_, err = RequestTopologyDeploymentApply(fixture.db, deployment.ID, time.Now())
	require.NoError(t, err)
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	step := topologyDeploymentStep(t, fixture.db, deployment.ID, "nft-entry")
	topologyDeploymentOperationState(t, fixture.db, step.ConfigureOperationID, "succeeded")
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	step = topologyDeploymentStep(t, fixture.db, deployment.ID, "nft-entry")
	topologyDeploymentOperationState(t, fixture.db, step.EnableOperationID, "succeeded")
	var configure, enable model.KernelOperation
	require.NoError(t, fixture.db.First(&configure, "id = ?", step.ConfigureOperationID).Error)
	require.NoError(t, fixture.db.First(&enable, "id = ?", step.EnableOperationID).Error)
	return deployment, step, configure, enable
}

func persistMatchingTopologyRuntimeObservation(t *testing.T, db *gorm.DB, step model.TopologyDeploymentStep, configure, enable model.KernelOperation) {
	t.Helper()
	observedAt := time.Now().Add(time.Millisecond)
	if enable.ObservedAt != nil {
		observedAt = enable.ObservedAt.Add(time.Millisecond)
	}
	require.NoError(t, db.Create(&model.NodePluginObservedState{
		NodeID: step.NodeID, PluginID: step.PluginID, Version: step.TargetVersion,
		DesiredRevision: enable.Revision, ObservedRevision: enable.Revision, ConfigHash: configure.ConfigHash,
		Health: "healthy", RulesetSHA256: strings.Repeat("d", 64),
		CountersJSON: `[{"rule_id":"tcp-443","packets":1,"bytes":128}]`, ObservedAt: observedAt, ReceivedAt: observedAt,
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
	require.Contains(t, status.Deployment.LastError, "plugin.configure ended in failed")
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
	var observedHealth topologyPromotionNodeHealth
	require.NoError(t, json.Unmarshal([]byte(status.Observed[0].HealthJSON), &observedHealth))
	require.True(t, observedHealth.Healthy)
	require.Equal(t, "agent_operation", observedHealth.Source)
	require.Len(t, observedHealth.Steps, 1)
	require.Equal(t, "healthy", observedHealth.Steps[0].State.Health)
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

func TestTopologyDeploymentPromotionRejectsUnhealthyOrMissingAgentState(t *testing.T) {
	tests := []struct {
		name   string
		result func(model.KernelOperation) string
	}{
		{name: "missing result"},
		{name: "unhealthy result", result: func(operation model.KernelOperation) string {
			document, err := json.Marshal(map[string]any{
				"id": operation.PluginID, "desired_version": operation.TargetVersion,
				"observed_version": operation.TargetVersion, "enabled": true, "health": "unhealthy",
				"desired_revision": operation.Revision, "observed_revision": operation.Revision,
				"last_error": "kernel ruleset mismatch",
			})
			require.NoError(t, err)
			return string(document)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRolloutTopologyFixture(t, 1)
			deployment, _, err := PlanTopologyDeployment(fixture.db, TopologyDeploymentPlanInput{
				TopologyID: fixture.topology.ID, RevisionID: fixture.revision.ID, ActorID: 1, RolloutGroup: "canary",
			})
			require.NoError(t, err)
			_, err = RequestTopologyDeploymentApply(fixture.db, deployment.ID, time.Now())
			require.NoError(t, err)
			executor, err := NewTopologyDeploymentExecutor(fixture.db)
			require.NoError(t, err)
			_, err = executor.RunOnce(context.Background())
			require.NoError(t, err)

			step := topologyDeploymentStep(t, fixture.db, deployment.ID, "vertex-1")
			topologyDeploymentOperationState(t, fixture.db, step.ConfigureOperationID, "succeeded")
			_, err = executor.RunOnce(context.Background())
			require.NoError(t, err)
			step = topologyDeploymentStep(t, fixture.db, deployment.ID, "vertex-1")
			require.NotEmpty(t, step.EnableOperationID)
			var operation model.KernelOperation
			require.NoError(t, fixture.db.First(&operation, "id = ?", step.EnableOperationID).Error)
			resultJSON := ""
			if test.result != nil {
				resultJSON = test.result(operation)
			}
			require.NoError(t, fixture.db.Model(&operation).Updates(map[string]any{
				"state": "succeeded", "result_json": resultJSON, "last_error": "",
			}).Error)

			_, err = executor.RunOnce(context.Background())
			require.NoError(t, err)
			status, err := GetTopologyDeploymentStatus(fixture.db, deployment.ID)
			require.NoError(t, err)
			require.Equal(t, topologyDeploymentStateRollbackRequested, status.Deployment.State)
			require.Contains(t, status.Deployment.LastError, "kernel-observed health gate failed")
			require.Empty(t, status.Observed)

			var topology model.Topology
			require.NoError(t, fixture.db.First(&topology, fixture.topology.ID).Error)
			require.NotNil(t, topology.ActiveRevisionID)
			require.Equal(t, fixture.oldRevision.ID, *topology.ActiveRevisionID)
		})
	}
}

func TestTopologyDeploymentWaitsForFreshSignedRuntimeObservationThenPromotes(t *testing.T) {
	fixture := newObservedTopologyDeploymentFixture(t)
	executor, err := NewTopologyDeploymentExecutor(fixture.db)
	require.NoError(t, err)
	deployment, step, configure, enable := prepareObservedTopologyForHealthGate(t, fixture, executor)

	// A terminal enable is not enough for an observed-state package. The first
	// pass starts a durable grace window instead of rolling back immediately.
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	var waiting model.TopologyDeployment
	require.NoError(t, fixture.db.First(&waiting, deployment.ID).Error)
	require.Equal(t, topologyDeploymentStateApplying, waiting.State)
	require.NotNil(t, waiting.HealthGateDeadlineAt)

	persistMatchingTopologyRuntimeObservation(t, fixture.db, step, configure, enable)
	// Unknown Agent fields are intentionally discarded by the fixed projection.
	var state map[string]any
	require.NoError(t, json.Unmarshal([]byte(enable.ResultJSON), &state))
	state["secret"] = "must-not-reach-observed-state"
	mutated, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, fixture.db.Model(&enable).Update("result_json", string(mutated)).Error)
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	status, err := GetTopologyDeploymentStatus(fixture.db, deployment.ID)
	require.NoError(t, err)
	require.Equal(t, topologyDeploymentStateSucceeded, status.Deployment.State)
	require.Len(t, status.Observed, 1)
	require.NotContains(t, status.Observed[0].HealthJSON, "secret")
	var health topologyPromotionNodeHealth
	require.NoError(t, json.Unmarshal([]byte(status.Observed[0].HealthJSON), &health))
	require.Len(t, health.Steps, 1)
	require.NotNil(t, health.Steps[0].Runtime)
	require.Equal(t, strings.Repeat("d", 64), health.Steps[0].Runtime.RulesetSHA256)
	require.NotContains(t, status.Observed[0].HealthJSON, "must-not-reach-observed-state")
}

func TestTopologyDeploymentObservedStateMismatchRollsBackAndMissingStateTimesOut(t *testing.T) {
	t.Run("mismatch", func(t *testing.T) {
		fixture := newObservedTopologyDeploymentFixture(t)
		executor, err := NewTopologyDeploymentExecutor(fixture.db)
		require.NoError(t, err)
		deployment, step, configure, enable := prepareObservedTopologyForHealthGate(t, fixture, executor)
		persistMatchingTopologyRuntimeObservation(t, fixture.db, step, configure, enable)
		require.NoError(t, fixture.db.Model(&model.NodePluginObservedState{}).
			Where("node_id = ? AND plugin_id = ?", step.NodeID, step.PluginID).
			Update("config_hash", strings.Repeat("e", 64)).Error)
		_, err = executor.RunOnce(context.Background())
		require.NoError(t, err)
		status, err := GetTopologyDeploymentStatus(fixture.db, deployment.ID)
		require.NoError(t, err)
		require.Equal(t, topologyDeploymentStateRollbackRequested, status.Deployment.State)
		require.Contains(t, status.Deployment.LastError, "does not match terminal desired state")
	})

	t.Run("deadline", func(t *testing.T) {
		fixture := newObservedTopologyDeploymentFixture(t)
		executor, err := NewTopologyDeploymentExecutor(fixture.db)
		require.NoError(t, err)
		deployment, _, _, _ := prepareObservedTopologyForHealthGate(t, fixture, executor)
		clock := time.Now()
		executor.now = func() time.Time { return clock }
		_, err = executor.RunOnce(context.Background())
		require.NoError(t, err)
		var waiting model.TopologyDeployment
		require.NoError(t, fixture.db.First(&waiting, deployment.ID).Error)
		require.NotNil(t, waiting.HealthGateDeadlineAt)
		clock = waiting.HealthGateDeadlineAt.Add(time.Millisecond)
		_, err = executor.RunOnce(context.Background())
		require.NoError(t, err)
		status, err := GetTopologyDeploymentStatus(fixture.db, deployment.ID)
		require.NoError(t, err)
		require.Equal(t, topologyDeploymentStateRollbackRequested, status.Deployment.State)
		require.Contains(t, status.Deployment.LastError, "health gate timed out")
	})
}

func TestTopologyRuntimeObservationRejectsStaleEvidenceAfterTerminalOperation(t *testing.T) {
	fixture := newObservedTopologyDeploymentFixture(t)
	executor, err := NewTopologyDeploymentExecutor(fixture.db)
	require.NoError(t, err)
	_, step, configure, enable := prepareObservedTopologyForHealthGate(t, fixture, executor)
	persistMatchingTopologyRuntimeObservation(t, fixture.db, step, configure, enable)

	terminalAt := *enable.ObservedAt
	observedAt := terminalAt.Add(time.Millisecond)
	require.NoError(t, fixture.db.Model(&model.NodePluginObservedState{}).
		Where("node_id = ? AND plugin_id = ?", step.NodeID, step.PluginID).
		Updates(map[string]any{"observed_at": observedAt, "received_at": observedAt}).Error)

	_, err = topologyRuntimeObservation(
		fixture.db, step, enable, configure.ConfigHash, step.ConfigJSON,
		terminalAt.Add(topologyRuntimeObservationFreshness+2*time.Millisecond),
	)
	_, pending := topologyHealthGatePending(err)
	require.True(t, pending, err)
}

func TestTopologyRollbackRuntimeEvidenceUsesPreviousNftablesRules(t *testing.T) {
	fixture := newObservedTopologyDeploymentFixture(t)
	deployment := model.TopologyDeployment{
		TopologyID: fixture.topology.ID, RevisionID: fixture.revision.ID,
		State: topologyDeploymentStateRollbackRequested, FailurePolicy: "stop_and_rollback", CreatedBy: 1,
	}
	require.NoError(t, fixture.db.Create(&deployment).Error)
	oldConfig := `{"rules":[{"id":"old-udp-53","protocol":"udp","listen_address":"198.51.100.10","listen_port":53,"target_address":"203.0.113.10","target_port":53}]}`
	newConfig := `{"rules":[{"id":"new-tcp-443","protocol":"tcp","listen_address":"198.51.100.10","listen_port":443,"target_address":"203.0.113.10","target_port":8443}]}`
	step := model.TopologyDeploymentStep{
		DeploymentID: deployment.ID, VertexID: 1, VertexKey: "nft-entry", NodeID: fixture.node.ID,
		PluginID: "nftables-forward", Role: "cn_dedicated_nftables", TargetVersion: "1.2.0",
		ApplyOrder: 1, ApplyAction: topologyApplyActionConfigureEnable, ConfigJSON: newConfig,
		RollbackMode: topologyRollbackModeRestore, RollbackConfigJSON: oldConfig,
		State:                        topologyStepStateRolledBack,
		RollbackConfigureOperationID: "restore-configure", RollbackEnableOperationID: "restore-enable",
	}
	require.NoError(t, fixture.db.Create(&step).Error)

	configHash := strings.Repeat("a", 64)
	terminalAt := time.Now().UTC()
	configure := model.KernelOperation{
		ID: "restore-configure", IdempotencyKey: "restore-configure-key", NodeID: &fixture.node.ID,
		PluginID: step.PluginID, TargetVersion: step.TargetVersion, Kind: "plugin.configure", Revision: 9,
		TopologyDeploymentID: &deployment.ID, TopologyStepID: &step.ID, TopologyRevision: fixture.revision.Revision,
		State: "succeeded", ConfigJSON: oldConfig, ConfigHash: configHash, ObservedAt: &terminalAt,
	}
	enabled := true
	desiredRevision := uint64(10)
	enableResult, err := json.Marshal(topologyAgentPluginResult{topologyPromotionAgentPluginState: topologyPromotionAgentPluginState{
		ID: step.PluginID, DesiredVersion: step.TargetVersion, ObservedVersion: step.TargetVersion,
		Enabled: &enabled, Health: "healthy", DesiredRevision: &desiredRevision, ObservedRevision: &desiredRevision, ConfigHash: configHash,
	}})
	require.NoError(t, err)
	enable := model.KernelOperation{
		ID: "restore-enable", IdempotencyKey: "restore-enable-key", NodeID: &fixture.node.ID,
		PluginID: step.PluginID, TargetVersion: step.TargetVersion, Kind: "plugin.enable", Revision: int64(desiredRevision),
		TopologyDeploymentID: &deployment.ID, TopologyStepID: &step.ID, TopologyRevision: fixture.revision.Revision,
		State: "succeeded", ConfigJSON: oldConfig, ResultJSON: string(enableResult), ObservedAt: &terminalAt,
	}
	require.NoError(t, fixture.db.Create(&configure).Error)
	require.NoError(t, fixture.db.Create(&enable).Error)
	observedAt := terminalAt.Add(time.Millisecond)
	require.NoError(t, fixture.db.Create(&model.NodePluginObservedState{
		NodeID: fixture.node.ID, PluginID: step.PluginID, Version: step.TargetVersion,
		DesiredRevision: int64(desiredRevision), ObservedRevision: int64(desiredRevision), ConfigHash: configHash,
		Health: "healthy", RulesetSHA256: strings.Repeat("b", 64),
		CountersJSON: `[{"rule_id":"old-udp-53","packets":2,"bytes":128}]`, ObservedAt: observedAt, ReceivedAt: observedAt,
	}).Error)

	health, err := topologyRollbackHealthByNode(fixture.db, []model.TopologyDeploymentStep{step}, observedAt.Add(time.Second))
	require.NoError(t, err)
	require.Contains(t, health, fixture.node.ID)
}

func TestTopologyObservedStateCapabilityCannotBeRemovedFromStoredManifest(t *testing.T) {
	fixture := newObservedTopologyDeploymentFixture(t)
	_, steps, err := PlanTopologyDeployment(fixture.db, TopologyDeploymentPlanInput{
		TopologyID: fixture.topology.ID, RevisionID: fixture.revision.ID, ActorID: 1,
	})
	require.NoError(t, err)
	require.Len(t, steps, 1)

	var release model.PluginRelease
	require.NoError(t, fixture.db.First(&release, "plugin_id = ? AND version = ?", "nftables-forward", "1.2.0").Error)
	manifest, err := DecodePluginReleaseManifest(release)
	require.NoError(t, err)
	manifest.Capabilities = nil
	canonical, err := CanonicalPluginManifest(*manifest)
	require.NoError(t, err)
	// Deliberately retain the former signature. A caller cannot downgrade the
	// gate by changing only stored manifest JSON after release registration.
	require.NoError(t, fixture.db.Model(&release).Update("manifest_json", string(canonical)).Error)

	requires, err := topologyStepRequiresRuntimeObservation(fixture.db, steps[0])
	require.ErrorContains(t, err, "verify topology step")
	require.False(t, requires)
}

func TestManualTopologyRollbackSkipsUnstartedStepsAndResetsHealthGate(t *testing.T) {
	fixture := newTopologyDeploymentFixture(t, []model.TopologyEdge{{SourceKey: "entry", TargetKey: "exit", Protocol: "tcp", ConfigJSON: `{}`}})
	deployment, steps, err := PlanTopologyDeployment(fixture.db, TopologyDeploymentPlanInput{
		TopologyID: fixture.topology.ID, RevisionID: fixture.revision.ID, ActorID: 1,
	})
	require.NoError(t, err)
	require.NotEmpty(t, steps)
	deadline := time.Now().Add(topologyRuntimeObservationGrace)
	require.NoError(t, fixture.db.Model(deployment).Update("health_gate_deadline_at", deadline).Error)

	_, err = RequestTopologyDeploymentRollback(fixture.db, deployment.ID, time.Now())
	require.NoError(t, err)
	var persisted model.TopologyDeployment
	require.NoError(t, fixture.db.First(&persisted, deployment.ID).Error)
	require.Equal(t, topologyDeploymentStateRollbackRequested, persisted.State)
	require.Nil(t, persisted.HealthGateDeadlineAt)
	var persistedSteps []model.TopologyDeploymentStep
	require.NoError(t, fixture.db.Where("deployment_id = ?", deployment.ID).Find(&persistedSteps).Error)
	for _, step := range persistedSteps {
		require.Equal(t, topologyStepStateSkipped, step.State)
	}

	executor, err := NewTopologyDeploymentExecutor(fixture.db)
	require.NoError(t, err)
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	status, err := GetTopologyDeploymentStatus(fixture.db, deployment.ID)
	require.NoError(t, err)
	require.Equal(t, topologyDeploymentStateRolledBack, status.Deployment.State)
	var operationCount int64
	require.NoError(t, fixture.db.Model(&model.KernelOperation{}).Where("topology_deployment_id = ?", deployment.ID).Count(&operationCount).Error)
	require.Zero(t, operationCount)
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
