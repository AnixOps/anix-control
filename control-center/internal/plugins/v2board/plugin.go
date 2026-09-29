package v2board

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/control-center/internal/core/plugin"
	"github.com/go-resty/resty/v2"
)

// Config holds v2board plugin configuration
type Config struct {
	Host    string `yaml:"host"`    // e.g., http://localhost:8080
	APIKey  string `yaml:"api_key"` // API token for authentication
	Timeout int    `yaml:"timeout"` // request timeout in seconds
}

// V2boardPlugin implements the plugin interface for v2board
type V2boardPlugin struct {
	config Config
	client *resty.Client
	status plugin.Status
	mu     sync.RWMutex
}

// New creates a new v2board plugin
func New() *V2boardPlugin {
	return &V2boardPlugin{
		status: plugin.Status{
			State:   string(plugin.StateUninitialized),
			Health:  string(plugin.HealthHealthy),
			Metrics: make(map[string]interface{}),
		},
	}
}

// Info returns plugin information
func (p *V2boardPlugin) Info() plugin.PluginInfo {
	return plugin.PluginInfo{
		Name:        "v2board",
		Version:     "1.0.0",
		Description: "V2Board panel management plugin",
		Author:      "AnixOps",
	}
}

// Init initializes the plugin
func (p *V2boardPlugin) Init(ctx context.Context, config map[string]interface{}) error {
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

	// Create HTTP client
	p.client = resty.New().
		SetBaseURL(p.config.Host).
		SetTimeout(time.Duration(p.config.Timeout)*time.Second).
		SetHeader("Content-Type", "application/json")

	if p.config.APIKey != "" {
		p.client.SetAuthToken(p.config.APIKey)
	}

	p.status.State = string(plugin.StateInitialized)
	return nil
}

// Start starts the plugin
func (p *V2boardPlugin) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Verify connection
	if _, err := p.healthCheck(ctx); err != nil {
		return fmt.Errorf("failed to connect to v2board: %w", err)
	}

	p.status.State = string(plugin.StateRunning)
	p.status.LastUpdated = time.Now().Unix()
	return nil
}

// Stop stops the plugin
func (p *V2boardPlugin) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status.State = string(plugin.StateStopped)
	p.status.LastUpdated = time.Now().Unix()
	return nil
}

// HealthCheck performs a health check
func (p *V2boardPlugin) HealthCheck(ctx context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	_, err := p.healthCheck(ctx)
	return err
}

func (p *V2boardPlugin) healthCheck(ctx context.Context) (map[string]interface{}, error) {
	resp, err := p.client.R().
		SetContext(ctx).
		Get("/health")

	if err != nil {
		return nil, err
	}

	if !resp.IsSuccess() {
		return nil, fmt.Errorf("health check failed: %s", resp.Status())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, err
	}

	return result, nil
}

// Capabilities returns plugin capabilities
func (p *V2boardPlugin) Capabilities() []string {
	return []string{
		"get_status",
		"get_nodes",
		"get_users",
		"get_stats",
		"deploy_node",
		"remove_node",
		"manage_user",
		"get_orders",
		"get_plans",
	}
}

// Execute executes an action
func (p *V2boardPlugin) Execute(ctx context.Context, action string, params map[string]interface{}) (plugin.Result, error) {
	switch action {
	case "get_status":
		return p.getStatus(ctx)
	case "get_nodes":
		return p.getNodes(ctx, params)
	case "get_users":
		return p.getUsers(ctx, params)
	case "get_stats":
		return p.getStats(ctx)
	case "deploy_node":
		return p.deployNode(ctx, params)
	case "remove_node":
		return p.removeNode(ctx, params)
	case "manage_user":
		return p.manageUser(ctx, params)
	case "get_orders":
		return p.getOrders(ctx, params)
	case "get_plans":
		return p.getPlans(ctx)
	default:
		return plugin.Result{}, fmt.Errorf("unknown action: %s", action)
	}
}

// GetStatus returns current status
func (p *V2boardPlugin) GetStatus(ctx context.Context) (plugin.Status, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Update health status
	if _, err := p.healthCheck(ctx); err != nil {
		p.status.Health = string(plugin.HealthUnhealthy)
	} else {
		p.status.Health = string(plugin.HealthHealthy)
	}

	return p.status, nil
}

// API Methods

