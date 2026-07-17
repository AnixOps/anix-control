package plugincontrol

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/model"
	"gorm.io/gorm"
)

const (
	NatEgressPluginID    = "nat-egress"
	NatEgressVersion     = "1.0.0"
	NatEgressStatusRoute = "/api/v3/plugins/nat-egress/status"
)

type NatEgressExecutor struct {
	db  *gorm.DB
	now func() time.Time
}

func NewNatEgressExecutor(db *gorm.DB) *NatEgressExecutor {
	return &NatEgressExecutor{db: db, now: time.Now}
}

func (e *NatEgressExecutor) PluginID() string { return NatEgressPluginID }
func (e *NatEgressExecutor) Version() string  { return NatEgressVersion }

type NatEgressSummary struct {
	Exits            int `json:"exits"`
	Ready            int `json:"ready"`
	Degraded         int `json:"degraded"`
	RollbackRequired int `json:"rollback_required"`
}

type NatEgressExitStatus struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	NodeID           uint   `json:"node_id"`
	NodeName         string `json:"node_name"`
	EgressInterface  string `json:"egress_interface,omitempty"`
	PublicIP         string `json:"public_ip,omitempty"`
	PolicyTable      int    `json:"policy_table,omitempty"`
	Health           string `json:"health"`
	Ready            bool   `json:"ready"`
	Degraded         bool   `json:"degraded"`
	CleanupPending   bool   `json:"cleanup_pending"`
	RollbackRequired bool   `json:"rollback_required"`
	DesiredRevision  int64  `json:"desired_revision"`
	ObservedRevision int64  `json:"observed_revision"`
	LastError        string `json:"last_error,omitempty"`
}

type NatEgressStatus struct {
	PluginID    string                `json:"plugin_id"`
	Version     string                `json:"version"`
	GeneratedAt time.Time             `json:"generated_at"`
	Summary     NatEgressSummary      `json:"summary"`
	Exits       []NatEgressExitStatus `json:"exits"`
}

type natEgressConfig struct {
	EgressInterface string `json:"egress_interface"`
	PolicyTable     int    `json:"policy_table"`
	HealthTarget    string `json:"health_check_target"`
}

func (e *NatEgressExecutor) HandleRoute(_ context.Context, request RouteRequest) (RouteResponse, error) {
	if request.Path != NatEgressStatusRoute {
		return RouteResponse{}, ErrRouteNotFound
	}
	if request.Method != http.MethodGet {
		return RouteResponse{}, ErrMethodNotAllowed
	}
	if e.db == nil {
		return RouteResponse{}, errors.New("nat-egress database is not initialized")
	}
	limit, err := gostMeshLimit(request.Query.Get("limit"))
	if err != nil {
		return RouteResponse{}, err
	}
	status, err := e.status(limit)
	if err != nil {
		return RouteResponse{}, err
	}
	return RouteResponse{Status: http.StatusOK, Data: status}, nil
}

func (e *NatEgressExecutor) ExecuteLifecycle(ctx context.Context, request LifecycleRequest) (json.RawMessage, error) {
	return executeReadOnlyAssignmentLifecycle(ctx, request, func(ctx context.Context) (any, error) {
		response, err := e.HandleRoute(ctx, RouteRequest{Method: http.MethodGet, Path: NatEgressStatusRoute})
		return response.Data, err
	})
}

func (e *NatEgressExecutor) status(limit int) (NatEgressStatus, error) {
	assignments, nodes, cursors, operations, err := loadPluginAssignmentState(e.db, NatEgressPluginID, limit)
	if err != nil {
		return NatEgressStatus{}, err
	}
	now := time.Now()
	if e.now != nil {
		now = e.now()
	}
	status := NatEgressStatus{
		PluginID: NatEgressPluginID, Version: NatEgressVersion, GeneratedAt: now,
		Exits: make([]NatEgressExitStatus, 0, len(assignments)),
	}
	for _, assignment := range assignments {
		node := nodes[assignment.NodeID]
		operation := operations[assignment.NodeID]
		observed := decodeObservedPluginResult(operation.ResultJSON)
		var config natEgressConfig
		_ = json.Unmarshal([]byte(operation.ConfigJSON), &config)
		row := natEgressStatusRow(assignment, node, cursors[assignment.NodeID], operation, observed, config)
		status.Exits = append(status.Exits, row)
		status.Summary.Exits++
		if row.Ready {
			status.Summary.Ready++
		}
		if row.Degraded {
			status.Summary.Degraded++
		}
		if row.RollbackRequired {
			status.Summary.RollbackRequired++
		}
	}
	return status, nil
}

func natEgressStatusRow(assignment model.NodeServiceAssignment, node model.Node, cursor model.NodeOperationRevision, operation model.KernelOperation, observed observedPluginResult, config natEgressConfig) NatEgressExitStatus {
	row := NatEgressExitStatus{
		ID: assignmentIdentity(assignment), Name: node.Name + " / " + assignment.Role,
		NodeID: assignment.NodeID, NodeName: node.Name, EgressInterface: config.EgressInterface,
		PolicyTable: config.PolicyTable, DesiredRevision: assignment.DesiredConfigRevision,
		ObservedRevision: maxInt64(cursor.ObservedRevision, observed.ObservedRevision), CleanupPending: observed.CleanupPending,
	}
	if host, _, err := net.SplitHostPort(config.HealthTarget); err == nil && net.ParseIP(host) != nil {
		row.PublicIP = host
	}
	observedVersion := observed.ObservedVersion
	if observedVersion == "" && operation.State == "succeeded" {
		observedVersion = operation.TargetVersion
	}
	row.Health = firstNonEmpty(observed.Health, boolHealth(node.RuntimeHealthy))
	row.LastError = firstNonEmpty(observed.LastError, operation.LastError, node.RuntimeError)
	row.RollbackRequired = operation.State == "failed" && (operation.Kind == "plugin.update" || operation.Kind == "plugin.rollback")
	reconciling := assignment.Enabled && (operationInFlight(operation.State) || (!row.RollbackRequired && row.ObservedRevision < row.DesiredRevision))
	row.Ready = assignment.Enabled && !row.CleanupPending && !row.RollbackRequired && !reconciling &&
		row.Health == "healthy" && observedVersion == assignment.DesiredVersion && row.ObservedRevision >= row.DesiredRevision
	row.Degraded = assignment.Enabled && !row.Ready && !reconciling && (row.LastError != "" || !strings.EqualFold(row.Health, "healthy"))
	return row
}

func assignmentIdentity(assignment model.NodeServiceAssignment) string {
	return "assignment-" + strconv.FormatUint(uint64(assignment.ID), 10)
}
