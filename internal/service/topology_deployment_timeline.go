package service

import (
	"sort"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// TopologyDeploymentOperation is the safe, linkage-focused projection used by
// deployment status. ConfigJSON and ResultJSON are intentionally omitted.
type TopologyDeploymentOperation struct {
	OperationID   string     `json:"operation_id"`
	StepID        *uint      `json:"step_id,omitempty"`
	NodeID        *uint      `json:"node_id,omitempty"`
	PluginID      string     `json:"plugin_id"`
	TargetVersion string     `json:"target_version"`
	Kind          string     `json:"kind"`
	Revision      int64      `json:"revision"`
	State         string     `json:"state"`
	Attempt       int        `json:"attempt"`
	DeadlineAt    *time.Time `json:"deadline_at,omitempty"`
	DispatchedAt  *time.Time `json:"dispatched_at,omitempty"`
	ObservedAt    *time.Time `json:"observed_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type TopologyDeploymentEvent struct {
	Type        string    `json:"type"`
	At          time.Time `json:"at"`
	OperationID string    `json:"operation_id,omitempty"`
	StepID      *uint     `json:"step_id,omitempty"`
	NodeID      *uint     `json:"node_id,omitempty"`
	PluginID    string    `json:"plugin_id,omitempty"`
	State       string    `json:"state"`
	Message     string    `json:"message,omitempty"`
}

func loadTopologyDeploymentTimeline(db *gorm.DB, deployment model.TopologyDeployment, steps []model.TopologyDeploymentStep, observed []model.TopologyObservedState) ([]TopologyDeploymentOperation, []TopologyDeploymentEvent, error) {
	if db == nil {
		return nil, nil, gorm.ErrInvalidDB
	}
	stepIDs := make([]uint, 0, len(steps))
	operationIDs := make(map[string]struct{})
	for _, step := range steps {
		stepIDs = append(stepIDs, step.ID)
		for _, operationID := range []string{
			step.ConfigureOperationID, step.EnableOperationID, step.DisableOperationID,
			step.RollbackConfigureOperationID, step.RollbackEnableOperationID, step.RollbackDisableOperationID,
		} {
			if operationID != "" {
				operationIDs[operationID] = struct{}{}
			}
		}
	}
	var rows []model.KernelOperation
	query := db.Where("topology_deployment_id = ?", deployment.ID)
	if len(stepIDs) > 0 {
		query = query.Or("topology_step_id IN ?", stepIDs)
	}
	if len(operationIDs) > 0 {
		ids := make([]string, 0, len(operationIDs))
		for operationID := range operationIDs {
			ids = append(ids, operationID)
		}
		sort.Strings(ids)
		query = query.Or("id IN ?", ids)
	}
	if err := query.Order("created_at, id").Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	operations := make([]TopologyDeploymentOperation, 0, len(rows))
	events := make([]TopologyDeploymentEvent, 0, len(rows)+len(observed)+2)
	for _, row := range rows {
		operation := TopologyDeploymentOperation{
			OperationID: row.ID, StepID: row.TopologyStepID, NodeID: row.NodeID,
			PluginID: row.PluginID, TargetVersion: row.TargetVersion, Kind: row.Kind,
			Revision: row.TopologyRevision, State: row.State, Attempt: row.Attempt,
			DeadlineAt: row.DeadlineAt, DispatchedAt: row.DispatchedAt,
			ObservedAt: row.ObservedAt, LastError: row.LastError, UpdatedAt: row.UpdatedAt,
		}
		operations = append(operations, operation)
		at := row.UpdatedAt
		if at.IsZero() {
			at = row.CreatedAt
		}
		events = append(events, TopologyDeploymentEvent{
			Type: "operation", At: at, OperationID: row.ID, StepID: row.TopologyStepID,
			NodeID: row.NodeID, PluginID: row.PluginID, State: row.State, Message: row.LastError,
		})
	}
	for _, row := range observed {
		nodeID := row.NodeID
		events = append(events, TopologyDeploymentEvent{
			Type: "observed", At: row.UpdatedAt, NodeID: &nodeID, State: row.State, Message: row.LastError,
		})
	}
	createdAt := deployment.CreatedAt
	if !createdAt.IsZero() {
		events = append(events, TopologyDeploymentEvent{Type: "deployment", At: createdAt, State: "planned"})
	}
	terminalAt := deployment.UpdatedAt
	if deployment.CompletedAt != nil {
		terminalAt = *deployment.CompletedAt
	}
	if !terminalAt.IsZero() && deployment.State != "planned" {
		events = append(events, TopologyDeploymentEvent{Type: "deployment", At: terminalAt, State: deployment.State, Message: deployment.LastError})
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].At.Equal(events[j].At) {
			return events[i].Type < events[j].Type
		}
		return events[i].At.Before(events[j].At)
	})
	return operations, events, nil
}
