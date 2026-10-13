package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Topology deletion (DELETE /api/v3/topologies/:id). A topology is never
// removed from the database: deleting it sets its deleted flag, which
// gorm.io/plugin/soft_delete filters out of every query on model.Topology, so
// the list, the revision, preview and diagnose reads, planning and revision
// creation all see a deleted topology as missing. Its revisions, deployments,
// steps, observed states and operations stay for audit.
//
// A topology without an active revision has nothing on the nodes and is
// deleted at once. One with an active revision is first torn down: a teardown
// deployment disables every plugin vertex of that revision through the
// ordinary executor, and the topology is deleted only when the teardown
// succeeds. If the teardown fails or is rolled back, the topology stays with
// its active revision.

// topologyDeletedFlag is the value of v3_kernel_topology.deleted for a deleted
// topology; live topologies have 0 (gorm.io/plugin/soft_delete's flag mode).
const topologyDeletedFlag = 1

var (
	// ErrTopologyDeleting refuses changes to a topology whose teardown
	// deployment is running.
	ErrTopologyDeleting = errors.New("topology is being deleted")
	// ErrTopologyDeleted refuses applying or rolling back a deployment of a
	// deleted topology.
	ErrTopologyDeleted = errors.New("topology has been deleted")
	// ErrTopologyTeardownRequiresExecution refuses deleting a topology whose
	// active revision runs on nodes while the topology executor is off.
	ErrTopologyTeardownRequiresExecution = errors.New("topology execution is disabled: a topology with an active revision is deleted only after a teardown deployment removes its plugins from the nodes, which needs plugins.topology_execution_enabled")
	// ErrTopologyTeardownInvalid wraps why the teardown deployment could not
	// be compiled (for example a vertex whose node assignment was removed).
	ErrTopologyTeardownInvalid = errors.New("topology teardown cannot be planned")
)

// topologyDeploymentIdleStates are the deployment states in which nothing
// executes: planned and the terminal states. Every other state is in flight.
var topologyDeploymentIdleStates = []string{
	topologyDeploymentStatePlanned, topologyDeploymentStateSucceeded, topologyDeploymentStateFailed,
	topologyDeploymentStateRolledBack, topologyDeploymentStateCancelled,
}

// TopologyDeleteInput identifies the topology to delete.
type TopologyDeleteInput struct {
	TopologyID uint
	ActorID    uint
	// ExecutionEnabled is plugins.topology_execution_enabled: only the
	// executor can tear down a topology whose active revision has plugin
	// vertices.
	ExecutionEnabled bool
	At               time.Time
}

// TopologyDeleteResult says what DeleteTopology did. Deleted is true when the
// topology was deleted at once; otherwise Deployment is the teardown
// deployment that deletes it when it succeeds.
type TopologyDeleteResult struct {
	Topology   model.Topology
	Deleted    bool
	Deployment *model.TopologyDeployment
}

// GetTopology loads a live topology. A missing or deleted topology is
// gorm.ErrRecordNotFound.
func GetTopology(db *gorm.DB, topologyID uint) (*model.Topology, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	var topology model.Topology
	if err := db.First(&topology, topologyID).Error; err != nil {
		return nil, err
	}
	return &topology, nil
}