func (p *V2boardPlugin) getStatus(ctx context.Context) (plugin.Result, error) {
	result, err := p.healthCheck(ctx)
	if err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}
	return plugin.Result{Success: true, Data: result}, nil
}

func (p *V2boardPlugin) getNodes(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	resp, err := p.client.R().
		SetContext(ctx).
		Get("/api/v2/admin/nodes")

	if err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	var result map[string]interface{}
	json.Unmarshal(resp.Body(), &result)

	return plugin.Result{Success: resp.IsSuccess(), Data: result}, nil
}

func (p *V2boardPlugin) getUsers(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	resp, err := p.client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"page":     getStringParam(params, "page", "1"),
			"per_page": getStringParam(params, "per_page", "50"),
		}).
		Get("/api/v2/admin/users")

	if err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	var result map[string]interface{}
	json.Unmarshal(resp.Body(), &result)

	return plugin.Result{Success: resp.IsSuccess(), Data: result}, nil
}

func (p *V2boardPlugin) getStats(ctx context.Context) (plugin.Result, error) {
	resp, err := p.client.R().
		SetContext(ctx).
		Get("/api/v2/admin/dashboard")

	if err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	var result map[string]interface{}
	json.Unmarshal(resp.Body(), &result)

	return plugin.Result{Success: resp.IsSuccess(), Data: result}, nil
}

func (p *V2boardPlugin) deployNode(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	resp, err := p.client.R().
		SetContext(ctx).
		SetBody(params).
		Post("/api/v2/admin/nodes")

	if err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	var result map[string]interface{}
	json.Unmarshal(resp.Body(), &result)

	return plugin.Result{
		Success: resp.IsSuccess(),
		Data:    result,
		Metrics: map[string]interface{}{
			"node_name": params["name"],
			"action":    "deploy",
		},
	}, nil
}

func (p *V2boardPlugin) removeNode(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	nodeID, ok := params["id"].(float64)
	if !ok {
		return plugin.Result{Success: false, Error: "node id required"}, nil
	}

	resp, err := p.client.R().
		SetContext(ctx).
		Delete(fmt.Sprintf("/api/v2/admin/nodes/%d", int(nodeID)))

	if err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	return plugin.Result{Success: resp.IsSuccess()}, nil
}

func (p *V2boardPlugin) manageUser(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	action, ok := params["action"].(string)
	if !ok {
		return plugin.Result{Success: false, Error: "action required"}, nil
	}

	userID, ok := params["id"].(float64)
	if !ok {
		return plugin.Result{Success: false, Error: "user id required"}, nil
	}

	var resp *resty.Response
	var err error

	switch action {
	case "ban":
		resp, err = p.client.R().
			SetContext(ctx).
			Post(fmt.Sprintf("/api/v2/admin/users/%d/ban", int(userID)))
	case "unban":
		resp, err = p.client.R().
			SetContext(ctx).
			Post(fmt.Sprintf("/api/v2/admin/users/%d/unban", int(userID)))
	case "update":
		resp, err = p.client.R().
			SetContext(ctx).
			SetBody(params["data"]).
			Put(fmt.Sprintf("/api/v2/admin/users/%d", int(userID)))
	default:
		return plugin.Result{Success: false, Error: fmt.Sprintf("unknown action: %s", action)}, nil
	}

	if err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	return plugin.Result{Success: resp.IsSuccess()}, nil
}

func (p *V2boardPlugin) getOrders(ctx context.Context, params map[string]interface{}) (plugin.Result, error) {
	resp, err := p.client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"page":     getStringParam(params, "page", "1"),
			"per_page": getStringParam(params, "per_page", "50"),
		}).
		Get("/api/v2/admin/orders")

	if err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	var result map[string]interface{}
	json.Unmarshal(resp.Body(), &result)

	return plugin.Result{Success: resp.IsSuccess(), Data: result}, nil
}

func (p *V2boardPlugin) getPlans(ctx context.Context) (plugin.Result, error) {
	resp, err := p.client.R().
		SetContext(ctx).
		Get("/api/v2/admin/plans")

	if err != nil {
		return plugin.Result{Success: false, Error: err.Error()}, nil
	}

	var result map[string]interface{}
	json.Unmarshal(resp.Body(), &result)

	return plugin.Result{Success: resp.IsSuccess(), Data: result}, nil
}

func getStringParam(params map[string]interface{}, key, defaultVal string) string {
	if v, ok := params[key].(string); ok {
		return v
	}
	return defaultVal
}
