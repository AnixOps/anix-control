package service

import (
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
)

// KernelOperationStatus is the public, non-secret projection of a durable
// operation. ConfigJSON, ResultJSON, session identity, and raw Agent errors
// intentionally stay inside the kernel persistence boundary.
type KernelOperationStatus struct {
	ID                   string     `json:"id"`
	NodeID               *uint      `json:"node_id,omitempty"`
	PluginID             string     `json:"plugin_id"`
	TargetVersion        string     `json:"target_version"`
	TopologyDeploymentID *uint      `json:"topology_deployment_id,omitempty"`
	TopologyStepID       *uint      `json:"topology_step_id,omitempty"`
	TopologyRevision     int64      `json:"topology_revision,omitempty"`
	Kind                 string     `json:"kind"`
	Revision             int64      `json:"revision"`
	ConfigHash           string     `json:"config_hash,omitempty"`
	State                string     `json:"state"`
	DeadlineAt           *time.Time `json:"deadline_at,omitempty"`
	DispatchedAt         *time.Time `json:"dispatched_at,omitempty"`
	AcknowledgedAt       *time.Time `json:"acknowledged_at,omitempty"`
	ObservedAt           *time.Time `json:"observed_at,omitempty"`
	CancelAt             *time.Time `json:"cancel_at,omitempty"`
	Attempt              int        `json:"attempt"`
	HasResult            bool       `json:"has_result"`
	HasError             bool       `json:"has_error"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// PluginInstallationStatus is the public projection of a plugin installation.
// Installation LastError can originate in a Control executor or from an Agent
// lifecycle result, so it is reduced to a stable state summary before it
// crosses an HTTP boundary.
type PluginInstallationStatus struct {
	ID                  uint       `json:"id"`
	PluginID            string     `json:"plugin_id"`
	Target              string     `json:"target"`
	DesiredVersion      string     `json:"desired_version"`
	ObservedVersion     string     `json:"observed_version"`
	PreviousVersion     string     `json:"previous_version"`
	State               string     `json:"state"`
	Enabled             bool       `json:"enabled"`
	LifecycleGeneration int64      `json:"lifecycle_generation"`
	ConfigRevision      int64      `json:"config_revision"`
	HasError            bool       `json:"has_error"`
	LastError           string     `json:"last_error,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	DisabledAt          *time.Time `json:"disabled_at,omitempty"`
}

// PublicPluginInstallation keeps package lifecycle metadata useful to the
// WebUI while withholding raw executor and Agent diagnostics.
func PublicPluginInstallation(installation model.PluginInstallation) PluginInstallationStatus {
	hasError := strings.TrimSpace(installation.LastError) != ""
	switch strings.TrimSpace(installation.State) {
	case "failed", "degraded":
		hasError = true
	}
	return PluginInstallationStatus{
		ID: installation.ID, PluginID: installation.PluginID, Target: installation.Target,
		DesiredVersion: installation.DesiredVersion, ObservedVersion: installation.ObservedVersion,
		PreviousVersion: installation.PreviousVersion, State: installation.State, Enabled: installation.Enabled,
		LifecycleGeneration: installation.LifecycleGeneration, ConfigRevision: installation.ConfigRevision,
		HasError: hasError, LastError: pluginInstallationErrorSummary(installation.State, hasError),
		CreatedAt: installation.CreatedAt, UpdatedAt: installation.UpdatedAt, DisabledAt: installation.DisabledAt,
	}
}

func pluginInstallationErrorSummary(state string, hasError bool) string {
	if !hasError {
		return ""
	}
	switch strings.TrimSpace(state) {
	case "failed":
		return "plugin installation failed"
	case "degraded":
		return "plugin installation is degraded"
	case "disabled":
		return "plugin installation was disabled after an error"
	default:
		return "plugin installation reported an error"
	}
}

func PublicKernelOperation(operation model.KernelOperation) KernelOperationStatus {
	return KernelOperationStatus{
		ID: operation.ID, NodeID: operation.NodeID, PluginID: operation.PluginID, TargetVersion: operation.TargetVersion,
		TopologyDeploymentID: operation.TopologyDeploymentID, TopologyStepID: operation.TopologyStepID,
		TopologyRevision: operation.TopologyRevision, Kind: operation.Kind, Revision: operation.Revision,
		ConfigHash: operation.ConfigHash, State: operation.State, DeadlineAt: operation.DeadlineAt,
		DispatchedAt: operation.DispatchedAt, AcknowledgedAt: operation.AcknowledgedAt, ObservedAt: operation.ObservedAt,
		CancelAt: operation.CancelAt, Attempt: operation.Attempt,
		HasResult: strings.TrimSpace(operation.ResultJSON) != "", HasError: strings.TrimSpace(operation.LastError) != "",
		CreatedAt: operation.CreatedAt, UpdatedAt: operation.UpdatedAt,
	}
}
