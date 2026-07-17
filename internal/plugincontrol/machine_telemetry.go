package plugincontrol

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

const (
	MachineTelemetryPluginID     = "machine-telemetry"
	MachineTelemetryVersion      = "1.0.0"
	MachineTelemetryStatusRoute  = "/api/v3/plugins/machine-telemetry/status"
	machineTelemetryDefaultLimit = 100
	machineTelemetryMaximumLimit = 500
	machineTelemetryStaleAfter   = 5 * time.Minute
)

// MachineTelemetryExecutor is the first complete Control-side reference
// package. It is intentionally read-only: metrics come from the existing node
// heartbeat table, so enabling it cannot alter proxy or forwarding traffic.
type MachineTelemetryExecutor struct {
	db  *gorm.DB
	now func() time.Time
}

func NewMachineTelemetryExecutor(db *gorm.DB) *MachineTelemetryExecutor {
	return &MachineTelemetryExecutor{db: db, now: time.Now}
}

func (e *MachineTelemetryExecutor) PluginID() string { return MachineTelemetryPluginID }
func (e *MachineTelemetryExecutor) Version() string  { return MachineTelemetryVersion }

type MachineTelemetryNode struct {
	ID             uint             `json:"id"`
	Name           string           `json:"name"`
	Host           string           `json:"host"`
	Status         model.NodeStatus `json:"status"`
	Online         bool             `json:"online"`
	RuntimeHealthy bool             `json:"runtime_healthy"`
	RuntimeError   string           `json:"runtime_error,omitempty"`
	CPUUsage       float64          `json:"cpu_usage"`
	MemoryUsage    float64          `json:"memory_usage"`
	DiskUsage      float64          `json:"disk_usage"`
	Uptime         int64            `json:"uptime"`
	OnlineUsers    int              `json:"online_users"`
	LastCheckAt    *int64           `json:"last_check_at,omitempty"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

type MachineTelemetrySummary struct {
	Total     int `json:"total"`
	Online    int `json:"online"`
	Offline   int `json:"offline"`
	Disabled  int `json:"disabled"`
	Unhealthy int `json:"unhealthy"`
}

type MachineTelemetryStatus struct {
	PluginID    string                  `json:"plugin_id"`
	Version     string                  `json:"version"`
	GeneratedAt time.Time               `json:"generated_at"`
	Summary     MachineTelemetrySummary `json:"summary"`
	Nodes       []MachineTelemetryNode  `json:"nodes"`
}

func (e *MachineTelemetryExecutor) HandleRoute(_ context.Context, request RouteRequest) (RouteResponse, error) {
	if request.Path != MachineTelemetryStatusRoute {
		return RouteResponse{}, ErrRouteNotFound
	}
	if request.Method != http.MethodGet {
		return RouteResponse{}, ErrMethodNotAllowed
	}
	if e.db == nil {
		return RouteResponse{}, errors.New("machine telemetry database is not initialized")
	}
	limit, err := telemetryLimit(request.Query.Get("limit"))
	if err != nil {
		return RouteResponse{}, err
	}
	var rows []model.Node
	if err := e.db.Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return RouteResponse{}, err
	}
	now := time.Now()
	if e.now != nil {
		now = e.now()
	}
	status := MachineTelemetryStatus{
		PluginID: MachineTelemetryPluginID, Version: MachineTelemetryVersion,
		GeneratedAt: now, Nodes: make([]MachineTelemetryNode, 0, len(rows)),
	}
	for _, node := range rows {
		online := nodeOnlineAt(node, now)
		item := MachineTelemetryNode{
			ID: node.ID, Name: node.Name, Host: node.Host, Status: node.Status,
			Online: online, RuntimeHealthy: node.RuntimeHealthy, RuntimeError: node.RuntimeError,
			CPUUsage: node.CPUUsage, MemoryUsage: node.MemoryUsage, DiskUsage: node.DiskUsage,
			Uptime: node.Uptime, OnlineUsers: node.OnlineUsers, LastCheckAt: node.LastCheckAt,
			UpdatedAt: node.UpdatedAt,
		}
		status.Nodes = append(status.Nodes, item)
		status.Summary.Total++
		switch {
		case node.Status == model.NodeStatusDisabled:
			status.Summary.Disabled++
		case online:
			status.Summary.Online++
		default:
			status.Summary.Offline++
		}
		if !node.RuntimeHealthy {
			status.Summary.Unhealthy++
		}
	}
	return RouteResponse{Status: http.StatusOK, Data: status}, nil
}

func (e *MachineTelemetryExecutor) ExecuteLifecycle(ctx context.Context, request LifecycleRequest) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(request.Config) == 0 {
		request.Config = json.RawMessage(`{}`)
	}
	if !json.Valid(request.Config) || strings.TrimSpace(string(request.Config))[0] != '{' {
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
		response, err := e.HandleRoute(ctx, RouteRequest{Method: http.MethodGet, Path: MachineTelemetryStatusRoute})
		if err != nil {
			return nil, err
		}
		return json.Marshal(response.Data)
	default:
		return nil, errors.New("unsupported machine telemetry lifecycle operation")
	}
}

func telemetryLimit(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return machineTelemetryDefaultLimit, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > machineTelemetryMaximumLimit {
		return 0, ErrInvalidPluginInput
	}
	return value, nil
}

func nodeOnlineAt(node model.Node, now time.Time) bool {
	if node.Status == model.NodeStatusDisabled || node.LastCheckAt == nil {
		return false
	}
	return now.Unix()-*node.LastCheckAt < int64(machineTelemetryStaleAfter/time.Second)
}
