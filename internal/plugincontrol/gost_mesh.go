package plugincontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/model"
	"gorm.io/gorm"
)

const (
	GostMeshPluginID     = "gost-mesh"
	GostMeshVersion      = "1.0.0"
	GostMeshStatusRoute  = "/api/v3/plugins/gost-mesh/status"
	gostMeshDefaultLimit = 100
	gostMeshMaximumLimit = 500
)

type GostMeshExecutor struct {
	db  *gorm.DB
	now func() time.Time
}

func NewGostMeshExecutor(db *gorm.DB) *GostMeshExecutor {
	return &GostMeshExecutor{db: db, now: time.Now}
}

func (e *GostMeshExecutor) PluginID() string { return GostMeshPluginID }
func (e *GostMeshExecutor) Version() string  { return GostMeshVersion }

type GostMeshSummary struct {
	Tunnels          int `json:"tunnels"`
	Ready            int `json:"ready"`
	Reconciling      int `json:"reconciling"`
	CleanupPending   int `json:"cleanup_pending"`
	RollbackRequired int `json:"rollback_required"`
}

type GostMeshTunnelStatus struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	NodeID           uint      `json:"node_id"`
	Role             string    `json:"role"`
	EntryNode        string    `json:"entry_node,omitempty"`
	ExitNode         string    `json:"exit_node,omitempty"`
	Transport        string    `json:"transport,omitempty"`
	Listen           string    `json:"listen,omitempty"`
	Upstream         string    `json:"upstream,omitempty"`
	DesiredVersion   string    `json:"desired_version,omitempty"`
	ObservedVersion  string    `json:"observed_version,omitempty"`
	DesiredRevision  int64     `json:"desired_revision"`
	ObservedRevision int64     `json:"observed_revision"`
	Health           string    `json:"health"`
	Ready            bool      `json:"ready"`
	Reconciling      bool      `json:"reconciling"`
	CleanupPending   bool      `json:"cleanup_pending"`
	RollbackRequired bool      `json:"rollback_required"`
	LastError        string    `json:"last_error,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type GostMeshStatus struct {
	PluginID    string                 `json:"plugin_id"`
	Version     string                 `json:"version"`
	GeneratedAt time.Time              `json:"generated_at"`
	Summary     GostMeshSummary        `json:"summary"`
	Tunnels     []GostMeshTunnelStatus `json:"tunnels"`
}

type gostMeshEndpoint struct {
	Address string `json:"address"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
}

type gostMeshTunnelConfig struct {
	ID        string            `json:"id"`
	Role      string            `json:"role"`
	Transport string            `json:"transport"`
	Listen    *gostMeshEndpoint `json:"listen"`
	Remote    *gostMeshEndpoint `json:"remote"`
}

type gostMeshRuntimeConfig struct {
	Tunnels []gostMeshTunnelConfig `json:"tunnels"`
}

type gostMeshObservedResult struct {
	ObservedVersion  string `json:"observed_version"`
	ObservedRevision int64  `json:"observed_revision"`
	Health           string `json:"health"`
	CleanupPending   bool   `json:"cleanup_pending"`
	LastError        string `json:"last_error"`
}

