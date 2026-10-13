package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// activeTopologyFixture is the two-node executor fixture with its revision
// already active on the nodes, as a successful deployment leaves it.
func activeTopologyFixture(t *testing.T) topologyDeploymentFixture {
	t.Helper()
	fixture := newTopologyDeploymentFixture(t, []model.TopologyEdge{{SourceKey: "entry", TargetKey: "exit", Protocol: "tcp", ConfigJSON: `{}`}})
	require.NoError(t, fixture.db.Model(&fixture.topology).Update("active_revision_id", fixture.revision.ID).Error)
	require.NoError(t, fixture.db.Model(&fixture.revision).Update("state", "active").Error)
	fixture.topology.ActiveRevisionID = &fixture.revision.ID
	return fixture
}

func unscopedTopology(t *testing.T, db *gorm.DB, id uint) model.Topology {
	t.Helper()
	var topology model.Topology
	require.NoError(t, db.Unscoped().First(&topology, id).Error)
	return topology
}

func topologyDeploymentState(t *testing.T, db *gorm.DB, id uint) model.TopologyDeployment {
	t.Helper()
	var deployment model.TopologyDeployment
	require.NoError(t, db.First(&deployment, id).Error)
	return deployment
}

// driveTopologyDeployment runs the executor and answers every pending
// operation of the deployment with success until the deployment is terminal.
func driveTopologyDeployment(t *testing.T, executor *TopologyDeploymentExecutor, db *gorm.DB, deploymentID uint) model.TopologyDeployment {
	t.Helper()
	for range 20 {
		_, err := executor.RunOnce(context.Background())
		require.NoError(t, err)
		deployment := topologyDeploymentState(t, db, deploymentID)
		if deployment.State != topologyDeploymentStateApplying && deployment.State != topologyDeploymentStateRollbackRequested {
			return deployment
		}
		var pending []model.KernelOperation
		require.NoError(t, db.Where("topology_deployment_id = ? AND state = ?", deploymentID, "pending").Find(&pending).Error)
		for _, operation := range pending {
			topologyDeploymentOperationState(t, db, operation.ID, "succeeded")
		}
	}
	t.Fatalf("topology deployment %d did not finish", deploymentID)
	return model.TopologyDeployment{}
}

func TestDeleteTopologyWithoutActiveRevisionFlagsItAndFreesItsName(t *testing.T) {
	db := newTopologyDeploymentExecutorTestDB(t)
	topology := model.Topology{Name: "edge-mesh", ServiceScope: "forward"}
	require.NoError(t, db.Create(&topology).Error)
	revision := model.TopologyRevision{TopologyID: topology.ID, Revision: 1, State: "draft", ContentHash: strings.Repeat("a", 64), CreatedBy: 1}
	require.NoError(t, db.Create(&revision).Error)
	at := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)

	result, err := DeleteTopology(db, TopologyDeleteInput{TopologyID: topology.ID, ActorID: 1, At: at})
	require.NoError(t, err)
	require.True(t, result.Deleted)
	require.Nil(t, result.Deployment)
	require.Equal(t, topology.ID, result.Topology.ID)
	require.NotNil(t, result.Topology.DeletedAt)

	// The row stays, flagged.
	stored := unscopedTopology(t, db, topology.ID)
	require.EqualValues(t, 1, stored.Deleted)
	require.NotNil(t, stored.DeletedAt)
	require.True(t, stored.DeletedAt.Equal(at))
	require.Nil(t, stored.DeleteRequestedAt)
	var flagged int64
	require.NoError(t, db.Unscoped().Model(&model.Topology{}).Where("id = ? AND deleted = ?", topology.ID, 1).Count(&flagged).Error)
	require.EqualValues(t, 1, flagged)
	var revisions int64
	require.NoError(t, db.Model(&model.TopologyRevision{}).Where("topology_id = ?", topology.ID).Count(&revisions).Error)
	require.EqualValues(t, 1, revisions, "revisions are kept for audit")

	// Every read path sees it as missing.
	_, err = GetTopology(db, topology.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	var live []model.Topology
	require.NoError(t, db.Find(&live).Error)
	require.Empty(t, live)
	_, err = DeleteTopology(db, TopologyDeleteInput{TopologyID: topology.ID, ActorID: 1})
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = CreateTopologyRevision(db, topology.ID, 1, TopologyRevisionInput{Vertices: []model.TopologyVertex{{Key: "a", Kind: "endpoint", ConfigJSON: `{}`}}})
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, _, err = PlanTopologyDeployment(db, TopologyDeploymentPlanInput{TopologyID: topology.ID, RevisionID: revision.ID, ActorID: 1})
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = PreviewTopologyDeployment(db, TopologyDeploymentPreviewInput{TopologyID: topology.ID, RevisionID: revision.ID})
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// The name is free again, but only once among live topologies.
	reused := model.Topology{Name: "edge-mesh", ServiceScope: "forward"}
	require.NoError(t, db.Create(&reused).Error)
	require.NotEqual(t, topology.ID, reused.ID)
	require.Error(t, db.Create(&model.Topology{Name: "edge-mesh", ServiceScope: "forward"}).Error)
	// A second deleted topology with the same name does not collide either.
	_, err = DeleteTopology(db, TopologyDeleteInput{TopologyID: reused.ID, ActorID: 1})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.Topology{Name: "edge-mesh", ServiceScope: "forward"}).Error)
}

