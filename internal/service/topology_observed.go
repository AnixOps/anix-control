package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TopologyObservedStateUpdate is the transport-neutral observed-state write
// contract used by the Agent bridge and the future topology executor.
type TopologyObservedStateUpdate struct {
	DeploymentID     uint
	NodeID           uint
	DesiredRevision  int64
	ObservedRevision int64
	State            string
	HealthJSON       string
	LastError        string
	ObservedAt       time.Time
}

var allowedTopologyObservedStates = map[string]bool{
	"planned": true, "applying": true, "healthy": true, "succeeded": true,
	"failed": true, "stale": true, "rolled_back": true, "cancelled": true,
}

// ApplyTopologyObservedState writes one node observation with monotonic
// desired/observed revisions. A stale or out-of-order observation returns the
// current row with changed=false and never regresses deployment state.
func ApplyTopologyObservedState(db *gorm.DB, update TopologyObservedStateUpdate) (*model.TopologyObservedState, bool, error) {
	if db == nil {
		return nil, false, errors.New("database is not initialized")
	}
	var result *model.TopologyObservedState
	var changed bool
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		result, changed, err = ApplyTopologyObservedStateTx(tx, update)
		return err
	})
	return result, changed, err
}

// ApplyTopologyObservedStateTx is the transaction form used by gRPC observed
// callbacks so operation and topology state commit atomically.
func ApplyTopologyObservedStateTx(tx *gorm.DB, update TopologyObservedStateUpdate) (*model.TopologyObservedState, bool, error) {
	if tx == nil {
		return nil, false, errors.New("database is not initialized")
	}
	if update.DeploymentID == 0 || update.NodeID == 0 {
		return nil, false, errors.New("deployment_id and node_id are required")
	}
	if update.DesiredRevision <= 0 || update.ObservedRevision < 0 || update.ObservedRevision > update.DesiredRevision {
		return nil, false, errors.New("topology observed revision is outside the desired revision")
	}
	update.State = strings.TrimSpace(strings.ToLower(update.State))
	if update.State == "running" {
		update.State = "applying"
	}
	if update.State == "complete" {
		update.State = "succeeded"
	}
	if !allowedTopologyObservedStates[update.State] {
		return nil, false, fmt.Errorf("unsupported topology observed state %q", update.State)
	}
	if strings.TrimSpace(update.HealthJSON) != "" && !json.Valid([]byte(update.HealthJSON)) {
		return nil, false, errors.New("topology observed health must be valid JSON")
	}
	if update.ObservedAt.IsZero() {
		update.ObservedAt = time.Now()
	}

	var deployment model.TopologyDeployment
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&deployment, update.DeploymentID).Error; err != nil {
		return nil, false, err
	}
	var revision model.TopologyRevision
	if err := tx.First(&revision, deployment.RevisionID).Error; err != nil {
		return nil, false, err
	}
	if update.DesiredRevision != revision.Revision {
		return nil, false, fmt.Errorf("desired revision %d does not match deployment revision %d", update.DesiredRevision, revision.Revision)
	}
	var vertexCount int64
	if err := tx.Model(&model.TopologyVertex{}).
		Where("revision_id = ? AND node_id = ?", deployment.RevisionID, update.NodeID).
		Count(&vertexCount).Error; err != nil {
		return nil, false, err
	}
	if vertexCount == 0 {
		return nil, false, fmt.Errorf("node %d is not part of topology deployment %d", update.NodeID, update.DeploymentID)
	}

	var current model.TopologyObservedState
	lookupErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current,
		"deployment_id = ? AND node_id = ?", update.DeploymentID, update.NodeID).Error
	found := lookupErr == nil
	if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		return nil, false, lookupErr
	}
	if found {
		if update.DesiredRevision < current.DesiredRevision || update.ObservedRevision < current.ObservedRevision {
			return &current, false, nil
		}
		allowControlledRollback := deployment.State == "rollback_requested" && update.State == "rolled_back" && (current.State == "succeeded" || current.State == "healthy")
		if update.ObservedRevision == current.ObservedRevision && (!allowControlledRollback && (topologyObservedTerminal(current.State) || update.ObservedAt.Before(current.UpdatedAt))) {
			return &current, false, nil
		}
	}

	if !found {
		current = model.TopologyObservedState{DeploymentID: update.DeploymentID, NodeID: update.NodeID}
	}
	current.DesiredRevision = update.DesiredRevision
	current.ObservedRevision = update.ObservedRevision
	current.State = update.State
	current.HealthJSON = strings.TrimSpace(update.HealthJSON)
	current.LastError = strings.TrimSpace(update.LastError)
	current.UpdatedAt = update.ObservedAt
	if found {
		if err := tx.Save(&current).Error; err != nil {
			return nil, false, err
		}
	} else if err := tx.Create(&current).Error; err != nil {
		return nil, false, err
	}
	if err := aggregateTopologyDeploymentState(tx, &deployment, update.ObservedAt); err != nil {
		return nil, false, err
	}
	return &current, true, nil
}

func topologyObservedTerminal(state string) bool {
	switch state {
	case "healthy", "succeeded", "failed", "rolled_back", "cancelled":
		return true
	default:
		return false
	}
}

func aggregateTopologyDeploymentState(tx *gorm.DB, deployment *model.TopologyDeployment, at time.Time) error {
	var rows []model.TopologyObservedState
	if err := tx.Where("deployment_id = ?", deployment.ID).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	var expectedNodes int64
	if err := tx.Model(&model.TopologyVertex{}).
		Where("revision_id = ? AND node_id IS NOT NULL", deployment.RevisionID).
		Distinct("node_id").Count(&expectedNodes).Error; err != nil {
		return err
	}
	allSucceeded := expectedNodes > 0 && int64(len(rows)) == expectedNodes
	allRolledBack := expectedNodes > 0 && int64(len(rows)) == expectedNodes
	hasFailure := false
	hasRollback := false
	hasCancellation := false
	for _, row := range rows {
		switch row.State {
		case "failed":
			hasFailure = true
			allSucceeded = false
		case "rolled_back":
			hasRollback = true
			allSucceeded = false
		case "cancelled":
			hasCancellation = true
			allSucceeded = false
		case "succeeded", "healthy":
		default:
			allSucceeded = false
		}
		if row.State != "rolled_back" {
			allRolledBack = false
		}
	}
	state := "applying"
	if hasFailure {
		state = "failed"
	} else if hasRollback && allRolledBack {
		state = "rolled_back"
	} else if hasCancellation {
		state = "cancelled"
	} else if allSucceeded {
		state = "succeeded"
	}
	if deployment.State == "rollback_requested" && state == "applying" {
		state = "rollback_requested"
	}
	updates := map[string]any{"state": state, "completed_at": nil}
	if state != "applying" {
		updates["completed_at"] = at
	}
	return tx.Model(deployment).Updates(updates).Error
}
