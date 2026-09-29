package v2bx

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/control-center/internal/core/plugin"
	"github.com/go-resty/resty/v2"
)

// Config holds V2bX plugin configuration
type Config struct {
	Hosts   []NodeConfig `yaml:"hosts"`   // Multiple V2bX nodes
	Timeout int          `yaml:"timeout"` // request timeout in seconds
}

// NodeConfig holds configuration for a single V2bX node
type NodeConfig struct {
	ID       int    `yaml:"id"`
	Name     string `yaml:"name"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	APIKey   string `yaml:"api_key"`
	GRPCPort int    `yaml:"grpc_port"`
}

// V2bXPlugin implements the plugin interface for V2bX
type V2bXPlugin struct {
	config  Config
	clients map[int]*resty.Client
	status  plugin.Status
	mu      sync.RWMutex
}

// New creates a new V2bX plugin
func New() *V2bXPlugin {
	return &V2bXPlugin{
		clients: make(map[int]*resty.Client),
		status: plugin.Status{
			State:   string(plugin.StateUninitialized),
			Health:  string(plugin.HealthHealthy),
			Metrics: make(map[string]interface{}),
		},
	}
}

// Info returns plugin information
func (p *V2bXPlugin) Info() plugin.PluginInfo {
	return plugin.PluginInfo{
		Name:        "v2bx",
		Version:     "1.0.0",
		Description: "V2bX proxy node management plugin",
		Author:      "AnixOps",
	}
}

// Init initializes the plugin
func (p *V2bXPlugin) Init(ctx context.Context, config map[string]interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Parse config
	configBytes, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := json.Unmarshal(configBytes, &p.config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults
	if p.config.Timeout == 0 {
		p.config.Timeout = 30
	}

	// Create clients for each node
	for _, node := range p.config.Hosts {
		client := resty.New().
			SetBaseURL(fmt.Sprintf("http://%s:%d", node.Host, node.Port)).
			SetTimeout(time.Duration(p.config.Timeout)*time.Second).
			SetHeader("Content-Type", "application/json")

		if node.APIKey != "" {
			client.SetAuthToken(node.APIKey)
		}

		p.clients[node.ID] = client
	}

	p.status.State = string(plugin.StateInitialized)
	p.status.Metrics["nodes"] = len(p.clients)
	return nil
}

// Start starts the plugin
func (p *V2bXPlugin) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Verify connections to all nodes
	healthyNodes := 0
	for _, client := range p.clients {
		resp, err := client.R().Get("/health")
		if err == nil && resp.IsSuccess() {
			healthyNodes++
		}
	}

	p.status.State = string(plugin.StateRunning)
	p.status.Metrics["healthy_nodes"] = healthyNodes
	p.status.LastUpdated = time.Now().Unix()

	if healthyNodes == 0 {
		p.status.Health = string(plugin.HealthUnhealthy)
	} else if healthyNodes < len(p.clients) {
		p.status.Health = string(plugin.HealthDegraded)
	} else {
		p.status.Health = string(plugin.HealthHealthy)
	}

	return nil
}

// Stop stops the plugin
func (p *V2bXPlugin) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status.State = string(plugin.StateStopped)
	p.status.LastUpdated = time.Now().Unix()
	return nil
}

// HealthCheck performs a health check
func (p *V2bXPlugin) HealthCheck(ctx context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.status.Health == string(plugin.HealthUnhealthy) {
		return fmt.Errorf("plugin unhealthy")
	}
	return nil
}

// Capabilities returns plugin capabilities
func (p *V2bXPlugin) Capabilities() []string {
	return []string{
		"get_nodes",
		"get_node_status",
		"get_users",
		"get_stats",
		"restart_node",
		"get_config",
		"get_logs",
	}
}

// Execute executes an action
func (p *V2bXPlugin) Execute(ctx context.Context, action string, params map[string]interface{}) (plugin.Result, error) {
	switch action {
	case "get_nodes":
		return p.getNodes(ctx)
	case "get_node_status":
		return p.getNodeStatus(ctx, params)
	case "get_users":
		return p.getUsers(ctx, params)
	case "get_stats":
		return p.getStats(ctx, params)
	case "restart_node":
		return p.restartNode(ctx, params)
	case "get_config":
		return p.getConfig(ctx, params)
	case "get_logs":
		return p.getLogs(ctx, params)
	default:
		return plugin.Result{}, fmt.Errorf("unknown action: %s", action)
	}
}

// GetStatus returns current status
func (p *V2bXPlugin) GetStatus(ctx context.Context) (plugin.Status, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status, nil
}

// API Methods

func (p *V2bXPlugin) getNodes(ctx context.Context) (plugin.Result, error) {
	nodes := []map[string]interface{}{}

	for _, nodeCfg := range p.config.Hosts {
		nodes = append(nodes, map[string]interface{}{
			"id":     nodeCfg.ID,
			"name":   nodeCfg.Name,
			"host":   nodeCfg.Host,
			"port":   nodeCfg.Port,
			"status": "online", // TODO: actual status check
		})
	}

	return plugin.Result{
		Success: true,
		Data:    nodes,
		Metrics: map[string]interface{}{
			"total": len(nodes),
		},
	}, nil
}

func (p *V2bXPlugin) getNodeStatus(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	nodeID, ok := params["node_id"].(float64)
	if !ok {
		return plugin.Result{Success: false, Error: "node_id required"}, nil
	}

	p.mu.RLock()
	client, ok := p.clients[int(nodeID)]
	p.mu.RUnlock()

	if !ok {
		return plugin.Result{Success: false, Error: "node not found"}, nil
	}

	resp, err := client.R().
		SetContext(ctx).
		Get("/api/status")

	if err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	var result map[string]interface{}
	json.Unmarshal(resp.Body(), &result)

	return plugin.Result{
		Success: resp.IsSuccess(),
		Data:    result,
	}, nil
}

func (p *V2bXPlugin) getUsers(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	nodeID, ok := params["node_id"].(float64)
	if !ok {
		return plugin.Result{Success: false, Error: "node_id required"}, nil
	}

	p.mu.RLock()
	client, ok := p.clients[int(nodeID)]
	p.mu.RUnlock()

	if !ok {
		return plugin.Result{Success: false, Error: "node not found"}, nil
	}

	resp, err := client.R().
		SetContext(ctx).
		Get("/api/users")

	if err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	var result map[string]interface{}
	json.Unmarshal(resp.Body(), &result)

	return plugin.Result{
		Success: resp.IsSuccess(),
		Data:    result,
	}, nil
}

func (p *V2bXPlugin) getStats(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	nodeID, _ := params["node_id"].(float64)

	// Get stats from specific node or all nodes
	if nodeID > 0 {
		p.mu.RLock()
		client, ok := p.clients[int(nodeID)]
		p.mu.RUnlock()

		if !ok {
			return plugin.Result{Success: false, Error: "node not found"}, nil
		}

		resp, err := client.R().Get("/api/stats")
		if err != nil {
			return plugin.Result{Success: false, Error: err.Error()}, nil
		}

		var result map[string]interface{}
		json.Unmarshal(resp.Body(), &result)

		return plugin.Result{Success: true, Data: result}, nil
	}

	// Aggregate stats from all nodes
	allStats := []map[string]interface{}{}
	p.mu.RLock()
	for id, client := range p.clients {
		resp, err := client.R().Get("/api/stats")
		if err == nil && resp.IsSuccess() {
			var stats map[string]interface{}
			json.Unmarshal(resp.Body(), &stats)
			stats["node_id"] = id
			allStats = append(allStats, stats)
		}
	}
	p.mu.RUnlock()

	return plugin.Result{
		Success: true,
		Data:    allStats,
		Metrics: map[string]interface{}{
			"nodes": len(allStats),
		},
	}, nil
}

func (p *V2bXPlugin) restartNode(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	nodeID, ok := params["node_id"].(float64)
	if !ok {
		return plugin.Result{Success: false, Error: "node_id required"}, nil
	}

	p.mu.RLock()
	client, ok := p.clients[int(nodeID)]
	p.mu.RUnlock()

	if !ok {
		return plugin.Result{Success: false, Error: "node not found"}, nil
	}

	resp, err := client.R().
		SetContext(ctx).
		Post("/api/restart")

	if err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	return plugin.Result{
		Success: resp.IsSuccess(),
		Metrics: map[string]interface{}{
			"node_id": int(nodeID),
			"action":  "restart",
		},
	}, nil
}

func (p *V2bXPlugin) getConfig(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	nodeID, ok := params["node_id"].(float64)
	if !ok {
		return plugin.Result{Success: false, Error: "node_id required"}, nil
	}

	p.mu.RLock()
	client, ok := p.clients[int(nodeID)]
	p.mu.RUnlock()

	if !ok {
		return plugin.Result{Success: false, Error: "node not found"}, nil
	}

	resp, err := client.R().
		SetContext(ctx).
		Get("/api/config")

	if err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	var result map[string]interface{}
	json.Unmarshal(resp.Body(), &result)

	return plugin.Result{
		Success: resp.IsSuccess(),
		Data:    result,
	}, nil
}

func (p *V2bXPlugin) getLogs(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	nodeID, ok := params["node_id"].(float64)
	if !ok {
		return plugin.Result{Success: false, Error: "node_id required"}, nil
	}

	p.mu.RLock()
	client, ok := p.clients[int(nodeID)]
	p.mu.RUnlock()

	if !ok {
		return plugin.Result{Success: false, Error: "node not found"}, nil
	}

	lines := 100
	if l, ok := params["lines"].(float64); ok {
		lines = int(l)
	}

	resp, err := client.R().
		SetContext(ctx).
		SetQueryParam("lines", fmt.Sprintf("%d", lines)).
		Get("/api/logs")

	if err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	return plugin.Result{
		Success: resp.IsSuccess(),
		Data:    string(resp.Body()),
	}, nil
}
