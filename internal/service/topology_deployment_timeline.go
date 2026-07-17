package service

import (
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// TopologyDeploymentOperation is the safe, linkage-focused projection used by
// deployment status. ConfigJSON, ResultJSON, and raw Agent errors are
// intentionally omitted. LastError is a stable kernel-generated summary, not
// the persisted operation error.
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
	HasError      bool       `json:"has_error"`
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
	HasError    bool      `json:"has_error"`
	// Message is a stable kernel-generated lifecycle summary. It is never a
	// persisted deployment, observed-state, or Agent error.
	Message string `json:"message,omitempty"`
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
		hasError := topologyOperationHasError(row)
		operation := TopologyDeploymentOperation{
			OperationID: row.ID, StepID: row.TopologyStepID, NodeID: row.NodeID,
			PluginID: row.PluginID, TargetVersion: row.TargetVersion, Kind: row.Kind,
			Revision: row.TopologyRevision, State: row.State, Attempt: row.Attempt,
			DeadlineAt: row.DeadlineAt, DispatchedAt: row.DispatchedAt,
			ObservedAt: row.ObservedAt, HasError: hasError,
			LastError: topologyOperationErrorSummary(row.Kind, row.State, row.LastError), UpdatedAt: row.UpdatedAt,
		}
		operations = append(operations, operation)
		at := row.UpdatedAt
		if at.IsZero() {
			at = row.CreatedAt
		}
		events = append(events, TopologyDeploymentEvent{
			Type: "operation", At: at, OperationID: row.ID, StepID: row.TopologyStepID,
			NodeID: row.NodeID, PluginID: row.PluginID, State: row.State,
			HasError: operation.HasError, Message: operation.LastError,
		})
	}
	for _, row := range observed {
		nodeID := row.NodeID
		hasError := topologyObservedStateHasError(row)
		events = append(events, TopologyDeploymentEvent{
			Type: "observed", At: row.UpdatedAt, NodeID: &nodeID, State: row.State,
			HasError: hasError, Message: topologyObservedStateErrorSummary(row.State, row.LastError),
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
		events = append(events, TopologyDeploymentEvent{
			Type: "deployment", At: terminalAt, State: deployment.State,
			HasError: topologyDeploymentHasError(deployment),
			Message:  topologyDeploymentErrorSummary(deployment.State, deployment.LastError),
		})
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].At.Equal(events[j].At) {
			return events[i].Type < events[j].Type
		}
		return events[i].At.Before(events[j].At)
	})
	return operations, events, nil
}

func topologyOperationHasError(operation model.KernelOperation) bool {
	if strings.TrimSpace(operation.LastError) != "" {
		return true
	}
	switch strings.TrimSpace(operation.State) {
	case "failed", "timed_out", "cancelled":
		return true
	default:
		return false
	}
}

func topologyOperationErrorSummary(kind, state, lastError string) string {
	if strings.TrimSpace(lastError) == "" {
		switch strings.TrimSpace(state) {
		case "failed":
			return topologyOperationFailedSummary(kind)
		case "timed_out":
			return "operation timed out"
		case "cancelled":
			return "operation cancelled"
		default:
			return ""
		}
	}
	switch strings.TrimSpace(state) {
	case "timed_out":
		return "operation timed out"
	case "cancelled":
		return "operation cancelled"
	case "cancel_requested":
		return "operation cancellation requested"
	case "failed":
		return topologyOperationFailedSummary(kind)
	default:
		return "operation reported an error"
	}
}

func topologyOperationFailedSummary(kind string) string {
	switch strings.TrimSpace(kind) {
	case "plugin.configure":
		return "plugin configuration failed"
	case "plugin.enable":
		return "plugin enable failed"
	case "plugin.disable":
		return "plugin disable failed"
	case "plugin.install":
		return "plugin installation failed"
	case "plugin.update":
		return "plugin update failed"
	case "plugin.rollback":
		return "plugin rollback failed"
	default:
		return "operation failed"
	}
}

func topologyObservedStateHasError(observed model.TopologyObservedState) bool {
	if strings.TrimSpace(observed.LastError) != "" {
		return true
	}
	switch strings.TrimSpace(observed.State) {
	case "failed", "stale", "cancelled":
		return true
	default:
		return false
	}
}

func topologyObservedStateErrorSummary(state, lastError string) string {
	if strings.TrimSpace(lastError) == "" {
		switch strings.TrimSpace(state) {
		case "failed":
			return "observed state failed"
		case "stale":
			return "observed state is stale"
		case "cancelled":
			return "observed state was cancelled"
		default:
			return ""
		}
	}
	switch strings.TrimSpace(state) {
	case "failed":
		return "observed state failed"
	case "stale":
		return "observed state is stale"
	case "cancelled":
		return "observed state was cancelled"
	default:
		return "observed state reported an error"
	}
}

func topologyDeploymentHasError(deployment model.TopologyDeployment) bool {
	return strings.TrimSpace(deployment.LastError) != "" || strings.TrimSpace(deployment.State) == "failed"
}

func topologyDeploymentErrorSummary(state, lastError string) string {
	if strings.TrimSpace(lastError) == "" && strings.TrimSpace(state) != "failed" {
		return ""
	}
	switch strings.TrimSpace(state) {
	case "failed":
		return "deployment failed"
	case "rolled_back":
		return "deployment rolled back after an unsuccessful change"
	case "rollback_requested":
		return "deployment rollback is in progress"
	case "cancelled":
		return "deployment was cancelled"
	default:
		return "deployment reported an error"
	}
}