func TestDeleteTopologyCancelsPlannedDeploymentsThatCanNoLongerBeApplied(t *testing.T) {
	fixture := newTopologyDeploymentFixture(t, []model.TopologyEdge{{SourceKey: "entry", TargetKey: "exit", Protocol: "tcp", ConfigJSON: `{}`}})
	planned, steps, err := PlanTopologyDeployment(fixture.db, TopologyDeploymentPlanInput{TopologyID: fixture.topology.ID, RevisionID: fixture.revision.ID, ActorID: 1})
	require.NoError(t, err)
	require.Len(t, steps, 2)

	result, err := DeleteTopology(fixture.db, TopologyDeleteInput{TopologyID: fixture.topology.ID, ActorID: 1, ExecutionEnabled: true})
	require.NoError(t, err, "a planned deployment does not block a delete")
	require.True(t, result.Deleted)

	cancelled := topologyDeploymentState(t, fixture.db, planned.ID)
	require.Equal(t, topologyDeploymentStateCancelled, cancelled.State)
	require.NotNil(t, cancelled.CompletedAt)
	var stepStates []string
	require.NoError(t, fixture.db.Model(&model.TopologyDeploymentStep{}).Where("deployment_id = ?", planned.ID).Pluck("state", &stepStates).Error)
	require.Equal(t, []string{topologyStepStateSkipped, topologyStepStateSkipped}, stepStates)

	_, err = RequestTopologyDeploymentApply(fixture.db, planned.ID, time.Now())
	require.ErrorIs(t, err, ErrTopologyDeleted)
	_, err = RequestTopologyDeploymentRollback(fixture.db, planned.ID, time.Now())
	require.ErrorIs(t, err, ErrTopologyDeleted)

	// Even a deployment left planned (written before deletion) is refused.
	require.NoError(t, fixture.db.Model(&model.TopologyDeployment{}).Where("id = ?", planned.ID).Update("state", topologyDeploymentStatePlanned).Error)
	_, err = RequestTopologyDeploymentApply(fixture.db, planned.ID, time.Now())
	require.ErrorIs(t, err, ErrTopologyDeleted)
	require.Equal(t, topologyDeploymentStatePlanned, topologyDeploymentState(t, fixture.db, planned.ID).State)

	// The deployment stays readable for audit.
	status, err := GetTopologyDeploymentStatus(fixture.db, planned.ID)
	require.NoError(t, err)
	require.Len(t, status.Steps, 2)
	public := PublicTopologyDeployment(cancelled)
	require.Equal(t, "deployment was cancelled", public.LastError)
}

