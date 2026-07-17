package service

import (
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
)

// TopologyDeploymentStatusView is the only topology deployment status shape
// intended for HTTP clients. It deliberately leaves configuration documents,
// rollback documents, Agent result documents, Agent health documents, and raw
// persisted errors on the kernel side of the boundary.
type TopologyDeploymentStatusView struct {
	Deployment TopologyDeploymentView        `json:"deployment"`
	Steps      []TopologyDeploymentStepView  `json:"steps"`
	Observed   []TopologyObservedStateView   `json:"observed_states"`
	Operations []TopologyDeploymentOperation `json:"operations"`
	Events     []TopologyDeploymentEvent     `json:"events"`
}

// TopologyDeploymentView is a status-only projection of a durable deployment.
// LastError is a stable summary derived from state, never the persisted value.
type TopologyDeploymentView struct {
	ID                   uint       `json:"id"`
	TopologyID           uint       `json:"topology_id"`
	RevisionID           uint       `json:"revision_id"`
	PreviousRevisionID   *uint      `json:"previous_revision_id,omitempty"`
	RolloutGroup         string     `json:"rollout_group"`
	State                string     `json:"state"`
	FailurePolicy        string     `json:"failure_policy"`
	HasError             bool       `json:"has_error"`
	LastError            string     `json:"last_error,omitempty"`
	HealthGateDeadlineAt *time.Time `json:"health_gate_deadline_at,omitempty"`
	RollbackStartedAt    *time.Time `json:"rollback_started_at,omitempty"`
	RollbackCompletedAt  *time.Time `json:"rollback_completed_at,omitempty"`
	CreatedBy            uint       `json:"created_by"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
}

// TopologyDeploymentStepView is a state-only projection. The active and
// rollback configuration documents remain write-only kernel records.
type TopologyDeploymentStepView struct {
	ID                           uint      `json:"id"`
	DeploymentID                 uint      `json:"deployment_id"`
	VertexID                     uint      `json:"vertex_id"`
	VertexKey                    string    `json:"vertex_key"`
	NodeID                       uint      `json:"node_id"`
	PluginID                     string    `json:"plugin_id"`
	Role                         string    `json:"role"`
	TargetVersion                string    `json:"target_version"`
	ApplyOrder                   int       `json:"apply_order"`
	Removal                      bool      `json:"removal"`
	ApplyAction                  string    `json:"apply_action"`
	RollbackMode                 string    `json:"rollback_mode"`
	State                        string    `json:"state"`
	HasError                     bool      `json:"has_error"`
	LastError                    string    `json:"last_error,omitempty"`
	ConfigureOperationID         string    `json:"configure_operation_id,omitempty"`
	EnableOperationID            string    `json:"enable_operation_id,omitempty"`
	DisableOperationID           string    `json:"disable_operation_id,omitempty"`
	RollbackConfigureOperationID string    `json:"rollback_configure_operation_id,omitempty"`
	RollbackEnableOperationID    string    `json:"rollback_enable_operation_id,omitempty"`
	RollbackDisableOperationID   string    `json:"rollback_disable_operation_id,omitempty"`
	CreatedAt                    time.Time `json:"created_at"`
	UpdatedAt                    time.Time `json:"updated_at"`
}

// TopologyObservedStateView contains only monotonic state metadata. Health is
// represented by a presence bit because plugin-provided health JSON is not a
// stable or safe API contract.
type TopologyObservedStateView struct {
	ID               uint      `json:"id"`
	DeploymentID     uint      `json:"deployment_id"`
	NodeID           uint      `json:"node_id"`
	DesiredRevision  int64     `json:"desired_revision"`
	ObservedRevision int64     `json:"observed_revision"`
	State            string    `json:"state"`
	HealthReported   bool      `json:"health_reported"`
	HasError         bool      `json:"has_error"`
	LastError        string    `json:"last_error,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// PublicTopologyDeploymentStatus projects an internal status read to its
// explicit public representation. Do not return TopologyDeploymentStatus from
// an HTTP handler: it contains persistence models with secret-bearing fields.
func PublicTopologyDeploymentStatus(status TopologyDeploymentStatus) TopologyDeploymentStatusView {
	view := TopologyDeploymentStatusView{
		Deployment: PublicTopologyDeployment(status.Deployment),
		Steps:      make([]TopologyDeploymentStepView, 0, len(status.Steps)),
		Observed:   PublicTopologyObservedStates(status.Observed),
		Operations: make([]TopologyDeploymentOperation, 0, len(status.Operations)),
		Events:     make([]TopologyDeploymentEvent, 0, len(status.Events)),
	}
	for _, step := range status.Steps {
		view.Steps = append(view.Steps, PublicTopologyDeploymentStep(step))
	}
	for _, operation := range status.Operations {
		view.Operations = append(view.Operations, PublicTopologyDeploymentOperation(operation))
	}
	for _, event := range status.Events {
		view.Events = append(view.Events, PublicTopologyDeploymentEvent(event))
	}
	return view
}

func PublicTopologyDeployment(deployment model.TopologyDeployment) TopologyDeploymentView {
	return TopologyDeploymentView{
		ID: deployment.ID, TopologyID: deployment.TopologyID, RevisionID: deployment.RevisionID,
		PreviousRevisionID: deployment.PreviousRevisionID, RolloutGroup: deployment.RolloutGroup,
		State: deployment.State, FailurePolicy: deployment.FailurePolicy,
		HasError:             topologyDeploymentHasError(deployment),
		LastError:            topologyDeploymentErrorSummary(deployment.State, deployment.LastError),
		HealthGateDeadlineAt: deployment.HealthGateDeadlineAt,
		RollbackStartedAt:    deployment.RollbackStartedAt, RollbackCompletedAt: deployment.RollbackCompletedAt,
		CreatedBy: deployment.CreatedBy, CreatedAt: deployment.CreatedAt, UpdatedAt: deployment.UpdatedAt,
		CompletedAt: deployment.CompletedAt,
	}
}

func PublicTopologyDeploymentStep(step model.TopologyDeploymentStep) TopologyDeploymentStepView {
	hasError := topologyDeploymentStepHasError(step)
	return TopologyDeploymentStepView{
		ID: step.ID, DeploymentID: step.DeploymentID, VertexID: step.VertexID, VertexKey: step.VertexKey,
		NodeID: step.NodeID, PluginID: step.PluginID, Role: step.Role, TargetVersion: step.TargetVersion,
		ApplyOrder: step.ApplyOrder, Removal: step.Removal, ApplyAction: step.ApplyAction,
		RollbackMode: step.RollbackMode, State: step.State, HasError: hasError,
		LastError:            topologyDeploymentStepErrorSummary(step.State, step.LastError),
		ConfigureOperationID: step.ConfigureOperationID, EnableOperationID: step.EnableOperationID,
		DisableOperationID:           step.DisableOperationID,
		RollbackConfigureOperationID: step.RollbackConfigureOperationID,
		RollbackEnableOperationID:    step.RollbackEnableOperationID,
		RollbackDisableOperationID:   step.RollbackDisableOperationID,
		CreatedAt:                    step.CreatedAt, UpdatedAt: step.UpdatedAt,
	}
}

// PublicTopologyDeploymentOperation defensively regenerates the error summary
// even though the timeline loader already does so. This keeps the HTTP view
// safe if an in-process caller constructs a TopologyDeploymentStatus manually.
func PublicTopologyDeploymentOperation(operation TopologyDeploymentOperation) TopologyDeploymentOperation {
	view := operation
	view.HasError = operation.HasError || topologyPublicOperationStateHasError(operation.State)
	view.LastError = topologyOperationErrorSummary(operation.Kind, operation.State, topologyPublicErrorMarker(view.HasError))
	return view
}

// PublicTopologyDeploymentEvent never trusts a pre-existing Message field:
// callers only receive a summary regenerated from the event type and state.
func PublicTopologyDeploymentEvent(event TopologyDeploymentEvent) TopologyDeploymentEvent {
	view := event
	switch event.Type {
	case "operation":
		view.HasError = event.HasError || topologyPublicOperationStateHasError(event.State)
		view.Message = topologyOperationErrorSummary("", event.State, topologyPublicErrorMarker(view.HasError))
	case "observed":
		view.HasError = event.HasError || topologyPublicObservedStateHasError(event.State)
		view.Message = topologyObservedStateErrorSummary(event.State, topologyPublicErrorMarker(view.HasError))
	case "deployment":
		view.HasError = event.HasError || strings.TrimSpace(event.State) == "failed"
		view.Message = topologyDeploymentErrorSummary(event.State, topologyPublicErrorMarker(view.HasError))
	default:
		view.HasError = event.HasError
		if view.HasError {
			view.Message = "deployment event reported an error"
		} else {
			view.Message = ""
		}
	}
	return view
}

func PublicTopologyObservedStates(rows []model.TopologyObservedState) []TopologyObservedStateView {
	response := make([]TopologyObservedStateView, 0, len(rows))
	for _, row := range rows {
		response = append(response, PublicTopologyObservedState(row))
	}
	return response
}

func PublicTopologyObservedState(observed model.TopologyObservedState) TopologyObservedStateView {
	return TopologyObservedStateView{
		ID: observed.ID, DeploymentID: observed.DeploymentID, NodeID: observed.NodeID,
		DesiredRevision: observed.DesiredRevision, ObservedRevision: observed.ObservedRevision,
		State: observed.State, HealthReported: strings.TrimSpace(observed.HealthJSON) != "",
		HasError:  topologyObservedStateHasError(observed),
		LastError: topologyObservedStateErrorSummary(observed.State, observed.LastError),
		UpdatedAt: observed.UpdatedAt,
	}
}

func topologyDeploymentStepHasError(step model.TopologyDeploymentStep) bool {
	if strings.TrimSpace(step.LastError) != "" {
		return true
	}
	switch strings.TrimSpace(step.State) {
	case "failed", "rollback_failed":
		return true
	default:
		return false
	}
}

func topologyPublicOperationStateHasError(state string) bool {
	switch strings.TrimSpace(state) {
	case "failed", "timed_out", "cancelled":
		return true
	default:
		return false
	}
}

func topologyPublicObservedStateHasError(state string) bool {
	switch strings.TrimSpace(state) {
	case "failed", "stale", "cancelled":
		return true
	default:
		return false
	}
}

func topologyPublicErrorMarker(hasError bool) string {
	if hasError {
		return "recorded"
	}
	return ""
}

func topologyDeploymentStepErrorSummary(state, lastError string) string {
	if strings.TrimSpace(lastError) == "" {
		switch strings.TrimSpace(state) {
		case "failed":
			return "deployment step failed"
		case "rollback_failed":
			return "deployment rollback step failed"
		default:
			return ""
		}
	}
	switch strings.TrimSpace(state) {
	case "rollback_failed":
		return "deployment rollback step failed"
	case "failed":
		return "deployment step failed"
	default:
		return "deployment step reported an error"
	}
}