func (e *GostMeshExecutor) HandleRoute(_ context.Context, request RouteRequest) (RouteResponse, error) {
	if request.Path != GostMeshStatusRoute {
		return RouteResponse{}, ErrRouteNotFound
	}
	if request.Method != http.MethodGet {
		return RouteResponse{}, ErrMethodNotAllowed
	}
	if e.db == nil {
		return RouteResponse{}, errors.New("gost-mesh database is not initialized")
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

func (e *GostMeshExecutor) ExecuteLifecycle(ctx context.Context, request LifecycleRequest) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(request.Config) == 0 {
		request.Config = json.RawMessage(`{}`)
	}
	if !json.Valid(request.Config) || !strings.HasPrefix(strings.TrimSpace(string(request.Config)), "{") {
		return nil, ErrInvalidPluginInput
	}
	switch request.Kind {
	case "plugin.install":
		return json.RawMessage(`{"state":"installed"}`), nil
	case "plugin.disable":
		return json.RawMessage(`{"state":"disabled"}`), nil
	case "plugin.configure":
		return append(json.RawMessage(nil), request.Config...), nil
	case "plugin.enable", "plugin.update", "plugin.rollback", "plugin.health", "plugin.inspect":
		response, err := e.HandleRoute(ctx, RouteRequest{Method: http.MethodGet, Path: GostMeshStatusRoute})
		if err != nil {
			return nil, err
		}
		return json.Marshal(response.Data)
	default:
		return nil, errors.New("unsupported gost-mesh lifecycle operation")
	}
}

func (e *GostMeshExecutor) status(limit int) (GostMeshStatus, error) {
	var assignments []model.NodeServiceAssignment
	if err := e.db.Where("plugin_id = ?", GostMeshPluginID).Order("id ASC").Limit(limit).Find(&assignments).Error; err != nil {
		return GostMeshStatus{}, err
	}
	nodeIDs := make([]uint, 0, len(assignments))
	for _, assignment := range assignments {
		nodeIDs = append(nodeIDs, assignment.NodeID)
	}
	nodes := make(map[uint]model.Node, len(nodeIDs))
	cursors := make(map[uint]model.NodeOperationRevision, len(nodeIDs))
	operations := make(map[uint]model.KernelOperation, len(nodeIDs))
	if len(nodeIDs) > 0 {
		var nodeRows []model.Node
		if err := e.db.Where("id IN ?", nodeIDs).Find(&nodeRows).Error; err != nil {
			return GostMeshStatus{}, err
		}
		for _, node := range nodeRows {
			nodes[node.ID] = node
		}
		var cursorRows []model.NodeOperationRevision
		if err := e.db.Where("node_id IN ?", nodeIDs).Find(&cursorRows).Error; err != nil {
			return GostMeshStatus{}, err
		}
		for _, cursor := range cursorRows {
			cursors[cursor.NodeID] = cursor
		}
		var operationRows []model.KernelOperation
		if err := e.db.Where("plugin_id = ? AND node_id IN ?", GostMeshPluginID, nodeIDs).
			Order("updated_at DESC, id DESC").Find(&operationRows).Error; err != nil {
			return GostMeshStatus{}, err
		}
		for _, operation := range operationRows {
			if operation.NodeID == nil {
				continue
			}
			if _, exists := operations[*operation.NodeID]; !exists {
				operations[*operation.NodeID] = operation
			}
		}
	}

	now := time.Now()
	if e.now != nil {
		now = e.now()
	}
	status := GostMeshStatus{
		PluginID: GostMeshPluginID, Version: GostMeshVersion, GeneratedAt: now,
		Tunnels: make([]GostMeshTunnelStatus, 0, len(assignments)),
	}
	for _, assignment := range assignments {
		node := nodes[assignment.NodeID]
		cursor := cursors[assignment.NodeID]
		operation := operations[assignment.NodeID]
		observed := decodeGostMeshResult(operation.ResultJSON)
		config := decodeGostMeshConfig(operation.ConfigJSON)
		matched := false
		for index := range config.Tunnels {
			if strings.TrimSpace(config.Tunnels[index].Role) != strings.TrimSpace(assignment.Role) {
				continue
			}
			matched = true
			status.Tunnels = append(status.Tunnels, gostMeshStatusRow(assignment, node, cursor, operation, observed, &config.Tunnels[index]))
		}
		if !matched {
			status.Tunnels = append(status.Tunnels, gostMeshStatusRow(assignment, node, cursor, operation, observed, nil))
		}
	}
	for _, tunnel := range status.Tunnels {
		status.Summary.Tunnels++
		if tunnel.Ready {
			status.Summary.Ready++
		}
		if tunnel.Reconciling {
			status.Summary.Reconciling++
		}
		if tunnel.CleanupPending {
			status.Summary.CleanupPending++
		}
		if tunnel.RollbackRequired {
			status.Summary.RollbackRequired++
		}
	}
	return status, nil
}

func gostMeshStatusRow(assignment model.NodeServiceAssignment, node model.Node, cursor model.NodeOperationRevision, operation model.KernelOperation, observed gostMeshObservedResult, tunnel *gostMeshTunnelConfig) GostMeshTunnelStatus {
	row := GostMeshTunnelStatus{
		ID: fmt.Sprintf("assignment-%d", assignment.ID), Name: node.Name + " / " + assignment.Role,
		NodeID: assignment.NodeID, Role: assignment.Role, DesiredVersion: assignment.DesiredVersion,
		ObservedVersion: observed.ObservedVersion, DesiredRevision: assignment.DesiredConfigRevision,
		ObservedRevision: maxInt64(cursor.ObservedRevision, observed.ObservedRevision), UpdatedAt: assignment.UpdatedAt,
	}
	if row.ObservedVersion == "" && operation.State == "succeeded" {
		row.ObservedVersion = operation.TargetVersion
	}
	if tunnel != nil {
		if strings.TrimSpace(tunnel.ID) != "" {
			row.ID, row.Name = tunnel.ID, tunnel.ID
		}
		row.Role, row.Transport = tunnel.Role, tunnel.Transport
		if tunnel.Listen != nil {
			row.Listen = formatEndpoint(tunnel.Listen.Address, tunnel.Listen.Port)
		}
		if tunnel.Remote != nil {
			row.Upstream = formatEndpoint(tunnel.Remote.Host, tunnel.Remote.Port)
		}
	}
	switch row.Role {
	case "entry":
		row.EntryNode = node.Name
	case "exit":
		row.ExitNode = node.Name
	}
	row.CleanupPending = observed.CleanupPending
	row.RollbackRequired = operation.State == "failed" && (operation.Kind == "plugin.update" || operation.Kind == "plugin.rollback")
	row.Reconciling = assignment.Enabled && operationInFlight(operation.State)
	row.LastError = firstNonEmpty(observed.LastError, operation.LastError, node.RuntimeError)
	row.Health = firstNonEmpty(observed.Health, boolHealth(node.RuntimeHealthy))
	row.Ready = assignment.Enabled && !row.CleanupPending && !row.RollbackRequired && !row.Reconciling &&
		row.Health == "healthy" && row.ObservedVersion == assignment.DesiredVersion &&
		row.ObservedRevision >= assignment.DesiredConfigRevision
	return row
}

func decodeGostMeshConfig(raw string) gostMeshRuntimeConfig {
	var value gostMeshRuntimeConfig
	_ = json.Unmarshal([]byte(raw), &value)
	return value
}

func decodeGostMeshResult(raw string) gostMeshObservedResult {
	var value gostMeshObservedResult
	_ = json.Unmarshal([]byte(raw), &value)
	return value
}

func gostMeshLimit(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return gostMeshDefaultLimit, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > gostMeshMaximumLimit {
		return 0, ErrInvalidPluginInput
	}
	return value, nil
}

func operationInFlight(state string) bool {
	switch state {
	case "pending", "queued", "running", "dispatched", "acknowledged", "cancel_requested":
		return true
	default:
		return false
	}
}

func boolHealth(healthy bool) string {
	if healthy {
		return "healthy"
	}
	return "unhealthy"
}

func formatEndpoint(host string, port int) string {
	host = strings.TrimSpace(host)
	if host == "" || port <= 0 {
		return ""
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func maxInt64(left, right int64) int64 {
	if right > left {
		return right
	}
	return left
}