// DeleteTopology deletes a live topology, or starts the teardown that deletes
// it. It answers gorm.ErrRecordNotFound for a missing or deleted topology,
// ErrTopologyDeleting while its teardown runs, ErrTopologyDeploymentBusy while
// another deployment of it executes, and ErrTopologyTeardownRequiresExecution
// when a teardown is needed and the executor is off. Planned deployments of
// the topology are cancelled: they can never be applied.
func DeleteTopology(db *gorm.DB, input TopologyDeleteInput) (*TopologyDeleteResult, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	if input.TopologyID == 0 {
		return nil, errors.New("topology_id is required")
	}
	at := input.At
	if at.IsZero() {
		at = time.Now()
	}
	var result TopologyDeleteResult
	err := db.Transaction(func(tx *gorm.DB) error {
		result = TopologyDeleteResult{}
		var topology model.Topology
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&topology, input.TopologyID).Error; err != nil {
			return err
		}
		inFlight, err := topologyInFlightDeployment(tx, topology.ID)
		if err != nil {
			return err
		}
		if inFlight != nil {
			if inFlight.Teardown {
				return ErrTopologyDeleting
			}
			return ErrTopologyDeploymentBusy
		}

		var steps []model.TopologyDeploymentStep
		if topology.ActiveRevisionID != nil {
			deployable, err := topologyActiveDeployableVertices(tx, topology)
			if err != nil {
				return err
			}
			if deployable > 0 {
				if !input.ExecutionEnabled {
					return ErrTopologyTeardownRequiresExecution
				}
				if steps, err = compileTopologyTeardownSteps(tx, topology); err != nil {
					return fmt.Errorf("%w: %w", ErrTopologyTeardownInvalid, err)
				}
			}
		}

		if err := cancelPlannedTopologyDeployments(tx, topology.ID, at); err != nil {
			return err
		}
		if len(steps) == 0 {
			if err := markTopologyDeleted(tx, &topology, at); err != nil {
				return err
			}
			result.Topology, result.Deleted = topology, true
			return nil
		}

		deployment := model.TopologyDeployment{
			TopologyID: topology.ID, RevisionID: *topology.ActiveRevisionID, PreviousRevisionID: topology.ActiveRevisionID,
			State: topologyDeploymentStateApplying, FailurePolicy: "stop_and_rollback", Teardown: true, CreatedBy: input.ActorID,
		}
		if err := tx.Create(&deployment).Error; err != nil {
			return err
		}
		for i := range steps {
			steps[i].DeploymentID = deployment.ID
		}
		if err := tx.Create(&steps).Error; err != nil {
			return err
		}
		if err := tx.Model(&topology).Update("delete_requested_at", at).Error; err != nil {
			return err
		}
		topology.DeleteRequestedAt = &at
		result.Topology, result.Deployment = topology, &deployment
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// topologyInFlightDeployment returns the topology's executing deployment, if
// any.
func topologyInFlightDeployment(tx *gorm.DB, topologyID uint) (*model.TopologyDeployment, error) {
	var deployment model.TopologyDeployment
	err := tx.Where("topology_id = ? AND state NOT IN ?", topologyID, topologyDeploymentIdleStates).Order("id").First(&deployment).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &deployment, nil
}

// topologyDeletePending reports whether the topology's teardown deployment is
// still running. A delete mark whose teardown has ended does not count; the
// executor clears it when a teardown fails or is rolled back.
func topologyDeletePending(tx *gorm.DB, topology model.Topology) (bool, error) {
	if topology.DeleteRequestedAt == nil {
		return false, nil
	}
	var count int64
	err := tx.Model(&model.TopologyDeployment{}).
		Where("topology_id = ? AND teardown = ? AND state NOT IN ?", topology.ID, true, topologyDeploymentIdleStates).
		Count(&count).Error
	return count > 0, err
}

// checkTopologyAcceptsDeployment refuses applying or rolling back a deployment
// whose topology was deleted, and any deployment but the teardown while the
// topology is being deleted.
func checkTopologyAcceptsDeployment(tx *gorm.DB, deployment model.TopologyDeployment) error {
	var topology model.Topology
	if err := tx.Unscoped().First(&topology, deployment.TopologyID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTopologyDeleted
		}
		return err
	}
	if topology.Deleted != 0 {
		return ErrTopologyDeleted
	}
	if deployment.Teardown {
		return nil
	}
	deleting, err := topologyDeletePending(tx, topology)
	if err != nil {
		return err
	}
	if deleting {
		return ErrTopologyDeleting
	}
	return nil
}

// topologyActiveDeployableVertices counts the vertices of the active revision
// that run a plugin on a node: the ones a teardown has to disable.
func topologyActiveDeployableVertices(tx *gorm.DB, topology model.Topology) (int, error) {
	if topology.ActiveRevisionID == nil {
		return 0, nil
	}
	var vertices []model.TopologyVertex
	if err := tx.Where("revision_id = ?", *topology.ActiveRevisionID).Find(&vertices).Error; err != nil {
		return 0, err
	}
	count := 0
	for _, vertex := range vertices {
		if vertex.NodeID != nil && strings.TrimSpace(vertex.PluginID) != "" {
			count++
		}
	}
	return count, nil
}

// cancelPlannedTopologyDeployments closes the planned deployments of a topology
// being deleted, and skips their unstarted steps, as a rollback before
// dispatch does.
func cancelPlannedTopologyDeployments(tx *gorm.DB, topologyID uint, at time.Time) error {
	var ids []uint
	if err := tx.Model(&model.TopologyDeployment{}).
		Where("topology_id = ? AND state = ?", topologyID, topologyDeploymentStatePlanned).
		Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	if err := tx.Model(&model.TopologyDeployment{}).Where("id IN ?", ids).Updates(map[string]any{
		"state": topologyDeploymentStateCancelled, "completed_at": at,
		"last_error": "the topology was deleted before this deployment was applied",
	}).Error; err != nil {
		return err
	}
	return tx.Model(&model.TopologyDeploymentStep{}).
		Where("deployment_id IN ? AND state = ?", ids, topologyStepStatePlanned).
		Update("state", topologyStepStateSkipped).Error
}

// markTopologyDeleted sets the deleted flag and clears the active revision and
// the delete mark: nothing of the topology runs on the nodes any more.
func markTopologyDeleted(tx *gorm.DB, topology *model.Topology, at time.Time) error {
	result := tx.Model(&model.Topology{}).Where("id = ?", topology.ID).Updates(map[string]any{
		"deleted": topologyDeletedFlag, "deleted_at": at,
		"active_revision_id": nil, "delete_requested_at": nil,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	topology.Deleted = topologyDeletedFlag
	topology.DeletedAt, topology.ActiveRevisionID, topology.DeleteRequestedAt = &at, nil, nil
	return nil
}

// clearTopologyDeleteRequest ends a teardown that did not succeed: the
// topology stays, with its active revision, and can be deleted again.
func clearTopologyDeleteRequest(tx *gorm.DB, deployment model.TopologyDeployment) error {
	if !deployment.Teardown {
		return nil
	}
	return tx.Model(&model.Topology{}).Where("id = ?", deployment.TopologyID).Update("delete_requested_at", nil).Error
}
