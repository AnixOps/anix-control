package plugincontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

const (
	NftablesForwardPluginID    = "nftables-forward"
	NftablesForwardVersion     = "1.0.0"
	NftablesForwardStatusRoute = "/api/v3/plugins/nftables-forward/status"
)

type NftablesForwardExecutor struct {
	db  *gorm.DB
	now func() time.Time
}

func NewNftablesForwardExecutor(db *gorm.DB) *NftablesForwardExecutor {
	return &NftablesForwardExecutor{db: db, now: time.Now}
}

func (e *NftablesForwardExecutor) PluginID() string { return NftablesForwardPluginID }
func (e *NftablesForwardExecutor) Version() string  { return NftablesForwardVersion }

type NftablesForwardSummary struct {
	Rules            int `json:"rules"`
	Ready            int `json:"ready"`
	Reconciling      int `json:"reconciling"`
	RollbackRequired int `json:"rollback_required"`
}

type NftablesForwardRuleStatus struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	NodeID           uint   `json:"node_id"`
	NodeName         string `json:"node_name"`
	Topology         string `json:"topology,omitempty"`
	Protocol         string `json:"protocol"`
	Listen           string `json:"listen,omitempty"`
	Target           string `json:"target,omitempty"`
	DesiredRevision  int64  `json:"desired_revision"`
	ObservedRevision int64  `json:"observed_revision"`
	Enabled          bool   `json:"enabled"`
	Ready            bool   `json:"ready"`
	Reconciling      bool   `json:"reconciling"`
	RollbackRequired bool   `json:"rollback_required"`
	LastError        string `json:"last_error,omitempty"`
}

type NftablesForwardStatus struct {
	PluginID    string                      `json:"plugin_id"`
	Version     string                      `json:"version"`
	GeneratedAt time.Time                   `json:"generated_at"`
	Summary     NftablesForwardSummary      `json:"summary"`
	Rules       []NftablesForwardRuleStatus `json:"rules"`
}

type nftablesForwardConfig struct {
	Family string                `json:"family"`
	Table  string                `json:"table"`
	Rules  []nftablesForwardRule `json:"rules"`
}

type nftablesForwardRule struct {
	ID            string `json:"id"`
	Protocol      string `json:"protocol"`
	ListenAddress string `json:"listen_address"`
	ListenPort    int    `json:"listen_port"`
	TargetAddress string `json:"target_address"`
	TargetPort    int    `json:"target_port"`
	Comment       string `json:"comment"`
}

func (e *NftablesForwardExecutor) HandleRoute(_ context.Context, request RouteRequest) (RouteResponse, error) {
	if request.Path != NftablesForwardStatusRoute {
		return RouteResponse{}, ErrRouteNotFound
	}
	if request.Method != http.MethodGet {
		return RouteResponse{}, ErrMethodNotAllowed
	}
	if e.db == nil {
		return RouteResponse{}, errors.New("nftables-forward database is not initialized")
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

func (e *NftablesForwardExecutor) ExecuteLifecycle(ctx context.Context, request LifecycleRequest) (json.RawMessage, error) {
	return executeReadOnlyAssignmentLifecycle(ctx, request, func(ctx context.Context) (any, error) {
		response, err := e.HandleRoute(ctx, RouteRequest{Method: http.MethodGet, Path: NftablesForwardStatusRoute})
		return response.Data, err
	})
}

func (e *NftablesForwardExecutor) status(limit int) (NftablesForwardStatus, error) {
	assignments, nodes, cursors, operations, err := loadPluginAssignmentState(e.db, NftablesForwardPluginID, limit)
	if err != nil {
		return NftablesForwardStatus{}, err
	}
	now := time.Now()
	if e.now != nil {
		now = e.now()
	}
	status := NftablesForwardStatus{
		PluginID: NftablesForwardPluginID, Version: NftablesForwardVersion, GeneratedAt: now,
		Rules: make([]NftablesForwardRuleStatus, 0, len(assignments)),
	}
	for _, assignment := range assignments {
		node := nodes[assignment.NodeID]
		operation := operations[assignment.NodeID]
		observed := decodeObservedPluginResult(operation.ResultJSON)
		var config nftablesForwardConfig
		_ = json.Unmarshal([]byte(operation.ConfigJSON), &config)
		if len(config.Rules) == 0 {
			status.Rules = append(status.Rules, nftablesForwardRuleRow(assignment, node, cursors[assignment.NodeID], operation, observed, config, nil))
			continue
		}
		for index := range config.Rules {
			status.Rules = append(status.Rules, nftablesForwardRuleRow(assignment, node, cursors[assignment.NodeID], operation, observed, config, &config.Rules[index]))
		}
	}
	for _, rule := range status.Rules {
		status.Summary.Rules++
		if rule.Ready {
			status.Summary.Ready++
		}
		if rule.Reconciling {
			status.Summary.Reconciling++
		}
		if rule.RollbackRequired {
			status.Summary.RollbackRequired++
		}
	}
	return status, nil
}

func nftablesForwardRuleRow(assignment model.NodeServiceAssignment, node model.Node, cursor model.NodeOperationRevision, operation model.KernelOperation, observed observedPluginResult, config nftablesForwardConfig, rule *nftablesForwardRule) NftablesForwardRuleStatus {
	row := NftablesForwardRuleStatus{
		ID: fmt.Sprintf("assignment-%d", assignment.ID), Name: node.Name + " / " + assignment.Role,
		NodeID: assignment.NodeID, NodeName: node.Name, Topology: assignment.Role, Protocol: "tcp+udp",
		DesiredRevision:  assignment.DesiredConfigRevision,
		ObservedRevision: maxInt64(cursor.ObservedRevision, observed.ObservedRevision), Enabled: assignment.Enabled,
	}
	if rule != nil {
		if rule.ID != "" {
			row.ID = rule.ID
		}
		row.Name = firstNonEmpty(rule.Comment, rule.ID, row.Name)
		row.Protocol = firstNonEmpty(rule.Protocol, row.Protocol)
		row.Listen = formatEndpoint(rule.ListenAddress, rule.ListenPort)
		row.Target = formatEndpoint(rule.TargetAddress, rule.TargetPort)
	} else if config.Table != "" || config.Family != "" {
		row.Topology = strings.TrimSpace(strings.Trim(strings.Join([]string{assignment.Role, config.Family, config.Table}, " / "), "/"))
	}
	observedVersion := observed.ObservedVersion
	if observedVersion == "" && operation.State == "succeeded" {
		observedVersion = operation.TargetVersion
	}
	row.RollbackRequired = operation.State == "failed" && (operation.Kind == "plugin.update" || operation.Kind == "plugin.rollback")
	row.Reconciling = assignment.Enabled && (operationInFlight(operation.State) || (!row.RollbackRequired && row.ObservedRevision < row.DesiredRevision))
	health := firstNonEmpty(observed.Health, boolHealth(node.RuntimeHealthy))
	row.LastError = firstNonEmpty(observed.LastError, operation.LastError, node.RuntimeError)
	row.Ready = assignment.Enabled && !row.RollbackRequired && !row.Reconciling && health == "healthy" &&
		observedVersion == assignment.DesiredVersion && row.ObservedRevision >= row.DesiredRevision
	return row
}