func TestDeleteTopologyRefusesWhileADeploymentExecutes(t *testing.T) {
	fixture := newTopologyDeploymentFixture(t, []model.TopologyEdge{{SourceKey: "entry", TargetKey: "exit", Protocol: "tcp", ConfigJSON: `{}`}})
	deployment, _, err := PlanTopologyDeployment(fixture.db, TopologyDeploymentPlanInput{TopologyID: fixture.topology.ID, RevisionID: fixture.revision.ID, ActorID: 1})
	require.NoError(t, err)
	_, err = RequestTopologyDeploymentApply(fixture.db, deployment.ID, time.Now())
	require.NoError(t, err)

	_, err = DeleteTopology(fixture.db, TopologyDeleteInput{TopologyID: fixture.topology.ID, ActorID: 1, ExecutionEnabled: true})
	require.ErrorIs(t, err, ErrTopologyDeploymentBusy)

	_, err = RequestTopologyDeploymentRollback(fixture.db, deployment.ID, time.Now())
	require.NoError(t, err)
	_, err = DeleteTopology(fixture.db, TopologyDeleteInput{TopologyID: fixture.topology.ID, ActorID: 1, ExecutionEnabled: true})
	require.ErrorIs(t, err, ErrTopologyDeploymentBusy, "rollback_requested executes too")

	// Any state the executor does not know as idle counts as executing.
	require.NoError(t, fixture.db.Model(&model.TopologyDeployment{}).Where("id = ?", deployment.ID).Update("state", "running").Error)
	_, err = DeleteTopology(fixture.db, TopologyDeleteInput{TopologyID: fixture.topology.ID, ActorID: 1, ExecutionEnabled: true})
	require.ErrorIs(t, err, ErrTopologyDeploymentBusy)

	stored := unscopedTopology(t, fixture.db, fixture.topology.ID)
	require.Zero(t, stored.Deleted)
	require.Nil(t, stored.DeleteRequestedAt)
	require.Equal(t, "running", topologyDeploymentState(t, fixture.db, deployment.ID).State)

	require.NoError(t, fixture.db.Model(&model.TopologyDeployment{}).Where("id = ?", deployment.ID).Update("state", topologyDeploymentStateRolledBack).Error)
	result, err := DeleteTopology(fixture.db, TopologyDeleteInput{TopologyID: fixture.topology.ID, ActorID: 1, ExecutionEnabled: true})
	require.NoError(t, err, "a terminal deployment does not block")
	require.True(t, result.Deleted)
}

func TestDeleteTopologyWithLiveRevisionNeedsTopologyExecution(t *testing.T) {
	fixture := activeTopologyFixture(t)
	planned, _, err := PlanTopologyDeployment(fixture.db, TopologyDeploymentPlanInput{TopologyID: fixture.topology.ID, RevisionID: fixture.revision.ID, ActorID: 1})
	require.NoError(t, err)

	_, err = DeleteTopology(fixture.db, TopologyDeleteInput{TopologyID: fixture.topology.ID, ActorID: 1, ExecutionEnabled: false})
	require.ErrorIs(t, err, ErrTopologyTeardownRequiresExecution)
	require.Contains(t, err.Error(), "plugins.topology_execution_enabled")

	// Nothing changed: the topology is live and its plan still planned.
	stored := unscopedTopology(t, fixture.db, fixture.topology.ID)
	require.Zero(t, stored.Deleted)
	require.Nil(t, stored.DeleteRequestedAt)
	require.Equal(t, topologyDeploymentStatePlanned, topologyDeploymentState(t, fixture.db, planned.ID).State)
	var deployments int64
	require.NoError(t, fixture.db.Model(&model.TopologyDeployment{}).Count(&deployments).Error)
	require.EqualValues(t, 1, deployments)
}

func TestDeleteTopologyWithoutDeployableVerticesSkipsTheTeardown(t *testing.T) {
	db := newTopologyDeploymentExecutorTestDB(t)
	topology := model.Topology{Name: "external-only", ServiceScope: "forward"}
	require.NoError(t, db.Create(&topology).Error)
	revision := model.TopologyRevision{TopologyID: topology.ID, Revision: 1, State: "active", ContentHash: strings.Repeat("c", 64), CreatedBy: 1}
	require.NoError(t, db.Create(&revision).Error)
	require.NoError(t, db.Create(&model.TopologyVertex{RevisionID: revision.ID, Key: "upstream", Kind: "endpoint", ConfigJSON: `{}`}).Error)
	require.NoError(t, db.Model(&topology).Update("active_revision_id", revision.ID).Error)

	result, err := DeleteTopology(db, TopologyDeleteInput{TopologyID: topology.ID, ActorID: 1})
	require.NoError(t, err, "nothing runs on a node, so no executor is needed")
	require.True(t, result.Deleted)
	stored := unscopedTopology(t, db, topology.ID)
	require.EqualValues(t, 1, stored.Deleted)
	require.Nil(t, stored.ActiveRevisionID)
}

func TestDeleteTopologyTearsDownTheActiveRevisionThenDeletes(t *testing.T) {
	fixture := activeTopologyFixture(t)
	planned, _, err := PlanTopologyDeployment(fixture.db, TopologyDeploymentPlanInput{TopologyID: fixture.topology.ID, RevisionID: fixture.revision.ID, ActorID: 1})
	require.NoError(t, err)

	result, err := DeleteTopology(fixture.db, TopologyDeleteInput{TopologyID: fixture.topology.ID, ActorID: 7, ExecutionEnabled: true})
	require.NoError(t, err)
	require.False(t, result.Deleted)
	require.NotNil(t, result.Deployment)
	teardown := *result.Deployment
	require.True(t, teardown.Teardown)
	require.Equal(t, topologyDeploymentStateApplying, teardown.State)
	require.Equal(t, fixture.revision.ID, teardown.RevisionID)
	require.Equal(t, fixture.revision.ID, *teardown.PreviousRevisionID)
	require.EqualValues(t, 7, teardown.CreatedBy)
	require.NotNil(t, result.Topology.DeleteRequestedAt)
	require.True(t, PublicTopologyDeployment(teardown).Teardown)

	var steps []model.TopologyDeploymentStep
	require.NoError(t, fixture.db.Where("deployment_id = ?", teardown.ID).Order("apply_order").Find(&steps).Error)
	require.Len(t, steps, 2)
	for _, step := range steps {
		require.True(t, step.Removal)
		require.Equal(t, topologyApplyActionDisable, step.ApplyAction)
		require.Equal(t, topologyRollbackModeRestore, step.RollbackMode)
		require.Equal(t, topologyStepStatePlanned, step.State)
	}
	require.Equal(t, topologyDeploymentStateCancelled, topologyDeploymentState(t, fixture.db, planned.ID).State)

	// While the teardown runs the topology is listed, but nothing else may
	// change it.
	deleting, err := GetTopology(fixture.db, fixture.topology.ID)
	require.NoError(t, err)
	require.NotNil(t, deleting.DeleteRequestedAt)
	_, err = DeleteTopology(fixture.db, TopologyDeleteInput{TopologyID: fixture.topology.ID, ActorID: 1, ExecutionEnabled: true})
	require.ErrorIs(t, err, ErrTopologyDeleting)
	_, _, err = PlanTopologyDeployment(fixture.db, TopologyDeploymentPlanInput{TopologyID: fixture.topology.ID, RevisionID: fixture.revision.ID, ActorID: 1})
	require.ErrorIs(t, err, ErrTopologyDeleting)
	_, err = CreateTopologyRevision(fixture.db, fixture.topology.ID, 1, TopologyRevisionInput{Vertices: []model.TopologyVertex{{Key: "a", Kind: "endpoint", ConfigJSON: `{}`}}})
	require.ErrorIs(t, err, ErrTopologyDeleting)

	executor, err := NewTopologyDeploymentExecutor(fixture.db)
	require.NoError(t, err)
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	require.NoError(t, fixture.db.Where("deployment_id = ?", teardown.ID).Order("apply_order").Find(&steps).Error)
	for _, step := range steps {
		require.Equal(t, topologyStepStateDisabling, step.State)
		var operation model.KernelOperation
		require.NoError(t, fixture.db.First(&operation, "id = ?", step.DisableOperationID).Error)
		require.Equal(t, "plugin.disable", operation.Kind)
		require.Equal(t, fixture.revision.Revision, operation.TopologyRevision)
	}
	stored := unscopedTopology(t, fixture.db, fixture.topology.ID)
	require.Zero(t, stored.Deleted, "the topology stays until the teardown succeeds")

	finished := driveTopologyDeployment(t, executor, fixture.db, teardown.ID)
	require.Equal(t, topologyDeploymentStateSucceeded, finished.State)
	stored = unscopedTopology(t, fixture.db, fixture.topology.ID)
	require.EqualValues(t, 1, stored.Deleted)
	require.NotNil(t, stored.DeletedAt)
	require.Nil(t, stored.ActiveRevisionID)
	require.Nil(t, stored.DeleteRequestedAt)
	_, err = GetTopology(fixture.db, fixture.topology.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// The record of the teardown stays, and a deleted topology's deployment
	// cannot be rolled back onto the nodes.
	status, err := GetTopologyDeploymentStatus(fixture.db, teardown.ID)
	require.NoError(t, err)
	require.Len(t, status.Observed, 2)
	require.NotEmpty(t, status.Operations)
	_, err = RequestTopologyDeploymentRollback(fixture.db, teardown.ID, time.Now())
	require.ErrorIs(t, err, ErrTopologyDeleted)
	require.NoError(t, fixture.db.Create(&model.Topology{Name: fixture.topology.Name, ServiceScope: "forward"}).Error)
}

func TestDeleteTopologyTeardownFailureKeepsTheTopology(t *testing.T) {
	fixture := activeTopologyFixture(t)
	result, err := DeleteTopology(fixture.db, TopologyDeleteInput{TopologyID: fixture.topology.ID, ActorID: 1, ExecutionEnabled: true})
	require.NoError(t, err)
	teardown := result.Deployment
	executor, err := NewTopologyDeploymentExecutor(fixture.db)
	require.NoError(t, err)
	_, err = executor.RunOnce(context.Background())
	require.NoError(t, err)
	entry := topologyDeploymentStep(t, fixture.db, teardown.ID, "entry")
	topologyDeploymentOperationState(t, fixture.db, entry.DisableOperationID, "failed")

	// Only the teardown may run while the topology is being deleted.
	other := model.TopologyDeployment{
		TopologyID: fixture.topology.ID, RevisionID: fixture.revision.ID, State: topologyDeploymentStateSucceeded,
		FailurePolicy: "stop_and_rollback", CreatedBy: 1,
	}
	require.NoError(t, fixture.db.Create(&other).Error)
	_, err = RequestTopologyDeploymentRollback(fixture.db, other.ID, time.Now())
	require.ErrorIs(t, err, ErrTopologyDeleting)

	finished := driveTopologyDeployment(t, executor, fixture.db, teardown.ID)
	require.Equal(t, topologyDeploymentStateRolledBack, finished.State)
	require.Contains(t, finished.LastError, "plugin.disable ended in failed")
	require.True(t, PublicTopologyDeployment(finished).HasError)

	stored := unscopedTopology(t, fixture.db, fixture.topology.ID)
	require.Zero(t, stored.Deleted)
	require.Nil(t, stored.DeletedAt)
	require.Nil(t, stored.DeleteRequestedAt, "the delete mark is cleared")
	require.NotNil(t, stored.ActiveRevisionID)
	require.Equal(t, fixture.revision.ID, *stored.ActiveRevisionID)

	// The topology can be changed again, and deleted again.
	_, err = RequestTopologyDeploymentRollback(fixture.db, other.ID, time.Now())
	require.NoError(t, err)
	require.NoError(t, fixture.db.Model(&model.TopologyDeployment{}).Where("id = ?", other.ID).Update("state", topologyDeploymentStateRolledBack).Error)
	again, err := DeleteTopology(fixture.db, TopologyDeleteInput{TopologyID: fixture.topology.ID, ActorID: 1, ExecutionEnabled: true})
	require.NoError(t, err)
	require.NotNil(t, again.Deployment)
	require.NotEqual(t, teardown.ID, again.Deployment.ID)
}

func TestTopologyTeardownRolledBackBeforeDispatchKeepsTheTopology(t *testing.T) {
	fixture := activeTopologyFixture(t)
	result, err := DeleteTopology(fixture.db, TopologyDeleteInput{TopologyID: fixture.topology.ID, ActorID: 1, ExecutionEnabled: true})
	require.NoError(t, err)
	_, err = RequestTopologyDeploymentRollback(fixture.db, result.Deployment.ID, time.Now())
	require.NoError(t, err, "rolling back the teardown cancels the deletion")
	executor, err := NewTopologyDeploymentExecutor(fixture.db)
	require.NoError(t, err)
	finished := driveTopologyDeployment(t, executor, fixture.db, result.Deployment.ID)
	require.Equal(t, topologyDeploymentStateRolledBack, finished.State)

	stored := unscopedTopology(t, fixture.db, fixture.topology.ID)
	require.Zero(t, stored.Deleted)
	require.Nil(t, stored.DeleteRequestedAt)
	_, _, err = PlanTopologyDeployment(fixture.db, TopologyDeploymentPlanInput{TopologyID: fixture.topology.ID, RevisionID: fixture.revision.ID, ActorID: 1})
	require.NoError(t, err)
}

func TestDeleteTopologyTeardownNeedsTheVertexAssignments(t *testing.T) {
	fixture := activeTopologyFixture(t)
	require.NoError(t, fixture.db.Model(&model.NodeServiceAssignment{}).Where("role = ?", "exit").Update("enabled", false).Error)
	_, err := DeleteTopology(fixture.db, TopologyDeleteInput{TopologyID: fixture.topology.ID, ActorID: 1, ExecutionEnabled: true})
	require.ErrorIs(t, err, ErrTopologyTeardownInvalid)
	require.Contains(t, err.Error(), "no enabled node assignment")
	stored := unscopedTopology(t, fixture.db, fixture.topology.ID)
	require.Zero(t, stored.Deleted)
	require.Nil(t, stored.DeleteRequestedAt)
}

// A stale delete mark (its teardown ended without clearing it) neither blocks
// changes nor a new delete.
func TestTopologyDeleteMarkWithoutRunningTeardownIsIgnored(t *testing.T) {
	fixture := activeTopologyFixture(t)
	stale := time.Now().Add(-time.Hour)
	require.NoError(t, fixture.db.Model(&fixture.topology).Update("delete_requested_at", stale).Error)
	_, _, err := PlanTopologyDeployment(fixture.db, TopologyDeploymentPlanInput{TopologyID: fixture.topology.ID, RevisionID: fixture.revision.ID, ActorID: 1})
	require.NoError(t, err)
	result, err := DeleteTopology(fixture.db, TopologyDeleteInput{TopologyID: fixture.topology.ID, ActorID: 1, ExecutionEnabled: true})
	require.NoError(t, err)
	require.NotNil(t, result.Deployment)
}
